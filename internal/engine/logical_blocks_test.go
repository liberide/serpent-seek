package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// logicalNode builds a logical (start/join) block with no provider instance.
func logicalNode(key, kind string) store.ChainNode {
	n := node(key, "")
	n.Kind = kind
	n.OnSuccess = "next"
	n.OnEmpty = "next"
	n.OnFail = "next"
	return n
}

// TestFullChainFanOutAndJoin exercises the requested topology:
// Start ─┬─> provider A ─┐
//
//	└─> provider B ─┴─> Join -> provider C
//
// Both branches run, the join merges their rows and removes duplicate URLs, and
// the block after the join only runs once the join is satisfied.
func TestFullChainFanOutAndJoin(t *testing.T) {
	start := logicalNode("start", store.NodeKindStart)
	start.IsStart = true
	join := logicalNode("join", store.NodeKindJoin)
	a := node("a", "p_a")
	b := node("b", "p_b")
	c := node("c", "p_c")
	chain := &store.Chain{
		ID: "fan", Name: "fan", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{start, a, b, join, c},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "a", Condition: "next"},
			{FromKey: "start", ToKey: "b", Condition: "next"},
			{FromKey: "a", ToKey: "join", Condition: "next"},
			{FromKey: "b", ToKey: "join", Condition: "next"},
			{FromKey: "join", ToKey: "c", Condition: "next"},
		},
	}
	pa := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://x/1", "https://dup/1")}}
	pb := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://dup/1", "https://z/1")}}
	pc := &scriptedProvider{code: "p_c", results: []providers.Result{rowsResult("https://downstream/1")}}
	h := newHarness(t, chain, pa, pb, pc)

	out, err := h.engine.Execute(context.Background(), Input{Query: "q", Count: 10})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("expected ok, got %s (err=%s)", out.Request.Status, out.Request.Error)
	}
	if len(out.Rows) != 4 {
		t.Fatalf("expected 4 unique rows (x, dup, z, downstream), got %d: %+v", len(out.Rows), out.Rows)
	}
	var dup, downstream *providers.Row
	for i := range out.Rows {
		switch out.Rows[i].Link {
		case "https://dup/1":
			dup = &out.Rows[i]
		case "https://downstream/1":
			downstream = &out.Rows[i]
		}
	}
	if dup == nil || len(dup.Sources) != 2 {
		t.Fatalf("duplicate link must merge both branch sources, got %+v", dup)
	}
	if downstream == nil {
		t.Fatalf("block after the join must run, got %+v", out.Rows)
	}
	// Merge stats count each provider once; one duplicate was removed.
	if out.Request.Merge == nil || out.Request.Merge.CollectedTotal != 5 || out.Request.Merge.DuplicatesRemoved != 1 {
		t.Fatalf("bad merge stats: %+v", out.Request.Merge)
	}
	// The logical blocks must not create provider steps.
	for _, step := range h.sink.steps {
		if step.NodeKey == "start" || step.NodeKey == "join" {
			t.Fatalf("logical block produced a provider step: %+v", step)
		}
	}
}

// TestFirstSuccessLogicalBlocksTransparent checks that a Start/Join pair is
// stepped through in fallback mode and the first provider is executed.
func TestFirstSuccessLogicalBlocksTransparent(t *testing.T) {
	start := logicalNode("start", store.NodeKindStart)
	start.IsStart = true
	join := logicalNode("join", store.NodeKindJoin)
	ok := node("ok", "p_ok")
	chain := &store.Chain{
		ID: "fb", Name: "fb", Active: true, Mode: store.ChainModeFirstSuccess,
		Nodes: []store.ChainNode{start, join, ok},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "join", Condition: "next"},
			{FromKey: "join", ToKey: "ok", Condition: "next"},
		},
	}
	provider := &scriptedProvider{code: "p_ok", results: []providers.Result{rowsResult("https://a/1")}}
	h := newHarness(t, chain, provider)

	out, err := h.engine.Execute(context.Background(), Input{Query: "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" || len(out.Rows) != 1 {
		t.Fatalf("expected ok with one row, got %s/%d", out.Request.Status, len(out.Rows))
	}
}

func TestValidateLogicalBlocks(t *testing.T) {
	start := logicalNode("start", store.NodeKindStart)
	join := logicalNode("join", store.NodeKindJoin)
	ok := node("ok", "p_ok")
	chain := &store.Chain{
		ID: "v", Name: "v", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{start, ok, join},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "ok", Condition: "next"},
			{FromKey: "ok", ToKey: "join", Condition: "next"},
		},
	}
	res := ValidateChain(chain, nil)
	if !res.OK {
		t.Fatalf("logical graph should validate, got %v", res.Errors)
	}

	// A join without incoming branches is rejected.
	lonely := &store.Chain{
		ID: "v2", Name: "v2", Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{
			{Key: "start", Kind: store.NodeKindStart},
			{Key: "join", Kind: store.NodeKindJoin},
		},
	}
	res = ValidateChain(lonely, nil)
	if res.OK {
		t.Fatal("join without incoming branches must be rejected")
	}
	foundIncoming := false
	for _, e := range res.Errors {
		if strings.Contains(e, "incoming") {
			foundIncoming = true
		}
	}
	if !foundIncoming {
		t.Fatalf("expected an incoming-branches error, got %v", res.Errors)
	}

	// A provider block with no provider instance is rejected.
	missing := &store.Chain{
		ID: "v3", Name: "v3", Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{
			{Key: "start", Kind: store.NodeKindStart},
			{Key: "p", Kind: store.NodeKindProvider},
		},
		Edges: []store.ChainEdge{{FromKey: "start", ToKey: "p", Condition: "next"}},
	}
	res = ValidateChain(missing, nil)
	if res.OK {
		t.Fatal("provider block without provider must be rejected")
	}
}

