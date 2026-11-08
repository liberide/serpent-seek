package engine

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// TestTreatEmptyAsFailCountsFailedAttempt verifies that an empty result counts
// as a failed attempt when treat_empty_as_fail is enabled.
func TestTreatEmptyAsFailCountsFailedAttempt(t *testing.T) {
	ctx := context.Background()
	p := &scriptedProvider{code: "p_empty", results: []providers.Result{emptyResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_empty")}, nil), p)
	if err := h.engine.settings.Set(ctx, config.KeyProviderFailures, "1"); err != nil {
		t.Fatalf("set threshold: %v", err)
	}
	if err := h.engine.settings.Set(ctx, config.KeyProviderCooldown, "60"); err != nil {
		t.Fatalf("set cooldown: %v", err)
	}

	out, err := h.engine.Execute(ctx, Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(h.sink.steps) != 1 || h.sink.steps[0].Status != "fail" {
		t.Fatalf("empty result must be recorded as a failed attempt, steps=%+v", h.sink.steps)
	}
	if h.engine.CooldownCount() != 1 {
		t.Fatalf("empty result must trip the circuit breaker, active=%d", h.engine.CooldownCount())
	}
	// The request status remains "empty".
	if out.Request.Status != "empty" {
		t.Fatalf("request status = %s, want empty", out.Request.Status)
	}
}

// TestTreatEmptyAsFailDisabledAcceptsEmpty verifies that empty results are
// accepted when treat_empty_as_fail is disabled.
func TestTreatEmptyAsFailDisabledAcceptsEmpty(t *testing.T) {
	ctx := context.Background()
	p := &scriptedProvider{code: "p_empty", results: []providers.Result{emptyResult()}}
	chainNode := node("a", "p_empty")
	chainNode.Params = map[string]string{"treat_empty_as_fail": "false"}
	h := newHarness(t, chainWith([]store.ChainNode{chainNode}, nil), p)
	if err := h.engine.settings.Set(ctx, config.KeyProviderFailures, "1"); err != nil {
		t.Fatalf("set threshold: %v", err)
	}
	if err := h.engine.settings.Set(ctx, config.KeyProviderCooldown, "60"); err != nil {
		t.Fatalf("set cooldown: %v", err)
	}

	out, err := h.engine.Execute(ctx, Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("request status = %s, want ok", out.Request.Status)
	}
	if len(h.sink.steps) != 1 || h.sink.steps[0].Status != "empty" {
		t.Fatalf("accepted empty must stay an empty attempt, steps=%+v", h.sink.steps)
	}
	if h.engine.CooldownCount() != 0 {
		t.Fatalf("accepted empty must not trip the breaker, active=%d", h.engine.CooldownCount())
	}
}

// TestTreatEmptyAsFailAppliesBackoff verifies retry and backoff for empty attempts.
func TestTreatEmptyAsFailAppliesBackoff(t *testing.T) {
	ctx := context.Background()
	p := &scriptedProvider{code: "p_empty", results: []providers.Result{emptyResult(), okResult()}}
	chainNode := node("a", "p_empty")
	chainNode.Retries = 1
	chainNode.RetryDelayMS = 20
	h := newHarness(t, chainWith([]store.ChainNode{chainNode}, nil), p)

	out, err := h.engine.Execute(ctx, Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("retry after an empty attempt should succeed, got %s", out.Request.Status)
	}
	if len(h.sink.steps) != 2 {
		t.Fatalf("expected two attempts, got %d", len(h.sink.steps))
	}
	if h.sink.steps[0].Status != "fail" || h.sink.steps[0].WaitBeforeMS <= 0 {
		t.Fatalf("empty attempt must back off before retrying, step=%+v", h.sink.steps[0])
	}
}
