package api

import (
	"sort"
	"strings"
	"sync"
)

// Operation is one documented HTTP operation, derived from the embedded spec.
// ID ("METHOD /path") is the stable identifier used for developer-token
// permissions and matches chi's route pattern.
type Operation struct {
	ID      string `json:"id"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Tag     string `json:"tag,omitempty"`
	Summary string `json:"summary,omitempty"`
	Public  bool   `json:"public"`
}

var (
	opsOnce sync.Once
	ops     []Operation
)

// Operations returns the operations documented in the embedded OpenAPI spec,
// sorted by path then method. Parsed once and cached.
//
// The spec is intentionally parsed with a tiny line scanner (no YAML
// dependency): it is our own file, kept in a strict, machine-checked format
// (validated as YAML by web/scripts/check-openapi.mjs and kept in sync with the
// router by the parity test).
func Operations() []Operation {
	opsOnce.Do(func() { ops = parseOperations(OpenAPISpec) })
	out := make([]Operation, len(ops))
	copy(out, ops)
	return out
}

// EndpointIDs returns the set of valid endpoint ids ("METHOD /path").
func EndpointIDs() map[string]bool {
	ids := make(map[string]bool)
	for _, op := range Operations() {
		ids[op.ID] = true
	}
	return ids
}

// methodRank orders operations within a path and doubles as the set of HTTP
// methods we recognise (see isHTTPMethod).
var methodRank = map[string]int{
	"get": 0, "post": 1, "put": 2, "patch": 3, "delete": 4,
	"head": 5, "options": 6, "trace": 7,
}

func isHTTPMethod(name string) bool {
	_, ok := methodRank[name]
	return ok
}

func parseOperations(spec []byte) []Operation {
	var out []Operation
	var path string
	var cur *Operation
	commit := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, raw := range strings.Split(string(spec), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		switch indent := len(raw) - len(strings.TrimLeft(raw, " ")); indent {
		case 2:
			// A top-level path key, e.g. "  /api/chains:".
			if strings.HasPrefix(trimmed, "/") && strings.HasSuffix(trimmed, ":") {
				commit()
				path = strings.TrimSuffix(trimmed, ":")
			}
		case 4:
			// An operation method, e.g. "    get:".
			name := strings.TrimSuffix(trimmed, ":")
			if isHTTPMethod(name) && strings.HasSuffix(trimmed, ":") {
				commit()
				cur = &Operation{Method: strings.ToUpper(name), Path: path}
				cur.ID = cur.Method + " " + path
			}
		case 6:
			// Operation metadata (tags/summary/security) at this indent.
			if cur == nil {
				continue
			}
			switch {
			case strings.HasPrefix(trimmed, "tags:"):
				if cur.Tag == "" {
					cur.Tag = firstTag(strings.TrimSpace(strings.TrimPrefix(trimmed, "tags:")))
				}
			case strings.HasPrefix(trimmed, "summary:"):
				cur.Summary = strings.TrimSpace(strings.TrimPrefix(trimmed, "summary:"))
			case strings.HasPrefix(trimmed, "security:"):
				if strings.TrimSpace(strings.TrimPrefix(trimmed, "security:")) == "[]" {
					cur.Public = true
				}
			}
		}
	}
	commit()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return methodRank[strings.ToLower(out[i].Method)] < methodRank[strings.ToLower(out[j].Method)]
	})
	return out
}

// firstTag extracts the first tag from an inline YAML flow list like "[users]".
func firstTag(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") {
		return ""
	}
	value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = value[:i]
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