// TestOnSuccessNextContinuesAndMerges proves the previously dead on_success
// property now steers the fallback walk: a block that finishes successfully
// keeps going and its rows join the merged result.
func TestOnSuccessNextContinuesAndMerges(t *testing.T) {
	a := node("a", "p_a")
	a.OnSuccess = "next"
	b := node("b", "p_b")
	b.OnSuccess = "stop"
	start := logicalNode("start", store.NodeKindStart)
	chain := &store.Chain{
		ID: "cont", Name: "cont", Active: true, Mode: store.ChainModeFirstSuccess,
		Nodes: []store.ChainNode{start, a, b},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "a", Condition: "next"},
			{FromKey: "a", ToKey: "b", Condition: "next"},
		},
	}
	pa := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://a/1")}}
	pb := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://b/1")}}
	h := newHarness(t, chain, pa, pb)

	out, err := h.engine.Execute(context.Background(), Input{Query: "q", Count: 10})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" || len(out.Rows) != 2 {
		t.Fatalf("expected both rows merged, got %s/%d: %+v", out.Request.Status, len(out.Rows), out.Rows)
	}
}

// TestLegacyChainsKeepOldSemantics locks in the backward-compatibility gate:
// chains saved before logical blocks (no kind=start) must run exactly as
// before, ignoring the new on_success continuation and the new fan-out.
func TestLegacyChainsKeepOldSemantics(t *testing.T) {
	// first_success: on_success=next used to be inert and must stay inert.
	a := node("a", "p_a")
	a.OnSuccess = "next"
	a.IsStart = true
	b := node("b", "p_b")
	run := &store.Chain{
		ID: "legacy-fs", Name: "legacy-fs", Active: true, Mode: store.ChainModeFirstSuccess,
		Nodes: []store.ChainNode{a, b},
		Edges: []store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "next"}},
	}
	pa := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://a/1")}}
	pb := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://b/1")}}
	h := newHarness(t, run, pa, pb)
	out, err := h.engine.Execute(context.Background(), Input{Query: "q", Count: 10})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" || len(out.Rows) != 1 || out.Rows[0].Link != "https://a/1" {
		t.Fatalf("legacy first_success must stop at the first success, got %s: %+v", out.Request.Status, out.Rows)
	}

	// full_chain: a legacy chain with two neutral successors walked only the
	// first branch before; it must still do so.
	a2 := node("a", "p_a")
	a2.IsStart = true
	b2 := node("b", "p_b")
	c2 := node("c", "p_c")
	fan := &store.Chain{
		ID: "legacy-fc", Name: "legacy-fc", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{a2, b2, c2},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "next"},
			{FromKey: "a", ToKey: "c", Condition: "next"},
		},
	}
	p2a := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://a/1")}}
	p2b := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://b/1")}}
	p2c := &scriptedProvider{code: "p_c", results: []providers.Result{rowsResult("https://c/1")}}
	h2 := newHarness(t, fan, p2a, p2b, p2c)
	out2, err := h2.engine.Execute(context.Background(), Input{Query: "q", Count: 10})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out2.Rows) != 2 {
		t.Fatalf("legacy full_chain must follow only the first neutral branch (a,b), got %+v", out2.Rows)
	}
	for _, row := range out2.Rows {
		if row.Link == "https://c/1" {
			t.Fatalf("legacy full_chain must not run the second branch: %+v", out2.Rows)
		}
	}

	// With an explicit Start block the new fan-out applies and both branches run.
	start := logicalNode("start", store.NodeKindStart)
	nfan := &store.Chain{
		ID: "new-fc", Name: "new-fc", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{start, a2, b2, c2},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "a", Condition: "next"},
			{FromKey: "a", ToKey: "b", Condition: "next"},
			{FromKey: "a", ToKey: "c", Condition: "next"},
		},
	}
	n2a := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://a/1")}}
	n2b := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://b/1")}}
	n2c := &scriptedProvider{code: "p_c", results: []providers.Result{rowsResult("https://c/1")}}
	h3 := newHarness(t, nfan, n2a, n2b, n2c)
	out3, err := h3.engine.Execute(context.Background(), Input{Query: "q", Count: 10})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out3.Rows) != 3 {
		t.Fatalf("chain with a Start block must fan out to both branches, got %+v", out3.Rows)
	}
}

