// Package store defines the persistence interface and domain models.
package store

import (
	"strconv"
	"strings"
)

// Timestamps are stored as RFC3339 strings in UTC to keep SQLite and
// PostgreSQL behaviour identical without driver-specific time handling.

// User is an account that can own API keys and passkeys.
type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"` // admin|viewer
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at"`
	// Quota is the runtime view of the user's per-user search allowance,
	// computed by the HTTP layer and never persisted with the user row.
	// Nil means the user is unlimited (no quota row or requests = 0).
	Quota *UserQuotaStatus `json:"quota,omitempty"`
}

// UserQuotaStatus is the runtime view of a user's quota: the configured
// allowance plus current usage. It is computed by the HTTP layer; only the
// UserQuota row is persisted.
type UserQuotaStatus struct {
	Requests    int    `json:"requests"` // 0 = unlimited
	Period      string `json:"period"`
	WindowHours int    `json:"window_hours"`
	TzOffset    int    `json:"tz_offset"`
	Used        int    `json:"used"`
	Remaining   int    `json:"remaining"`
	// ResetAt is the RFC3339 UTC moment the current period ends; empty for
	// rolling quotas (a sliding window has no fixed reset).
	ResetAt   string `json:"reset_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Quota reset periods for UserQuota.
const (
	// QuotaPeriodDaily resets the allowance at local midnight (tz_offset).
	QuotaPeriodDaily = "daily"
	// QuotaPeriodWeekly resets the allowance on Monday 00:00 (tz_offset).
	QuotaPeriodWeekly = "weekly"
	// QuotaPeriodMonthly resets the allowance on the 1st of the month 00:00.
	QuotaPeriodMonthly = "monthly"
	// QuotaPeriodRolling is a sliding window of WindowHours; the allowance
	// is "requests per trailing window" counted from usage_rolling hits.
	QuotaPeriodRolling = "rolling"
)

// UserQuota is the per-user search allowance enforced by the HTTP guard.
// A user without a row (or with Requests == 0) is unlimited and falls back
// to the global search_daily_quota / search_rate_limit_per_minute settings.
type UserQuota struct {
	UserID string `json:"user_id"`
	// Requests is the maximum number of search requests per period.
	// 0 means unlimited.
	Requests int `json:"requests"`
	// Period is one of the QuotaPeriod* constants.
	Period string `json:"period"`
	// WindowHours is the sliding-window length; only meaningful when
	// Period == QuotaPeriodRolling.
	WindowHours int `json:"window_hours"`
	// TzOffset shifts calendar reset boundaries by this many minutes
	// (-720..840). 0 = pure UTC. A fixed offset is used instead of an IANA
	// name to avoid a tzdata dependency.
	TzOffset  int    `json:"tz_offset"`
	UpdatedAt string `json:"updated_at"`
}

// APIKey is a hashed access token. The plaintext is never stored.
type APIKey struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name,omitempty"`
	Name     string `json:"name"`
	Prefix   string `json:"prefix"`
	Hash     string `json:"-"`
	Scopes   string `json:"scopes"`
	// ChainID optionally binds this token to a chain. Empty means the token
	// uses the globally active chain. The reference is cleared (set to NULL)
	// when the chain is deleted, so the token never breaks.
	ChainID string `json:"chain_id"`
	// Permissions is a JSON array of endpoint ids ("METHOD /path") a developer
	// token may call. Empty means "all endpoints"; non-developer keys ignore it.
	Permissions string `json:"-"`
	LastUsedAt  string `json:"last_used_at"`
	ExpiresAt   string `json:"expires_at"`
	RevokedAt   string `json:"revoked_at"`
	CreatedAt   string `json:"created_at"`
	// UsedToday is the number of search requests made with this key today
	// (UTC). It is computed by the HTTP layer and never persisted with the key.
	UsedToday int `json:"used_today"`
}

// Passkey is a WebAuthn credential.
type Passkey struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	CredentialID []byte `json:"-"`
	PublicKey    []byte `json:"-"`
	SignCount    uint32 `json:"sign_count"`
	Transports   string `json:"transports"`
	AAGUID       []byte `json:"-"`
	CreatedAt    string `json:"created_at"`
	LastUsedAt   string `json:"last_used_at"`
}

