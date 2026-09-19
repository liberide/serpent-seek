package httpd

import (
	"net/http"

	"github.com/liberide/serpent-seek/internal/store"
)

// handleListLogs returns filtered, paginated log entries.
func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := pagination(r, 100)
	q := r.URL.Query()
	items, total, err := s.store.ListLogs(r.Context(),
		store.LogFilter{Level: q.Get("level"), Query: q.Get("q")},
		store.Page{Limit: limit, Offset: offset})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if items == nil {
		items = []*store.LogEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "limit": limit,
	})
}