// barrierProvider blocks inside Search until released, signalling the moment
// it is entered. Tests use it to prove that sibling branches run concurrently
// and that a join waits for all of them.
type barrierProvider struct {
	code    string
	rows    providers.Rows
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *barrierProvider) Code() string { return p.code }

func (p *barrierProvider) Schema() providers.ProviderSchema {
	return providers.ProviderSchema{Code: p.code, Name: p.code}
}

func (p *barrierProvider) Search(ctx context.Context, _ providers.Query, _ providers.Credentials, _ providers.Params) providers.Result {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
	case <-ctx.Done():
		return providers.Result{Provider: p.code, Kind: providers.KindNet, Error: ctx.Err().Error()}
	}
	return providers.Result{OK: true, Kind: providers.KindOK, Provider: p.code, Rows: p.rows}
}

// TestFourBranchesStartInParallelAndJoinWaits proves the requested behaviour:
// Start fans out to four providers that all enter Search before any of them is
// released, and the request cannot finish until the Join has seen them all.
func TestFourBranchesStartInParallelAndJoinWaits(t *testing.T) {
	start := logicalNode("start", store.NodeKindStart)
	join := logicalNode("join", store.NodeKindJoin)
	nodes := []store.ChainNode{start}
	edges := []store.ChainEdge{}
	var registered []providers.Provider
	var barriers []*barrierProvider
	for _, code := range []string{"p_a", "p_b", "p_c", "p_d"} {
		nodes = append(nodes, node(code, code))
		edges = append(edges,
			store.ChainEdge{FromKey: "start", ToKey: code, Condition: "next"},
			store.ChainEdge{FromKey: code, ToKey: "join", Condition: "next"},
		)
		bp := &barrierProvider{
			code: code, rows: rowsResult("https://" + code + "/1").Rows,
			started: make(chan struct{}), release: make(chan struct{}),
		}
		barriers = append(barriers, bp)
		registered = append(registered, bp)
	}
	nodes = append(nodes, join)
	chain := &store.Chain{ID: "par", Name: "par", Active: true, Mode: store.ChainModeFullChain, Nodes: nodes, Edges: edges}
	h := newHarness(t, chain, registered...)

	type outcome struct {
		out *Output
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := h.engine.Execute(context.Background(), Input{Query: "q", Count: 10})
		done <- outcome{out, err}
	}()

	// All four providers must enter Search before any is released, otherwise the
	// engine would be walking them one after another.
	allStarted := make(chan struct{})
	go func() {
		for _, bp := range barriers {
			<-bp.started
		}
		close(allStarted)
	}()
	select {
	case <-allStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("branches did not start in parallel")
	}

	// While all four are blocked the Join cannot be satisfied, so nothing may
	// finish yet.
	select {
	case r := <-done:
		t.Fatalf("request finished before the branches were released: %+v", r)
	default:
	}

	for _, bp := range barriers {
		close(bp.release)
	}
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Execute: %v", r.err)
		}
		if r.out.Request.Status != "ok" || len(r.out.Rows) != 4 {
			t.Fatalf("expected all four branches merged, got %s/%d: %+v", r.out.Request.Status, len(r.out.Rows), r.out.Rows)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not finish after releasing the branches")
	}
}

