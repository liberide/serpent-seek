package httpd

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/engine"
	"github.com/liberide/serpent-seek/internal/store"
)

// validateDraft normalizes and validates a chain graph from the editor.
func (s *Server) validateDraft(ctx context.Context, chain *store.Chain) engine.ValidationResult {
	if chain.Mode == "" {
		chain.Mode = store.ChainModeFirstSuccess
	}
	maxAttempts := s.settings.GetInt(ctx, config.KeyMaxAttempts)
	return engine.ValidateChain(chain, s.knownProviders(ctx), maxAttempts)
}

func (s *Server) handleListChains(w http.ResponseWriter, r *http.Request) {
	chains, err := s.store.ListChains(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if chains == nil {
		chains = []*store.Chain{}
	}
	writeJSON(w, http.StatusOK, chains)
}

func (s *Server) handleGetChain(w http.ResponseWriter, r *http.Request) {
	chain, err := s.store.GetChain(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "chain not found")
		return
	}
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleCreateChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var chain store.Chain
	if !readJSON(w, r, &chain) {
		return
	}
	if validation := s.validateDraft(ctx, &chain); !validation.OK {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{
			"code": "invalid_chain", "message": "chain validation failed", "details": validation.Errors,
		}})
		return
	}
	chain.ID = newID()
	chain.Active = false
	if err := s.store.SaveChain(r.Context(), &chain); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, chain)
}

func (s *Server) handleUpdateChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	var chain store.Chain
	if !readJSON(w, r, &chain) {
		return
	}
	if validation := s.validateDraft(ctx, &chain); !validation.OK {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{
			"code": "invalid_chain", "message": "chain validation failed", "details": validation.Errors,
		}})
		return
	}
	existing, err := s.store.GetChain(r.Context(), id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "chain not found")
		return
	}
	chain.ID = id
	chain.Active = existing.Active
	chain.CreatedAt = existing.CreatedAt
	if err := s.store.SaveChain(r.Context(), &chain); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleDeleteChain(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteChain(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleActivateChain makes a chain active. A chain with graph errors
// (no/duplicate start block, cycle, unreachable nodes) cannot be activated.
func (s *Server) handleActivateChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chain, err := s.store.GetChain(ctx, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "chain not found")
		return
	}
	if validation := s.validateDraft(ctx, chain); !validation.OK {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{
			"code": "invalid_chain", "message": "chain validation failed", "details": validation.Errors,
		}})
		return
	}
	if err := s.store.ActivateChain(ctx, chain.ID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, r, http.StatusNotFound, "not_found", "chain not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDuplicateChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	src, err := s.store.GetChain(ctx, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "chain not found")
		return
	}
	copyChain := &store.Chain{
		ID:      newID(),
		Name:    src.Name + " (copy)",
		Active:  false,
		Mode:    src.Mode,
		Version: 0,
		Nodes:   make([]store.ChainNode, len(src.Nodes)),
		Edges:   make([]store.ChainEdge, len(src.Edges)),
	}
	for i, n := range src.Nodes {
		n.ID = ""
		n.ChainID = ""
		copyChain.Nodes[i] = n
	}
	for i, e := range src.Edges {
		e.ID = ""
		e.ChainID = ""
		copyChain.Edges[i] = e
	}
	if err := s.store.SaveChain(ctx, copyChain); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, copyChain)
}

func (s *Server) handleValidateChain(w http.ResponseWriter, r *http.Request) {
	var chain store.Chain
	if !readJSON(w, r, &chain) {
		return
	}
	writeJSON(w, http.StatusOK, s.validateDraft(r.Context(), &chain))
}

// knownProviders returns the set of configured provider *instance* ids.
func (s *Server) knownProviders(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	if providers, err := s.store.ListProviders(ctx); err == nil {
		for _, p := range providers {
			out[p.ID] = true
		}
	}
	return out
}
