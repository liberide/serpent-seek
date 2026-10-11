package httpd

import (
	"context"
	"net/http"

	"github.com/liberide/serpent-seek/internal/store"
)

// audit records an administrative mutation in the audit log. It never fails the
// request: a storage error is logged and ignored. Callers must never put secret
// values into details (only key names / non-secret identifiers).
func (s *Server) audit(ctx context.Context, r *http.Request, action, target string, details map[string]string) {
	if s.store == nil {
		return
	}
	entry := &store.AuditEntry{
		ID: newID(), TS: store.Now(), Action: action, Target: target, Details: details,
	}
	if id := s.identity(r); id != nil {
		entry.Via = id.Via
		if id.User != nil {
			entry.UserID = id.User.ID
			entry.User = id.User.Name
		}
	}
	if s.clientIP != nil {
		entry.IP = s.clientIP.ClientIP(r)
	}
	if err := s.store.AddAudit(ctx, entry); err != nil {
		s.log.Warn("", "", "audit write failed: "+err.Error())
	}
}

// handleListAudit returns a filtered, paginated page of audit records.
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := pagination(r, 100)
	q := r.URL.Query()
	items, total, err := s.store.ListAudit(r.Context(), store.AuditFilter{
		Action: q.Get("action"),
		Actor:  q.Get("actor"),
		From:   q.Get("from"),
		To:     q.Get("to"),
		Query:  q.Get("q"),
	}, store.Page{Limit: limit, Offset: offset})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if items == nil {
		items = []*store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "limit": limit,
	})
}
