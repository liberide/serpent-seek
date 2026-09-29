package httpd

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/store"
)

// proxyView hides the write-only password and reports whether one is set.
type proxyView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        string `json:"port"`
	Username    string `json:"username"`
	PasswordSet bool   `json:"password_set"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toProxyView(p *store.Proxy) proxyView {
	return proxyView{
		ID: p.ID, Name: p.Name, Enabled: p.Enabled, Type: p.Type,
		Host: p.Host, Port: p.Port, Username: p.Username,
		PasswordSet: p.Password != "", CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func (s *Server) handleListProxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := s.store.ListProxies(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	views := make([]proxyView, 0, len(proxies))
	for _, p := range proxies {
		views = append(views, toProxyView(p))
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleGetProxy(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetProxy(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "proxy not found")
		return
	}
	writeJSON(w, http.StatusOK, toProxyView(p))
}

type proxyPayload struct {
	Name     string `json:"name"`
	Enabled  *bool  `json:"enabled"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// normalizeProxyType maps aliases to a supported proxy scheme.
func normalizeProxyType(t string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "http":
		return "http", true
	case "https":
		return "https", true
	case "socks", "socks5", "socks5h":
		return "socks5", true
	default:
		return "", false
	}
}

func (s *Server) handleCreateProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body proxyPayload
	if !readJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if s.proxyNameTaken(ctx, name, "") {
		writeError(w, r, http.StatusConflict, "name_taken", "proxy name is already in use: "+name)
		return
	}
	host := strings.TrimSpace(body.Host)
	if host == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "host is required")
		return
	}
	proxyType, ok := normalizeProxyType(body.Type)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid proxy type: "+body.Type)
		return
	}
	p := &store.Proxy{
		ID: newID(), Name: name, Enabled: true, Type: proxyType,
		Host: host, Port: strings.TrimSpace(body.Port),
		Username: strings.TrimSpace(body.Username), Password: body.Password,
		CreatedAt: store.Now(),
	}
	if body.Enabled != nil {
		p.Enabled = *body.Enabled
	}
	if err := s.store.UpsertProxy(ctx, p); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toProxyView(p))
}

func (s *Server) handleUpdateProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	existing, err := s.store.GetProxy(ctx, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "proxy not found")
		return
	}
	var body proxyPayload
	if !readJSON(w, r, &body) {
		return
	}
	if name := strings.TrimSpace(body.Name); name != "" && name != existing.Name {
		if s.proxyNameTaken(ctx, name, id) {
			writeError(w, r, http.StatusConflict, "name_taken", "proxy name is already in use: "+name)
			return
		}
		existing.Name = name
	}
	if host := strings.TrimSpace(body.Host); host != "" {
		existing.Host = host
	}
	if proxyType, ok := normalizeProxyType(body.Type); ok {
		existing.Type = proxyType
	}
	existing.Port = strings.TrimSpace(body.Port)
	existing.Username = strings.TrimSpace(body.Username)
	// Password is write-only: a blank value keeps the stored secret.
	if body.Password != "" {
		existing.Password = body.Password
	}
	if body.Enabled != nil {
		existing.Enabled = *body.Enabled
	}
	if err := s.store.UpsertProxy(ctx, existing); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toProxyView(existing))
}

// handlePatchProxy toggles only the enabled flag (used by the list switch).
func (s *Server) handlePatchProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "nothing to update (enabled required)")
		return
	}
	if err := s.store.SetProxyEnabled(ctx, id, *body.Enabled); err != nil {
		if err == store.ErrNotFound {
			writeError(w, r, http.StatusNotFound, "not_found", "proxy not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	p, _ := s.store.GetProxy(ctx, id)
	writeJSON(w, http.StatusOK, toProxyView(p))
}

// handleDeleteProxy removes a proxy and clears it from any provider that had
// selected it (providers fall back to a direct connection).
func (s *Server) handleDeleteProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if _, err := s.store.GetProxy(ctx, id); err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "proxy not found")
		return
	}
	names := s.providersUsingProxy(ctx, id)
	affected, err := s.store.ClearProxyRefs(ctx, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if err := s.store.DeleteProxy(ctx, id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": affected, "providers": names})
}

func (s *Server) proxyNameTaken(ctx context.Context, name, excludeID string) bool {
	proxies, err := s.store.ListProxies(ctx)
	if err != nil {
		return false
	}
	for _, p := range proxies {
		if p.ID != excludeID && strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}