// TestValidateRejectsInvalidKind ensures the new kind field is whitelisted and
// never silently coerced.
func TestValidateRejectsInvalidKind(t *testing.T) {
	chain := &store.Chain{
		ID: "vk", Name: "vk", Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{
			{Key: "start", Kind: store.NodeKindStart},
			{Key: "p", Kind: "bogus"},
		},
		Edges: []store.ChainEdge{{FromKey: "start", ToKey: "p", Condition: "next"}},
	}
	res := ValidateChain(chain, nil)
	if res.OK {
		t.Fatal("invalid kind must be rejected")
	}
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e, "invalid kind") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an invalid-kind error, got %v", res.Errors)
	}
}

// TestLegacyFullChainLogicalTransparent checks that a logical block on a chain
// without an explicit Start (legacy path) is stepped through instead of being
// mistaken for an unknown provider driver and recorded as a skip.
func TestLegacyFullChainLogicalTransparent(t *testing.T) {
	a := node("a", "p_a")
	a.IsStart = true
	join := logicalNode("join", store.NodeKindJoin)
	b := node("b", "p_b")
	chain := &store.Chain{
		ID: "legacy-join", Name: "legacy-join", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{a, join, b},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "join", Condition: "next"},
			{FromKey: "join", ToKey: "b", Condition: "next"},
		},
	}
	pa := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://a/1")}}
	pb := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://b/1")}}
	h := newHarness(t, chain, pa, pb)

	out, err := h.engine.Execute(context.Background(), Input{Query: "q", Count: 10})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("expected both providers merged, got %+v", out.Rows)
	}
	for _, step := range h.sink.steps {
		if step.NodeKey == "join" {
			t.Fatalf("logical block must be transparent, got step %+v", step)
		}
	}
}

// TestFullChainAnswerDeterministic pins the generated answer to the last answer
// block in plan order, even when the branches ran in parallel.
func TestFullChainAnswerDeterministic(t *testing.T) {
	start := logicalNode("start", store.NodeKindStart)
	aa := node("aa", "p_aa")
	aa.Mode = store.NodeModeAnswer
	bb := node("bb", "p_bb")
	bb.Mode = store.NodeModeAnswer
	chain := &store.Chain{
		ID: "ans", Name: "ans", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{start, aa, bb},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "aa", Condition: "next"},
			{FromKey: "start", ToKey: "bb", Condition: "next"},
		},
	}
	pA := &scriptedProvider{code: "p_aa", results: []providers.Result{{OK: true, Kind: providers.KindOK, Answer: "A"}}}
	pB := &scriptedProvider{code: "p_bb", results: []providers.Result{{OK: true, Kind: providers.KindOK, Answer: "B"}}}
	h := newHarness(t, chain, pA, pB)

	out, err := h.engine.Execute(context.Background(), Input{Query: "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Answer != "B" {
		t.Fatalf("answer must be deterministic (last in plan order), got %q", out.Request.Answer)
	}
}

// TestJoinIgnoreCountLiftsCap checks the Join block's ignore_count property:
// with it on, the request count is ignored and every unique result is returned.
func TestJoinIgnoreCountLiftsCap(t *testing.T) {
	build := func(ignore bool) *store.Chain {
		start := logicalNode("start", store.NodeKindStart)
		join := logicalNode("join", store.NodeKindJoin)
		if ignore {
			join.Params = map[string]string{"ignore_count": "1"}
		}
		a := node("a", "p_a")
		b := node("b", "p_b")
		return &store.Chain{
			ID: "cap", Name: "cap", Active: true, Mode: store.ChainModeFullChain,
			Nodes: []store.ChainNode{start, a, b, join},
			Edges: []store.ChainEdge{
				{FromKey: "start", ToKey: "a", Condition: "next"},
				{FromKey: "start", ToKey: "b", Condition: "next"},
				{FromKey: "a", ToKey: "join", Condition: "next"},
				{FromKey: "b", ToKey: "join", Condition: "next"},
			},
		}
	}
	newProviders := func() (providers.Provider, providers.Provider) {
		a := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://a/1", "https://a/2", "https://a/3")}}
		b := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://b/1")}}
		return a, b
	}

	pa, pb := newProviders()
	h := newHarness(t, build(false), pa, pb)
	capped, err := h.engine.Execute(context.Background(), Input{Query: "q", Count: 1})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(capped.Rows) != 1 {
		t.Fatalf("without ignore_count the result must be capped at count=1, got %d", len(capped.Rows))
	}

	pa2, pb2 := newProviders()
	h2 := newHarness(t, build(true), pa2, pb2)
	all, err := h2.engine.Execute(context.Background(), Input{Query: "q", Count: 1})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(all.Rows) != 4 {
		t.Fatalf("ignore_count must return all 4 unique rows, got %d: %+v", len(all.Rows), all.Rows)
	}
	if all.Request.Count != 0 {
		t.Fatalf("ignore_count must persist count=0 (everything), got %d", all.Request.Count)
	}
}

