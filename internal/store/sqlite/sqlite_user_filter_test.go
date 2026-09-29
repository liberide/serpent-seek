package sqlite

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

// TestRequestUserFilter checks owner filtering.
func TestRequestUserFilter(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	requests := []*store.Request{
		{ID: "a1", RID: "a1", Query: "mine", Status: "ok", UserID: "user-a", CreatedAt: store.Now()},
		{ID: "b1", RID: "b1", Query: "theirs", Status: "ok", UserID: "user-b", CreatedAt: store.Now()},
	}
	for _, req := range requests {
		if err := st.CreateRequest(ctx, req); err != nil {
			t.Fatalf("CreateRequest: %v", err)
		}
	}
	items, total, err := st.ListRequests(ctx, store.RequestFilter{UserID: "user-a"}, store.Page{Limit: 10})
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "a1" {
		t.Fatalf("user filter returned %d items total=%d", len(items), total)
	}
	if _, total, _ := st.ListRequests(ctx, store.RequestFilter{}, store.Page{Limit: 10}); total != 2 {
		t.Fatalf("unfiltered total = %d, want 2", total)
	}

	summary, err := st.SummaryForUser(ctx, 7, "user-a")
	if err != nil {
		t.Fatalf("SummaryForUser: %v", err)
	}
	if summary.TotalRequests != 1 || summary.RequestsToday != 1 {
		t.Fatalf("scoped summary = %+v", summary)
	}
}
