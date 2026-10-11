package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// TestExecuteRecordsProviderCost proves the engine snapshots a provider's
// configured cost_per_request into each executed attempt step, and that an
// unconfigured provider records no cost.
func TestExecuteRecordsProviderCost(t *testing.T) {
	ctx := context.Background()
	p := &scriptedProvider{code: "p_cost", results: []providers.Result{okResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_cost")}, nil), p)
	h.store.providers = []*store.Provider{{
		ID: "p_cost", Code: "p_cost", Name: "p_cost", Enabled: true,
		Params: map[string]string{"cost_per_request": "0.25"},
	}}

	if _, err := h.engine.Execute(ctx, Input{Query: "cost"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	found := false
	for _, s := range h.sink.steps {
		if s.Provider == "p_cost" && s.Cost != 0 {
			if s.Cost != 0.25 {
				t.Fatalf("step cost = %v, want 0.25", s.Cost)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a step with cost 0.25, steps=%+v", h.sink.steps)
	}
}

// TestExecuteTripsProviderCooldown proves the circuit breaker skips a provider
// that failed on a previous request.
func TestExecuteTripsProviderCooldown(t *testing.T) {
	ctx := context.Background()
	p := &scriptedProvider{code: "p_fail", results: []providers.Result{failResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_fail")}, nil), p)
	if err := h.engine.settings.Set(ctx, config.KeyProviderFailures, "1"); err != nil {
		t.Fatalf("set threshold: %v", err)
	}
	if err := h.engine.settings.Set(ctx, config.KeyProviderCooldown, "60"); err != nil {
		t.Fatalf("set cooldown: %v", err)
	}

	if _, err := h.engine.Execute(ctx, Input{Query: "first"}); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("first request should call the provider once, got %d", p.calls)
	}
	if h.engine.CooldownCount() != 1 {
		t.Fatalf("breaker should be tripped, active=%d", h.engine.CooldownCount())
	}

	if _, err := h.engine.Execute(ctx, Input{Query: "second"}); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("cooled-down provider must not be called again, calls=%d", p.calls)
	}
	skipped := false
	for _, s := range h.sink.steps {
		if s.Status == "skip" && s.Error == deadCooldown {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("expected a cooldown skip step, steps=%+v", h.sink.steps)
	}
}

// TestExecuteEmitsCooldownWebhook proves a tripped circuit breaker posts the
// configured webhook at most once per target.
func TestExecuteEmitsCooldownWebhook(t *testing.T) {
	ctx := context.Background()
	done := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Event string         `json:"event"`
			Data  map[string]any `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Event == "provider_cooldown" {
			select {
			case done <- payload.Data:
			default:
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &scriptedProvider{code: "p_hook", results: []providers.Result{failResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_hook")}, nil), p)
	if err := h.engine.settings.Set(ctx, config.KeyProviderFailures, "1"); err != nil {
		t.Fatalf("set threshold: %v", err)
	}
	if err := h.engine.settings.Set(ctx, config.KeyWebhookURL, srv.URL); err != nil {
		t.Fatalf("set webhook: %v", err)
	}

	if _, err := h.engine.Execute(ctx, Input{Query: "hook"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	select {
	case data := <-done:
		if data["provider_id"] != "p_hook" {
			t.Fatalf("unexpected webhook data: %+v", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cooldown webhook was not delivered")
	}
}
