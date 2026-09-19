package engine

import (
	"errors"
	"strings"

	"github.com/liberide/serpent-seek/internal/store"
)

// ErrNoStartNode is returned when the graph has no unambiguous start node.
var ErrNoStartNode = errors.New("engine: chain has no start node")

// Plan is an indexed, executable view of a chain graph.
type Plan struct {
	chain *store.Chain
	nodes map[string]*store.ChainNode
	out   map[string][]store.ChainEdge
	order []string
	start *store.ChainNode
}

// BuildPlan indexes a chain and resolves its start node: exactly one node
// must be flagged is_start (the validator guarantees this for saved chains).
func BuildPlan(c *store.Chain) (*Plan, error) {
	if c == nil || len(c.Nodes) == 0 {
		return nil, errors.New("engine: empty chain")
	}
	p := &Plan{
		chain: c,
		nodes: map[string]*store.ChainNode{},
		out:   map[string][]store.ChainEdge{},
	}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		p.nodes[n.Key] = n
		p.order = append(p.order, n.Key)
	}
	for _, e := range c.Edges {
		p.out[e.FromKey] = append(p.out[e.FromKey], e)
	}
	for _, key := range p.order {
		if p.nodes[key].IsStart {
			p.start = p.nodes[key]
			break
		}
	}
	if p.start == nil {
		return nil, ErrNoStartNode
	}
	return p, nil
}

// Start returns the start node.
func (p *Plan) Start() *store.ChainNode { return p.start }

// Node resolves a node by key.
func (p *Plan) Node(key string) *store.ChainNode { return p.nodes[key] }

// Mode returns the normalized execution mode of the chain.
func (p *Plan) Mode() string {
	if p.chain.Mode == store.ChainModeFullChain {
		return store.ChainModeFullChain
	}
	return store.ChainModeFirstSuccess
}

// Snapshot returns a deep copy of the chain suitable for history storage.
func (p *Plan) Snapshot() *store.Chain {
	out := &store.Chain{
		ID:        p.chain.ID,
		Name:      p.chain.Name,
		Active:    p.chain.Active,
		Mode:      p.Mode(),
		Version:   p.chain.Version,
		CreatedAt: p.chain.CreatedAt,
		UpdatedAt: p.chain.UpdatedAt,
	}
	for _, n := range p.chain.Nodes {
		cp := n
		cp.Params = map[string]string{}
		for k, v := range n.Params {
			cp.Params[k] = v
		}
		out.Nodes = append(out.Nodes, cp)
	}
	out.Edges = append(out.Edges, p.chain.Edges...)
	return out
}

// Next resolves the following node for an outcome (success|empty|fail) using
// the node's per-outcome policy and the outgoing edge conditions.
func (p *Plan) Next(node *store.ChainNode, outcome string) *store.ChainNode {
	if node == nil {
		return nil
	}
	policy := policyFor(node, outcome)
	if policy == "stop" {
		return nil
	}
	edges := p.out[node.Key]
	if len(edges) == 0 {
		return nil
	}
	if policy == "edge" {
		for _, e := range edges {
			if edgeMatches(e.Condition, outcome) {
				return p.nodes[e.ToKey]
			}
		}
		return nil
	}
	// "next": prefer a matching condition, then a wildcard/next edge, then the
	// first edge to keep the chain moving.
	for _, e := range edges {
		if e.Condition == outcome {
			return p.nodes[e.ToKey]
		}
	}
	for _, e := range edges {
		if e.Condition == "next" || e.Condition == "any" || e.Condition == "" {
			return p.nodes[e.ToKey]
		}
	}
	return p.nodes[edges[0].ToKey]
}

// NextInFullChain resolves the following node in full-chain mode: only
// neutral "next" connectivity matters here (ok/empty/fail edges carry
// diagnostics colours but do not steer the walk).
func (p *Plan) NextInFullChain(node *store.ChainNode) *store.ChainNode {
	if node == nil {
		return nil
	}
	for _, e := range p.out[node.Key] {
		switch e.Condition {
		case "next", "any", "":
			return p.nodes[e.ToKey]
		}
	}
	return nil
}

func policyFor(node *store.ChainNode, outcome string) string {
	switch outcome {
	case "success":
		return defaultPolicy(node.OnSuccess, "stop")
	case "empty":
		return defaultPolicy(node.OnEmpty, "next")
	default:
		return defaultPolicy(node.OnFail, "next")
	}
}

func defaultPolicy(v, fallback string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return fallback
	}
	return v
}

func edgeMatches(condition, outcome string) bool {
	switch condition {
	case "", "next", "any":
		return true
	default:
		return condition == outcome
	}
}
