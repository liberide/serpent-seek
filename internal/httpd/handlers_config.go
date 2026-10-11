package httpd

import (
	"net/http"

	"github.com/liberide/serpent-seek/internal/store"
)

// configBundle is a portable, secret-free snapshot of the instance
// configuration: provider instances (no credentials), chain graphs and
// non-secret, non-environment settings. Credentials must be re-entered after an
// import; password/secret values are never exported or accepted.
type configBundle struct {
	Version    int               `json:"version"`
	ExportedAt string            `json:"exported_at"`
	Providers  []configProvider  `json:"providers"`
	Chains     []*store.Chain    `json:"chains"`
	Settings   map[string]string `json:"settings"`
}

// configProvider keeps the source instance id so chain nodes can be remapped to
// the local provider id by name during an import.
type configProvider struct {
	ID      string            `json:"id"`
	Code    string            `json:"code"`
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	BaseURL string            `json:"base_url"`
	Params  map[string]string `json:"params"`
}

// configImportResult summarises what an import changed.
type configImportResult struct {
	ProvidersCreated int `json:"providers_created"`
	ProvidersUpdated int `json:"providers_updated"`
	ChainsCreated    int `json:"chains_created"`
	ChainsUpdated    int `json:"chains_updated"`
	ChainsSkipped    int `json:"chains_skipped"`
	SettingsApplied  int `json:"settings_applied"`
}

// handleExportConfig returns a secret-free configuration snapshot.
func (s *Server) handleExportConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providers, err := s.store.ListProviders(ctx)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	chains, err := s.store.ListChains(ctx)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	out := configBundle{Version: 1, ExportedAt: store.Now(), Chains: chains, Settings: map[string]string{}}
	for _, p := range providers {
		out.Providers = append(out.Providers, configProvider{
			ID: p.ID, Code: p.Code, Name: p.Name, Enabled: p.Enabled, BaseURL: p.BaseURL, Params: p.Params,
		})
	}
	if out.Providers == nil {
		out.Providers = []configProvider{}
	}
	if out.Chains == nil {
		out.Chains = []*store.Chain{}
	}
	// Only explicitly stored, non-secret, non-environment settings are exported.
	stored, _ := s.store.AllSettings(ctx)
	env := s.cfg.EnvKeys()
	for _, d := range s.cfg.Descriptors() {
		if d.Secret || d.EnvOnly || env[d.Key] {
			continue
		}
		if v, ok := stored[d.Key]; ok {
			out.Settings[d.Key] = v
		}
	}
	writeJSONIndent(w, http.StatusOK, out)
}

// handleImportConfig applies a configuration bundle. It never imports secrets:
// existing credentials and proxy passwords are preserved. With ?overwrite=true
// existing providers and chains (matched by name) are updated instead of skipped.
func (s *Server) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var bundle configBundle
	if !readJSON(w, r, &bundle) {
		return
	}
	overwrite := r.URL.Query().Get("overwrite") == "true"
	res := configImportResult{}

	// Providers, matched by unique name; source id -> name for chain remapping.
	localProviders := map[string]string{}
	srcName := map[string]string{}
	existingProviders, _ := s.store.ListProviders(ctx)
	for _, p := range existingProviders {
		localProviders[p.Name] = p.ID
	}
	for _, p := range bundle.Providers {
		if p.Name == "" || p.Code == "" {
			continue
		}
		if p.ID != "" {
			srcName[p.ID] = p.Name
		}
		if id, ok := localProviders[p.Name]; ok {
			if !overwrite {
				continue
			}
			existing, err := s.store.GetProvider(ctx, id)
			if err != nil {
				continue
			}
			existing.Code, existing.Enabled, existing.BaseURL = p.Code, p.Enabled, p.BaseURL
			existing.Params = p.Params
			existing.UpdatedAt = store.Now()
			if err := s.store.UpsertProvider(ctx, existing); err == nil {
				res.ProvidersUpdated++
			}
			continue
		}
		np := &store.Provider{
			ID: newID(), Code: p.Code, Name: p.Name, Enabled: p.Enabled, BaseURL: p.BaseURL,
			Params: p.Params, Credentials: map[string]string{}, UpdatedAt: store.Now(),
		}
		if np.Params == nil {
			np.Params = map[string]string{}
		}
		if err := s.store.UpsertProvider(ctx, np); err == nil {
			localProviders[np.Name] = np.ID
			res.ProvidersCreated++
		}
	}

	// Chains, matched by name. Node provider ids are remapped by provider name.
	existingChains, _ := s.store.ListChains(ctx)
	byName := map[string]*store.Chain{}
	for _, c := range existingChains {
		byName[c.Name] = c
	}
	for _, c := range bundle.Chains {
		if c == nil || c.Name == "" {
			continue
		}
		nodes := make([]store.ChainNode, len(c.Nodes))
		for i, n := range c.Nodes {
			n.ID, n.ChainID = "", ""
			if name, ok := srcName[n.ProviderID]; ok {
				if localID, ok := localProviders[name]; ok {
					n.ProviderID = localID
				}
			}
			nodes[i] = n
		}
		edges := make([]store.ChainEdge, len(c.Edges))
		for i, e := range c.Edges {
			e.ID, e.ChainID = "", ""
			edges[i] = e
		}
		draft := &store.Chain{
			ID: newID(), Name: c.Name, Active: false, Mode: c.Mode,
			Nodes: nodes, Edges: edges,
		}
		if existing, ok := byName[c.Name]; ok {
			if !overwrite {
				res.ChainsSkipped++
				continue
			}
			draft.ID = existing.ID
			draft.Active = existing.Active
			draft.Version = existing.Version
			draft.CreatedAt = existing.CreatedAt
		}
		if validation := s.validateDraft(ctx, draft); !validation.OK {
			res.ChainsSkipped++
			continue
		}
		if err := s.store.SaveChain(ctx, draft); err != nil {
			res.ChainsSkipped++
			continue
		}
		if _, existed := byName[c.Name]; existed {
			res.ChainsUpdated++
		} else {
			res.ChainsCreated++
		}
		byName[c.Name] = draft
	}

	// Settings: only descriptors that exist and are not secret/env-pinned.
	descriptors := map[string]bool{}
	for _, d := range s.cfg.Descriptors() {
		if !d.Secret && !d.EnvOnly {
			descriptors[d.Key] = true
		}
	}
	env := s.cfg.EnvKeys()
	for key, value := range bundle.Settings {
		if !descriptors[key] || env[key] {
			continue
		}
		if err := s.settings.Set(ctx, key, value); err == nil {
			res.SettingsApplied++
		}
	}

	s.audit(ctx, r, "config.import", "", map[string]string{
		"providers_created": itoa(res.ProvidersCreated),
		"chains_created":    itoa(res.ChainsCreated),
		"settings_applied":  itoa(res.SettingsApplied),
	})
	writeJSON(w, http.StatusOK, res)
}
