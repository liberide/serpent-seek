// Package providers implements the pluggable search backend registry.
package providers

import (
	"context"
	"sort"
	"strings"
)

// ParamType describes the UI control for a non-secret provider parameter.
type ParamType string

const (
	ParamTypeText     ParamType = "text"
	ParamTypeNumber   ParamType = "number"
	ParamTypeSelect   ParamType = "select"
	ParamTypeBoolean  ParamType = "boolean"
	ParamTypeTextarea ParamType = "textarea"
)

// ParamOption is a single option for a select parameter.
type ParamOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// CredentialField describes one secret input.
type CredentialField struct {
	Key string `json:"key"`
	// Multiline asks the UI to render a textarea instead of a one-line
	// password input (e.g. a service-account JSON blob).
	Multiline bool `json:"multiline,omitempty"`
}

// ParamField describes one non-secret provider parameter.
type ParamField struct {
	Key     string        `json:"key"`
	Label   string        `json:"label"`
	Type    ParamType     `json:"type"`
	Default string        `json:"default,omitempty"`
	Options []ParamOption `json:"options,omitempty"`
	Hint    string        `json:"hint,omitempty"`
	// Required marks a parameter that must be non-empty before the provider
	// instance can be saved (validated by the HTTP handler).
	Required bool `json:"required,omitempty"`
}

// ProviderSchema is the static metadata the UI uses to render the provider
// configuration form (credentials, parameters, hints).
type ProviderSchema struct {
	Code        string            `json:"code"`
	Name        string            `json:"name"`
	Deprecated  bool              `json:"deprecated"`
	Credentials []CredentialField `json:"credentials"`
	Params      []ParamField      `json:"params"`
	Hints       []string          `json:"hints,omitempty"`
}

// Query is a normalized search request handed to a provider.
type Query struct {
	Text    string
	Count   int
	Lang    string
	Country string
	// Extra carries engine/attempt hints and node parameter overrides.
	Extra map[string]string
}

// With returns a shallow copy with an extra key set.
func (q Query) With(key, value string) Query {
	out := q
	out.Extra = map[string]string{}
	for k, v := range q.Extra {
		out.Extra[k] = v
	}
	out.Extra[key] = value
	return out
}

// Get returns an extra value or the fallback.
func (q Query) Get(key, fallback string) string {
	if q.Extra != nil {
		if v, ok := q.Extra[key]; ok && v != "" {
			return v
		}
	}
	return fallback
}

// Row is one normalized search result. Sources is only populated by the
// full-chain merger (history/dedupe view): it lists every provider whose
// response contained the link.
type Row struct {
	Link    string   `json:"link"`
	Title   string   `json:"title"`
	Snippet string   `json:"snippet"`
	Sources []string `json:"sources,omitempty"`
}

// Rows is the normalized result list.
type Rows []Row

// Credentials holds write-only provider secrets.
type Credentials map[string]string

// Params holds non-secret provider and node parameters.
type Params map[string]string

// Result kinds, mirroring the shared error taxonomy.
const (
	KindOK      = "ok"
	KindEmpty   = "empty"
	KindAPI     = "api"
	KindHTTP    = "http"
	KindNet     = "net"
	KindTimeout = "timeout"
	KindRead    = "read"
	KindJSON    = "json"
)

// Result is the classified outcome of one upstream call.
type Result struct {
	OK         bool
	Rows       Rows
	Kind       string
	Error      string
	Permanent  bool
	HTTPStatus int
	Engine     string
	Provider   string
	RawStatus  string
	// RetryAfterMS asks the engine to wait at least this long before retrying
	// the request or moving on (Stack Exchange `backoff` field, Retry-After header).
	RetryAfterMS int
	// QuotaRemaining/QuotaMax mirror upstream quota counters when the response
	// exposes them (Stack Exchange). HasQuota distinguishes "0 remaining" from
	// "not reported".
	QuotaRemaining int
	QuotaMax       int
	HasQuota       bool
	// Answer carries a generated answer text of answer-type drivers (yandex_gen).
	// It is informational only and is never mixed into Rows.
	Answer string
}

// Provider is a pluggable search backend.
type Provider interface {
	// Code is the stable provider identifier.
	Code() string
	// Schema describes the provider's configuration UI.
	Schema() ProviderSchema
	// Search performs one upstream call and classifies the outcome.
	Search(ctx context.Context, q Query, c Credentials, p Params) Result
}

// Registry maps provider codes to implementations.
type Registry struct {
	m map[string]Provider
}

// NewRegistry builds the registry with the built-in providers.
func NewRegistry(client *HTTPClient) *Registry {
	r := &Registry{m: map[string]Provider{}}
	r.add(apiserpentProvider{http: client})
	r.add(serpbaseProvider{http: client})
	r.add(searxngProvider{http: client})
	r.add(yandexProvider{http: client})
	r.add(yandexGenProvider{http: client})
	r.add(googleProvider{http: client})
	r.add(googleCSEProvider{http: client})
	r.add(googleDispatcher{vertex: googleProvider{http: client}, cse: googleCSEProvider{http: client}})
	r.add(vertexSearchProvider{http: client})
	r.add(pubmedProvider{http: client})
	r.add(anthropicProvider{http: client})
	for _, p := range newRestRegistry(client) {
		r.add(p)
	}
	return r
}

func (r *Registry) add(p Provider) { r.m[p.Code()] = p }

// Register adds or replaces a provider implementation (used by tests and
// third-party extensions).
func (r *Registry) Register(p Provider) { r.add(p) }

// Get looks up a provider by code.
func (r *Registry) Get(code string) (Provider, bool) {
	p, ok := r.m[strings.ToLower(strings.TrimSpace(code))]
	return p, ok
}

// Codes returns the sorted list of registered provider codes.
func (r *Registry) Codes() []string {
	out := make([]string, 0, len(r.m))
	for c := range r.m {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// fail builds a failed Result with a redacted error message.
func fail(kind string, permanent bool, provider, msg string) Result {
	return Result{Kind: kind, Permanent: permanent, Provider: provider, Error: msg}
}
