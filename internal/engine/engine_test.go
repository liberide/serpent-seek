package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

type memKV struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKV) GetSetting(_ context.Context, key string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if v, ok := k.m[key]; ok {
		return v, nil
	}
	return "", store.ErrNotFound
}

func (k *memKV) AllSettings(_ context.Context) (map[string]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := map[string]string{}
	for key, value := range k.m {
		out[key] = value
	}
	return out, nil
}

func (k *memKV) SetSetting(_ context.Context, key, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[key] = value
	return nil
}

type fakeStore struct {
	store.Storage
	chain     *store.Chain
	providers []*store.Provider
	mu        sync.Mutex
	req       *store.Request
	steps     []*store.RequestStep
}

func (f *fakeStore) GetActiveChain(context.Context) (*store.Chain, error) {
	if f.chain == nil {
		return nil, store.ErrNotFound
	}
	return f.chain, nil
}
func (f *fakeStore) ListProviders(context.Context) ([]*store.Provider, error) {
	return f.providers, nil
}
func (f *fakeStore) GetProvider(_ context.Context, code string) (*store.Provider, error) {
	for _, p := range f.providers {
		if p.Code == code {
			return p, nil
		}
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) CreateRequest(_ context.Context, r *store.Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.req = r
	return nil
}
func (f *fakeStore) UpdateRequest(_ context.Context, r *store.Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.req = r
	return nil
}
func (f *fakeStore) AddStep(_ context.Context, s *store.RequestStep) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, s)
	return nil
}
func (f *fakeStore) GetRequest(_ context.Context, id string) (*store.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.req == nil {
		return nil, store.ErrNotFound
	}
	f.req.Steps = f.steps
	return f.req, nil
}
func (f *fakeStore) RecomputeDailyStats(context.Context, string) error { return nil }

type fakeSink struct {
	mu     sync.Mutex
	events []string
	req    *store.Request
	steps  []*store.RequestStep
}

func (s *fakeSink) CreateRequest(_ context.Context, r *store.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.req = r
	return nil
}
func (s *fakeSink) SaveStep(_ context.Context, step *store.RequestStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, step)
	return nil
}
func (s *fakeSink) UpdateRequest(_ context.Context, r *store.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.req = r
	return nil
}
func (s *fakeSink) Publish(_, event string, _ any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}
func (s *fakeSink) PublishGlobal(event string, _ any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, "global:"+event)
}

type scriptedProvider struct {
	code    string
	mu      sync.Mutex
	results []providers.Result
	idx     int
	// gate, when non-nil, blocks Search until it is closed or ctx is done.
	// Tests use it to hold an async run in the "running" state deterministically.
	gate chan struct{}
}

func (p *scriptedProvider) Code() string { return p.code }

func (p *scriptedProvider) Schema() providers.ProviderSchema {
	return providers.ProviderSchema{Code: p.code, Name: p.code}
}

func (p *scriptedProvider) Search(ctx context.Context, _ providers.Query, _ providers.Credentials, _ providers.Params) providers.Result {
	if p.gate != nil {
		select {
		case <-p.gate:
		case <-ctx.Done():
			return providers.Result{Provider: p.code, Kind: providers.KindNet, Error: ctx.Err().Error()}
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.results) == 0 {
		return providers.Result{Kind: providers.KindNet, Error: "no script"}
	}
	result := p.results[p.idx]
	if p.idx < len(p.results)-1 {
		p.idx++
	}
	result.Provider = p.code
	return result
}

func okResult() providers.Result {
	return providers.Result{OK: true, Kind: providers.KindOK, Rows: providers.Rows{{Link: "https://a", Title: "A"}}}
}

func failResult() providers.Result {
	return providers.Result{Kind: providers.KindHTTP, HTTPStatus: 500, Error: "HTTP 500"}
}

func emptyResult() providers.Result {
	return providers.Result{OK: true, Kind: providers.KindOK}
}

func permResult() providers.Result {
	return providers.Result{Kind: providers.KindAPI, Permanent: true, Error: "fatal"}
}

type harness struct {
	engine *Engine
	store  *fakeStore
	sink   *fakeSink
	kv     *memKV
}

func newHarness(t *testing.T, chain *store.Chain, fakeProviders ...providers.Provider) *harness {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	kv := &memKV{m: map[string]string{}}
	settings := config.NewManager(cfg, kv)
	if err := settings.Refresh(context.Background()); err != nil {
		t.Fatalf("settings refresh: %v", err)
	}
	registry := providers.NewRegistry(providers.NewHTTPClient("test-agent"))
	for _, p := range fakeProviders {
		registry.Register(p)
	}
	logger := logging.New(logging.Options{Level: "error"})
	fs := &fakeStore{chain: chain}
	sink := &fakeSink{}
	engine := New(fs, registry, settings, sink, logger)
	return &harness{engine: engine, store: fs, sink: sink, kv: kv}
}

func chainWith(nodes []store.ChainNode, edges []store.ChainEdge) *store.Chain {
	// Tests bypass SaveChain/validator, so flag the first node as the start
	// block when the fixture did not set one explicitly.
	hasStart := false
	for i := range nodes {
		if nodes[i].IsStart {
			hasStart = true
			break
		}
	}
	if !hasStart && len(nodes) > 0 {
		nodes[0].IsStart = true
	}
	return &store.Chain{ID: "c1", Name: "test", Active: true, Nodes: nodes, Edges: edges}
}

func TestExecuteSuccessFirstNode(t *testing.T) {
	provider := &scriptedProvider{code: "p_ok", results: []providers.Result{okResult()}}
	h := newHarness(t, chainWith([]store.ChainNode{node("a", "p_ok")}, nil), provider)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello", Count: 3})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" || len(out.Rows) != 1 || out.Request.UsedProvider != "p_ok" {
		t.Fatalf("unexpected output: status=%s rows=%d used=%s", out.Request.Status, len(out.Rows), out.Request.UsedProvider)
	}
}

func TestExecuteFailoverToSecondNode(t *testing.T) {
	first := &scriptedProvider{code: "p_fail", results: []providers.Result{failResult()}}
	second := &scriptedProvider{code: "p_ok", results: []providers.Result{okResult()}}
	chain := chainWith(
		[]store.ChainNode{node("a", "p_fail"), node("b", "p_ok")},
		[]store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "fail"}},
	)
	h := newHarness(t, chain, first, second)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.UsedProvider != "p_ok" {
		t.Fatalf("expected failover to p_ok, got %q", out.Request.UsedProvider)
	}
}

