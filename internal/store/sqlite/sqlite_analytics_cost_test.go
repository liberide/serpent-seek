package sqlite

import (
	"context"
	"math"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

func TestAnalyticsCostAggregation(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	now := store.Now()

	req := &store.Request{ID: "rc1", RID: "0000-cost", Query: "q", Status: "ok", CreatedAt: now}
	if err := st.CreateRequest(ctx, req); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	steps := []*store.RequestStep{
		{ID: "s1", RequestID: req.ID, Idx: 0, Provider: "p", Status: "ok", Kind: "ok", Cost: 0.5, StartedAt: now, FinishedAt: now},
		{ID: "s2", RequestID: req.ID, Idx: 0, Provider: "p", Status: "fail", Kind: "http", Cost: 0.5, StartedAt: now, FinishedAt: now},
		{ID: "s3", RequestID: req.ID, Idx: 1, Provider: "q", Status: "ok", Kind: "ok", Cost: 0, StartedAt: now, FinishedAt: now},
	}
	for _, s := range steps {
		if err := st.AddStep(ctx, s); err != nil {
			t.Fatalf("AddStep %s: %v", s.ID, err)
		}
	}

	// Cost survives the round-trip (history view).
	loaded, err := st.ListSteps(ctx, req.ID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(loaded) != 3 || math.Abs(loaded[0].Cost-0.5) > 1e-9 {
		t.Fatalf("ListSteps cost lost: %+v", loaded)
	}

	report, err := st.Analytics(ctx, store.AnalyticsFilter{})
	if err != nil {
		t.Fatalf("Analytics: %v", err)
	}
	byName := map[string]store.ProviderStat{}
	for _, p := range report.Providers {
		byName[p.Provider] = p
	}
	if math.Abs(byName["p"].Cost-1.0) > 1e-9 {
		t.Fatalf("provider p cost = %v, want 1.0", byName["p"].Cost)
	}
	if byName["q"].Cost != 0 {
		t.Fatalf("provider q cost = %v, want 0", byName["q"].Cost)
	}
	if math.Abs(report.CostTotal-1.0) > 1e-9 {
		t.Fatalf("report cost total = %v, want 1.0", report.CostTotal)
	}
}
