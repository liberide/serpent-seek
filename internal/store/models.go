// Package store defines the persistence interface and domain models.
package store

// Timestamps are stored as RFC3339 strings in UTC to keep SQLite and
// PostgreSQL behaviour identical without driver-specific time handling.

// User is an account that can own API keys and passkeys.
type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"` // admin|viewer
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at"`
}

// APIKey is a hashed access token. The plaintext is never stored.
type APIKey struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name,omitempty"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Hash       string `json:"-"`
	Scopes     string `json:"scopes"`
	LastUsedAt string `json:"last_used_at"`
	ExpiresAt  string `json:"expires_at"`
	RevokedAt  string `json:"revoked_at"`
	CreatedAt  string `json:"created_at"`
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
	ID          string            `json:"id"`
	Code        string            `json:"code"` // driver type registered in the provider registry
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	BaseURL     string            `json:"base_url"`
	Credentials map[string]string `json:"-"`
	Params      map[string]string `json:"params"`
	UpdatedAt   string            `json:"updated_at"`
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
}

// Chain node modes. A search node is a Row source; an answer node produces a
// generated answer (Result.Answer) and is excluded from the Row merge.
const (
	NodeModeSearch = "search"
	NodeModeAnswer = "answer"
)

// ChainNode is a single provider block in a chain.
type ChainNode struct {
	ID           string            `json:"id"`
	ChainID      string            `json:"chain_id"`
	Key          string            `json:"key"`
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
	IsStart      bool              `json:"is_start"`     // exactly one node per chain is the entry point
	PosX         float64           `json:"pos_x"`
	PosY         float64           `json:"pos_y"`
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
	Client        string `json:"client"`
	Error         string `json:"error"`
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
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
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
