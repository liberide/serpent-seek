package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/liberide/serpent-seek/internal/store"
)

// ValidationResult reports graph validation outcome.
type ValidationResult struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// ValidateChain checks a chain graph for cycles, orphans, duplicate keys and
// invalid policies. knownProviders, when non-nil, is used to warn about unknown
// provider codes. maxAttempts, when given (settings value), adds a warning when
// the potential step count exceeds the global attempt budget.
func ValidateChain(c *store.Chain, knownProviders map[string]bool, maxAttempts ...int) ValidationResult {
	res := ValidationResult{OK: true}
	errf := func(format string, args ...any) {
		res.OK = false
		res.Errors = append(res.Errors, fmt.Sprintf(format, args...))
	}
	warnf := func(format string, args ...any) {
		res.Warnings = append(res.Warnings, fmt.Sprintf(format, args...))
	}
	if c == nil || len(c.Nodes) == 0 {
		errf("chain has no nodes")
		return res
	}

	mode := c.Mode
	if mode == "" {
		mode = store.ChainModeFirstSuccess
	}
	switch mode {
	case store.ChainModeFirstSuccess, store.ChainModeFullChain:
	default:
		errf("chain mode %q is invalid (allowed: first_success, full_chain)", c.Mode)
	}

	nodes := map[string]*store.ChainNode{}
	incoming := map[string]int{}
	providerCount := 0
	for i := range c.Nodes {
		n := &c.Nodes[i]
		if strings.TrimSpace(n.Key) == "" {
			errf("node %d has an empty key", i)
			continue
		}
		if _, dup := nodes[n.Key]; dup {
			errf("duplicate node key %q", n.Key)
		}
		nodes[n.Key] = n
		// Reject unknown kinds instead of silently coercing them to provider;
		// this keeps the on-disk vocabulary (enforced by a CHECK constraint)
		// and the runtime view in agreement.
		if raw := strings.ToLower(strings.TrimSpace(n.Kind)); raw != "" {
			switch raw {
			case store.NodeKindProvider, store.NodeKindStart, store.NodeKindJoin:
			default:
				errf("node %q has invalid kind %q (allowed: provider, start, join)", n.Key, n.Kind)
			}
		}
		kind := Kind(n)
		if kind == store.NodeKindProvider {
			providerCount++
		}
		switch kind {
		case store.NodeKindProvider:
			if strings.TrimSpace(n.ProviderID) == "" {
				errf("node %q has no provider", n.Key)
			} else if knownProviders != nil && !knownProviders[lower(n.ProviderID)] {
				warnf("node %q uses unknown provider %q", n.Key, n.ProviderID)
			}
		case store.NodeKindStart, store.NodeKindJoin:
			// Logical blocks carry graph structure only; a stale provider_id is
			// ignored but flag it so the editor can clean it up.
			if strings.TrimSpace(n.ProviderID) != "" {
				warnf("logical node %q ignores its provider reference", n.Key)
			}
		}
		switch n.Mode {
		case "", store.NodeModeSearch, store.NodeModeAnswer:
		default:
			errf("node %q has invalid mode %q (allowed: search, answer)", n.Key, n.Mode)
		}
		if kind != store.NodeKindProvider && strings.EqualFold(strings.TrimSpace(n.Mode), store.NodeModeAnswer) {
			errf("logical node %q cannot be an answer node", n.Key)
		}
		if (kind == store.NodeKindStart || n.IsStart) && n.Mode == store.NodeModeAnswer {
			errf("start block %q cannot be an answer node", n.Key)
		}
		if n.TimeoutMS < 0 {
			errf("node %q has a negative timeout", n.Key)
		} else if n.TimeoutMS > 0 && (n.TimeoutMS < 1000 || n.TimeoutMS > 120000) {
			errf("node %q timeout %dms is out of range (1000–120000)", n.Key, n.TimeoutMS)
		}
		if n.Retries < 0 {
			errf("node %q has negative retries", n.Key)
		} else if n.Retries > 10 {
			errf("node %q retries %d exceed the limit of 10", n.Key, n.Retries)
		}
		if n.RetryDelayMS < 0 {
			errf("node %q has a negative retry delay", n.Key)
		}
		for name, policy := range map[string]string{"on_success": n.OnSuccess, "on_empty": n.OnEmpty, "on_fail": n.OnFail} {
			if policy == "" {
				continue
			}
			switch policy {
			case "stop", "next", "edge":
			default:
				errf("node %q has invalid %s policy %q", n.Key, name, policy)
			}
		}
	}
	// A chain that is only logical blocks can never produce a result.
	if providerCount == 0 {
		errf("chain has no provider block")
	}
	for _, e := range c.Edges {
		if _, ok := nodes[e.FromKey]; !ok {
			errf("edge references unknown source node %q", e.FromKey)
		}
		if _, ok := nodes[e.ToKey]; !ok {
			errf("edge references unknown target node %q", e.ToKey)
		}
		switch e.Condition {
		case "", "next", "any", "success", "empty", "fail":
		default:
			errf("edge %s->%s has invalid condition %q", e.FromKey, e.ToKey, e.Condition)
		}
		incoming[e.ToKey]++
	}

	// Join blocks must collect at least one branch and never fan out as a
	// provider would (they still may have a single neutral continuation edge).
	for i := range c.Nodes {
		n := &c.Nodes[i]
		if Kind(n) == store.NodeKindJoin && incoming[n.Key] == 0 {
			errf("join block %q has no incoming branches", n.Key)
		}
	}

	// Exactly one explicit start block. A logical start (kind=start) and the
	// LEGACY is_start flag are interchangeable entry markers; the union of both
	// must contain exactly one node.
	entry := map[string]bool{}
	for i := range c.Nodes {
		if c.Nodes[i].IsStart || Kind(&c.Nodes[i]) == store.NodeKindStart {
			entry[c.Nodes[i].Key] = true
		}
	}
	var starts []string
	for _, key := range sortedKeys(entry) {
		starts = append(starts, key)
	}
	if len(starts) == 0 {
		errf("chain has no start block (add a Start block or toggle is_start on exactly one node)")
	}
	if len(starts) > 1 {
		errf("chain has %d start blocks, expected exactly one: %s", len(starts), strings.Join(starts, ", "))
	}

	// Both the DAG executor and the fallback walker follow neutral (next)
	// edges out of a logical Start; a Join is only satisfied by neutral branches
	// in full_chain. Colored edges are diagnostics and would silently disconnect
	// the graph.
	if len(starts) == 1 {
		for i := range c.Nodes {
			n := &c.Nodes[i]
			switch Kind(n) {
			case store.NodeKindStart:
				if neutralSuccessorCount(c.Edges, n.Key) == 0 {
					errf("start block %q has no neutral (next) outgoing edge", n.Key)
				}
			case store.NodeKindJoin:
				if mode == store.ChainModeFullChain && neutralIncomingCount(c.Edges, n.Key) == 0 {
					errf("join block %q has no neutral (next) incoming branch", n.Key)
				}
			}
		}
	}

	// Cycle detection over the directed graph.
	if cycle := findCycle(nodes, c.Edges); cycle != "" {
		errf("chain contains a cycle: %s", cycle)
	}

	// Reachability from the start block.
	if len(starts) == 1 {
		seen := reachableFrom(starts[0], c.Edges, nil)
		for key := range nodes {
			if !seen[key] {
				errf("node %q is unreachable from the start block", key)
			}
		}

		// Attempt budget: retries inside each visited provider block all consume
		// steps; logical start/join blocks do not.
		if len(maxAttempts) > 0 && maxAttempts[0] > 0 {
			potential := 0
			for key := range seen {
				if n, ok := nodes[key]; ok && Kind(n) == store.NodeKindProvider {
					potential += n.Retries + 1
				}
			}
			if potential > maxAttempts[0] {
				warnf("chain may not finish: potentially %d steps > budget MAX_ATTEMPTS=%d", potential, maxAttempts[0])
			}
		}

		// Full-chain mode only follows neutral "next" connectivity.
		if mode == store.ChainModeFullChain {
			nextSeen := reachableFrom(starts[0], c.Edges, func(condition string) bool {
				switch condition {
				case "next", "any", "":
					return true
				}
				return false
			})
			for key := range nodes {
				if seen[key] && !nextSeen[key] {
					warnf("node %q is not reachable via next edges and will not join the full chain", key)
				}
			}
		}

		// Fallback mode is linear: fan-out and join blocks are inert there.
		if mode == store.ChainModeFirstSuccess {
			for i := range c.Nodes {
				n := &c.Nodes[i]
				if n.Key == starts[0] && neutralSuccessorCount(c.Edges, n.Key) > 1 {
					warnf("start block %q fans out to several blocks but first_success mode follows only the first", n.Key)
				}
				if Kind(n) == store.NodeKindJoin {
					warnf("join block %q only merges branches in full_chain mode", n.Key)
				}
			}
		}
	}
	return res
}

