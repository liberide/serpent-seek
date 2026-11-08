package sqlite

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

// TestFailStaleRunningRequests verifies stale running rows are finalized.
func TestFailStaleRunningRequests(t *testing.T) {
	ctx := context.Background()
	st := *openTest(t)
	cutoff := "2026-06-01T00:00:00Z"

	// Stale running request and step.
	if err := st.CreateRequest(ctx, &store.Request{
		ID: "old-run", RID: "r1", Query: "q", Status: "running", CreatedAt: "2026-05-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if err := st.AddStep(ctx, &store.RequestStep{
		ID: "s1", RequestID: "old-run", Provider: "p", Status: "running", AttemptNo: 1,
	}); err != nil {
		t.Fatalf("AddStep: %v", err)
	}
	// Stale finished request: unchanged.
	if err := st.CreateRequest(ctx, &store.Request{
		ID: "old-done", RID: "r2", Query: "q", Status: "ok", CreatedAt: "2026-05-02T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	// Fresh running request: unchanged.
	if err := st.CreateRequest(ctx, &store.Request{
		ID: "fresh-run", RID: "r3", Query: "q", Status: "running", CreatedAt: "2026-06-02T00:00:00Z",
	}); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	n, err := st.FailStaleRunningRequests(ctx, cutoff, "interrupted by service restart")
	if err != nil {
		t.Fatalf("FailStaleRunningRequests: %v", err)
	}
	if n != 1 {
		t.Fatalf("reconciled = %d, want 1", n)
	}

	old, err := st.GetRequest(ctx, "old-run")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if old.Status != "fail" || old.Error != "interrupted by service restart" {
		t.Fatalf("stale request not finalized: %+v", old)
	}
	steps, err := st.ListSteps(ctx, "old-run")
	if err != nil || len(steps) != 1 {
		t.Fatalf("ListSteps: %v len=%d", err, len(steps))
	}
	if steps[0].Status != "fail" || steps[0].Error != "interrupted by service restart" {
		t.Fatalf("stale step not finalized: %+v", steps[0])
	}

	done, _ := st.GetRequest(ctx, "old-done")
	if done.Status != "ok" {
		t.Fatalf("finished request was touched: %+v", done)
	}
	fresh, _ := st.GetRequest(ctx, "fresh-run")
	if fresh.Status != "running" {
		t.Fatalf("fresh request was touched: %+v", fresh)
	}
}
