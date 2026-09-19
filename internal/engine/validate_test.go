package engine

import (
	"errors"
	"testing"

	"github.com/liberide/serpent-seek/internal/store"
)

func node(key, provider string) store.ChainNode {
	return store.ChainNode{
		Key: key, ProviderID: provider, TimeoutMS: 1000, Retries: 0,
		OnSuccess: "stop", OnEmpty: "next", OnFail: "next",
	}
}

// startNode returns a node flagged as the chain start block.
func startNode(key, provider string) store.ChainNode {
	n := node(key, provider)
	n.IsStart = true
	return n
}

func TestValidateChainOK(t *testing.T) {
	chain := &store.Chain{
		Nodes: []store.ChainNode{startNode("a", "apiserpent"), node("b", "searxng")},
		Edges: []store.ChainEdge{{FromKey: "a", ToKey: "b", Condition: "fail"}},
	}
	res := ValidateChain(chain, map[string]bool{"apiserpent": true, "searxng": true})
	if !res.OK {
		t.Fatalf("expected valid chain, got errors %v", res.Errors)
	}
}

func TestValidateChainEmpty(t *testing.T) {
	res := ValidateChain(&store.Chain{}, nil)
	if res.OK || len(res.Errors) == 0 {
		t.Fatal("empty chain must be invalid")
	}
}

func TestValidateChainCycle(t *testing.T) {
	chain := &store.Chain{
		Nodes: []store.ChainNode{startNode("a", "x"), node("b", "x")},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "fail"},
			{FromKey: "b", ToKey: "a", Condition: "fail"},
		},
	}
	res := ValidateChain(chain, nil)
	if res.OK {
		t.Fatal("cycle must be rejected")
	}
}

func TestValidateChainNoStart(t *testing.T) {
	chain := &store.Chain{
		Nodes: []store.ChainNode{node("a", "x")},
	}
	res := ValidateChain(chain, nil)
	if res.OK {
		t.Fatal("chain without a start block must be invalid")
	}
	found := false
	for _, err := range res.Errors {
		if contains(err, "no start block") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected no-start error, got %v", res.Errors)
	}
}

func TestValidateChainMultipleStarts(t *testing.T) {
	chain := &store.Chain{
		Nodes: []store.ChainNode{startNode("a", "x"), startNode("b", "x")},
	}
	res := ValidateChain(chain, nil)
	found := false
	for _, err := range res.Errors {
		if contains(err, "start blocks") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected multiple-start error, got %v", res.Errors)
	}
}

func TestValidateChainInvalidMode(t *testing.T) {
	chain := &store.Chain{Mode: "banana", Nodes: []store.ChainNode{startNode("a", "x")}}
	res := ValidateChain(chain, nil)
	if res.OK {
		t.Fatal("unknown mode must be rejected")
	}
}

func TestValidateChainUnreachable(t *testing.T) {
	chain := &store.Chain{
		Nodes: []store.ChainNode{startNode("a", "x"), node("b", "x"), node("orphan", "x")},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "fail"},
		},
	}
	res := ValidateChain(chain, nil)
	found := false
	for _, err := range res.Errors {
		if contains(err, "unreachable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unreachable error, got %v", res.Errors)
	}
}

func TestValidateChainBudgetWarning(t *testing.T) {
	budget := node("b", "x")
	budget.Retries = 2
	heavy := node("c", "x")
	heavy.Retries = 3
	chain := &store.Chain{
		Nodes: []store.ChainNode{startNode("a", "x"), budget, heavy},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "next"},
			{FromKey: "b", ToKey: "c", Condition: "next"},
		},
	}
	// 1 + 3 + 4 = 8 potential steps.
	res := ValidateChain(chain, nil, 10)
	if !res.OK {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	for _, w := range res.Warnings {
		if contains(w, "budget") {
			t.Fatalf("8 steps must fit into budget 10, got %v", res.Warnings)
		}
	}
	res = ValidateChain(chain, nil, 5)
	found := false
	for _, w := range res.Warnings {
		if contains(w, "MAX_ATTEMPTS=5") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a budget warning, got %v", res.Warnings)
	}
}

