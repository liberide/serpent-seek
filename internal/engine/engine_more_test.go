package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

func TestUnknownProviderFails(t *testing.T) {
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "does_not_exist")}, nil))
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "fail" {
		t.Fatalf("expected fail for unknown provider, got %s", out.Request.Status)
	}
	if len(h.sink.steps) != 1 || h.sink.steps[0].Kind != "skip" {
		t.Fatalf("expected one skip step, got %+v", h.sink.steps)
	}
}

func TestDisabledProviderIsSkipped(t *testing.T) {
	provider := &scriptedProvider{code: "p_ok", results: []providers.Result{okResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_ok")}, nil), provider)
	h.store.providers = []*store.Provider{{ID: "p_ok", Code: "p_ok", Enabled: false, Credentials: map[string]string{}}}
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "fail" {
		t.Fatalf("expected fail, got %s", out.Request.Status)
	}
	hasSkip := false
	for _, step := range h.sink.steps {
		if step.Status == "skip" {
			hasSkip = true
		}
	}
	if !hasSkip {
		t.Fatalf("expected skip step, got %+v", h.sink.steps)
	}
}

func TestMissingKeyMarksProviderDead(t *testing.T) {
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "apiserpent")}, nil))
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "fail" {
		t.Fatalf("expected fail, got %s", out.Request.Status)
	}
	hasDead := false
	for _, event := range h.sink.events {
		if event == EventProviderDead {
			hasDead = true
		}
	}
	if !hasDead {
		t.Fatalf("expected provider_dead event, got %v", h.sink.events)
	}
}

func TestTreatEmptyAsFailOverride(t *testing.T) {
	provider := &scriptedProvider{code: "p_empty", results: []providers.Result{emptyResult()}}
	chainNode := node("a", "p_empty")
	chainNode.Params = map[string]string{"treat_empty_as_fail": "false"}
	h := newHarness(t, chainWith([]store.ChainNode{chainNode}, nil), provider)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("empty should be accepted when treat_empty_as_fail=false, got %s", out.Request.Status)
	}
}

func TestStartAsync(t *testing.T) {
	provider := &scriptedProvider{code: "p_ok", results: []providers.Result{okResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_ok")}, nil), provider)
	req, err := h.engine.Start(context.Background(), Input{Query: "hello", Count: 2})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if req.Status != "running" {
		t.Fatalf("expected running request, got %s", req.Status)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.sink.mu.Lock()
		status := h.sink.req.Status
		h.sink.mu.Unlock()
		if status != "running" {
			if status != "ok" {
				t.Fatalf("expected ok, got %s", status)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("async run did not complete in time")
}

func TestNextRIDFormat(t *testing.T) {
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_ok")}, nil))
	rid := h.engine.nextRID()
	if len(rid) != 9 || rid[4] != '-' || len(strings.Split(rid, "-")) != 2 {
		t.Fatalf("unexpected rid format: %q", rid)
	}
}

func TestSearxngWithoutExternalURLIsSkipped(t *testing.T) {
	// SearXNG is external: with no URL configured the node must be skipped and
	// the chain must continue without touching the network.
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "searxng")}, nil))
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "fail" {
		t.Fatalf("expected fail, got %s", out.Request.Status)
	}
	if len(h.sink.steps) != 1 || h.sink.steps[0].Kind != "skip" {
		t.Fatalf("expected a single skip step, got %+v", h.sink.steps)
	}
	if !strings.Contains(h.sink.steps[0].Error, "searxng") {
		t.Fatalf("expected a searxng url reason, got %q", h.sink.steps[0].Error)
	}
}

func TestFinalizeRecomputesStats(t *testing.T) {
	provider := &scriptedProvider{code: "p_ok", results: []providers.Result{okResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_ok")}, nil), provider)
	if _, err := h.engine.Execute(context.Background(), Input{Query: "hello"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if h.sink.req == nil || h.sink.req.Status != "ok" {
		t.Fatalf("expected a persisted ok request, got %+v", h.sink.req)
	}
	if h.sink.req.StepsCount != 1 {
		t.Fatalf("expected steps_count=1, got %d", h.sink.req.StepsCount)
	}
}

// captureProvider records the Count the engine asked for.
type captureProvider struct {
	code  string
	count int
}

func (p *captureProvider) Code() string { return p.code }

func (p *captureProvider) Schema() providers.ProviderSchema {
	return providers.ProviderSchema{Code: p.code, Name: p.code}
}

func (p *captureProvider) Search(_ context.Context, q providers.Query, _ providers.Credentials, _ providers.Params) providers.Result {
	p.count = q.Count
	return providers.Result{OK: true, Kind: providers.KindOK, Rows: providers.Rows{{Link: "https://a.example", Title: "A"}}}
}

func TestCountZeroAsksForEverything(t *testing.T) {
	provider := &captureProvider{code: "p_cap"}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_cap")}, nil), provider)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello", Count: 0})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if provider.count != 0 {
		t.Fatalf("expected provider to be asked with Count=0 (unlimited), got %d", provider.count)
	}
	if out.Request.Count != 0 || out.Request.Status != "ok" {
		t.Fatalf("unexpected request: count=%d status=%s", out.Request.Count, out.Request.Status)
	}
}

func TestIgnoreRequestCountToggleForcesUnlimited(t *testing.T) {
	provider := &captureProvider{code: "p_cap"}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_cap")}, nil), provider)
	h.kv.m["ignore_request_count"] = "true"
	_ = h.engine.settings.Refresh(context.Background())
	if _, err := h.engine.Execute(context.Background(), Input{Query: "hello", Count: 5}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if provider.count != 0 {
		t.Fatalf("toggle must override the requested count 5 with 0, got %d", provider.count)
	}
}

func TestCountStillCappedAtTwenty(t *testing.T) {
	provider := &captureProvider{code: "p_cap"}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_cap")}, nil), provider)
	if _, err := h.engine.Execute(context.Background(), Input{Query: "hello", Count: 50}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if provider.count != 20 {
		t.Fatalf("count must be capped at 20, got %d", provider.count)
	}
}