// Session is a browser session with a hashed token.
type Session struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	TokenHash string `json:"-"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	IP        string `json:"ip"`
	UA        string `json:"ua"`
}

// Provider is a configured search backend *instance*. Several instances may
// share the same Code (driver) with different names and credentials; ID is the
// identity referenced by chain nodes. Credentials are write-only.
type Provider struct {
	ID      string `json:"id"`
	Code    string `json:"code"` // driver type registered in the provider registry
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	// ProxyID optionally routes this instance's outbound requests through a
	// named proxy from the proxies table. Empty means a direct connection.
	ProxyID     string            `json:"proxy_id"`
	Credentials map[string]string `json:"-"`
	Params      map[string]string `json:"params"`
	UpdatedAt   string            `json:"updated_at"`
}

// CostPerRequest returns the operator-declared cost of one call to this provider
// instance, read from the `cost_per_request` param. It returns 0 when the value
// is absent, empty, non-numeric or not positive — 0 means "do not count cost".
func (p *Provider) CostPerRequest() float64 {
	if p == nil || p.Params == nil {
		return 0
	}
	raw := strings.TrimSpace(p.Params["cost_per_request"])
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

// Proxy is a named outbound proxy that provider instances may route through.
// Password is write-only: the API never returns it.
type Proxy struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Type      string `json:"type"` // http|https|socks5
	Host      string `json:"host"`
	Port      string `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"-"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Chain execution modes.
const (
	// ChainModeFirstSuccess stops the walk at the first usable result.
	ChainModeFirstSuccess = "first_success"
	// ChainModeFullChain walks every reachable block and merges the results.
	ChainModeFullChain = "full_chain"
)

