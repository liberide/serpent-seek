package httpd

import (
	"net/http"
	"strconv"

	"github.com/liberide/serpent-seek/internal/store"
)

// handleListLogs returns filtered log entries. A `cursor` from a previous
// response's next_cursor takes precedence over `page`.
func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := pagination(r, 100)
	q := r.URL.Query()
	cursor := q.Get("cursor")
	if cursor != "" {
		offset, page = 0, 1
	}
	items, total, err := s.store.ListLogs(r.Context(),
		store.LogFilter{Level: q.Get("level"), Query: q.Get("q"), Cursor: cursor},
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
		"next_cursor": nextLogCursor(items, limit),
	})
}

// nextLogCursor returns the keyset cursor of the last row, or nil when the page
// is the last one.
func nextLogCursor(items []*store.LogEntry, limit int) any {
	if limit <= 0 || len(items) < limit {
		return nil
	}
	return store.EncodeCursor(strconv.FormatInt(items[len(items)-1].ID, 10))
}
