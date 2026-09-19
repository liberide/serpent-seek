package httpd

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// providerView masks credentials and reports which secrets are set.
type providerView struct {
	ID             string            `json:"id"`
	Code           string            `json:"code"`
	Name           string            `json:"name"`
	Enabled        bool              `json:"enabled"`
	BaseURL        string            `json:"base_url"`
	Params         map[string]string `json:"params"`
	CredentialsSet map[string]bool   `json:"credentials_set"`
	UpdatedAt      string            `json:"updated_at"`
}

func toProviderView(p *store.Provider) providerView {
	set := map[string]bool{}
	for k, v := range p.Credentials {
		if v != "" {
			set[k] = true
		}
	}
	params := p.Params
	if params == nil {
		params = map[string]string{}
	}
	return providerView{
		ID: p.ID, Code: p.Code, Name: p.Name, Enabled: p.Enabled, BaseURL: p.BaseURL,
		Params: params, CredentialsSet: set, UpdatedAt: p.UpdatedAt,
	}
}

// driverCodes returns the known provider driver types (registry).
func (s *Server) driverCodes() map[string]bool {
	out := map[string]bool{}
	if s.engine != nil && s.engine.Registry() != nil {
		for _, code := range s.engine.Registry().Codes() {
			out[code] = true
		}
	}
	return out
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.store.ListProviders(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	views := make([]providerView, 0, len(providers))
	for _, p := range providers {
		views = append(views, toProviderView(p))
	}
	writeJSON(w, http.StatusOK, views)
}

// driverSchema is the public driver metadata used by the provider form.
type driverSchema struct {
	Code        string                      `json:"code"`
	Name        string                      `json:"name"`
	Deprecated  bool                        `json:"deprecated"`
	Credentials []providers.CredentialField `json:"credentials"`
	Params      []providers.ParamField      `json:"params"`
	Hints       []string                    `json:"hints,omitempty"`
}

func (s *Server) handleListDrivers(w http.ResponseWriter, r *http.Request) {
	var out []driverSchema
	if s.engine != nil && s.engine.Registry() != nil {
		for _, code := range s.engine.Registry().Codes() {
			p, ok := s.engine.Registry().Get(code)
			if !ok {
				continue
			}
			schema := p.Schema()
			out = append(out, driverSchema{
				Code:        schema.Code,
				Name:        schema.Name,
				Deprecated:  schema.Deprecated,
				Credentials: schema.Credentials,
				Params:      schema.Params,
				Hints:       schema.Hints,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetProvider(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "provider not found")
		return
	}
	writeJSON(w, http.StatusOK, toProviderView(p))
}

type providerPayload struct {
	Code        string            `json:"code"`
	Name        string            `json:"name"`
	Enabled     *bool             `json:"enabled"`
	BaseURL     *string           `json:"base_url"`
	Params      map[string]string `json:"params"`
	Credentials map[string]string `json:"credentials"`
}

// handleCreateProvider adds a new provider instance. Multiple instances may
// share the same driver `code` with different names and credentials.
func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body providerPayload
	if !readJSON(w, r, &body) {
		return
	}
	code := strings.ToLower(strings.TrimSpace(body.Code))
	if code == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "code (driver type) is required")
		return
	}
	if !s.driverCodes()[code] {
		writeError(w, r, http.StatusBadRequest, "unknown_driver", "unknown provider driver: "+code)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = code
	}
	if s.nameTaken(ctx, name, "") {
		writeError(w, r, http.StatusConflict, "name_taken", "provider name is already in use: "+name)
		return
	}
	p := &store.Provider{
		ID: newID(), Code: code, Name: name, Enabled: true,
		Credentials: map[string]string{}, Params: map[string]string{},
	}
	if body.Enabled != nil {
		p.Enabled = *body.Enabled
	}
	if body.BaseURL != nil {
		p.BaseURL = *body.BaseURL
	}
	for k, v := range body.Params {
		p.Params[k] = v
	}
	for k, v := range body.Credentials {
		if v != "" {
			p.Credentials[k] = v
		}
	}
	if key := s.requiredParamError(code, p.Params); key != "" {
		writeError(w, r, http.StatusBadRequest, "missing_param", "required provider parameter is empty: "+key)
		return
	}
	if err := s.store.UpsertProvider(ctx, p); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toProviderView(p))
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	existing, err := s.store.GetProvider(ctx, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "provider not found")
		return
	}
	var body providerPayload
	if !readJSON(w, r, &body) {
		return
	}
	if name := strings.TrimSpace(body.Name); name != "" && name != existing.Name {
		if s.nameTaken(ctx, name, id) {
			writeError(w, r, http.StatusConflict, "name_taken", "provider name is already in use: "+name)
			return
		}
		existing.Name = name
	}
	if body.Enabled != nil {
		existing.Enabled = *body.Enabled
	}
	if body.BaseURL != nil {
		existing.BaseURL = *body.BaseURL
	}
	if body.Params != nil {
		existing.Params = body.Params
	}
	if existing.Credentials == nil {
		existing.Credentials = map[string]string{}
	}
	// Credentials are write-only: only non-empty values are stored, so an
	// existing secret is preserved when the field is left blank in the UI.
	for k, v := range body.Credentials {
		if v != "" {
			existing.Credentials[k] = v
		}
	}
	if key := s.requiredParamError(existing.Code, existing.Params); key != "" {
		writeError(w, r, http.StatusBadRequest, "missing_param", "required provider parameter is empty: "+key)
		return
	}
	if err := s.store.UpsertProvider(ctx, existing); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toProviderView(existing))
}

