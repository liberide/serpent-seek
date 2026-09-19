// Package sqlstore provides a single SQL implementation of store.Storage that
// works over both SQLite and PostgreSQL by rebinding placeholders and using a
// portable Bool scanner.
package sqlstore

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/liberide/serpent-seek/internal/store"
)

// Store implements store.Storage over database/sql.
type Store struct {
	db        *sql.DB
	driver    string
	migrateFn func(context.Context) error
}

// New wraps an open *sql.DB. driver is "sqlite" or "postgres".
func New(db *sql.DB, driver string) *Store {
	return &Store{db: db, driver: driver}
}

// SetMigrate registers the driver-specific migration function.
func (s *Store) SetMigrate(fn func(context.Context) error) { s.migrateFn = fn }

// DB exposes the underlying handle (used for goose migrations).
func (s *Store) DB() *sql.DB { return s.db }

// Driver returns the storage driver name.
func (s *Store) Driver() string { return s.driver }

// Ping verifies connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Close releases the connection pool.
func (s *Store) Close() error { return s.db.Close() }

// Migrate applies pending migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if s.migrateFn == nil {
		return nil
	}
	return s.migrateFn(ctx)
}

// rebind converts '?' placeholders to '$N' for PostgreSQL.
func (s *Store) rebind(query string) string {
	if s.driver != "postgres" {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *Store) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.rebind(q), args...)
}

func (s *Store) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.rebind(q), args...)
}

func (s *Store) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.rebind(q), args...)
}

// --- Settings ---

// GetSetting returns a settings value.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.queryRow(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", store.ErrNotFound
	}
	return value, err
}

// SetSetting upserts a settings value.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.exec(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, store.Now())
	return err
}

// AllSettings returns every settings key/value.
func (s *Store) AllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// --- Users ---

// CreateUser inserts a user.
func (s *Store) CreateUser(ctx context.Context, u *store.User) error {
	_, err := s.exec(ctx, `INSERT INTO users (id, name, role, disabled, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Name, u.Role, store.Bool(u.Disabled), u.CreatedAt)
	return err
}

func scanUser(row interface{ Scan(...any) error }) (*store.User, error) {
	var u store.User
	var disabled store.Bool
	if err := row.Scan(&u.ID, &u.Name, &u.Role, &disabled, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.Disabled = bool(disabled)
	return &u, nil
}

// GetUser fetches a user by id.
func (s *Store) GetUser(ctx context.Context, id string) (*store.User, error) {
	u, err := scanUser(s.queryRow(ctx, `SELECT id, name, role, disabled, created_at FROM users WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return u, err
}

// GetUserByName fetches a user by unique name.
func (s *Store) GetUserByName(ctx context.Context, name string) (*store.User, error) {
	u, err := scanUser(s.queryRow(ctx, `SELECT id, name, role, disabled, created_at FROM users WHERE name = ?`, name))
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return u, err
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]*store.User, error) {
	rows, err := s.query(ctx, `SELECT id, name, role, disabled, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUser updates mutable user fields.
func (s *Store) UpdateUser(ctx context.Context, u *store.User) error {
	_, err := s.exec(ctx, `UPDATE users SET name = ?, role = ?, disabled = ? WHERE id = ?`,
		u.Name, u.Role, store.Bool(u.Disabled), u.ID)
	return err
}

// DeleteUser removes a user (cascading to keys, passkeys and sessions).
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// CountAdmins returns the number of enabled admins.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = ?`, store.Bool(false)).Scan(&n)
	return n, err
}

// --- API keys ---