func TestExecutePermanentMarksProviderDeadAndSkips(t *testing.T) {
	provider := &scriptedProvider{code: "p_perm", results: []providers.Result{permResult()}}
	chain := chainWith(
		[]store.ChainNode{node("a", "p_perm"), node("b", "p_perm")},
		[]store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "fail"}},
	)
	h := newHarness(t, chain, provider)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "fail" {
		t.Fatalf("expected fail, got %s", out.Request.Status)
	}
	hasSkip := false
	for _, step := range h.sink.steps {
		if step.Status == "skip" && step.Provider == "p_perm" {
			hasSkip = true
		}
	}
	if !hasSkip {
		t.Fatalf("expected a skip step for the dead provider, steps=%+v", h.sink.steps)
	}
}

func TestExecuteEmptyThenSuccess(t *testing.T) {
	first := &scriptedProvider{code: "p_empty", results: []providers.Result{emptyResult()}}
	second := &scriptedProvider{code: "p_ok", results: []providers.Result{okResult()}}
	chain := chainWith(
		[]store.ChainNode{node("a", "p_empty"), node("b", "p_ok")},
		[]store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "empty"}},
	)
	h := newHarness(t, chain, first, second)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" || out.Request.UsedProvider != "p_ok" {
		t.Fatalf("expected p_ok success, got %s/%s", out.Request.Status, out.Request.UsedProvider)
	}
}

func TestExecuteEmptyFinalStatus(t *testing.T) {
	provider := &scriptedProvider{code: "p_empty", results: []providers.Result{emptyResult()}}
	chain := chainWith([]store.ChainNode{node("a", "p_empty")}, nil)
	h := newHarness(t, chain, provider)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "empty" {
		t.Fatalf("expected empty final status, got %s", out.Request.Status)
	}
}

func TestExecuteRetriesThenSuccess(t *testing.T) {
	provider := &scriptedProvider{code: "p_retry", results: []providers.Result{failResult(), okResult()}}
	chainNode := node("a", "p_retry")
	chainNode.Retries = 1
	chainNode.RetryDelayMS = 0
	h := newHarness(t, chainWith([]store.ChainNode{chainNode}, nil), provider)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("expected success after retry, got %s", out.Request.Status)
	}
	if len(h.sink.steps) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(h.sink.steps))
	}
}

func TestExecuteRespectsAttemptBudget(t *testing.T) {
	provider := &scriptedProvider{code: "p_fail", results: []providers.Result{failResult()}}
	chainNode := node("a", "p_fail")
	chainNode.Retries = 5
	chainNode.RetryDelayMS = 0
	h := newHarness(t, chainWith([]store.ChainNode{chainNode}, nil), provider)
	h.kv.m["max_attempts"] = "2"
	_ = h.engine.settings.Refresh(context.Background())
	_, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(h.sink.steps) != 2 {
		t.Fatalf("expected 2 attempts due to budget, got %d", len(h.sink.steps))
	}
}

func TestExecuteNoActiveChain(t *testing.T) {
	registry := providers.NewRegistry(providers.NewHTTPClient("test-agent"))
	cfg, _ := config.Load()
	settings := config.NewManager(cfg, &memKV{m: map[string]string{}})
	engine := New(&fakeStore{}, registry, settings, &fakeSink{}, logging.New(logging.Options{Level: "error"}))
	if _, err := engine.Execute(context.Background(), Input{Query: "x"}); err == nil {
		t.Fatal("expected an error when no active chain exists")
	}
}

func TestBackoffDelayPolicies(t *testing.T) {
	n := node("a", "x")
	n.RetryDelayMS = 100
	n.DelayPolicy = "fixed"
	if got := backoffDelay(&n, 3, true); got != 100*time.Millisecond {
		t.Fatalf("fixed delay mismatch: %v", got)
	}
	n.DelayPolicy = "linear"
	if got := backoffDelay(&n, 2, true); got != 300*time.Millisecond {
		t.Fatalf("linear delay mismatch: %v", got)
	}
	n.DelayPolicy = "none"
	if got := backoffDelay(&n, 2, true); got != 0 {
		t.Fatalf("none delay mismatch: %v", got)
	}
	if got := backoffDelay(&n, 2, false); got != 0 {
		t.Fatalf("failover delay mismatch: %v", got)
	}
}