// handlePatchProvider toggles enabled and/or edits the base_url (never a secret).
func (s *Server) handlePatchProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	var body struct {
		BaseURL *string `json:"base_url"`
		Enabled *bool   `json:"enabled"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.BaseURL == nil && body.Enabled == nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "nothing to update (base_url or enabled required)")
		return
	}
	if body.Enabled != nil {
		if err := s.store.SetProviderEnabled(ctx, id, *body.Enabled); err != nil {
			if err == store.ErrNotFound {
				writeError(w, r, http.StatusNotFound, "not_found", "provider not found")
				return
			}
			writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
	}
	if body.BaseURL != nil {
		if err := s.store.UpdateProviderBaseURL(ctx, id, *body.BaseURL); err != nil {
			if err == store.ErrNotFound {
				writeError(w, r, http.StatusNotFound, "not_found", "provider not found")
				return
			}
			writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
	}
	p, _ := s.store.GetProvider(ctx, id)
	writeJSON(w, http.StatusOK, toProviderView(p))
}

// handleDeleteProvider removes an instance, refusing when any chain references it.
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if chains := s.chainsUsingProvider(ctx, id); len(chains) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"error": map[string]any{
			"code": "provider_in_use", "message": "provider is used by chains", "chains": chains,
		}})
		return
	}
	if err := s.store.DeleteProvider(ctx, id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTestProvider runs a small query against the provider instance.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	test := s.engine.TestProvider(r.Context(), chi.URLParam(r, "id"))
	resp := map[string]any{
		"kind":    test.Result.Kind,
		"ok":      test.Result.OK,
		"http":    test.Result.HTTPStatus,
		"engine":  test.Result.Engine,
		"error":   test.Result.Error,
		"took_ms": test.TookMS,
		"results": test.Result.Rows,
	}
	if test.Result.Answer != "" {
		resp["answer"] = test.Result.Answer
	}
	if len(test.Result.Rows) > 3 {
		resp["results"] = test.Result.Rows[:3]
	}
	writeJSON(w, http.StatusOK, resp)
}

// requiredParamError returns the key of the first empty required parameter for
// the given driver, or "" when every required parameter is satisfied.
func (s *Server) requiredParamError(code string, params map[string]string) string {
	if s.engine == nil || s.engine.Registry() == nil {
		return ""
	}
	p, ok := s.engine.Registry().Get(code)
	if !ok {
		return ""
	}
	for _, f := range p.Schema().Params {
		if !f.Required {
			continue
		}
		if strings.TrimSpace(params[f.Key]) == "" {
			return f.Key
		}
	}
	return ""
}

func (s *Server) nameTaken(ctx context.Context, name, excludeID string) bool {
	providers, err := s.store.ListProviders(ctx)
	if err != nil {
		return false
	}
	for _, p := range providers {
		if p.ID != excludeID && strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

// chainsUsingProvider returns the names of chains that reference a provider id.
func (s *Server) chainsUsingProvider(ctx context.Context, id string) []string {
	chains, err := s.store.ListChains(ctx)
	if err != nil {
		return nil
	}
	var out []string
	for _, chain := range chains {
		for _, node := range chain.Nodes {
			if node.ProviderID == id {
				out = append(out, chain.Name)
				break
			}
		}
	}
	return out
}
