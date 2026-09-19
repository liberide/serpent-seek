package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// fullChain builds a full-chain mode fixture: nodes linked by next edges in
// the given order, the first node being the start block.
func fullChain(providers ...string) *store.Chain {
	chain := &store.Chain{ID: "fc", Name: "full", Active: true, Mode: store.ChainModeFullChain}
	prev := ""
	for i, code := range providers {
		n := node("n"+code+"-"+string(rune('a'+i)), code)
		n.TimeoutMS = 1000
		n.Retries = 0
		if i == 0 {
			n.IsStart = true
		} else {
			chain.Edges = append(chain.Edges, store.ChainEdge{FromKey: prev, ToKey: n.Key, Condition: "next"})
		}
		prev = n.Key
		chain.Nodes = append(chain.Nodes, n)
	}
	return chain
}

func rowsResult(links ...string) providers.Result {
	rows := providers.Rows{}
	for _, link := range links {
		rows = append(rows, providers.Row{Link: link, Title: link})
	}
	return providers.Result{OK: true, Kind: providers.KindOK, Rows: rows}
}

func TestNormalizeLink(t *testing.T) {
	cases := map[string]string{
		"HTTPS://WWW.Example.COM/Page/":             "https://example.com/Page",
		"https://example.com/?utm_source=news&id=7": "https://example.com?id=7",
		"https://example.com/x#section":             "https://example.com/x",
		"https://example.com/x?gclid=1&ref=2":       "https://example.com/x?ref=2",
		"https://example.com/x?UTM_MEDIUM=a&keep=b": "https://example.com/x?keep=b",
		"not a url":                  "not a url",
		"https://example.com:8443/a": "https://example.com:8443/a",
		"http://example.com/a?fbclid=zz&utm_term=t&q=": "http://example.com/a?q=",
	}
	for in, want := range cases {
		if got := normalizeLink(in); got != want {
			t.Fatalf("normalizeLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFullChainMergesAndDedupes(t *testing.T) {
	first := &scriptedProvider{code: "p_a", results: []providers.Result{
		rowsResult("https://example.com/x", "https://www.site.com/page/?utm_source=news&id=1"),
	}}
	second := &scriptedProvider{code: "p_b", results: []providers.Result{
		rowsResult("https://site.com/page?id=1", "https://other.org/z#frag"),
	}}
	third := &scriptedProvider{code: "p_c", results: []providers.Result{failResult()}}
	h := newHarness(t, fullChain("p_a", "p_b", "p_c"), first, second, third)

	out, err := h.engine.Execute(context.Background(), Input{Query: "hello", Count: 5})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("expected ok, got %s (err=%s)", out.Request.Status, out.Request.Error)
	}
	if len(out.Rows) != 3 {
		t.Fatalf("expected 3 unique rows, got %d: %+v", len(out.Rows), out.Rows)
	}
	// The duplicated link keeps the first occurrence and remembers both sources.
	dup := out.Rows[1]
	if dup.Link != "https://www.site.com/page/?utm_source=news&id=1" {
		t.Fatalf("first occurrence must win, got %+v", dup)
	}
	if len(dup.Sources) != 2 || dup.Sources[0] != "p_a" || dup.Sources[1] != "p_b" {
		t.Fatalf("expected sources [p_a p_b], got %v", dup.Sources)
	}
	merge := out.Request.Merge
	if merge == nil || merge.CollectedTotal != 4 || merge.UniqueLinks != 3 || merge.DuplicatesRemoved != 1 {
		t.Fatalf("bad merge stats: %+v", merge)
	}
	mergeDone := false
	mergeProgress := false
	for _, event := range h.sink.events {
		if event == EventMergeDone {
			mergeDone = true
		}
		if event == EventMergeProgress {
			mergeProgress = true
		}
	}
	if !mergeDone || !mergeProgress {
		t.Fatalf("expected merge_progress and merge_done events, got %v", h.sink.events)
	}
	// The failing tail block must be painted in the trace without aborting the walk.
	statuses := map[string]string{}
	for _, step := range h.sink.steps {
		statuses[step.Provider] = step.Status
	}
	if statuses["p_c"] != "fail" || statuses["p_a"] != "ok" || statuses["p_b"] != "ok" {
		t.Fatalf("unexpected step statuses %v", statuses)
	}
	if len(out.Request.Results) != 3 || len(out.Request.Results[1].Sources) != 2 {
		t.Fatalf("persisted results must carry sources, got %+v", out.Request.Results)
	}
}

func TestFullChainContinuesAfterFailure(t *testing.T) {
	first := &scriptedProvider{code: "p_fail", results: []providers.Result{failResult()}}
	second := &scriptedProvider{code: "p_ok", results: []providers.Result{rowsResult("https://b.example/1")}}
	h := newHarness(t, fullChain("p_fail", "p_ok"), first, second)

	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" || len(out.Rows) != 1 {
		t.Fatalf("expected ok with one row, got %s/%d", out.Request.Status, len(out.Rows))
	}
}

func TestFullChainDeadProviderSkipsButWalkContinues(t *testing.T) {
	perm := &scriptedProvider{code: "p_perm", results: []providers.Result{permResult()}}
	okp := &scriptedProvider{code: "p_ok", results: []providers.Result{rowsResult("https://c.example/9")}}
	chain := fullChain("p_perm", "p_perm", "p_ok")
	h := newHarness(t, chain, perm, okp)

	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "ok" || len(out.Rows) != 1 {
		t.Fatalf("expected ok via p_ok, got %s rows=%d", out.Request.Status, len(out.Rows))
	}
	hasSkip := false
	for _, step := range h.sink.steps {
		if step.Status == "skip" && step.Provider == "p_perm" {
			hasSkip = true
		}
	}
	if !hasSkip {
		t.Fatalf("expected the dead provider's second block to be skipped, got %+v", h.sink.steps)
	}
}

func TestFullChainRespectsAttemptBudget(t *testing.T) {
	p := &scriptedProvider{code: "p_fail", results: []providers.Result{failResult()}}
	chain := fullChain("p_fail", "p_fail", "p_fail", "p_fail", "p_fail", "p_fail")
	for i := range chain.Nodes {
		chain.Nodes[i].Retries = 2
		chain.Nodes[i].RetryDelayMS = 0
	}
	h := newHarness(t, chain, p)
	h.kv.m["max_attempts"] = "10"
	_ = h.engine.settings.Refresh(context.Background())

	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(h.sink.steps) != 10 {
		t.Fatalf("expected the walk to stop at the budget of 10 steps, got %d", len(h.sink.steps))
	}
	if out.Request.Status != "fail" {
		t.Fatalf("expected fail, got %s", out.Request.Status)
	}
	if out.Request.Merge == nil || out.Request.Merge.CollectedTotal != 0 {
		t.Fatalf("expected empty merge stats, got %+v", out.Request.Merge)
	}
}

func TestFullChainFinalStatusMatrix(t *testing.T) {
	// All blocks empty, last one empty -> empty.
	empty := &scriptedProvider{code: "p_empty", results: []providers.Result{emptyResult()}}
	h := newHarness(t, fullChain("p_empty", "p_empty"), empty)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Status != "empty" {
		t.Fatalf("all-empty full chain must end empty, got %s", out.Request.Status)
	}

	// Tail fails with zero collected links -> fail with the last error.
	okEmpty := &scriptedProvider{code: "p_e2", results: []providers.Result{emptyResult()}}
	fail := &scriptedProvider{code: "p_f2", results: []providers.Result{failResult()}}
	h2 := newHarness(t, fullChain("p_e2", "p_f2"), okEmpty, fail)
	out2, err := h2.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out2.Request.Status != "fail" || out2.Request.Error == "" {
		t.Fatalf("expected fail with error, got %s err=%q", out2.Request.Status, out2.Request.Error)
	}
}

func TestFullChainHonoursCountCap(t *testing.T) {
	first := &scriptedProvider{code: "p_a", results: []providers.Result{
		rowsResult("https://a.example/1", "https://a.example/2", "https://a.example/3"),
	}}
	second := &scriptedProvider{code: "p_b", results: []providers.Result{
		rowsResult("https://b.example/1", "https://b.example/2"),
	}}
	h := newHarness(t, fullChain("p_a", "p_b"), first, second)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello", Count: 2})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("merged response must be capped at count=2, got %d", len(out.Rows))
	}
	// Blocks contribute raw (uncapped) rows; the cap applies to the final list:
	// collected 5, unique returned 2 (= unique_links per spec), duplicates 0.
	merge := out.Request.Merge
	if merge == nil || merge.CollectedTotal != 5 || merge.UniqueLinks != 2 || merge.DuplicatesRemoved != 0 {
		t.Fatalf("unexpected merge stats: %+v", merge)
	}
}

func TestNormalizeLinkKeepsDistinctRows(t *testing.T) {
	if normalizeLink("https://a.com/x") == normalizeLink("https://b.com/x") {
		t.Fatal("different hosts must not collide")
	}
	if normalizeLink("https://a.com/x?ref=2") == normalizeLink("https://a.com/x?gclid=1") {
		t.Fatal("non-tracking query params must survive normalization")
	}
	if !strings.HasPrefix(normalizeLink("https://WWW.A.COM/"), "https://a.com") {
		t.Fatal("www./casing normalization broken")
	}
}
