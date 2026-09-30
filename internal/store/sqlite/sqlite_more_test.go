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

func TestAnalyticsAggregation(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	requests := []*store.Request{
		{ID: "a1", RID: "a1", Query: "alpha", Count: 1, Status: "ok", UsedProvider: "Stub", TotalMS: 100, CreatedAt: now.Format(time.RFC3339Nano)},
		{ID: "a2", RID: "a2", Query: "beta", Count: 1, Status: "fail", UsedProvider: "Stub", TotalMS: 300, CreatedAt: now.Format(time.RFC3339Nano)},
		{ID: "a3", RID: "a3", Query: "alpha", Count: 1, Status: "ok", UsedProvider: "Stub", TotalMS: 200, CreatedAt: now.AddDate(0, 0, -1).Format(time.RFC3339Nano)},
	}
	for _, req := range requests {
		if err := st.CreateRequest(ctx, req); err != nil {
			t.Fatalf("CreateRequest: %v", err)
		}
	}
	steps := []*store.RequestStep{
		{ID: "s1", RequestID: "a1", Idx: 0, Provider: "Stub", Status: "ok", TookMS: 100, StartedAt: store.Now(), FinishedAt: store.Now()},
		{ID: "s2", RequestID: "a2", Idx: 0, Provider: "Stub", Status: "fail", TookMS: 300, StartedAt: store.Now(), FinishedAt: store.Now()},
		{ID: "s3", RequestID: "a2", Idx: 1, Provider: "Other", Status: "ok", TookMS: 50, StartedAt: store.Now(), FinishedAt: store.Now()},
		{ID: "s4", RequestID: "a3", Idx: 0, Provider: "Stub", Status: "ok", TookMS: 200, StartedAt: store.Now(), FinishedAt: store.Now()},
	}
	for _, step := range steps {
		if err := st.AddStep(ctx, step); err != nil {
			t.Fatalf("AddStep: %v", err)
		}
	}

	report, err := st.Analytics(ctx, store.AnalyticsFilter{From: today, To: today})
	if err != nil {
		t.Fatalf("Analytics: %v", err)
	}
	if report.TotalRequests != 2 || report.OK != 1 || report.Fail != 1 {
		t.Fatalf("unexpected totals: %+v", report)
	}
	if len(report.Daily) != 1 || report.Daily[0].Requests != 2 {
		t.Fatalf("unexpected daily series: %+v", report.Daily)
	}
	if len(report.Hourly) != 24 {
		t.Fatalf("expected 24 hourly buckets, got %d", len(report.Hourly))
	}
	if len(report.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %+v", report.Providers)
	}
	var stub *store.ProviderStat
	for i := range report.Providers {
		if report.Providers[i].Provider == "Stub" {
			stub = &report.Providers[i]
		}
	}
	if stub == nil || stub.OK != 1 || stub.Fail != 1 || stub.Total != 2 {
		t.Fatalf("unexpected Stub stats: %+v", stub)
	}
	if len(report.TopQueries) != 2 || report.TopQueries[0].Query != "alpha" && report.TopQueries[0].Query != "beta" {
		t.Fatalf("unexpected top queries: %+v", report.TopQueries)
	}

	full, err := st.Analytics(ctx, store.AnalyticsFilter{From: yesterday, To: today})
	if err != nil {
		t.Fatalf("Analytics full: %v", err)
	}
	if full.TotalRequests != 3 || len(full.Daily) != 2 || full.ActiveDays != 2 {
		t.Fatalf("unexpected full-range report: %+v", full)
	}
}