// Chain is a named search pipeline graph.
type Chain struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Active    bool        `json:"active"`
	Mode      string      `json:"mode"` // first_success|full_chain
	Version   int         `json:"version"`
	Nodes     []ChainNode `json:"nodes"`
	Edges     []ChainEdge `json:"edges"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
	// BoundKeys is the number of API tokens bound to this chain. It is
	// computed by the HTTP layer and not persisted with the chain row.
	BoundKeys int `json:"bound_keys"`
	// ActiveBoundKeys is the subset of BoundKeys that is still usable (not
	// revoked). Only these tokens must be confirmed and fall back to the
	// default (active) chain when the bound chain is deleted.
	ActiveBoundKeys int `json:"active_bound_keys"`
}

// Chain node modes. A search node is a Row source; an answer node produces a
// generated answer (Result.Answer) and is excluded from the Row merge.
const (
	NodeModeSearch = "search"
	NodeModeAnswer = "answer"
)

// Chain node kinds. Logical nodes carry graph structure only: the start node
// is the single entry point (it can fan out to several providers), the join
// node merges the branches feeding it and removes duplicate URLs. A provider
// node runs one configured provider instance.
const (
	NodeKindProvider = "provider"
	NodeKindStart    = "start"
	NodeKindJoin     = "join"
)

// ChainNode is a single block in a chain: a provider instance or a logical
// start/join node.
type ChainNode struct {
	ID           string            `json:"id"`
	ChainID      string            `json:"chain_id"`
	Key          string            `json:"key"`
	Kind         string            `json:"kind"`        // provider|start|join
	ProviderID   string            `json:"provider_id"` // references providers.id (a provider instance)
	Label        string            `json:"label"`
	Mode         string            `json:"mode"` // search|answer
	Params       map[string]string `json:"params"`
	TimeoutMS    int               `json:"timeout_ms"`
	Retries      int               `json:"retries"`
	RetryDelayMS int               `json:"retry_delay_ms"`
	DelayPolicy  string            `json:"delay_policy"` // none|fixed|linear
	OnSuccess    string            `json:"on_success"`   // stop|edge
	OnEmpty      string            `json:"on_empty"`     // next|stop|edge
	OnFail       string            `json:"on_fail"`      // next|stop|edge
	IsStart      bool              `json:"is_start"`     // LEGACY: entry-point flag superseded by Kind=start
	PosX         float64           `json:"pos_x"`
	PosY         float64           `json:"pos_y"`
}

// IsLogical reports whether the node is a logical start/join block without a
// backing provider instance.
func (n *ChainNode) IsLogical() bool {
	return n.Kind == NodeKindStart || n.Kind == NodeKindJoin
}

// ChainEdge is a directed edge between two nodes with an outcome condition.
type ChainEdge struct {
	ID        string `json:"id"`
	ChainID   string `json:"chain_id"`
	FromKey   string `json:"from_key"`
	ToKey     string `json:"to_key"`
	Condition string `json:"condition"` // success|empty|fail|next|any
}

// MergeStats is the full-chain merge summary attached to a request.
type MergeStats struct {
	CollectedTotal    int `json:"collected_total"`
	UniqueLinks       int `json:"unique_links"`
	DuplicatesRemoved int `json:"duplicates_removed"`
}

// RequestResult is a persisted final result row. In full-chain mode Sources
// lists every provider that returned the link (after dedupe).
type RequestResult struct {
	Link    string   `json:"link"`
	Title   string   `json:"title,omitempty"`
	Snippet string   `json:"snippet,omitempty"`
	Sources []string `json:"sources,omitempty"`
}

// Request is a single search request with its execution summary.
type Request struct {
	ID            string `json:"id"`
	RID           string `json:"rid"`
	Query         string `json:"query"`
	Count         int    `json:"count"`
	Status        string `json:"status"` // running|ok|empty|fail
	UsedProvider  string `json:"used_provider"`
	ResultsCount  int    `json:"results_count"`
	TotalMS       int    `json:"total_ms"`
	StepsCount    int    `json:"steps_count"`
	ChainID       string `json:"chain_id"`
	ChainSnapshot *Chain `json:"chain_snapshot,omitempty"`
	// UserID is the owning user (empty when unset).
	UserID string `json:"user_id,omitempty"`
	Client string `json:"client"`
	Error  string `json:"error"`
	// Answer is the generated text of the last answer-mode node (empty when
	// the chain only has search nodes).
	Answer    string          `json:"answer,omitempty"`
	CreatedAt string          `json:"created_at"`
	Merge     *MergeStats     `json:"merge,omitempty"`
	Results   []RequestResult `json:"results,omitempty"`
	Steps     []*RequestStep  `json:"steps,omitempty"`
}

// RequestStep is one executed (or skipped) chain node attempt outcome.
type RequestStep struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Idx       int    `json:"idx"`
	NodeKey   string `json:"node_key"`
	Provider  string `json:"provider"`
	Engine    string `json:"engine"`
	AttemptNo int    `json:"attempt_no"`
	// Stage names one upstream call of a multi-call node (pubmed esearch|
	// esummary|efetch); empty for regular single-call attempts.
	Stage        string `json:"stage,omitempty"`
	Status       string `json:"status"` // running|ok|empty|fail|skip
	Kind         string `json:"kind"`
	HTTPStatus   int    `json:"http_status"`
	Error        string `json:"error"`
	Permanent    bool   `json:"permanent"`
	ResultsCount int    `json:"results_count"`
	WaitBeforeMS int    `json:"wait_before_ms"`
	TookMS       int    `json:"took_ms"`
	// Cost is the snapshot of the provider instance's configured per-request
	// cost at execution time (0 = not counted).
	Cost       float64 `json:"cost,omitempty"`
	StartedAt  string  `json:"started_at"`
	FinishedAt string  `json:"finished_at"`
}

// LogEntry is an in-DB log line surfaced in the UI.
type LogEntry struct {
	ID      int64  `json:"id"`
	TS      string `json:"ts"`
	Level   string `json:"level"`
	RID     string `json:"rid"`
	API     string `json:"api"`
	Message string `json:"message"`
}

// AuditEntry is one recorded administrative mutation (settings, providers,
// chains, keys, users, proxies, maintenance). Secrets are never stored in
// Details.
type AuditEntry struct {
	ID      string            `json:"id"`
	TS      string            `json:"ts"`
	UserID  string            `json:"user_id,omitempty"`
	User    string            `json:"user,omitempty"`
	Via     string            `json:"via,omitempty"` // key|session|disabled
	IP      string            `json:"ip,omitempty"`
	Action  string            `json:"action"`
	Target  string            `json:"target,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// AuditFilter narrows an audit listing.
type AuditFilter struct {
	Action string
	Actor  string
	From   string
	To     string
	Query  string
}

// DailyStat is a per-day rollup for the dashboard.
type DailyStat struct {
	Date     string `json:"date"`
	Requests int    `json:"requests"`
	OK       int    `json:"ok"`
	Empty    int    `json:"empty"`
	Fail     int    `json:"fail"`
	AvgMS    int    `json:"avg_ms"`
}

// StatsSummary is the dashboard summary payload.
type StatsSummary struct {
	RequestsToday int         `json:"requests_today"`
	SuccessRate   float64     `json:"success_rate"`
	AvgMS         int         `json:"avg_ms"`
	ErrorsToday   int         `json:"errors_today"`
	TotalRequests int         `json:"total_requests"`
	Daily         []DailyStat `json:"daily"`
	Recent        []*Request  `json:"recent"`
}

