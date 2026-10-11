package httpd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/api"
	"github.com/liberide/serpent-seek/internal/auth"
	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/store"
)

// developerScope marks an API key as a developer token. It is stored in the
// api_keys.scopes column alongside the admin/read/search scopes so the
// infrastructure is reused without a schema change.
const developerScope = "developer"

// developerTokenScopes is the scope set granted to every developer token: full
// administrative access plus the search/read scopes shared by all keys. The
// `developer` marker keeps the token identifiable in the key list.
//
// Administrative scope is what lets the token reach the admin route groups;
// which endpoints it may actually call is decided separately by its per-endpoint
// permission list (see s.endpointAllowed).
const developerTokenScopes = developerScope + ",admin,search,read"

// developerToken is the secret-free view of a developer token.
type developerToken struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
	RevokedAt  string `json:"revoked_at"`
	// Permissions lists the granted endpoint ids ("METHOD /path"). It is empty
	// when AllEndpoints is true.
	Permissions  []string `json:"permissions"`
	AllEndpoints bool     `json:"all_endpoints"`
}

// developerStatus is the payload of GET /api/developer.
type developerStatus struct {
	Enabled   bool             `json:"enabled"`
	EnvLocked bool             `json:"env_locked"`
	Tokens    []developerToken `json:"tokens"`
}

// isDeveloperKey reports whether a stored key carries the developer scope.
func isDeveloperKey(k *store.APIKey) bool {
	if k == nil {
		return false
	}
	for _, s := range strings.Split(k.Scopes, ",") {
		if strings.TrimSpace(s) == developerScope {
			return true
		}
	}
	return false
}

// developerTokenView renders the safe view, collapsing the "full access"
// representations (empty or ["*"]) into AllEndpoints.
func developerTokenView(k *store.APIKey) developerToken {
	perms := auth.ParsePermissions(k.Permissions)
	all := auth.IsFullAccess(perms)
	if all {
		perms = []string{}
	}
	return developerToken{
		ID: k.ID, Name: k.Name, Prefix: k.Prefix,
		CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt, RevokedAt: k.RevokedAt,
		Permissions: perms, AllEndpoints: all,
	}
}

// normalizePermissions validates a client-supplied permission list and returns
// the JSON string to store. A nil slice means "full access" (["*"]); an explicit
// empty list means "no endpoints"; "*" is the full-access wildcard.
func normalizePermissions(perms *[]string) (string, bool) {
	if perms == nil {
		return `["*"]`, true
	}
	valid := api.EndpointIDs()
	seen := map[string]bool{}
	out := []string{}
	for _, p := range *perms {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if p == auth.PermissionAll {
			return `["*"]`, true
		}
		if !valid[p] {
			return "", false
		}
		seen[p] = true
		out = append(out, p)
	}
	b, _ := json.Marshal(out)
	return string(b), true
}

// developerEnabled reports whether developer mode is currently on.
func (s *Server) developerEnabled(ctx context.Context) bool {
	return s.settings.GetBool(ctx, config.KeyDeveloperMode)
}

// developerStatusPayload lists developer tokens. Revoked tokens are included
// so the UI can show (and clean up) their history.
func (s *Server) developerStatusPayload(ctx context.Context) (*developerStatus, error) {
	keys, err := s.store.ListAPIKeys(ctx, "")
	if err != nil {
		return nil, err
	}
	out := &developerStatus{
		Enabled:   s.developerEnabled(ctx),
		EnvLocked: s.cfg.IsEnv(config.KeyDeveloperMode),
		Tokens:    []developerToken{},
	}
	for _, k := range keys {
		if isDeveloperKey(k) {
			out.Tokens = append(out.Tokens, developerTokenView(k))
		}
	}
	return out, nil
}

// writeDeveloperStatus serialises the current developer-mode state and token
// list, or an error. Shared by GET /api/developer and PUT /api/developer/mode.
func (s *Server) writeDeveloperStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.developerStatusPayload(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleDeveloperStatus returns developer-mode state and the token list.
// GET /api/developer (admin)
func (s *Server) handleDeveloperStatus(w http.ResponseWriter, r *http.Request) {
	s.writeDeveloperStatus(w, r)
}

// handleDeveloperEndpoints lists the endpoint ids a token can be granted. Only
// non-public operations are offered (public endpoints need no permission).
// GET /api/developer/endpoints (admin)
func (s *Server) handleDeveloperEndpoints(w http.ResponseWriter, r *http.Request) {
	ops := api.Operations()
	out := make([]api.Operation, 0, len(ops))
	for _, op := range ops {
		if op.Public {
			continue
		}
		out = append(out, op)
	}
	writeJSON(w, http.StatusOK, out)
}

type developerModePayload struct {
	Enabled *bool `json:"enabled"`
}

// handleDeveloperMode toggles developer mode. PUT /api/developer/mode (admin)
func (s *Server) handleDeveloperMode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body developerModePayload
	if !readJSON(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "enabled is required")
		return
	}
	value := "false"
	if *body.Enabled {
		value = "true"
	}
	if err := s.settings.Set(ctx, config.KeyDeveloperMode, value); err != nil {
		writeError(w, r, http.StatusBadRequest, "readonly_setting",
			"developer_mode is pinned by an environment variable and is read-only")
		return
	}
	s.audit(ctx, r, "developer.mode", "", map[string]string{"enabled": value})
	s.writeDeveloperStatus(w, r)
}