func TestValidateFullChainNextReachabilityWarning(t *testing.T) {
	chain := &store.Chain{
		Mode:  store.ChainModeFullChain,
		Nodes: []store.ChainNode{startNode("a", "x"), node("b", "x"), node("c", "x")},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "next"},
			{FromKey: "b", ToKey: "c", Condition: "fail"}, // not followed in full_chain
		},
	}
	res := ValidateChain(chain, nil)
	if !res.OK {
		t.Fatalf("unreachable-via-next must only warn, got %v", res.Errors)
	}
	found := false
	for _, w := range res.Warnings {
		if contains(w, "next edges") && contains(w, "c") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a full-chain reachability warning, got %v", res.Warnings)
	}
}

func TestValidateChainUnknownProviderWarning(t *testing.T) {
	chain := &store.Chain{Nodes: []store.ChainNode{startNode("a", "mystery")}}
	res := ValidateChain(chain, map[string]bool{"apiserpent": true})
	if !res.OK {
		t.Fatalf("unknown provider should be a warning, got %v", res.Errors)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning for the unknown provider")
	}
}

func TestPlanStartAndNext(t *testing.T) {
	chain := &store.Chain{
		Nodes: []store.ChainNode{startNode("a", "p1"), node("b", "p2"), node("c", "p1")},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "fail"},
			{FromKey: "b", ToKey: "c", Condition: "empty"},
		},
	}
	plan, err := BuildPlan(chain)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Start().Key != "a" {
		t.Fatalf("unexpected start node %q", plan.Start().Key)
	}
	if next := plan.Next(plan.Node("a"), "fail"); next == nil || next.Key != "b" {
		t.Fatalf("on_fail should go to b, got %v", next)
	}
	if next := plan.Next(plan.Node("a"), "success"); next != nil {
		t.Fatalf("on_success=stop should terminate, got %v", next)
	}
	if next := plan.Next(plan.Node("b"), "empty"); next == nil || next.Key != "c" {
		t.Fatalf("on_empty should go to c, got %v", next)
	}
}

func TestPlanRequiresExplicitStart(t *testing.T) {
	if _, err := BuildPlan(&store.Chain{Nodes: []store.ChainNode{node("a", "p1")}}); !errors.Is(err, ErrNoStartNode) {
		t.Fatalf("expected ErrNoStartNode without is_start, got %v", err)
	}
}

func TestPlanStartPrefersExplicitFlag(t *testing.T) {
	// b has an incoming edge and is flagged as the start block; the explicit
	// flag must win over the legacy no-incoming-edge heuristic.
	chain := &store.Chain{
		Nodes: []store.ChainNode{node("a", "p1"), startNode("b", "p2")},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "next"},
		},
	}
	plan, err := BuildPlan(chain)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Start().Key != "b" {
		t.Fatalf("expected explicit start b, got %q", plan.Start().Key)
	}
}

func TestPlanNextInFullChain(t *testing.T) {
	chain := &store.Chain{
		Nodes: []store.ChainNode{startNode("a", "p1"), node("b", "p2"), node("c", "p3")},
		Edges: []store.ChainEdge{
			{FromKey: "a", ToKey: "b", Condition: "fail"},
			{FromKey: "a", ToKey: "c", Condition: "next"},
		},
	}
	plan, err := BuildPlan(chain)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if next := plan.NextInFullChain(plan.Node("a")); next == nil || next.Key != "c" {
		t.Fatalf("full chain must follow only next edges, got %v", next)
	}
	plan2, err := BuildPlan(&store.Chain{Nodes: []store.ChainNode{startNode("a", "p1")}})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan2.Mode() != store.ChainModeFirstSuccess {
		t.Fatalf("default mode must be first_success, got %q", plan2.Mode())
	}
}

func TestPlanSnapshotIsDeepCopy(t *testing.T) {
	chain := &store.Chain{Nodes: []store.ChainNode{startNode("a", "p1")}}
	plan, _ := BuildPlan(chain)
	snap := plan.Snapshot()
	snap.Nodes[0].Key = "changed"
	if chain.Nodes[0].Key != "a" {
		t.Fatal("snapshot mutated the source chain")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