// CreateAPIKey inserts a hashed key.
func (s *Store) CreateAPIKey(ctx context.Context, k *store.APIKey) error {
	_, err := s.exec(ctx,
		`INSERT INTO api_keys (id, user_id, name, prefix, hash, scopes, last_used_at, expires_at, revoked_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.UserID, k.Name, k.Prefix, k.Hash, k.Scopes, k.LastUsedAt, k.ExpiresAt, k.RevokedAt, k.CreatedAt)
	return err
}

func scanAPIKey(row interface{ Scan(...any) error }) (*store.APIKey, error) {
	var k store.APIKey
	err := row.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.Hash, &k.Scopes,
		&k.LastUsedAt, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

const apiKeyCols = `id, user_id, name, prefix, hash, scopes,
	COALESCE(last_used_at, ''), COALESCE(expires_at, ''), COALESCE(revoked_at, ''), created_at`

// GetAPIKeyByPrefix looks up a key by its public prefix.
func (s *Store) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*store.APIKey, error) {
	k, err := scanAPIKey(s.queryRow(ctx, `SELECT `+apiKeyCols+` FROM api_keys WHERE prefix = ?`, prefix))
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return k, err
}

// ListAPIKeys returns keys for a user (or all when userID is empty).
func (s *Store) ListAPIKeys(ctx context.Context, userID string) ([]*store.APIKey, error) {
	q := `SELECT ` + apiKeyCols + ` FROM api_keys`
	var args []any
	if userID != "" {
		q += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpdateAPIKey updates usage/expiry/revocation fields.
func (s *Store) UpdateAPIKey(ctx context.Context, k *store.APIKey) error {
	_, err := s.exec(ctx, `UPDATE api_keys SET name = ?, scopes = ?, last_used_at = ?, expires_at = ?, revoked_at = ? WHERE id = ?`,
		k.Name, k.Scopes, k.LastUsedAt, k.ExpiresAt, k.RevokedAt, k.ID)
	return err
}

// DeleteAPIKey removes a key.
func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	return err
}

// --- Passkeys ---

// CreatePasskey inserts a WebAuthn credential.
func (s *Store) CreatePasskey(ctx context.Context, p *store.Passkey) error {
	_, err := s.exec(ctx,
		`INSERT INTO passkeys (id, user_id, name, credential_id, public_key, sign_count, transports, aaguid, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.UserID, p.Name, p.CredentialID, p.PublicKey, int64(p.SignCount), p.Transports, p.AAGUID, p.CreatedAt, p.LastUsedAt)
	return err
}

func scanPasskey(row interface{ Scan(...any) error }) (*store.Passkey, error) {
	var p store.Passkey
	var signCount int64
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.CredentialID, &p.PublicKey, &signCount,
		&p.Transports, &p.AAGUID, &p.CreatedAt, &p.LastUsedAt)
	if err != nil {
		return nil, err
	}
	p.SignCount = uint32(signCount)
	return &p, nil
}

const passkeyCols = `id, user_id, COALESCE(name, ''), credential_id, public_key, sign_count,
	COALESCE(transports, ''), aaguid, created_at, COALESCE(last_used_at, '')`

// GetPasskeyByCredentialID finds a credential by its raw id.
func (s *Store) GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*store.Passkey, error) {
	p, err := scanPasskey(s.queryRow(ctx, `SELECT `+passkeyCols+` FROM passkeys WHERE credential_id = ?`, credentialID))
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return p, err
}

// ListPasskeys returns credentials for a user.
func (s *Store) ListPasskeys(ctx context.Context, userID string) ([]*store.Passkey, error) {
	rows, err := s.query(ctx, `SELECT `+passkeyCols+` FROM passkeys WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.Passkey
	for rows.Next() {
		p, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdatePasskey updates the sign counter and last usage.
func (s *Store) UpdatePasskey(ctx context.Context, p *store.Passkey) error {
	_, err := s.exec(ctx, `UPDATE passkeys SET sign_count = ?, last_used_at = ?, name = ? WHERE id = ?`,
		int64(p.SignCount), p.LastUsedAt, p.Name, p.ID)
	return err
}

// DeletePasskey removes a credential.
func (s *Store) DeletePasskey(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM passkeys WHERE id = ?`, id)
	return err
}

// --- Sessions ---

