package httpd

import (
	"context"
	"net/http"
	"strconv"

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

// fillChainKeyCounts annotates a chain with how many API tokens are bound to
// it and how many of those are still active (not revoked). These values are
// derived, not stored on the chain row.
func (s *Server) fillChainKeyCounts(ctx context.Context, chain *store.Chain) {
	if n, err := s.store.CountKeysByChain(ctx, chain.ID); err == nil {
		chain.BoundKeys = n
	}
	if n, err := s.store.CountActiveKeysByChain(ctx, chain.ID); err == nil {
		chain.ActiveBoundKeys = n
	}
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
	for _, c := range chains {
		s.fillChainKeyCounts(r.Context(), c)
	}
	writeJSON(w, http.StatusOK, chains)
}

func (s *Server) handleGetChain(w http.ResponseWriter, r *http.Request) {
	chain, err := s.store.GetChain(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "chain not found")
		return
	}
	if n, err := s.store.CountKeysByChain(r.Context(), chain.ID); err == nil {
		chain.BoundKeys = n
	}
	if n, err := s.store.CountActiveKeysByChain(r.Context(), chain.ID); err == nil {
		chain.ActiveBoundKeys = n
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
	s.audit(ctx, r, "chain.create", chain.ID, map[string]string{"name": chain.Name})
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
	chain.Version = existing.Version
	chain.CreatedAt = existing.CreatedAt
	if err := s.store.SaveChain(r.Context(), &chain); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(ctx, r, "chain.update", chain.ID, map[string]string{
		"name": chain.Name, "version": strconv.Itoa(chain.Version),
	})
	if n, err := s.store.CountKeysByChain(ctx, chain.ID); err == nil {
		chain.BoundKeys = n
	}
	if n, err := s.store.CountActiveKeysByChain(ctx, chain.ID); err == nil {
		chain.ActiveBoundKeys = n
	}
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleDeleteChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	chain, err := s.store.GetChain(ctx, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "chain not found")
		return
	}
	// The active chain is the fallback for every unbound token; deleting it
	// would leave those tokens without a runnable chain.
	if chain.Active {
		writeError(w, r, http.StatusConflict, "chain_active", "the active chain cannot be deleted; activate another chain first")
		return
	}
	// Bound tokens (including active ones) are simply unbound and fall back to
	// the default (active) chain, so deleting a chain never breaks a token.
	active, _ := s.store.CountActiveKeysByChain(ctx, id)
	unbound, err := s.store.ClearKeyChainRefs(ctx, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if err := s.store.DeleteChain(ctx, id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	s.audit(ctx, r, "chain.delete", id, map[string]string{
		"name": chain.Name, "unbound_keys": strconv.FormatInt(unbound, 10),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unbound_keys": unbound, "moved_to_default": active})
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
	s.audit(ctx, r, "chain.activate", chain.ID, map[string]string{"name": chain.Name})
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
	s.audit(ctx, r, "chain.duplicate", copyChain.ID, map[string]string{"source": src.ID, "name": copyChain.Name})
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