// AnonymousUserID is the pseudo identifier used by the analytics API for
// requests that were made without a user (auth disabled, or an anonymous
// caller). It never matches a real user id (those are UUIDs).
const AnonymousUserID = "anonymous"

// AnalyticsFilter narrows an analytics report. From and To are inclusive
// calendar dates (YYYY-MM-DD) interpreted in UTC; empty values fall back to the
// last 30 days. UserID narrows the report to one owner: a user id, or
// AnonymousUserID for requests without a user; empty means no filtering.
type AnalyticsFilter struct {
	From     string
	To       string
	Provider string
	Status   string
	UserID   string
}

// ProviderStat aggregates request outcomes for one provider over a period.
// Counts are derived from executed request steps, so a provider that was called
// several times within one request contributes one entry per call.
type ProviderStat struct {
	Provider    string  `json:"provider"`
	Total       int     `json:"total"`
	OK          int     `json:"ok"`
	Empty       int     `json:"empty"`
	Fail        int     `json:"fail"`
	Skip        int     `json:"skip"`
	AvgMS       int     `json:"avg_ms"`
	SuccessRate float64 `json:"success_rate"`
	// Cost is the summed per-request cost of executed attempts (0 when no
	// provider in the group had a cost configured).
	Cost float64 `json:"cost,omitempty"`
}

// HourStat holds request counts for one hour of the day (0-23, UTC).
type HourStat struct {
	Hour     int `json:"hour"`
	Requests int `json:"requests"`
	OK       int `json:"ok"`
	Empty    int `json:"empty"`
	Fail     int `json:"fail"`
}

// TopQuery is a frequently requested query within the selected period.
type TopQuery struct {
	Query string `json:"query"`
	Count int    `json:"count"`
	OK    int    `json:"ok"`
	Fail  int    `json:"fail"`
}

// AnalyticsReport aggregates request statistics over a date range.
type AnalyticsReport struct {
	TotalRequests int         `json:"total_requests"`
	OK            int         `json:"ok"`
	Empty         int         `json:"empty"`
	Fail          int         `json:"fail"`
	SuccessRate   float64     `json:"success_rate"`
	AvgMS         int         `json:"avg_ms"`
	P95MS         int         `json:"p95_ms"`
	ActiveDays    int         `json:"active_days"`
	BusiestDate   string      `json:"busiest_date"`
	BusiestCount  int         `json:"busiest_count"`
	Daily         []DailyStat `json:"daily"`
	Hourly        []HourStat  `json:"hourly"`
	TopQueries    []TopQuery  `json:"top_queries"`
	// CostTotal is the summed per-request cost of every executed attempt in the
	// selected range; CostConfigured is set by the HTTP layer and reports whether
	// any provider has a cost configured (so the UI can hide cost when unused).
	CostTotal      float64 `json:"cost_total,omitempty"`
	CostConfigured bool    `json:"cost_configured"`
	// Providers is consumed by the HTTP layer to build the enriched provider
	// table; it is not serialised inside the report to avoid duplication.
	Providers []ProviderStat `json:"-"`
}

// AnalyticsProvider is a provider stat enriched with configuration metadata and
// returned by the analytics endpoint (the store only knows display names).
type AnalyticsProvider struct {
	Provider    string  `json:"provider"`
	Code        string  `json:"code"`
	Enabled     bool    `json:"enabled"`
	Configured  bool    `json:"configured"`
	Total       int     `json:"total"`
	OK          int     `json:"ok"`
	Empty       int     `json:"empty"`
	Fail        int     `json:"fail"`
	Skip        int     `json:"skip"`
	AvgMS       int     `json:"avg_ms"`
	SuccessRate float64 `json:"success_rate"`
	Cost        float64 `json:"cost,omitempty"`
	// CostConfigured reports whether this provider has a cost configured (or
	// recorded), so the UI can show "—" instead of a misleading 0.
	CostConfigured bool `json:"cost_configured"`
}

// UserStat aggregates request outcomes for one owner over a period; it backs
// the per-user analytics leaderboard. UserID is a real user id or
// AnonymousUserID for requests made without a user.
type UserStat struct {
	UserID     string `json:"user_id"`
	Requests   int    `json:"requests"`
	OK         int    `json:"ok"`
	Empty      int    `json:"empty"`
	Fail       int    `json:"fail"`
	AvgMS      int    `json:"avg_ms"`
	LastActive string `json:"last_active,omitempty"`
	// Cost is the summed per-request cost of the owner's executed attempts
	// (0 when no provider has a cost configured).
	Cost float64 `json:"cost,omitempty"`
}
