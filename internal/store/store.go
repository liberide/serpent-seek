package store

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned on unique constraint violations.
var ErrConflict = errors.New("conflict")

// RequestFilter narrows a history listing.
type RequestFilter struct {
	Status   string
	Provider string
	From     string
	To       string
	Query    string
}

// LogFilter narrows a log listing.
type LogFilter struct {
	Level string
	Query string
}

// Page describes limit/offset pagination.
type Page struct {
	Limit  int
	Offset int
}

// Storage is the persistence contract implemented by the sqlite and postgres
// drivers. All methods are safe for concurrent use.
type Storage interface {
	// Lifecycle
	Ping(ctx context.Context) error
	Close() error
	Driver() string
	Migrate(ctx context.Context) error

	// Settings
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	AllSettings(ctx context.Context) (map[string]string, error)

	// Users
	CreateUser(ctx context.Context, u *User) error
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByName(ctx context.Context, name string) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)
	UpdateUser(ctx context.Context, u *User) error
	DeleteUser(ctx context.Context, id string) error
	CountAdmins(ctx context.Context) (int, error)

	// API keys
	CreateAPIKey(ctx context.Context, k *APIKey) error
	GetAPIKeyByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	ListAPIKeys(ctx context.Context, userID string) ([]*APIKey, error)
	UpdateAPIKey(ctx context.Context, k *APIKey) error
	DeleteAPIKey(ctx context.Context, id string) error

	// Passkeys
	CreatePasskey(ctx context.Context, p *Passkey) error
	GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*Passkey, error)
	ListPasskeys(ctx context.Context, userID string) ([]*Passkey, error)
	UpdatePasskey(ctx context.Context, p *Passkey) error
	DeletePasskey(ctx context.Context, id string) error

	// Sessions
	CreateSession(ctx context.Context, s *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	UpdateSessionExpiry(ctx context.Context, id, expiresAt string) error
	DeleteSession(ctx context.Context, id string) error
	DeleteUserSessions(ctx context.Context, userID string) error
	DeleteExpiredSessions(ctx context.Context, now string) (int64, error)

	// Providers (instances; id is the identity, code is the driver type)
	ListProviders(ctx context.Context) ([]*Provider, error)
	GetProvider(ctx context.Context, id string) (*Provider, error)
	UpsertProvider(ctx context.Context, p *Provider) error
	UpdateProviderBaseURL(ctx context.Context, id, baseURL string) error
	SetProviderEnabled(ctx context.Context, id string, enabled bool) error
	DeleteProvider(ctx context.Context, id string) error

	// Chains
	ListChains(ctx context.Context) ([]*Chain, error)
	GetChain(ctx context.Context, id string) (*Chain, error)
	GetActiveChain(ctx context.Context) (*Chain, error)
	SaveChain(ctx context.Context, c *Chain) error
	DeleteChain(ctx context.Context, id string) error
	ActivateChain(ctx context.Context, id string) error

	// Requests and steps
	CreateRequest(ctx context.Context, r *Request) error
	UpdateRequest(ctx context.Context, r *Request) error
	GetRequest(ctx context.Context, id string) (*Request, error)
	GetRequestByRID(ctx context.Context, rid string) (*Request, error)
	ListRequests(ctx context.Context, f RequestFilter, p Page) ([]*Request, int, error)
	AddStep(ctx context.Context, s *RequestStep) error
	ListSteps(ctx context.Context, requestID string) ([]*RequestStep, error)
	ClearHistory(ctx context.Context) (int64, error)
	DeleteOldRequests(ctx context.Context, before string) (int64, error)

	// Logs
	AddLog(ctx context.Context, e *LogEntry) error
	ListLogs(ctx context.Context, f LogFilter, p Page) ([]*LogEntry, int, error)
	ClearLogs(ctx context.Context) (int64, error)
	DeleteOldLogs(ctx context.Context, before string) (int64, error)

	// Stats
	Summary(ctx context.Context, days int) (*StatsSummary, error)
	RecomputeDailyStats(ctx context.Context, date string) error

	// Maintenance
	Vacuum(ctx context.Context) error
}
