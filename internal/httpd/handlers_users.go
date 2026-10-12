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
	ctx := r.Context()
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if users == nil {
		users = []*store.User{}
	}
	quotas, err := s.store.ListUserQuotas(ctx)
	if err != nil {
		// A quota read failure must not break the user list.
		s.log.Warn("", "", "list quotas failed: "+err.Error())
		quotas = nil
	}
	for _, u := range users {
		q := quotas[u.ID]
		if q == nil || q.Requests <= 0 {
			continue
		}
		u.Quota = s.quotaStatus(ctx, u.ID, q)
	}
	writeJSON(w, http.StatusOK, users)
}

// quotaStatus builds the runtime quota view (configured allowance + current
// usage + reset moment) for the quota page and the user list.
func (s *Server) quotaStatus(ctx context.Context, userID string, q *store.UserQuota) *store.UserQuotaStatus {
	st := &store.UserQuotaStatus{
		Requests:    q.Requests,
		Period:      q.Period,
		WindowHours: q.WindowHours,
		TzOffset:    q.TzOffset,
		UpdatedAt:   q.UpdatedAt,
	}
	if q.Requests > 0 {
		now := time.Now()
		if used, err := s.quotaUsage(ctx, "user:"+userID, q, now); err == nil {
			st.Used = used
			st.Remaining = q.Requests - used
			if st.Remaining < 0 {
				st.Remaining = 0
			}
			if resetAt, ok := quotaResetAt(q, now); ok {
				st.ResetAt = resetAt.UTC().Format(time.RFC3339)
			}
		}
	}
	return st
}

type quotaPayload struct {
	Requests    *int    `json:"requests"`
	Period      *string `json:"period"`
	WindowHours *int    `json:"window_hours"`
	TzOffset    *int    `json:"tz_offset"`
}

// validateQuota checks a quota payload and returns the normalised quota.
// A nil field means "keep the current value" on update; on create the
// defaults apply (daily, 24h window, UTC).
func validateQuota(body quotaPayload, current *store.UserQuota) (*store.UserQuota, string) {
	q := &store.UserQuota{}
	if current != nil {
		*q = *current
	}
	if body.Requests != nil {
		if *body.Requests < 0 {
			return nil, "requests must be >= 0"
		}
		q.Requests = *body.Requests
	}
	if body.Period != nil {
		switch *body.Period {
		case store.QuotaPeriodDaily, store.QuotaPeriodWeekly, store.QuotaPeriodMonthly, store.QuotaPeriodRolling:
			q.Period = *body.Period
		default:
			return nil, "period must be one of daily|weekly|monthly|rolling"
		}
	}
	if q.Period == "" {
		q.Period = store.QuotaPeriodDaily
	}
	if body.WindowHours != nil {
		if *body.WindowHours < 1 || *body.WindowHours > 720 {
			return nil, "window_hours must be between 1 and 720"
		}
		q.WindowHours = *body.WindowHours
	}
	if q.WindowHours <= 0 {
		q.WindowHours = 24
	}
	if body.TzOffset != nil {
		if *body.TzOffset < -720 || *body.TzOffset > 840 {
			return nil, "tz_offset must be between -720 and 840 minutes"
		}
		q.TzOffset = *body.TzOffset
	}
	return q, ""
}

func (s *Server) handleGetUserQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "id")
	if _, err := s.store.GetUser(ctx, userID); err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "user not found")
		return
	}
	q, err := s.store.GetUserQuota(ctx, userID)
	if err == store.ErrNotFound {
		writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "requests": 0, "period": ""})
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.quotaStatus(ctx, userID, q))
}

func (s *Server) handlePutUserQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "id")
	if _, err := s.store.GetUser(ctx, userID); err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "user not found")
		return
	}
	var body quotaPayload
	if !readJSON(w, r, &body) {
		return
	}
	current, err := s.store.GetUserQuota(ctx, userID)
	if err != nil && err != store.ErrNotFound {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if err == store.ErrNotFound {
		current = nil
	}
	q, verr := validateQuota(body, current)
	if verr != "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", verr)
		return
	}
	q.UserID = userID
	q.UpdatedAt = store.Now()
	if err := s.store.SetUserQuota(ctx, q); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.quotas.invalidate(userID)
	s.audit(ctx, r, "quota.set", userID, map[string]string{
		"requests": itoa(q.Requests), "period": q.Period,
		"window_hours": itoa(q.WindowHours), "tz_offset": itoa(q.TzOffset),
	})
	writeJSON(w, http.StatusOK, s.quotaStatus(ctx, userID, q))
}

func (s *Server) handleDeleteUserQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "id")
	if _, err := s.store.GetUser(ctx, userID); err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err := s.store.DeleteUserQuota(ctx, userID); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.quotas.invalidate(userID)
	s.audit(ctx, r, "quota.delete", userID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	// The quota row is cascade-deleted; drop any cached copy.
	s.quotas.invalidate(id)
	s.audit(r.Context(), r, "user.delete", id, nil)
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