func TestAnalyticsFiltersAndEdgeCases(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	base := time.Now().UTC()
	day := func(offset int) string { return base.AddDate(0, 0, offset).Format(time.RFC3339Nano) }
	dayOnly := func(offset int) string { return base.AddDate(0, 0, offset).Format("2006-01-02") }

	requests := []*store.Request{
		{ID: "f1", RID: "f1", Query: "alpha", Count: 1, Status: "ok", UsedProvider: "Alpha", TotalMS: 100, CreatedAt: day(-2)},
		{ID: "f2", RID: "f2", Query: "beta", Count: 1, Status: "fail", UsedProvider: "Beta", TotalMS: 200, CreatedAt: day(-2)},
		{ID: "f3", RID: "f3", Query: "gamma", Count: 1, Status: "ok", UsedProvider: "Alpha, Beta", TotalMS: 150, CreatedAt: day(-1)},
		{ID: "f4", RID: "f4", Query: "delta", Count: 1, Status: "empty", UsedProvider: "", TotalMS: 80, CreatedAt: day(-1)},
		{ID: "f5", RID: "f5", Query: "epsilon", Count: 1, Status: "ok", UsedProvider: "Alpha", TotalMS: 50, CreatedAt: day(0)},
	}
	for _, req := range requests {
		if err := st.CreateRequest(ctx, req); err != nil {
			t.Fatalf("CreateRequest: %v", err)
		}
	}
	steps := []*store.RequestStep{
		{ID: "fs1", RequestID: "f1", Idx: 0, Provider: "Alpha", Status: "ok", TookMS: 100, StartedAt: store.Now(), FinishedAt: store.Now()},
		{ID: "fs2", RequestID: "f2", Idx: 0, Provider: "Beta", Status: "fail", TookMS: 200, StartedAt: store.Now(), FinishedAt: store.Now()},
		{ID: "fs3", RequestID: "f3", Idx: 0, Provider: "Alpha", Status: "ok", TookMS: 150, StartedAt: store.Now(), FinishedAt: store.Now()},
		{ID: "fs4", RequestID: "f3", Idx: 1, Provider: "Beta", Status: "ok", TookMS: 250, StartedAt: store.Now(), FinishedAt: store.Now()},
		{ID: "fs5", RequestID: "f5", Idx: 0, Provider: "Alpha", Status: "ok", TookMS: 50, StartedAt: store.Now(), FinishedAt: store.Now()},
	}
	for _, step := range steps {
		if err := st.AddStep(ctx, step); err != nil {
			t.Fatalf("AddStep: %v", err)
		}
	}

	full := store.AnalyticsFilter{From: dayOnly(-2), To: dayOnly(0)}
	report, err := st.Analytics(ctx, full)
	if err != nil {
		t.Fatalf("Analytics: %v", err)
	}
	if report.TotalRequests != 5 || report.OK != 3 || report.Empty != 1 || report.Fail != 1 {
		t.Fatalf("unexpected full totals: %+v", report)
	}
	if len(report.Daily) != 3 || report.ActiveDays != 3 {
		t.Fatalf("unexpected daily/active days: %+v", report)
	}
	if report.P95MS != 200 {
		t.Fatalf("unexpected p95: %d", report.P95MS)
	}

	// Provider filter must also match full-chain comma lists.
	for name, want := range map[string]int{"Alpha": 3, "Beta": 2} {
		filtered := full
		filtered.Provider = name
		got, err := st.Analytics(ctx, filtered)
		if err != nil {
			t.Fatalf("Analytics(%s): %v", name, err)
		}
		if got.TotalRequests != want {
			t.Fatalf("provider %s: want %d, got %d", name, want, got.TotalRequests)
		}
	}

	// Status filter.
	failOnly := full
	failOnly.Status = "fail"
	if got, _ := st.Analytics(ctx, failOnly); got.TotalRequests != 1 || got.Fail != 1 {
		t.Fatalf("status filter failed: %+v", got)
	}

	// Single-day range.
	oneDay := store.AnalyticsFilter{From: dayOnly(-2), To: dayOnly(-2)}
	if got, _ := st.Analytics(ctx, oneDay); got.TotalRequests != 2 || len(got.Daily) != 1 {
		t.Fatalf("single-day range failed: %+v", got)
	}

	// Provider breakdown is independent of provider/status filters.
	alphaOnly := full
	alphaOnly.Provider = "Alpha"
	alphaOnly.Status = "fail"
	breakdown, _ := st.Analytics(ctx, alphaOnly)
	byName := map[string]store.ProviderStat{}
	for _, p := range breakdown.Providers {
		byName[p.Provider] = p
	}
	if byName["Alpha"].OK != 3 || byName["Beta"].OK != 1 || byName["Beta"].Fail != 1 {
		t.Fatalf("provider breakdown should ignore filters: %+v", breakdown.Providers)
	}

	// Empty range yields zero totals and no active days.
	empty := store.AnalyticsFilter{From: dayOnly(30), To: dayOnly(31)}
	got, err := st.Analytics(ctx, empty)
	if err != nil {
		t.Fatalf("Analytics(empty): %v", err)
	}
	if got.TotalRequests != 0 || got.ActiveDays != 0 || got.BusiestDate != "" || got.P95MS != 0 {
		t.Fatalf("unexpected empty report: %+v", got)
	}
}

func TestAnalyticsProviderFilterIncludesSkipped(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	day := time.Now().UTC().Format("2006-01-02")
	req := &store.Request{ID: "sk1", RID: "sk1", Query: "q", Count: 1, Status: "fail", UsedProvider: "", CreatedAt: store.Now()}
	if err := st.CreateRequest(ctx, req); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if err := st.AddStep(ctx, &store.RequestStep{ID: "sks1", RequestID: "sk1", Idx: 0, Provider: "Ghost", Status: "skip", StartedAt: store.Now(), FinishedAt: store.Now()}); err != nil {
		t.Fatalf("AddStep: %v", err)
	}

	got, err := st.Analytics(ctx, store.AnalyticsFilter{From: day, To: day, Provider: "Ghost"})
	if err != nil {
		t.Fatalf("Analytics: %v", err)
	}
	if got.TotalRequests != 1 || got.Fail != 1 {
		t.Fatalf("skip-only provider filter failed: %+v", got)
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