// developerTokenPayload is the create body. Permissions nil = full access.
type developerTokenPayload struct {
	Name        string    `json:"name"`
	Permissions *[]string `json:"permissions"`
}

// developerTokensPatch is the edit body. Pointer fields distinguish "unchanged"
// from an explicit clear.
type developerTokensPatch struct {
	Name        *string   `json:"name"`
	Permissions *[]string `json:"permissions"`
}

// developerOwner resolves the user a developer token should belong to: the
// authenticated user when present, otherwise the first administrator (the
// open-admin / auth-disabled case has no user identity).
func (s *Server) developerOwner(ctx context.Context, r *http.Request) (*store.User, bool) {
	if id := s.identity(r); id != nil && id.User != nil {
		return id.User, true
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, false
	}
	for _, u := range users {
		if u.Role == "admin" && !u.Disabled {
			return u, true
		}
	}
	return nil, false
}

// handleCreateDeveloperToken mints a new developer token. The plaintext is
// returned exactly once. POST /api/developer/tokens (admin)
func (s *Server) handleCreateDeveloperToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.developerEnabled(ctx) {
		writeError(w, r, http.StatusForbidden, "developer_mode_disabled",
			"developer mode is disabled; enable it before creating developer tokens")
		return
	}
	var body developerTokenPayload
	if !readJSON(w, r, &body) {
		return
	}
	permissions, ok := normalizePermissions(body.Permissions)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "permissions contain an unknown endpoint id")
		return
	}
	owner, ok := s.developerOwner(ctx, r)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_request",
			"no administrator account is available to own the token")
		return
	}
	plaintext, prefix, hash, err := auth.GenerateDeveloperKey()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "key_error", err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "developer token"
	}
	key := &store.APIKey{
		ID: newID(), UserID: owner.ID, Name: name, Prefix: prefix, Hash: hash,
		Scopes: developerTokenScopes, Permissions: permissions, CreatedAt: store.Now(),
	}
	if err := s.store.CreateAPIKey(ctx, key); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(ctx, r, "developer.token.create", key.ID, map[string]string{
		"user_id": key.UserID, "name": key.Name,
	})
	// The plaintext is returned exactly once and never persisted.
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":     developerTokenView(key),
		"plaintext": plaintext,
	})
}

// handleUpdateDeveloperToken edits a developer token's name and/or endpoint
// permissions. PATCH /api/developer/tokens/{id} (admin)
func (s *Server) handleUpdateDeveloperToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	key, err := s.store.GetAPIKey(ctx, id)
	if err != nil || !isDeveloperKey(key) {
		writeError(w, r, http.StatusNotFound, "not_found", "developer token not found")
		return
	}
	var body developerTokensPatch
	if !readJSON(w, r, &body) {
		return
	}
	if body.Name != nil {
		if name := strings.TrimSpace(*body.Name); name != "" {
			key.Name = name
		}
	}
	if body.Permissions != nil {
		permissions, ok := normalizePermissions(body.Permissions)
		if !ok {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "permissions contain an unknown endpoint id")
			return
		}
		key.Permissions = permissions
	}
	if err := s.store.UpdateAPIKey(ctx, key); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(ctx, r, "developer.token.update", id, map[string]string{"name": key.Name})
	writeJSON(w, http.StatusOK, developerTokenView(key))
}

// handleDeleteDeveloperToken revokes a developer token, or deletes it
// permanently when `?purge=true` is passed. DELETE /api/developer/tokens/{id}
// (admin)
func (s *Server) handleDeleteDeveloperToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	key, err := s.store.GetAPIKey(ctx, id)
	if err != nil || !isDeveloperKey(key) {
		writeError(w, r, http.StatusNotFound, "not_found", "developer token not found")
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("purge")), "true") {
		if err := s.store.DeleteAPIKey(ctx, id); err != nil {
			writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		s.audit(ctx, r, "developer.token.delete", id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true})
		return
	}
	if key.RevokedAt == "" {
		key.RevokedAt = store.Now()
		if err := s.store.UpdateAPIKey(ctx, key); err != nil {
			writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
	}
	s.audit(ctx, r, "developer.token.revoke", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
