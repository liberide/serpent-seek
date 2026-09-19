package httpd

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/store"
)

// handleSetupStatus reports first-run state to the SPA.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	admins, _ := s.store.CountAdmins(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup":            admins == 0,
		"auth_enabled":           s.settings.GetBool(ctx, config.KeyAuthEnabled),
		"auth_off_notice_hidden": s.settings.GetBool(ctx, config.KeyHideAuthOffNotice),
		"passkeys_enabled":       s.passkeys != nil && s.passkeys.Enabled(),
		"rp_id":                  s.passkeys.RPID(),
	})
}

type setupRequest struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// handleSetup creates the first admin user and its admin key.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	admins, err := s.store.CountAdmins(ctx)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if admins > 0 {
		writeError(w, r, http.StatusConflict, "already_setup", "setup has already been completed")
		return
	}
	var body setupRequest
	if !readJSON(w, r, &body) {
		return
	}
	if !s.validSetupToken(strings.TrimSpace(body.Token)) {
		writeError(w, r, http.StatusForbidden, "invalid_setup_token", "invalid setup token (copy it from the SETUP_TOKEN line in the server log)")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "admin"
	}
	user := &store.User{
		ID: newID(), Name: name, Role: "admin", CreatedAt: store.Now(),
	}
	if err := s.store.CreateUser(ctx, user); err != nil {
		writeError(w, r, http.StatusConflict, "user_exists", "failed to create admin: "+err.Error())
		return
	}
	plaintext, prefix, hash, err := auth.GenerateKey(true)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "key_error", err.Error())
		return
	}
	key := &store.APIKey{
		ID: newID(), UserID: user.ID, Name: "admin key", Prefix: prefix, Hash: hash,
		Scopes: "admin,search,read", CreatedAt: store.Now(),
	}
	if err := s.store.CreateAPIKey(ctx, key); err != nil {
		writeError(w, r, http.StatusInternalServerError, "key_error", err.Error())
		return
	}
	s.setupMu.Lock()
	s.setupToken = ""
	s.setupMu.Unlock()
	// Invalidate the persisted token so it cannot be reused.
	_ = s.store.SetSetting(ctx, config.KeySetupToken, "")
	s.log.Info("", "", "setup completed, admin user "+user.Name+" created")
	writeJSON(w, http.StatusCreated, map[string]any{"user": user, "api_key": plaintext})
}

func (s *Server) validSetupToken(token string) bool {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(strings.TrimSpace(s.setupToken))) == 1
}

type loginRequest struct {
	Key string `json:"key"`
}

// handleLogin exchanges an API key for a browser session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "key is required")
		return
	}
	identity, err := s.authn.LoginWithKey(r.Context(), body.Key)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "invalid_key", "invalid API key")
		return
	}
	token, err := auth.CreateSession(r.Context(), s.store, identity.User.ID, auth.ClientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "session_error", err.Error())
		return
	}
	if err := auth.SetSessionCookies(w, r, token, s.authn.Secure(r)); err != nil {
		writeError(w, r, http.StatusInternalServerError, "session_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": identity.User, "scopes": identity.Scopes})
}

// handleLogout destroys the current session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" {
		if sess, err := s.store.GetSessionByTokenHash(r.Context(), auth.HashToken(cookie.Value)); err == nil {
			_ = s.store.DeleteSession(r.Context(), sess.ID)
		}
	}
	auth.ClearSessionCookies(w, s.authn.Secure(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMe returns the current identity.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id := s.identity(r)
	if id == nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	resp := map[string]any{
		"via":              id.Via,
		"scopes":           id.Scopes,
		"admin":            id.IsAdmin(),
		"passkeys_enabled": s.passkeys != nil && s.passkeys.Enabled(),
	}
	if id.User != nil {
		resp["user"] = id.User
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Passkeys ---

func (s *Server) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := s.identity(r)
	if s.passkeys == nil || !s.passkeys.Enabled() {
		writeError(w, r, http.StatusBadRequest, "passkeys_disabled", "passkeys require HTTPS and a configured domain (PUBLIC_ORIGIN/RP_ID)")
		return
	}
	if id == nil || id.User == nil {
		writeError(w, r, http.StatusBadRequest, "session_required", "log in with an API key before registering a passkey")
		return
	}
	existing, err := s.store.ListPasskeys(ctx, id.User.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	creation, sessionID, err := s.passkeys.BeginRegistration(id.User, existing)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "passkey_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": sessionID, "options": creation})
}

func (s *Server) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := s.identity(r)
	if s.passkeys == nil || !s.passkeys.Enabled() {
		writeError(w, r, http.StatusBadRequest, "passkeys_disabled", "passkeys are not enabled")
		return
	}
	if id == nil || id.User == nil {
		writeError(w, r, http.StatusBadRequest, "session_required", "session required")
		return
	}
	sessionID := r.URL.Query().Get("session")
	existing, err := s.store.ListPasskeys(ctx, id.User.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	name := r.URL.Query().Get("name")
	cred, err := s.passkeys.FinishRegistration(id.User, existing, sessionID, r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "passkey_error", err.Error())
		return
	}
	pk := auth.FromWebAuthnCredential(id.User.ID, name, cred)
	pk.ID = newID()
	if err := s.store.CreatePasskey(ctx, pk); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": pk.ID})
}

func (s *Server) handlePasskeyAssertBegin(w http.ResponseWriter, r *http.Request) {
	if s.passkeys == nil || !s.passkeys.Enabled() {
		writeError(w, r, http.StatusBadRequest, "passkeys_disabled", "passkeys are not enabled")
		return
	}
	assertion, sessionID, err := s.passkeys.BeginLogin()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "passkey_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": sessionID, "options": assertion})
}

func (s *Server) handlePasskeyAssertFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.passkeys == nil || !s.passkeys.Enabled() {
		writeError(w, r, http.StatusBadRequest, "passkeys_disabled", "passkeys are not enabled")
		return
	}
	sessionID := r.URL.Query().Get("session")
	user, cred, err := s.passkeys.FinishLogin(sessionID, r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "passkey_error", err.Error())
		return
	}
	// Update the stored sign counter.
	if stored, err := s.store.GetPasskeyByCredentialID(ctx, cred.ID); err == nil {
		stored.SignCount = cred.Authenticator.SignCount
		stored.LastUsedAt = store.Now()
		_ = s.store.UpdatePasskey(ctx, stored)
	}
	token, err := auth.CreateSession(ctx, s.store, user.ID, auth.ClientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "session_error", err.Error())
		return
	}
	if err := auth.SetSessionCookies(w, r, token, s.authn.Secure(r)); err != nil {
		writeError(w, r, http.StatusInternalServerError, "session_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}
