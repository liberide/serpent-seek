package httpd

import (
	"net/http"

	"github.com/liberide/serpent-seek/internal/store"
)

// handleStatsSummary returns dashboard aggregates plus provider status.
func (s *Server) handleStatsSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		summary *store.StatsSummary
		err     error
	)
	if s.isAdmin(r) {
		summary, err = s.store.Summary(ctx, 7)
	} else {
		summary, err = s.store.SummaryForUser(ctx, 7, s.identityUserID(r))
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	providers, _ := s.store.ListProviders(ctx)
	views := make([]providerView, 0, len(providers))
	for _, p := range providers {
		views = append(views, toProviderView(p))
	}
	if summary.Recent == nil {
		summary.Recent = []*store.Request{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":   summary,
		"providers": views,
		"jobs":      s.jobs.Status(),
	})
}
