package sqlite

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

func TestAuditLog(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)

	if err := st.AddAudit(ctx, &store.AuditEntry{
		ID: "a1", TS: store.Now(), UserID: "u1", User: "admin", Via: "key",
		IP: "127.0.0.1", Action: "chain.create", Target: "c1",
		Details: map[string]string{"name": "search"},
	}); err != nil {
		t.Fatalf("AddAudit: %v", err)
	}
	if err := st.AddAudit(ctx, &store.AuditEntry{ID: "a2", TS: store.Now(), Action: "settings.update"}); err != nil {
		t.Fatalf("AddAudit 2: %v", err)
	}

	items, total, err := st.ListAudit(ctx, store.AuditFilter{Action: "chain.create"}, store.Page{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("ListAudit action filter: total=%d len=%d", total, len(items))
	}
	if items[0].User != "admin" || items[0].Target != "c1" || items[0].Details["name"] != "search" {
		t.Fatalf("unexpected audit entry: %+v", items[0])
	}
	if items[0].Via != "key" || items[0].IP != "127.0.0.1" {
		t.Fatalf("actor metadata lost: %+v", items[0])
	}

	// Free-text query matches details.
	_, total, err = st.ListAudit(ctx, store.AuditFilter{Query: "search"}, store.Page{Limit: 10})
	if err != nil || total != 1 {
		t.Fatalf("ListAudit query: total=%d err=%v", total, err)
	}

	// Cleanup removes only records older than the cutoff.
	if err := st.AddAudit(ctx, &store.AuditEntry{ID: "old", TS: "2000-01-01T00:00:00Z", Action: "old.action"}); err != nil {
		t.Fatalf("AddAudit old: %v", err)
	}
	n, err := st.DeleteOldAudit(ctx, "2001-01-01T00:00:00Z")
	if err != nil || n != 1 {
		t.Fatalf("DeleteOldAudit: n=%d err=%v", n, err)
	}
	if _, total, _ := st.ListAudit(ctx, store.AuditFilter{}, store.Page{Limit: 10}); total != 2 {
		t.Fatalf("expected 2 records after cleanup, got %d", total)
	}
}
