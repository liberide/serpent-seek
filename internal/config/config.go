// Package config loads environment configuration and resolves UI-editable
// settings with environment variables taking priority.
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// DefaultUserAgent mirrors the serpent-shim v11 browser signature. A generic
// (non-browser) UA is rejected by Cloudflare Browser Integrity Check with
// HTTP 403 "error code: 1010".
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

// DefaultUpdateCheckURL is the official release feed. Any other value means the
// instance checks a custom (unofficial) source and the UI marks it as such.
const DefaultUpdateCheckURL = "https://api.github.com/repos/liberide/serpent-seek/releases/latest"

// KV is the subset of the storage layer used to persist settings.
type KV interface {
	GetSetting(ctx context.Context, key string) (string, error)
	AllSettings(ctx context.Context) (map[string]string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Config is the immutable process configuration built from environment variables.
type Config struct {
	// Network
	Port         int
	Host         string
	PublicOrigin string
	RPID         string
	RPName       string

	// TrustedProxies lists proxy IPs/CIDRs whose forwarded headers may be trusted.
	TrustedProxies []string
	// BlockPrivateNetworks rejects outbound connections to non-public addresses.
	BlockPrivateNetworks bool

	// Storage
	DataDir       string
	SQLitePath    string
	StorageDriver string
	DatabaseURL   string
	EncryptionKey string

	// Behavior
	UserAgent      string
	MaxConcurrency int
	MaxAttempts    int
	LogLevel       string
	LogFormat      string
	CleanupCron    string
	VacuumCron     string
	SetupToken     string

	// Defaults for UI-editable settings
	DefaultAuthEnabled         bool
	DefaultHistoryRetention    int
	DefaultLogsRetention       int
	DefaultUsageRetention      int
	DefaultAuditRetention      int
	DefaultRetryHTTPCodes      []int
	DefaultAPRetryCodes        []string
	DefaultAPISerpentAPIKey    string
	DefaultSerpBaseAPIKey      string
	DefaultSearxngURL          string
	DefaultSearxngAPIKey       string
	DefaultSearxngLanguage     string
	DefaultYandexAPIKey        string
	DefaultYandexFolderID      string
	DefaultNCBIAPIKey          string
	DefaultGoogleAPIKey        string
	DefaultGoogleModel         string
	DefaultGoogleCX            string
	DefaultGoogleDriver        string
	DefaultHistoryCleanupRuns  bool
	DefaultProviderTimeoutMs   int
	DefaultTreatEmptyAsFailure bool
	DefaultUpdateCheck         bool
	DefaultUpdateCheckURL      string
	DefaultUpdateCheckInterval int
	DefaultWebhookURL          string
	DefaultSearchRateLimit     int
	DefaultSearchDailyQuota    int
	DefaultChainTimeout        int
	DefaultProviderFailures    int
	DefaultProviderCooldown    int
	DefaultDebug               bool
	DefaultDeveloperMode       bool

	// envSet records which keys came from the environment.
	envSet map[string]bool
}

// setting descriptor keys.
const (
	KeyAuthEnabled = "auth_enabled"
	// KeyHideAuthOffNotice hides the sidebar warning shown while
	// authentication is disabled.
	KeyHideAuthOffNotice = "hide_auth_off_notice"
	KeyHistoryRetention  = "history_retention_days"
	KeyLogsRetention     = "logs_retention_days"
	// KeyUsageRetention controls how long daily search-quota counters are kept.
	KeyUsageRetention = "usage_retention_days"
	// KeyAuditRetention controls how long the administrative audit trail is kept.
	KeyAuditRetention     = "audit_retention_days"
	KeyMaxConcurrency     = "max_concurrency"
	KeyMaxAttempts        = "max_attempts"
	KeyUserAgent          = "user_agent"
	KeyRetryHTTPCodes     = "retry_http_codes"
	KeyAPRetryCodes       = "ap_retry_codes"
	KeySearxngURL         = "searxng_url"
	KeySearxngAPIKey      = "searxng_api_key"
	KeySearxngLanguage    = "searxng_language"
	KeyYandexAPIKey       = "yandex_api_key"
	KeyYandexFolderID     = "yandex_folder_id"
	KeyGoogleAPIKey       = "google_api_key"
	KeyGoogleModel        = "google_model"
	KeyGoogleCX           = "google_cx"
	KeyGoogleDriver       = "google_driver"
	KeyPublicOrigin       = "public_origin"
	KeyRPID               = "rp_id"
	KeyRPName             = "rp_name"
	KeyLogLevel           = "log_level"
	KeyStorageDriver      = "storage_driver"
	KeyDatabaseURL        = "database_url"
	KeyTreatEmptyAsFail   = "treat_empty_as_fail"
	KeyIgnoreRequestCount = "ignore_request_count"
	// KeyAllowSparseSources lets answer-node sources (link+title, often no
	// snippet) be mixed into the Row result as sparse rows.
	KeyAllowSparseSources = "allow_sparse_sources"
	// KeySearchRateLimit caps search requests per minute per API key (or per
	// user/IP when no key is used). 0 disables the limit.
	KeySearchRateLimit = "search_rate_limit_per_minute"
	// KeySearchDailyQuota caps search requests per UTC day per API key (or per
	// user/IP). 0 means unlimited.
	KeySearchDailyQuota = "search_daily_quota"
	// KeyChainTimeout caps the total wall-clock time of one search (0 disables).
	KeyChainTimeout = "chain_timeout_seconds"
	// KeyProviderFailures is how many consecutive provider failures trip the
	// circuit breaker (0 disables it).
	KeyProviderFailures = "provider_failure_threshold"
	// KeyProviderCooldown is how many seconds a tripped provider is skipped.
	KeyProviderCooldown = "provider_cooldown_seconds"
	// KeyDebug enables the pprof debug endpoints under /debug/pprof.
	KeyDebug = "debug"
	// KeyDeveloperMode enables developer mode: admins may mint dedicated
	// developer tokens (seek_dev_…) that carry full administrative scope for
	// third-party/programmatic integrations. When disabled every developer
	// token is rejected at authentication time, without revoking it.
	KeyDeveloperMode = "developer_mode"
	// KeyUpdateCheck enables the outbound "newer release available" check.
	KeyUpdateCheck = "update_check_enabled"
	// KeyUpdateCheckURL is the release API endpoint (GitHub "latest release" by
	// default); point it elsewhere or clear it to disable.
	KeyUpdateCheckURL = "update_check_url"
	// KeyUpdateCheckInterval is how many hours between automatic checks.
	KeyUpdateCheckInterval = "update_check_interval_hours"
	// KeyWebhookURL is an optional endpoint that receives a JSON POST for
	// operational events (provider circuit-breaker trips, quota exhaustion).
	// Empty disables outbound notifications.
	KeyWebhookURL = "webhook_url"
	// KeySetupToken persists the one-time first-run setup token across restarts
	// so a restart never invalidates the token shown in the logs. It is not a
	// UI-editable setting.
	KeySetupToken = "setup_token"
)

// SettingDef describes a UI-editable setting.
type SettingDef struct {
	Key      string `json:"key"`
	Env      string `json:"env"`
	Type     string `json:"type"` // string|int|bool|csv|secret
	Default  string `json:"default"`
	EnvOnly  bool   `json:"env_only"`
	Secret   bool   `json:"secret"`
	Group    string `json:"group"`
	Required bool   `json:"required"`
	// Hidden keeps a setting out of the generic Settings form. It still has an
	// environment binding and a default, but it is managed through a dedicated
	// UI section/endpoint instead (e.g. developer_mode).
	Hidden bool `json:"hidden,omitempty"`
}

// envBindings maps a settings key to its environment variable, type and secret flag.
var envBindings = []SettingDef{
	{Key: KeyAuthEnabled, Env: "AUTH_ENABLED", Type: "bool", Default: "true", Group: "auth"},
	{Key: KeyHideAuthOffNotice, Env: "HIDE_AUTH_OFF_NOTICE", Type: "bool", Default: "false", Group: "auth"},
	{Key: KeyHistoryRetention, Env: "HISTORY_RETENTION_DAYS", Type: "int", Default: "30", Group: "retention"},
	{Key: KeyLogsRetention, Env: "LOGS_RETENTION_DAYS", Type: "int", Default: "14", Group: "retention"},
	{Key: KeyUsageRetention, Env: "USAGE_RETENTION_DAYS", Type: "int", Default: "90", Group: "retention"},
	{Key: KeyAuditRetention, Env: "AUDIT_RETENTION_DAYS", Type: "int", Default: "180", Group: "retention"},
	{Key: KeyMaxConcurrency, Env: "MAX_CONCURRENCY", Type: "int", Default: "32", Group: "network"},
	{Key: KeyMaxAttempts, Env: "MAX_ATTEMPTS", Type: "int", Default: "10", Group: "network"},
	{Key: KeyUserAgent, Env: "USER_AGENT", Type: "string", Default: DefaultUserAgent, Group: "network"},
	{Key: KeyRetryHTTPCodes, Env: "RETRY_HTTP_CODES", Type: "csv", Default: "429,500,502,503,504", Group: "providers"},
	{Key: KeyAPRetryCodes, Env: "AP_RETRY_CODES", Type: "csv", Default: "", Group: "providers"},
	{Key: KeyPublicOrigin, Env: "PUBLIC_ORIGIN", Type: "string", Default: "", Group: "auth"},
	{Key: KeyRPID, Env: "RP_ID", Type: "string", Default: "", Group: "auth"},
	{Key: KeyRPName, Env: "RP_NAME", Type: "string", Default: "SerpentSeek", Group: "auth"},
	{Key: KeyLogLevel, Env: "LOG_LEVEL", Type: "string", Default: "info", Group: "general"},
	{Key: KeyStorageDriver, Env: "STORAGE_DRIVER", Type: "string", Default: "sqlite", Group: "storage", EnvOnly: true},
	{Key: KeyDatabaseURL, Env: "DATABASE_URL", Type: "string", Default: "", Group: "storage", EnvOnly: true, Secret: true},
	{Key: KeyTreatEmptyAsFail, Env: "TREAT_EMPTY_AS_FAIL", Type: "bool", Default: "true", Group: "network"},
	{Key: KeyIgnoreRequestCount, Env: "IGNORE_REQUEST_COUNT", Type: "bool", Default: "false", Group: "network"},
	{Key: KeyAllowSparseSources, Env: "ALLOW_SPARSE_SOURCES", Type: "bool", Default: "false", Group: "network"},
	{Key: KeySearchRateLimit, Env: "SEARCH_RATE_LIMIT_PER_MINUTE", Type: "int", Default: "0", Group: "network"},
	{Key: KeySearchDailyQuota, Env: "SEARCH_DAILY_QUOTA", Type: "int", Default: "0", Group: "network"},
	{Key: KeyChainTimeout, Env: "CHAIN_TIMEOUT", Type: "int", Default: "0", Group: "network"},
	{Key: KeyProviderFailures, Env: "PROVIDER_FAILURE_THRESHOLD", Type: "int", Default: "0", Group: "network"},
	{Key: KeyProviderCooldown, Env: "PROVIDER_COOLDOWN_SECONDS", Type: "int", Default: "60", Group: "network"},
	{Key: KeyDebug, Env: "DEBUG", Type: "bool", Default: "false", Group: "general"},
	{Key: KeyDeveloperMode, Env: "DEVELOPER_MODE", Type: "bool", Default: "false", Group: "auth", Hidden: true},
	{Key: KeyUpdateCheck, Env: "UPDATE_CHECK_ENABLED", Type: "bool", Default: "true", Group: "general"},
	{Key: KeyUpdateCheckURL, Env: "UPDATE_CHECK_URL", Type: "string", Default: DefaultUpdateCheckURL, Group: "general", EnvOnly: true},
	{Key: KeyUpdateCheckInterval, Env: "UPDATE_CHECK_INTERVAL_HOURS", Type: "int", Default: "24", Group: "general"},
	{Key: KeyWebhookURL, Env: "WEBHOOK_URL", Type: "string", Default: "", Group: "general"},
}

// Load reads the process configuration from the environment.
func Load() (*Config, error) {
	c := &Config{envSet: map[string]bool{}}

	c.Port = envInt("PORT", 8080)
	c.Host = envStr("HOST", "0.0.0.0")
	c.PublicOrigin = envStr("PUBLIC_ORIGIN", "")
	c.RPID = envStr("RP_ID", "")
	c.RPName = envStr("RP_NAME", "SerpentSeek")
	c.TrustedProxies = envList("TRUSTED_PROXIES")
	c.BlockPrivateNetworks = envBool("BLOCK_PRIVATE_NETWORKS", false)

	// Default is relative so it resolves to /app/data inside the container
	// (WORKDIR /app) and to ./data when running the binary locally.
	c.DataDir = envStr("DATA_DIR", "data")
	c.SQLitePath = envStr("SQLITE_PATH", "")
	if c.SQLitePath == "" {
		c.SQLitePath = filepath.Join(c.DataDir, "serpentseek.db")
	}
	// Resolve relative paths against the current working directory up front so
	// the startup log shows the exact file the service will create/use.
	if abs, err := filepath.Abs(c.DataDir); err == nil {
		c.DataDir = abs
	}
	if abs, err := filepath.Abs(c.SQLitePath); err == nil {
		c.SQLitePath = abs
	}
	c.StorageDriver = strings.ToLower(envStr("STORAGE_DRIVER", "sqlite"))
	c.DatabaseURL = envStr("DATABASE_URL", "")
	c.EncryptionKey = envStr("ENCRYPTION_KEY", "")

	c.UserAgent = envStr("USER_AGENT", DefaultUserAgent)
	c.MaxConcurrency = envInt("MAX_CONCURRENCY", 32)
	c.MaxAttempts = envInt("MAX_ATTEMPTS", 10)
	c.LogLevel = strings.ToLower(envStr("LOG_LEVEL", "info"))
	c.LogFormat = strings.ToLower(envStr("LOG_FORMAT", "text"))
	c.CleanupCron = envStr("HISTORY_CLEANUP_CRON", "0 4 * * *")
	c.VacuumCron = envStr("VACUUM_CRON", "0 3 * * 0")
	c.SetupToken = envStr("SETUP_TOKEN", "")

	c.DefaultAuthEnabled = envBool("AUTH_ENABLED", true)
	c.DefaultHistoryRetention = envInt("HISTORY_RETENTION_DAYS", 30)
	c.DefaultLogsRetention = envInt("LOGS_RETENTION_DAYS", 14)
	c.DefaultUsageRetention = envInt("USAGE_RETENTION_DAYS", 90)
	c.DefaultAuditRetention = envInt("AUDIT_RETENTION_DAYS", 180)
	c.DefaultRetryHTTPCodes = envIntList("RETRY_HTTP_CODES", []int{429, 500, 502, 503, 504})
	c.DefaultAPRetryCodes = envStrList("AP_RETRY_CODES")
	c.DefaultAPISerpentAPIKey = envStr("APISERPENT_API_KEY", envStr("SERPENT_API_KEY", ""))
	c.DefaultSerpBaseAPIKey = envStr("SERPBASE_API_KEY", "")
	// SearXNG is always an externally hosted instance; the URL is empty until configured.
	c.DefaultSearxngURL = envStr("SEARXNG_URL", "")
	c.DefaultSearxngAPIKey = envStr("SEARXNG_API_KEY", "")
	c.DefaultSearxngLanguage = envStr("SEARXNG_LANGUAGE", "")
	c.DefaultYandexAPIKey = envStr("YANDEX_API_KEY", "")
	c.DefaultYandexFolderID = envStr("YANDEX_FOLDER_ID", "")
	c.DefaultNCBIAPIKey = envStr("NCBI_API_KEY", "")
	c.DefaultGoogleAPIKey = envStr("GOOGLE_API_KEY", "")
	c.DefaultGoogleModel = envStr("GOOGLE_MODEL", "gemini-3.8-flash")
	c.DefaultGoogleCX = envStr("GOOGLE_CX", "")
	c.DefaultGoogleDriver = strings.ToLower(envStr("GOOGLE_DRIVER", "vertex"))
	c.DefaultTreatEmptyAsFailure = envBool("TREAT_EMPTY_AS_FAIL", true)
	c.DefaultSearchRateLimit = envInt("SEARCH_RATE_LIMIT_PER_MINUTE", 0)
	c.DefaultSearchDailyQuota = envInt("SEARCH_DAILY_QUOTA", 0)
	c.DefaultChainTimeout = envInt("CHAIN_TIMEOUT", 0)
	c.DefaultProviderFailures = envInt("PROVIDER_FAILURE_THRESHOLD", 0)
	c.DefaultProviderCooldown = envInt("PROVIDER_COOLDOWN_SECONDS", 60)
	c.DefaultDebug = envBool("DEBUG", false)
	c.DefaultDeveloperMode = envBool("DEVELOPER_MODE", false)
	c.DefaultUpdateCheck = envBool("UPDATE_CHECK_ENABLED", true)
	c.DefaultUpdateCheckURL = envStr("UPDATE_CHECK_URL", DefaultUpdateCheckURL)
	c.DefaultUpdateCheckInterval = envInt("UPDATE_CHECK_INTERVAL_HOURS", 24)
	c.DefaultWebhookURL = envStr("WEBHOOK_URL", "")

	// Record which settings are provided by the environment.
	for _, b := range envBindings {
		if _, ok := os.LookupEnv(b.Env); ok {
			c.envSet[b.Key] = true
		}
	}
	if c.StorageDriver != "sqlite" && c.StorageDriver != "postgres" {
		return nil, fmt.Errorf("invalid STORAGE_DRIVER %q: must be sqlite or postgres", c.StorageDriver)
	}
	if c.StorageDriver == "postgres" && c.DatabaseURL == "" {
		return nil, fmt.Errorf("STORAGE_DRIVER=postgres requires DATABASE_URL")
	}
	if c.MaxConcurrency < 0 {
		c.MaxConcurrency = 0
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = 1
	}
	return c, nil
}

// IsEnv reports whether a settings key is pinned by the environment.
func (c *Config) IsEnv(key string) bool { return c.envSet[key] }

// EnvKeys returns all keys pinned by the environment.
func (c *Config) EnvKeys() map[string]bool {
	out := make(map[string]bool, len(c.envSet))
	for k, v := range c.envSet {
		out[k] = v
	}
	return out
}

// Descriptors returns the UI setting descriptors.
func (c *Config) Descriptors() []SettingDef {
	out := make([]SettingDef, len(envBindings))
	copy(out, envBindings)
	return out
}

// DefaultFor returns the compiled-in default for a settings key.
func (c *Config) DefaultFor(key string) string {
	for _, b := range envBindings {
		if b.Key == key {
			return b.Default
		}
	}
	return ""
}

// Manager resolves settings from environment defaults plus DB overrides. The
// environment always wins and such keys are reported as readonly to the UI.
type Manager struct {
	cfg    *Config
	kv     KV
	mu     sync.RWMutex
	cache  map[string]string
	loaded bool
}

// NewManager creates a settings manager.
func NewManager(cfg *Config, kv KV) *Manager {
	return &Manager{cfg: cfg, kv: kv}
}

// Refresh reloads DB settings into the cache.
func (m *Manager) Refresh(ctx context.Context) error {
	all, err := m.kv.AllSettings(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.cache = all
	m.loaded = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) lookup(ctx context.Context, key string) string {
	if m.cfg.IsEnv(key) && !m.authEnvOverridable(key) {
		return defaultResolved(m.cfg, key)
	}
	m.mu.RLock()
	if m.loaded {
		if v, ok := m.cache[key]; ok {
			m.mu.RUnlock()
			return v
		}
	}
	m.mu.RUnlock()
	// Fall back to a direct DB read (covers changes made by other replicas).
	if v, err := m.kv.GetSetting(ctx, key); err == nil && v != "" {
		return v
	}
	return defaultResolved(m.cfg, key)
}

func defaultResolved(cfg *Config, key string) string {
	switch key {
	case KeyAuthEnabled:
		return strconv.FormatBool(cfg.DefaultAuthEnabled)
	case KeyHistoryRetention:
		return strconv.Itoa(cfg.DefaultHistoryRetention)
	case KeyLogsRetention:
		return strconv.Itoa(cfg.DefaultLogsRetention)
	case KeyUsageRetention:
		return strconv.Itoa(cfg.DefaultUsageRetention)
	case KeyAuditRetention:
		return strconv.Itoa(cfg.DefaultAuditRetention)
	case KeyMaxConcurrency:
		return strconv.Itoa(cfg.MaxConcurrency)
	case KeyMaxAttempts:
		return strconv.Itoa(cfg.MaxAttempts)
	case KeyRetryHTTPCodes:
		return joinIntCSV(cfg.DefaultRetryHTTPCodes)
	case KeyAPRetryCodes:
		return strings.Join(cfg.DefaultAPRetryCodes, ",")
	case KeySearchRateLimit:
		return strconv.Itoa(cfg.DefaultSearchRateLimit)
	case KeySearchDailyQuota:
		return strconv.Itoa(cfg.DefaultSearchDailyQuota)
	case KeyChainTimeout:
		return strconv.Itoa(cfg.DefaultChainTimeout)
	case KeyProviderFailures:
		return strconv.Itoa(cfg.DefaultProviderFailures)
	case KeyProviderCooldown:
		return strconv.Itoa(cfg.DefaultProviderCooldown)
	case KeyDebug:
		return strconv.FormatBool(cfg.DefaultDebug)
	case KeyDeveloperMode:
		return strconv.FormatBool(cfg.DefaultDeveloperMode)
	case KeyUpdateCheck:
		return strconv.FormatBool(cfg.DefaultUpdateCheck)
	case KeyUpdateCheckURL:
		return cfg.DefaultUpdateCheckURL
	case KeyUpdateCheckInterval:
		return strconv.Itoa(cfg.DefaultUpdateCheckInterval)
	case KeyWebhookURL:
		return cfg.DefaultWebhookURL
	}
	for _, b := range envBindings {
		if b.Key == key {
			return b.Default
		}
	}
	return ""
}

// joinIntCSV renders an integer list as a comma separated string.
func joinIntCSV(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// GetString returns the effective string value of a setting.
func (m *Manager) GetString(ctx context.Context, key string) string {
	return m.lookup(ctx, key)
}

// GetInt returns the effective integer value of a setting.
func (m *Manager) GetInt(ctx context.Context, key string) int {
	v, err := strconv.Atoi(strings.TrimSpace(m.lookup(ctx, key)))
	if err != nil {
		d, _ := strconv.Atoi(defaultResolved(m.cfg, key))
		return d
	}
	return v
}

// GetBool returns the effective boolean value of a setting.
func (m *Manager) GetBool(ctx context.Context, key string) bool {
	v := strings.ToLower(strings.TrimSpace(m.lookup(ctx, key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// GetCSV returns a generic comma separated list.
func (m *Manager) GetCSV(ctx context.Context, key string) []string {
	raw := m.lookup(ctx, key)
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// GetIntCSV returns a comma separated list of integers.
func (m *Manager) GetIntCSV(ctx context.Context, key string) []int {
	var out []int
	for _, p := range m.GetCSV(ctx, key) {
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// authEnvOverridable reports whether an environment-provided AUTH_ENABLED=false
// may be overridden from the UI. The environment may force authentication ON,
// but it must never make it impossible to turn authentication back on.
func (m *Manager) authEnvOverridable(key string) bool {
	return key == KeyAuthEnabled && !m.cfg.DefaultAuthEnabled
}

// Set persists a value unless the key is pinned by the environment.
func (m *Manager) Set(ctx context.Context, key, value string) error {
	if m.cfg.IsEnv(key) && !m.authEnvOverridable(key) {
		return fmt.Errorf("setting %q is pinned by environment variable and is read-only", key)
	}
	if err := m.kv.SetSetting(ctx, key, value); err != nil {
		return err
	}
	m.mu.Lock()
	if m.cache == nil {
		m.cache = map[string]string{}
	}
	m.cache[key] = value
	m.loaded = true
	m.mu.Unlock()
	return nil
}

// Config returns the base process configuration.
func (m *Manager) Config() *Config { return m.cfg }

// Snapshot returns the effective values of all settings plus env markers.
func (m *Manager) Snapshot(ctx context.Context) (map[string]string, map[string]bool) {
	values := make(map[string]string, len(envBindings))
	for _, b := range envBindings {
		values[b.Key] = m.lookup(ctx, b.Key)
	}
	env := m.cfg.EnvKeys()
	// A false AUTH_ENABLED is only a default: it is not a read-only pin, so it
	// must not be flagged as an environment-managed value in the UI.
	if m.authEnvOverridable(KeyAuthEnabled) {
		delete(env, KeyAuthEnabled)
	}
	return values, env
}

// --- env helpers ---

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off", "":
			return false
		}
	}
	return def
}

func envIntList(key string, def []int) []int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	var out []int
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func envStrList(key string) []string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}

// envList splits a comma separated environment value without changing case.
func envList(key string) []string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
