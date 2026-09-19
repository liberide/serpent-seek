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

	// envSet records which keys came from the environment.
	envSet map[string]bool
}

// setting descriptor keys.
const (
	KeyAuthEnabled = "auth_enabled"
	// KeyHideAuthOffNotice hides the sidebar warning shown while
	// authentication is disabled.
	KeyHideAuthOffNotice  = "hide_auth_off_notice"
	KeyHistoryRetention   = "history_retention_days"
	KeyLogsRetention      = "logs_retention_days"
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
}

// envBindings maps a settings key to its environment variable, type and secret flag.
var envBindings = []SettingDef{
	{Key: KeyAuthEnabled, Env: "AUTH_ENABLED", Type: "bool", Default: "true", Group: "auth"},
	{Key: KeyHideAuthOffNotice, Env: "HIDE_AUTH_OFF_NOTICE", Type: "bool", Default: "false", Group: "auth"},
	{Key: KeyHistoryRetention, Env: "HISTORY_RETENTION_DAYS", Type: "int", Default: "30", Group: "retention"},
	{Key: KeyLogsRetention, Env: "LOGS_RETENTION_DAYS", Type: "int", Default: "14", Group: "retention"},
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
}

// Load reads the process configuration from the environment.
func Load() (*Config, error) {
	c := &Config{envSet: map[string]bool{}}

	c.Port = envInt("PORT", 8080)
	c.Host = envStr("HOST", "0.0.0.0")
	c.PublicOrigin = envStr("PUBLIC_ORIGIN", "")
	c.RPID = envStr("RP_ID", "")
	c.RPName = envStr("RP_NAME", "SerpentSeek")

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
	c.DefaultGoogleModel = envStr("GOOGLE_MODEL", "gemini-2.0-flash")
	c.DefaultGoogleCX = envStr("GOOGLE_CX", "")
	c.DefaultGoogleDriver = strings.ToLower(envStr("GOOGLE_DRIVER", "vertex"))
	c.DefaultTreatEmptyAsFailure = envBool("TREAT_EMPTY_AS_FAIL", true)

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
	case KeyMaxConcurrency:
		return strconv.Itoa(cfg.MaxConcurrency)
	case KeyMaxAttempts:
		return strconv.Itoa(cfg.MaxAttempts)
	case KeyRetryHTTPCodes:
		return joinIntCSV(cfg.DefaultRetryHTTPCodes)
	case KeyAPRetryCodes:
		return strings.Join(cfg.DefaultAPRetryCodes, ",")
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