// CreateSession inserts a session.
func (s *Store) CreateSession(ctx context.Context, sess *store.Session) error {
	_, err := s.exec(ctx, `INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at, ip, ua) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.TokenHash, sess.CreatedAt, sess.ExpiresAt, sess.IP, sess.UA)
	return err
}

// GetSession fetches a session by id.
func (s *Store) GetSession(ctx context.Context, id string) (*store.Session, error) {
	var sess store.Session
	err := s.queryRow(ctx, `SELECT id, user_id, token_hash, created_at, expires_at, COALESCE(ip, ''), COALESCE(ua, '') FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.CreatedAt, &sess.ExpiresAt, &sess.IP, &sess.UA)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// GetSessionByTokenHash fetches a session by its hashed token.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*store.Session, error) {
	var sess store.Session
	err := s.queryRow(ctx, `SELECT id, user_id, token_hash, created_at, expires_at, COALESCE(ip, ''), COALESCE(ua, '') FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.CreatedAt, &sess.ExpiresAt, &sess.IP, &sess.UA)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// UpdateSessionExpiry extends a session's rolling expiry.
func (s *Store) UpdateSessionExpiry(ctx context.Context, id, expiresAt string) error {
	_, err := s.exec(ctx, `UPDATE sessions SET expires_at = ? WHERE id = ?`, expiresAt, id)
	return err
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteUserSessions removes all sessions for a user.
func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.exec(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// DeleteExpiredSessions prunes expired sessions.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now string) (int64, error) {
	res, err := s.exec(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Providers ---

func scanProvider(row interface{ Scan(...any) error }) (*store.Provider, error) {
	var p store.Provider
	var enabled store.Bool
	var creds, params string
	err := row.Scan(&p.ID, &p.Code, &p.Name, &enabled, &p.BaseURL, &creds, &params, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Enabled = bool(enabled)
	p.Credentials = store.DecodeMap(creds)
	p.Params = store.DecodeMap(params)
	return &p, nil
}

const providerCols = `id, code, name, enabled, COALESCE(base_url, ''), COALESCE(credentials_json, '{}'), COALESCE(params_json, '{}'), updated_at`

// ListProviders returns all provider instances.
func (s *Store) ListProviders(ctx context.Context) ([]*store.Provider, error) {
	rows, err := s.query(ctx, `SELECT `+providerCols+` FROM providers ORDER BY code, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProvider fetches a provider instance by id.
func (s *Store) GetProvider(ctx context.Context, id string) (*store.Provider, error) {
	p, err := scanProvider(s.queryRow(ctx, `SELECT `+providerCols+` FROM providers WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return p, err
}

// UpsertProvider creates or fully updates a provider instance row.
func (s *Store) UpsertProvider(ctx context.Context, p *store.Provider) error {
	_, err := s.exec(ctx,
		`INSERT INTO providers (id, code, name, enabled, base_url, credentials_json, params_json, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET code = excluded.code, name = excluded.name, enabled = excluded.enabled,
		     base_url = excluded.base_url, credentials_json = excluded.credentials_json,
		     params_json = excluded.params_json, updated_at = excluded.updated_at`,
		p.ID, p.Code, p.Name, store.Bool(p.Enabled), p.BaseURL, store.EncodeMap(p.Credentials), store.EncodeMap(p.Params), store.Now())
	return err
}

// UpdateProviderBaseURL patches only the address (not a secret).
func (s *Store) UpdateProviderBaseURL(ctx context.Context, id, baseURL string) error {
	res, err := s.exec(ctx, `UPDATE providers SET base_url = ?, updated_at = ? WHERE id = ?`, baseURL, store.Now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// SetProviderEnabled toggles a provider instance on/off.
func (s *Store) SetProviderEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.exec(ctx, `UPDATE providers SET enabled = ?, updated_at = ? WHERE id = ?`, store.Bool(enabled), store.Now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// DeleteProvider removes a provider instance row.
func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM providers WHERE id = ?`, id)
	return err
}

// --- Chains ---

// ListChains returns all chains with nodes and edges.
func (s *Store) ListChains(ctx context.Context) ([]*store.Chain, error) {
	rows, err := s.query(ctx, `SELECT id, name, active, mode, version, created_at, updated_at FROM chains ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.Chain
	for rows.Next() {
		c := &store.Chain{}
		var active store.Bool
		if err := rows.Scan(&c.ID, &c.Name, &active, &c.Mode, &c.Version, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Active = bool(active)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range out {
		if err := s.loadChainGraph(ctx, c); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// GetChain returns a chain with its graph.
func (s *Store) GetChain(ctx context.Context, id string) (*store.Chain, error) {
	c := &store.Chain{}
	var active store.Bool
	err := s.queryRow(ctx, `SELECT id, name, active, mode, version, created_at, updated_at FROM chains WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &active, &c.Mode, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Active = bool(active)
	if err := s.loadChainGraph(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetActiveChain returns the single active chain.
func (s *Store) GetActiveChain(ctx context.Context) (*store.Chain, error) {
	c := &store.Chain{}
	var active store.Bool
	err := s.queryRow(ctx, `SELECT id, name, active, mode, version, created_at, updated_at FROM chains WHERE active = ? ORDER BY updated_at DESC`, store.Bool(true)).
		Scan(&c.ID, &c.Name, &active, &c.Mode, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Active = bool(active)
	if err := s.loadChainGraph(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Store) loadChainGraph(ctx context.Context, c *store.Chain) error {
	if err := s.loadChainNodesByID(ctx, c); err != nil {
		return err
	}
	edgeRows, err := s.query(ctx, `SELECT id, chain_id, from_key, to_key, condition FROM chain_edges WHERE chain_id = ?`, c.ID)
	if err != nil {
		return err
	}
	defer edgeRows.Close()
	for edgeRows.Next() {
		var e store.ChainEdge
		if err := edgeRows.Scan(&e.ID, &e.ChainID, &e.FromKey, &e.ToKey, &e.Condition); err != nil {
			return err
		}
		c.Edges = append(c.Edges, e)
	}
	return edgeRows.Err()
}

// nodeMode normalizes a chain-node mode for persistence (the column has a
// CHECK constraint, so an empty value must become the search default).
func nodeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), store.NodeModeAnswer) {
		return store.NodeModeAnswer
	}
	return store.NodeModeSearch
}

// loadChainNodesByID loads the chain graph nodes.
func (s *Store) loadChainNodesByID(ctx context.Context, c *store.Chain) error {
	nodeRows, err := s.query(ctx, `SELECT id, chain_id, key, provider_id, COALESCE(label, ''), COALESCE(params_json, '{}'),
		timeout_ms, retries, retry_delay_ms, COALESCE(delay_policy, 'linear'), COALESCE(on_success, ''),
		COALESCE(on_empty, ''), COALESCE(on_fail, ''), is_start, COALESCE(pos_x, 0), COALESCE(pos_y, 0), COALESCE(mode, 'search')
		FROM chain_nodes WHERE chain_id = ? ORDER BY key`, c.ID)
	if err != nil {
		return err
	}
	defer nodeRows.Close()
	for nodeRows.Next() {
		var n store.ChainNode
		var params string
		var isStart store.Bool
		if err := nodeRows.Scan(&n.ID, &n.ChainID, &n.Key, &n.ProviderID, &n.Label, &params,
			&n.TimeoutMS, &n.Retries, &n.RetryDelayMS, &n.DelayPolicy, &n.OnSuccess,
			&n.OnEmpty, &n.OnFail, &isStart, &n.PosX, &n.PosY, &n.Mode); err != nil {
			return err
		}
		n.Params = store.DecodeMap(params)
		n.IsStart = bool(isStart)
		c.Nodes = append(c.Nodes, n)
	}
	return nodeRows.Err()
}

// SaveChain inserts or replaces a chain and its graph inside a transaction.
func (s *Store) SaveChain(ctx context.Context, c *store.Chain) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := store.Now()
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Mode == "" {
		c.Mode = store.ChainModeFirstSuccess
	}
	var existing string
	err = tx.QueryRowContext(ctx, s.rebind(`SELECT id FROM chains WHERE id = ?`), c.ID).Scan(&existing)
	if err == sql.ErrNoRows {
		if _, err := tx.ExecContext(ctx, s.rebind(
			`INSERT INTO chains (id, name, active, mode, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
			c.ID, c.Name, store.Bool(c.Active), c.Mode, c.Version, c.CreatedAt, c.UpdatedAt); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		c.Version++
		if _, err := tx.ExecContext(ctx, s.rebind(
			`UPDATE chains SET name = ?, active = ?, mode = ?, version = ?, updated_at = ? WHERE id = ?`),
			c.Name, store.Bool(c.Active), c.Mode, c.Version, c.UpdatedAt, c.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM chain_nodes WHERE chain_id = ?`), c.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM chain_edges WHERE chain_id = ?`), c.ID); err != nil {
		return err
	}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		n.ChainID = c.ID
		if n.ID == "" {
			n.ID = newID()
		}
		if n.DelayPolicy == "" {
			n.DelayPolicy = "linear"
		}
		if _, err := tx.ExecContext(ctx, s.rebind(
			`INSERT INTO chain_nodes (id, chain_id, key, provider_id, label, params_json, timeout_ms, retries,
			 retry_delay_ms, delay_policy, on_success, on_empty, on_fail, is_start, pos_x, pos_y, mode)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			n.ID, c.ID, n.Key, n.ProviderID, n.Label, store.EncodeMap(n.Params), n.TimeoutMS, n.Retries,
			n.RetryDelayMS, n.DelayPolicy, n.OnSuccess, n.OnEmpty, n.OnFail, store.Bool(n.IsStart), n.PosX, n.PosY, nodeMode(n.Mode)); err != nil {
			return err
		}
	}
	for i := range c.Edges {
		e := &c.Edges[i]
		e.ChainID = c.ID
		if e.ID == "" {
			e.ID = newID()
		}
		if e.Condition == "" {
			e.Condition = "next"
		}
		if _, err := tx.ExecContext(ctx, s.rebind(
			`INSERT INTO chain_edges (id, chain_id, from_key, to_key, condition) VALUES (?, ?, ?, ?, ?)`),
			e.ID, c.ID, e.FromKey, e.ToKey, e.Condition); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteChain removes a chain and its graph.
func (s *Store) DeleteChain(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM chains WHERE id = ?`, id)
	return err
}

// ActivateChain makes one chain active and deactivates the rest.
func (s *Store) ActivateChain(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists string
	if err := tx.QueryRowContext(ctx, s.rebind(`SELECT id FROM chains WHERE id = ?`), id).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return store.ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE chains SET active = ? WHERE active = ?`), store.Bool(false), store.Bool(true)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`UPDATE chains SET active = ?, updated_at = ? WHERE id = ?`), store.Bool(true), store.Now(), id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Requests and steps ---

// CreateRequest inserts a request row.
func (s *Store) CreateRequest(ctx context.Context, r *store.Request) error {
	snapshot := ""
	if r.ChainSnapshot != nil {
		snapshot = store.EncodeJSON(r.ChainSnapshot)
	}
	_, err := s.exec(ctx,
		`INSERT INTO requests (id, rid, query, count, status, used_provider, results_count, total_ms, steps_count,
		 chain_id, chain_snapshot_json, client, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.RID, r.Query, r.Count, r.Status, r.UsedProvider, r.ResultsCount, r.TotalMS, r.StepsCount,
		r.ChainID, snapshot, r.Client, r.Error, r.CreatedAt)
	return err
}

// UpdateRequest updates an existing request row.
func (s *Store) UpdateRequest(ctx context.Context, r *store.Request) error {
	var mergeCollected, mergeUnique, mergeDuplicates any
	if r.Merge != nil {
		mergeCollected = r.Merge.CollectedTotal
		mergeUnique = r.Merge.UniqueLinks
		mergeDuplicates = r.Merge.DuplicatesRemoved
	}
	results := ""
	if len(r.Results) > 0 {
		results = store.EncodeJSON(r.Results)
	}
	_, err := s.exec(ctx,
		`UPDATE requests SET status = ?, used_provider = ?, results_count = ?, total_ms = ?, steps_count = ?, error = ?,
		 merge_collected = ?, merge_unique = ?, merge_duplicates = ?, results_json = ?, answer = ? WHERE id = ?`,
		r.Status, r.UsedProvider, r.ResultsCount, r.TotalMS, r.StepsCount, r.Error,
		mergeCollected, mergeUnique, mergeDuplicates, results, r.Answer, r.ID)
	return err
}

const requestCols = `id, rid, query, count, status, COALESCE(used_provider, ''), results_count, total_ms, steps_count,
	COALESCE(chain_id, ''), COALESCE(chain_snapshot_json, ''), COALESCE(client, ''), COALESCE(error, ''), created_at,
	merge_collected, merge_unique, merge_duplicates, COALESCE(results_json, ''), COALESCE(answer, '')`

func scanRequest(row interface{ Scan(...any) error }) (*store.Request, error) {
	r := &store.Request{}
	var snapshot, results string
	var mergeCollected, mergeUnique, mergeDuplicates *int
	if err := row.Scan(&r.ID, &r.RID, &r.Query, &r.Count, &r.Status, &r.UsedProvider, &r.ResultsCount,
		&r.TotalMS, &r.StepsCount, &r.ChainID, &snapshot, &r.Client, &r.Error, &r.CreatedAt,
		&mergeCollected, &mergeUnique, &mergeDuplicates, &results, &r.Answer); err != nil {
		return nil, err
	}
	if snapshot != "" {
		var c store.Chain
		if err := store.DecodeJSON(snapshot, &c); err == nil {
			r.ChainSnapshot = &c
		}
	}
	if mergeCollected != nil && mergeUnique != nil && mergeDuplicates != nil {
		r.Merge = &store.MergeStats{
			CollectedTotal:    *mergeCollected,
			UniqueLinks:       *mergeUnique,
			DuplicatesRemoved: *mergeDuplicates,
		}
	}
	if results != "" {
		_ = store.DecodeJSON(results, &r.Results)
	}
	return r, nil
}

// GetRequest fetches a request with its steps.
func (s *Store) GetRequest(ctx context.Context, id string) (*store.Request, error) {
	r, err := scanRequest(s.queryRow(ctx, `SELECT `+requestCols+` FROM requests WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	steps, err := s.ListSteps(ctx, id)
	if err != nil {
		return nil, err
	}
	r.Steps = steps
	return r, nil
}

// GetRequestByRID fetches a request by its human readable id.
func (s *Store) GetRequestByRID(ctx context.Context, rid string) (*store.Request, error) {
	r, err := scanRequest(s.queryRow(ctx, `SELECT `+requestCols+` FROM requests WHERE rid = ?`, rid))
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return r, err
}

// ListRequests returns filtered requests and the total count.
func (s *Store) ListRequests(ctx context.Context, f store.RequestFilter, p store.Page) ([]*store.Request, int, error) {
	where := []string{"1 = 1"}
	var args []any
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Provider != "" {
		where = append(where, "used_provider = ?")
		args = append(args, f.Provider)
	}
	if f.From != "" {
		where = append(where, "created_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, "created_at <= ?")
		args = append(args, f.To)
	}
	if f.Query != "" {
		where = append(where, "query LIKE ?")
		args = append(args, "%"+f.Query+"%")
	}
	clause := strings.Join(where, " AND ")

	var total int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM requests WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	q := `SELECT ` + requestCols + ` FROM requests WHERE ` + clause + ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := s.query(ctx, q, append(append([]any{}, args...), p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*store.Request
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// AddStep inserts a request step.
func (s *Store) AddStep(ctx context.Context, st *store.RequestStep) error {
	_, err := s.exec(ctx,
		`INSERT INTO request_steps (id, request_id, idx, node_key, provider, engine, attempt_no, status, kind,
		 http_status, error, permanent, results_count, wait_before_ms, took_ms, started_at, finished_at, stage)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.RequestID, st.Idx, st.NodeKey, st.Provider, st.Engine, st.AttemptNo, st.Status, st.Kind,
		st.HTTPStatus, st.Error, store.Bool(st.Permanent), st.ResultsCount, st.WaitBeforeMS, st.TookMS, st.StartedAt, st.FinishedAt, st.Stage)
	return err
}

// ListSteps returns steps ordered by index and attempt.
func (s *Store) ListSteps(ctx context.Context, requestID string) ([]*store.RequestStep, error) {
	rows, err := s.query(ctx,
		`SELECT id, request_id, idx, COALESCE(node_key, ''), provider, COALESCE(engine, ''), attempt_no, status,
		 COALESCE(kind, ''), COALESCE(http_status, 0), COALESCE(error, ''), permanent, results_count,
		 wait_before_ms, took_ms, COALESCE(started_at, ''), COALESCE(finished_at, ''), COALESCE(stage, '')
		 FROM request_steps WHERE request_id = ? ORDER BY idx, attempt_no, started_at`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.RequestStep
	for rows.Next() {
		var st store.RequestStep
		var permanent store.Bool
		if err := rows.Scan(&st.ID, &st.RequestID, &st.Idx, &st.NodeKey, &st.Provider, &st.Engine, &st.AttemptNo,
			&st.Status, &st.Kind, &st.HTTPStatus, &st.Error, &permanent, &st.ResultsCount,
			&st.WaitBeforeMS, &st.TookMS, &st.StartedAt, &st.FinishedAt, &st.Stage); err != nil {
			return nil, err
		}
		st.Permanent = bool(permanent)
		out = append(out, &st)
	}
	return out, rows.Err()
}

// ClearHistory deletes all requests and steps.
func (s *Store) ClearHistory(ctx context.Context) (int64, error) {
	// ON DELETE CASCADE removes steps as well.
	res, err := s.exec(ctx, `DELETE FROM requests`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteOldRequests removes requests created before the cutoff.
func (s *Store) DeleteOldRequests(ctx context.Context, before string) (int64, error) {
	res, err := s.exec(ctx, `DELETE FROM requests WHERE created_at < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Logs ---

// AddLog inserts a log line.
func (s *Store) AddLog(ctx context.Context, e *store.LogEntry) error {
	_, err := s.exec(ctx, `INSERT INTO logs (ts, level, rid, api, message) VALUES (?, ?, ?, ?, ?)`,
		e.TS, e.Level, e.RID, e.API, e.Message)
	return err
}

// ListLogs returns filtered logs and the total count.
func (s *Store) ListLogs(ctx context.Context, f store.LogFilter, p store.Page) ([]*store.LogEntry, int, error) {
	where := []string{"1 = 1"}
	var args []any
	if f.Level != "" {
		where = append(where, "level = ?")
		args = append(args, f.Level)
	}
	if f.Query != "" {
		where = append(where, "(message LIKE ? OR rid LIKE ?)")
		args = append(args, "%"+f.Query+"%", "%"+f.Query+"%")
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM logs WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}
	rows, err := s.query(ctx, `SELECT id, ts, level, COALESCE(rid, ''), COALESCE(api, ''), message FROM logs WHERE `+clause+
		` ORDER BY id DESC LIMIT ? OFFSET ?`, append(append([]any{}, args...), p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*store.LogEntry
	for rows.Next() {
		var e store.LogEntry
		if err := rows.Scan(&e.ID, &e.TS, &e.Level, &e.RID, &e.API, &e.Message); err != nil {
			return nil, 0, err
		}
		out = append(out, &e)
	}
	return out, total, rows.Err()
}

// ClearLogs deletes every log row.
func (s *Store) ClearLogs(ctx context.Context) (int64, error) {
	res, err := s.exec(ctx, `DELETE FROM logs`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteOldLogs removes logs older than the cutoff.
func (s *Store) DeleteOldLogs(ctx context.Context, before string) (int64, error) {
	res, err := s.exec(ctx, `DELETE FROM logs WHERE ts < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Stats ---

// Summary computes dashboard aggregates.
func (s *Store) Summary(ctx context.Context, days int) (*store.StatsSummary, error) {
	if days <= 0 {
		days = 7
	}
	out := &store.StatsSummary{}
	today := dayStartUTC()
	var reqToday, okToday, emptyToday, failToday int
	if err := s.queryRow(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'empty' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'fail' THEN 1 ELSE 0 END), 0)
		 FROM requests WHERE created_at >= ?`, today).
		Scan(&reqToday, &okToday, &emptyToday, &failToday); err != nil {
		return nil, err
	}
	out.RequestsToday = reqToday
	out.ErrorsToday = failToday
	if reqToday > 0 {
		out.SuccessRate = float64(okToday) / float64(reqToday) * 100
	}
	var avg float64
	if err := s.queryRow(ctx, `SELECT COALESCE(AVG(total_ms), 0) FROM requests WHERE created_at >= ?`, today).Scan(&avg); err != nil {
		return nil, err
	}
	out.AvgMS = int(avg)
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM requests`).Scan(&out.TotalRequests); err != nil {
		return nil, err
	}

	// Daily rollups for the requested window.
	from := dayStartUTCMinus(days - 1)
	rows, err := s.query(ctx,
		`SELECT substr(created_at, 1, 10) AS d,
		        COUNT(*),
		        COALESCE(SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'empty' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'fail' THEN 1 ELSE 0 END), 0),
		        COALESCE(AVG(total_ms), 0)
		 FROM requests WHERE created_at >= ? GROUP BY d ORDER BY d`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	daily := map[string]store.DailyStat{}
	for rows.Next() {
		var d store.DailyStat
		var avgMS float64
		if err := rows.Scan(&d.Date, &d.Requests, &d.OK, &d.Empty, &d.Fail, &avgMS); err != nil {
			return nil, err
		}
		d.AvgMS = int(avgMS)
		daily[d.Date] = d
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := days - 1; i >= 0; i-- {
		day := dateOnlyMinus(i)
		if d, ok := daily[day]; ok {
			out.Daily = append(out.Daily, d)
		} else {
			out.Daily = append(out.Daily, store.DailyStat{Date: day})
		}
	}

	recentRows, err := s.query(ctx, `SELECT `+requestCols+` FROM requests ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer recentRows.Close()
	for recentRows.Next() {
		r, err := scanRequest(recentRows)
		if err != nil {
			return nil, err
		}
		out.Recent = append(out.Recent, r)
	}
	return out, recentRows.Err()
}

// RecomputeDailyStats recalculates the rollup row for one date (YYYY-MM-DD).
func (s *Store) RecomputeDailyStats(ctx context.Context, date string) error {
	var requests, ok, empty, fail int
	var avg float64
	if err := s.queryRow(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'empty' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'fail' THEN 1 ELSE 0 END), 0),
		        COALESCE(AVG(total_ms), 0)
		 FROM requests WHERE substr(created_at, 1, 10) = ?`, date).
		Scan(&requests, &ok, &empty, &fail, &avg); err != nil {
		return err
	}
	_, err := s.exec(ctx,
		`INSERT INTO stats_daily (date, requests, ok, empty, fail, avg_ms) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (date) DO UPDATE SET requests = excluded.requests, ok = excluded.ok,
		     empty = excluded.empty, fail = excluded.fail, avg_ms = excluded.avg_ms`,
		date, requests, ok, empty, fail, int(avg))
	return err
}

// Vacuum compacts the database (SQLite only; PostgreSQL is a no-op).
func (s *Store) Vacuum(ctx context.Context) error {
	if s.driver != "sqlite" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `VACUUM`)
	return err
}
