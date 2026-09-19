package httpd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/store"
)

// handleListRequests returns a filtered, paginated history page.
func (s *Server) handleListRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset, page := pagination(r, 50)
	q := r.URL.Query()
	filter := store.RequestFilter{
		Status:   q.Get("status"),
		Provider: q.Get("provider"),
		From:     q.Get("from"),
		To:       q.Get("to"),
		Query:    q.Get("q"),
	}
	items, total, err := s.store.ListRequests(ctx, filter, store.Page{Limit: limit, Offset: offset})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if items == nil {
		items = []*store.Request{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "limit": limit,
	})
}

// handleGetRequest returns one request with steps and its chain snapshot.
func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	req, err := s.store.GetRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, r, http.StatusNotFound, "not_found", "request not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}
