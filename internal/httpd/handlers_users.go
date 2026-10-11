package httpd

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/store"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if users == nil {
		users = []*store.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

type userPayload struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Disabled *bool  `json:"disabled"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body userPayload
	if !readJSON(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	role := body.Role
	if role != "admin" && role != "viewer" {
		role = "viewer"
	}
	user := &store.User{ID: newID(), Name: body.Name, Role: role, CreatedAt: store.Now()}
	if err := s.store.CreateUser(r.Context(), user); err != nil {
		writeError(w, r, http.StatusConflict, "user_exists", err.Error())
		return
	}
	s.audit(r.Context(), r, "user.create", user.ID, map[string]string{"name": user.Name, "role": user.Role})
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, err := s.store.GetUser(ctx, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "user not found")
		return
	}
	var body userPayload
	if !readJSON(w, r, &body) {
		return
	}
	if body.Name != "" {
		user.Name = body.Name
	}
	if body.Role == "admin" || body.Role == "viewer" {
		user.Role = body.Role
	}
	if body.Disabled != nil {
		user.Disabled = *body.Disabled
	}
	if err := s.store.UpdateUser(ctx, user); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(ctx, r, "user.update", user.ID, map[string]string{"name": user.Name, "role": user.Role})
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteUser(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(r.Context(), r, "user.delete", chi.URLParam(r, "id"), nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- keys ---

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys, err := s.store.ListAPIKeys(ctx, r.URL.Query().Get("user_id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if keys == nil {
		keys = []*store.APIKey{}
	}
	// Attach today's request count per key (used by the Users page).
	day := time.Now().UTC().Format("2006-01-02")
	if usage, err := s.store.UsageByDay(ctx, day); err == nil {
		for _, k := range keys {
			k.UsedToday = usage["key:"+k.ID]
		}
	}
	writeJSON(w, http.StatusOK, keys)
}

type keyPayload struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Scope  string `json:"scope"` // search|admin
	// ChainID optionally binds the new token to a chain ("" = active chain).
	ChainID string `json:"chain_id"`
}

// chainExists reports whether a non-empty chain id refers to a stored chain.
func (s *Server) chainExists(ctx context.Context, id string) bool {
	if strings.TrimSpace(id) == "" {
		return true
	}
	_, err := s.store.GetChain(ctx, id)
	return err == nil
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body keyPayload
	if !readJSON(w, r, &body) {
		return
	}
	userID := body.UserID
	if userID == "" {
		if id := s.identity(r); id != nil && id.User != nil {
			userID = id.User.ID
		}
	}
	if userID == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}
	if _, err := s.store.GetUser(ctx, userID); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "user not found")
		return
	}
	chainID := strings.TrimSpace(body.ChainID)
	if !s.chainExists(ctx, chainID) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "chain not found")
		return
	}
	admin := body.Scope == "admin"
	plaintext, prefix, hash, err := auth.GenerateKey(admin)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "key_error", err.Error())
		return
	}
	scopes := "search"
	if admin {
		scopes = "admin,search,read"
	}
	name := body.Name
	if name == "" {
		name = "api key"
	}
	key := &store.APIKey{
		ID: newID(), UserID: userID, Name: name, Prefix: prefix, Hash: hash,
		Scopes: scopes, ChainID: chainID, CreatedAt: store.Now(),
	}
	if err := s.store.CreateAPIKey(ctx, key); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(ctx, r, "key.create", key.ID, map[string]string{"user_id": key.UserID, "name": key.Name, "scopes": key.Scopes})
	// The plaintext is returned exactly once.
	writeJSON(w, http.StatusCreated, map[string]any{"key": key, "plaintext": plaintext})
}

// keyUpdatePayload carries partial changes for PATCH /api/keys/{id}.
// Pointer fields distinguish "not provided" from an explicit clear.
type keyUpdatePayload struct {
	Name      *string `json:"name"`
	ChainID   *string `json:"chain_id"`
	ExpiresAt *string `json:"expires_at"`
	Revoke    *bool   `json:"revoke"`
}

func (s *Server) handleUpdateKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key, err := s.store.GetAPIKey(ctx, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "key not found")
		return
	}
	var body keyUpdatePayload
	if !readJSON(w, r, &body) {
		return
	}
	if body.Name != nil {
		if name := strings.TrimSpace(*body.Name); name != "" {
			key.Name = name
		}
	}
	if body.ChainID != nil {
		chainID := strings.TrimSpace(*body.ChainID)
		if !s.chainExists(ctx, chainID) {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "chain not found")
			return
		}
		key.ChainID = chainID
	}
	if body.ExpiresAt != nil {
		key.ExpiresAt = strings.TrimSpace(*body.ExpiresAt)
	}
	if body.Revoke != nil && *body.Revoke && key.RevokedAt == "" {
		key.RevokedAt = store.Now()
	}
	if err := s.store.UpdateAPIKey(ctx, key); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(ctx, r, "key.update", key.ID, map[string]string{"chain_id": key.ChainID, "name": key.Name})
	writeJSON(w, http.StatusOK, key)
}

func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys, err := s.store.ListAPIKeys(ctx, "")
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	for _, k := range keys {
		if k.ID == id {
			k.RevokedAt = store.Now()
			if err := s.store.UpdateAPIKey(ctx, k); err != nil {
				writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
				return
			}
			s.audit(ctx, r, "key.revoke", id, nil)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
	}
	writeError(w, r, http.StatusNotFound, "not_found", "key not found")
}

// --- passkeys ---

func (s *Server) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	passkeys, err := s.store.ListPasskeys(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(passkeys))
	for _, p := range passkeys {
		out = append(out, map[string]any{
			"id": p.ID, "user_id": p.UserID, "name": p.Name,
			"created_at": p.CreatedAt, "last_used_at": p.LastUsedAt,
			"sign_count": p.SignCount,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeletePasskey(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(r.Context(), r, "passkey.delete", chi.URLParam(r, "id"), nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