// reachableFrom walks the directed graph from key, optionally filtering edges.
func reachableFrom(start string, edges []store.ChainEdge, accept func(condition string) bool) map[string]bool {
	adj := map[string][]string{}
	for _, e := range edges {
		if accept != nil && !accept(e.Condition) {
			continue
		}
		adj[e.FromKey] = append(adj[e.FromKey], e.ToKey)
	}
	seen := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		queue = append(queue, adj[cur]...)
	}
	return seen
}

// findCycle returns a human readable cycle path or "" when acyclic.
func findCycle(nodes map[string]*store.ChainNode, edges []store.ChainEdge) string {
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.FromKey] = append(adj[e.FromKey], e.ToKey)
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var cycle string
	var dfs func(string) bool
	dfs = func(u string) bool {
		color[u] = gray
		stack = append(stack, u)
		for _, v := range adj[u] {
			if _, ok := nodes[v]; !ok {
				continue
			}
			switch color[v] {
			case gray:
				idx := 0
				for i, s := range stack {
					if s == v {
						idx = i
						break
					}
				}
				cycle = strings.Join(append(append([]string{}, stack[idx:]...), v), " -> ")
				return true
			case white:
				if dfs(v) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[u] = black
		return false
	}
	for key := range nodes {
		if color[key] == white {
			if dfs(key) {
				return cycle
			}
		}
	}
	return ""
}

func lower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// neutralSuccessorCount counts next/any edges leaving a node.
func neutralSuccessorCount(edges []store.ChainEdge, key string) int {
	n := 0
	for _, e := range edges {
		if e.FromKey != key {
			continue
		}
		switch e.Condition {
		case "next", "any", "":
			n++
		}
	}
	return n
}

// neutralIncomingCount counts next/any edges entering a node.
func neutralIncomingCount(edges []store.ChainEdge, key string) int {
	n := 0
	for _, e := range edges {
		if e.ToKey != key {
			continue
		}
		switch e.Condition {
		case "next", "any", "":
			n++
		}
	}
	return n
}

// sortedKeys returns map keys in deterministic order so validation messages
// are stable across runs.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
