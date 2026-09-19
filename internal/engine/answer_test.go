package engine

import (
	"context"
	"testing"

	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// answerResult is a generated answer with one (sparse) source row.
func answerResult(text string) providers.Result {
	return providers.Result{
		OK: true, Kind: providers.KindOK, Answer: text,
		Rows: providers.Rows{{Link: "https://src.example", Title: "Source"}},
	}
}

func answerNode(key, provider string) store.ChainNode {
	n := node(key, provider)
	n.Mode = store.NodeModeAnswer
	return n
}

func TestAnswerNodeStoresAnswerWithoutRowMerge(t *testing.T) {
	first := &scriptedProvider{code: "p_fail", results: []providers.Result{failResult()}}
	answer := &scriptedProvider{code: "p_ans", results: []providers.Result{answerResult("generated text")}}
	chain := chainWith(
		[]store.ChainNode{node("a", "p_fail"), answerNode("b", "p_ans")},
		[]store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "fail"}},
	)
	h := newHarness(t, chain, first, answer)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Answer != "generated text" {
		t.Fatalf("expected the generated answer, got %q", out.Request.Answer)
	}
	if out.Request.Status != "ok" {
		t.Fatalf("an answer without rows must still be ok, got %s", out.Request.Status)
	}
	if len(out.Rows) != 0 {
		t.Fatalf("answer-node sources must not join the Row result by default, got %+v", out.Rows)
	}
}

func TestAnswerNodeSparseSourcesOptIn(t *testing.T) {
	first := &scriptedProvider{code: "p_fail", results: []providers.Result{failResult()}}
	answer := &scriptedProvider{code: "p_ans", results: []providers.Result{answerResult("generated text")}}
	answerNode := answerNode("b", "p_ans")
	answerNode.Params = map[string]string{"allow_sparse_sources": "true"}
	chain := chainWith(
		[]store.ChainNode{node("a", "p_fail"), answerNode},
		[]store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "fail"}},
	)
	h := newHarness(t, chain, first, answer)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Rows) != 1 {
		t.Fatalf("allow_sparse_sources must mix the source into Rows, got %+v", out.Rows)
	}
}

func TestAnswerNodeFullChainNoRowMerge(t *testing.T) {
	search := &scriptedProvider{code: "p_ok", results: []providers.Result{okResult()}}
	answer := &scriptedProvider{code: "p_ans", results: []providers.Result{answerResult("generated text")}}
	chain := chainWith(
		[]store.ChainNode{node("a", "p_ok"), answerNode("b", "p_ans")},
		[]store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "next"}},
	)
	chain.Mode = store.ChainModeFullChain
	h := newHarness(t, chain, search, answer)
	out, err := h.engine.Execute(context.Background(), Input{Query: "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Request.Answer != "generated text" {
		t.Fatalf("expected the generated answer, got %q", out.Request.Answer)
	}
	if len(out.Rows) != 1 || out.Rows[0].Link != "https://a" {
		t.Fatalf("only the search node's row must be merged, got %+v", out.Rows)
	}
}

func TestValidateRejectsAnswerStart(t *testing.T) {
	start := answerNode("a", "p_ans")
	start.IsStart = true
	chain := &store.Chain{Nodes: []store.ChainNode{start}}
	res := ValidateChain(chain, nil)
	if res.OK {
		t.Fatal("an answer node must not be accepted as the start block")
	}
}