// TestStepIndicesFollowExecutionOrder guards the timeline ordering: the DB
// stores nodes ORDER BY key, but steps must be numbered in execution order.
func TestStepIndicesFollowExecutionOrder(t *testing.T) {
	// Slice sorted by key (as the store returns it), while the walk runs z->a->m.
	a := node("a_second", "p_a")
	m := node("m_third", "p_m")
	z := node("z_start", "p_z")
	z.IsStart = true
	chain := &store.Chain{
		ID: "idx", Name: "idx", Active: true, Mode: store.ChainModeFirstSuccess,
		Nodes: []store.ChainNode{a, m, z},
		Edges: []store.ChainEdge{
			{FromKey: "z_start", ToKey: "a_second", Condition: "next"},
			{FromKey: "a_second", ToKey: "m_third", Condition: "next"},
		},
	}
	pz := &scriptedProvider{code: "p_z", results: []providers.Result{failResult()}}
	pa := &scriptedProvider{code: "p_a", results: []providers.Result{failResult()}}
	pm := &scriptedProvider{code: "p_m", results: []providers.Result{failResult()}}
	h := newHarness(t, chain, pz, pa, pm)

	if _, err := h.engine.Execute(context.Background(), Input{Query: "q"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantKeys := []string{"z_start", "a_second", "m_third"}
	var gotKeys []string
	var gotIdx []int
	for _, step := range h.sink.steps {
		gotKeys = append(gotKeys, step.NodeKey)
		gotIdx = append(gotIdx, step.Idx)
	}
	if len(gotKeys) != 3 || gotKeys[0] != wantKeys[0] || gotKeys[1] != wantKeys[1] || gotKeys[2] != wantKeys[2] {
		t.Fatalf("steps must follow execution order, got keys=%v idx=%v", gotKeys, gotIdx)
	}
	for i, idx := range gotIdx {
		if idx != i {
			t.Fatalf("step %d must carry idx %d, got %d", i, i, idx)
		}
	}
}

// TestValidateRejectsChainWithoutProviders ensures a chain made only of logical
// blocks cannot be saved (it could never return a result).
func TestValidateRejectsChainWithoutProviders(t *testing.T) {
	chain := &store.Chain{
		ID: "np", Name: "np", Mode: store.ChainModeFirstSuccess,
		Nodes: []store.ChainNode{{Key: "start", Kind: store.NodeKindStart}},
	}
	res := ValidateChain(chain, nil)
	if res.OK {
		t.Fatal("chain without providers must be rejected")
	}
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e, "provider block") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a provider-block error, got %v", res.Errors)
	}
}

// TestValidateRejectsLogicalBlocksWithoutNeutralEdges covers two silent-graph
// traps: a Start wired only with colored edges, and a Join fed only by colored
// branches (neither steers the full-chain walk).
func TestValidateRejectsLogicalBlocksWithoutNeutralEdges(t *testing.T) {
	start := logicalNode("start", store.NodeKindStart)
	p := node("p", "p_a")
	chain := &store.Chain{
		ID: "n1", Name: "n1", Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{start, p},
		Edges: []store.ChainEdge{{FromKey: "start", ToKey: "p", Condition: "fail"}},
	}
	if res := ValidateChain(chain, nil); res.OK {
		t.Fatal("start without a neutral outgoing edge must be rejected")
	}

	start2 := logicalNode("start", store.NodeKindStart)
	p2 := node("p", "p_a")
	join := logicalNode("join", store.NodeKindJoin)
	chain2 := &store.Chain{
		ID: "n2", Name: "n2", Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{start2, p2, join},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "p", Condition: "next"},
			{FromKey: "p", ToKey: "join", Condition: "fail"},
		},
	}
	if res := ValidateChain(chain2, nil); res.OK {
		t.Fatal("join without a neutral incoming branch must be rejected")
	}
}

