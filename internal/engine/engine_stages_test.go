package engine

import (
	"context"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// stagedProvider reports two upstream calls per Search (like pubmed).
type stagedProvider struct{ code string }

func (p *stagedProvider) Code() string { return p.code }
func (p *stagedProvider) Schema() providers.ProviderSchema {
	return providers.ProviderSchema{Code: p.code, Name: p.code}
}

func (p *stagedProvider) Search(ctx context.Context, _ providers.Query, _ providers.Credentials, _ providers.Params) providers.Result {
	providers.ReportStage(ctx, "esearch", 200, 3, nil)
	providers.ReportStage(ctx, "esummary", 200, 5, nil)
	return providers.Result{OK: true, Kind: providers.KindOK, Rows: providers.Rows{{Link: "https://x", Title: "X"}}}
}

func TestStageStepsArePersistedAndBudgeted(t *testing.T) {
	provider := &stagedProvider{code: "p_staged"}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_staged")}, nil), provider)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello", Count: 3})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("unexpected status %s", out.Request.Status)
	}
	var aggregate, stages int
	for _, s := range h.sink.steps {
		if s.Stage == "" {
			aggregate++
		} else if s.Stage == "esearch" || s.Stage == "esummary" {
			stages++
		} else {
			t.Fatalf("unexpected stage %q", s.Stage)
		}
	}
	if aggregate != 1 || stages != 2 {
		t.Fatalf("expected 1 aggregate + 2 stage rows, got %d+%d (%d total)", aggregate, stages, len(h.sink.steps))
	}
	// The attempt budget is consumed per HTTP call: two calls = two steps.
	if out.Request.StepsCount != 2 {
		t.Fatalf("two-call node must spend 2 of the budget, got %d", out.Request.StepsCount)
	}
}

func TestRetryAfterRaisesWaitBeforeRetry(t *testing.T) {
	provider := &scriptedProvider{code: "p_ra", results: []providers.Result{
		{Kind: providers.KindHTTP, HTTPStatus: 429, Error: "rate limited", RetryAfterMS: 1200},
		okResult(),
	}}
	n := node("a", "p_ra")
	n.Retries = 1
	n.RetryDelayMS = 100
	n.DelayPolicy = "fixed"
	h := newHarness(t, chainWith([]store.ChainNode{n}, nil), provider)
	started := time.Now()
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("unexpected status %s", out.Request.Status)
	}
	if len(h.sink.steps) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(h.sink.steps))
	}
	// RetryAfter wins over the node's retry_delay_ms.
	if h.sink.steps[0].WaitBeforeMS != 1200 {
		t.Fatalf("wait_before_ms must be 1200 (Retry-After), got %d", h.sink.steps[0].WaitBeforeMS)
	}
	if elapsed := time.Since(started); elapsed < 1100*time.Millisecond {
		t.Fatalf("engine did not honour the retry-after pause: %v", elapsed)
	}
}

func TestRetryAfterAppliesBeforeNextNode(t *testing.T) {
	first := &scriptedProvider{code: "p_ra1", results: []providers.Result{
		{Kind: providers.KindHTTP, HTTPStatus: 429, Error: "rate limited", RetryAfterMS: 300},
	}}
	second := &scriptedProvider{code: "p_ok2", results: []providers.Result{okResult()}}
	chain := chainWith(
		[]store.ChainNode{node("a", "p_ra1"), node("b", "p_ok2")},
		[]store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "fail"}},
	)
	h := newHarness(t, chain, first, second)
	started := time.Now()
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.UsedProvider != "p_ok2" {
		t.Fatalf("expected failover, got %q", out.Request.UsedProvider)
	}
	if elapsed := time.Since(started); elapsed < 250*time.Millisecond {
		t.Fatalf("moving to the next node must respect Retry-After: %v", elapsed)
	}
}
