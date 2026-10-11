package httpd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/store"
)

// handleListRequests returns a filtered history page. A `cursor` from a
// previous response's next_cursor takes precedence over `page`.
func (s *Server) handleListRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, offset, page := pagination(r, 50)
	q := r.URL.Query()
	cursor := q.Get("cursor")
	filter := store.RequestFilter{
		Status:   q.Get("status"),
		Provider: q.Get("provider"),
		From:     q.Get("from"),
		To:       q.Get("to"),
		Query:    q.Get("q"),
		Cursor:   cursor,
	}
	if cursor != "" {
		offset, page = 0, 1
	}
	// Scope to the caller for non-admins.
	if !s.isAdmin(r) {
		filter.UserID = s.identityUserID(r)
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
		"next_cursor": nextRequestCursor(items, limit),
	})
}

// nextRequestCursor returns the keyset cursor of the last row, or nil when the
// page is the last one.
func nextRequestCursor(items []*store.Request, limit int) any {
	if limit <= 0 || len(items) < limit {
		return nil
	}
	last := items[len(items)-1]
	return store.EncodeCursor(last.CreatedAt, last.ID)
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
	if !s.isAdmin(r) {
		if owner := s.identityUserID(r); owner == "" || req.UserID != owner {
			writeError(w, r, http.StatusNotFound, "not_found", "request not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, req)
}