// TestPlanPrefersLogicalStart verifies BuildPlan picks kind=start over a stale
// is_start flag when both exist.
func TestPlanPrefersLogicalStart(t *testing.T) {
	legacy := node("legacy", "p_a")
	legacy.IsStart = true
	logical := logicalNode("start", store.NodeKindStart)
	chain := &store.Chain{
		ID: "p", Name: "p", Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{legacy, logical},
		Edges: []store.ChainEdge{{FromKey: "start", ToKey: "legacy", Condition: "next"}},
	}
	plan, err := BuildPlan(chain)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Start().Key != "start" {
		t.Fatalf("expected logical start, got %q", plan.Start().Key)
	}
}

// stepProviderIdxs returns the execution indices of the recorded steps in
// insertion order.
func stepProviderIdxs(t *testing.T, steps []*store.RequestStep) ([]string, []int) {
	t.Helper()
	var keys []string
	var idxs []int
	for _, s := range steps {
		keys = append(keys, s.NodeKey)
		idxs = append(idxs, s.Idx)
	}
	return keys, idxs
}

// TestStepIndicesLegacyFullChainOrder covers the legacy full-chain walker: the
// step index must follow the walk, not the (key-sorted) node slice order.
func TestStepIndicesLegacyFullChainOrder(t *testing.T) {
	a := node("a_second", "p_a")
	m := node("m_third", "p_m")
	z := node("z_start", "p_z")
	z.IsStart = true
	chain := &store.Chain{
		ID: "idxfc", Name: "idxfc", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{a, m, z},
		Edges: []store.ChainEdge{
			{FromKey: "z_start", ToKey: "a_second", Condition: "next"},
			{FromKey: "a_second", ToKey: "m_third", Condition: "next"},
		},
	}
	pz := &scriptedProvider{code: "p_z", results: []providers.Result{failResult()}}
	pa := &scriptedProvider{code: "p_a", results: []providers.Result{failResult()}}
	pm := &scriptedProvider{code: "p_m", results: []providers.Result{failResult()}}
	h := newHarness(t, chain, pz, pa, pm)

	if _, err := h.engine.Execute(context.Background(), Input{Query: "q"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	keys, idxs := stepProviderIdxs(t, h.sink.steps)
	want := []string{"z_start", "a_second", "m_third"}
	for i := range want {
		if i >= len(keys) || keys[i] != want[i] {
			t.Fatalf("legacy full-chain order: got keys=%v idx=%v", keys, idxs)
		}
		if idxs[i] != i {
			t.Fatalf("step %d must carry idx %d, got keys=%v idx=%v", i, i, keys, idxs)
		}
	}
}

// TestStepIndicesDAGSequential covers the new fan-out executor: sibling
// branches get distinct consecutive indices and a block after the Join is
// numbered last, so the timeline stays chronological.
func TestStepIndicesDAGSequential(t *testing.T) {
	start := logicalNode("start", store.NodeKindStart)
	join := logicalNode("join", store.NodeKindJoin)
	a := node("a", "p_a")
	b := node("b", "p_b")
	c := node("c", "p_c")
	chain := &store.Chain{
		ID: "idxdag", Name: "idxdag", Active: true, Mode: store.ChainModeFullChain,
		Nodes: []store.ChainNode{start, a, b, join, c},
		Edges: []store.ChainEdge{
			{FromKey: "start", ToKey: "a", Condition: "next"},
			{FromKey: "start", ToKey: "b", Condition: "next"},
			{FromKey: "a", ToKey: "join", Condition: "next"},
			{FromKey: "b", ToKey: "join", Condition: "next"},
			{FromKey: "join", ToKey: "c", Condition: "next"},
		},
	}
	pa := &scriptedProvider{code: "p_a", results: []providers.Result{rowsResult("https://a/1")}}
	pb := &scriptedProvider{code: "p_b", results: []providers.Result{rowsResult("https://b/1")}}
	pc := &scriptedProvider{code: "p_c", results: []providers.Result{rowsResult("https://c/1")}}
	h := newHarness(t, chain, pa, pb, pc)

	if _, err := h.engine.Execute(context.Background(), Input{Query: "q", Count: 10}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	keys, idxs := stepProviderIdxs(t, h.sink.steps)
	if len(keys) != 3 {
		t.Fatalf("expected 3 provider steps, got keys=%v idx=%v", keys, idxs)
	}
	seen := map[int]string{}
	for i, idx := range idxs {
		if _, dup := seen[idx]; dup {
			t.Fatalf("duplicate step idx %d (keys=%v idx=%v)", idx, keys, idxs)
		}
		seen[idx] = keys[i]
	}
	if seen[0] != "a" || seen[1] != "b" || seen[2] != "c" {
		t.Fatalf("DAG indices must be wave-ordered (a=0,b=1,c=2), got %v", seen)
	}
}
