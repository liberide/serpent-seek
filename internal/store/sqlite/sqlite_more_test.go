package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/store"
)

func TestRequestFiltersAndCleanup(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	requests := []*store.Request{
		{ID: "r1", RID: "rid1", Query: "alpha", Count: 1, Status: "ok", UsedProvider: "searxng", CreatedAt: store.Now()},
		{ID: "r2", RID: "rid2", Query: "beta", Count: 1, Status: "fail", UsedProvider: "yandex", CreatedAt: old},
	}
	for _, req := range requests {
		if err := st.CreateRequest(ctx, req); err != nil {
			t.Fatalf("CreateRequest: %v", err)
		}
	}

	if _, total, _ := st.ListRequests(ctx, store.RequestFilter{Query: "alpha"}, store.Page{Limit: 10}); total != 1 {
		t.Fatalf("text filter failed: %d", total)
	}
	if _, total, _ := st.ListRequests(ctx, store.RequestFilter{Provider: "yandex"}, store.Page{Limit: 10}); total != 1 {
		t.Fatalf("provider filter failed: %d", total)
	}
	from := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, total, _ := st.ListRequests(ctx, store.RequestFilter{From: from}, store.Page{Limit: 10}); total != 1 {
		t.Fatalf("from filter failed: %d", total)
	}
	if _, total, _ := st.ListRequests(ctx, store.RequestFilter{To: from}, store.Page{Limit: 10}); total != 1 {
		t.Fatalf("to filter failed: %d", total)
	}
	if items, _, _ := st.ListRequests(ctx, store.RequestFilter{}, store.Page{Limit: 1, Offset: 1}); len(items) != 1 {
		t.Fatalf("pagination failed: %d", len(items))
	}

	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	if n, err := st.DeleteOldRequests(ctx, cutoff); err != nil || n != 1 {
		t.Fatalf("DeleteOldRequests: %d %v", n, err)
	}
}

func TestGetMissingEntities(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	if _, err := st.GetUser(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for user, got %v", err)
	}
	if _, err := st.GetProvider(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for provider, got %v", err)
	}
	if _, err := st.GetChain(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for chain, got %v", err)
	}
	if _, err := st.GetRequest(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for request, got %v", err)
	}
	if err := st.UpdateProviderBaseURL(ctx, "nope", "x"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for base url update, got %v", err)
	}
	if err := st.ActivateChain(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for activate, got %v", err)
	}
}

func TestSummaryDailyRollups(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	today := time.Now().UTC()
	for i, status := range []string{"ok", "ok", "fail"} {
		req := &store.Request{
			ID: "s" + string(rune('a'+i)), RID: "summary" + string(rune('a'+i)),
			Query: "q", Count: 1, Status: status, TotalMS: 100, CreatedAt: today.Format(time.RFC3339Nano),
		}
		if err := st.CreateRequest(ctx, req); err != nil {
			t.Fatalf("CreateRequest: %v", err)
		}
	}
	summary, err := st.Summary(ctx, 7)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.RequestsToday != 3 || summary.ErrorsToday != 1 || len(summary.Daily) != 7 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.SuccessRate < 66 || summary.SuccessRate > 67 {
		t.Fatalf("unexpected success rate: %f", summary.SuccessRate)
	}
	if err := st.RecomputeDailyStats(ctx, today.Format("2006-01-02")); err != nil {
		t.Fatalf("RecomputeDailyStats: %v", err)
	}
}

func TestClearLogs(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	for i := 0; i < 3; i++ {
		_ = st.AddLog(ctx, &store.LogEntry{TS: store.Now(), Level: "info", Message: "m"})
	}
	if n, err := st.ClearLogs(ctx); err != nil || n != 3 {
		t.Fatalf("ClearLogs: %d %v", n, err)
	}
}
