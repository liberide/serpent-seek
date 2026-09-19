-- SerpentSeek initial schema (PostgreSQL dialect).
-- Differences vs SQLite: BOOLEAN instead of INTEGER for flags, BYTEA for blobs,
-- BIGSERIAL for the log primary key, and no AUTOINCREMENT.

-- +goose Up
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE users (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    role       TEXT NOT NULL CHECK (role IN ('admin', 'viewer')),
    disabled   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TEXT NOT NULL
);

CREATE TABLE api_keys (
    id           TEXT PRIMARY KEY,
    user_id      TEXT REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL UNIQUE,
    hash         TEXT NOT NULL,
    scopes       TEXT NOT NULL DEFAULT 'search',
    last_used_at TEXT,
    expires_at   TEXT,
    revoked_at   TEXT,
    created_at   TEXT NOT NULL
);
CREATE INDEX idx_api_keys_user ON api_keys(user_id);

CREATE TABLE passkeys (
    id            TEXT PRIMARY KEY,
    user_id       TEXT REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT,
    credential_id BYTEA UNIQUE NOT NULL,
    public_key    BYTEA NOT NULL,
    sign_count    BIGINT NOT NULL DEFAULT 0,
    transports    TEXT,
    aaguid        BYTEA,
    created_at    TEXT NOT NULL,
    last_used_at  TEXT
);
CREATE INDEX idx_passkeys_user ON passkeys(user_id);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    ip         TEXT,
    ua         TEXT
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE providers (
    code             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    base_url         TEXT,
    credentials_json TEXT,
    params_json      TEXT,
    updated_at       TEXT NOT NULL
);

CREATE TABLE chains (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    active     BOOLEAN NOT NULL DEFAULT FALSE,
    version    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE chain_nodes (
    id             TEXT PRIMARY KEY,
    chain_id       TEXT REFERENCES chains(id) ON DELETE CASCADE,
    key            TEXT NOT NULL,
    provider_code  TEXT NOT NULL,
    label          TEXT,
    params_json    TEXT,
    timeout_ms     INTEGER NOT NULL DEFAULT 20000,
    retries        INTEGER NOT NULL DEFAULT 0,
    retry_delay_ms INTEGER NOT NULL DEFAULT 700,
    delay_policy   TEXT NOT NULL DEFAULT 'linear',
    on_success     TEXT,
    on_empty       TEXT,
    on_fail        TEXT,
    pos_x          DOUBLE PRECISION,
    pos_y          DOUBLE PRECISION
);
CREATE INDEX idx_chain_nodes_chain ON chain_nodes(chain_id);

CREATE TABLE chain_edges (
    id        TEXT PRIMARY KEY,
    chain_id  TEXT REFERENCES chains(id) ON DELETE CASCADE,
    from_key  TEXT NOT NULL,
    to_key    TEXT NOT NULL,
    condition TEXT NOT NULL DEFAULT 'next'
);
CREATE INDEX idx_chain_edges_chain ON chain_edges(chain_id);

CREATE TABLE requests (
    id                  TEXT PRIMARY KEY,
    rid                 TEXT NOT NULL UNIQUE,
    query               TEXT NOT NULL,
    count               INTEGER NOT NULL,
    status              TEXT NOT NULL,
    used_provider       TEXT,
    results_count       INTEGER NOT NULL DEFAULT 0,
    total_ms            INTEGER NOT NULL DEFAULT 0,
    steps_count         INTEGER NOT NULL DEFAULT 0,
    chain_id            TEXT,
    chain_snapshot_json TEXT,
    client              TEXT,
    error               TEXT,
    created_at          TEXT NOT NULL
);
CREATE INDEX idx_requests_created ON requests(created_at);

CREATE TABLE request_steps (
    id            TEXT PRIMARY KEY,
    request_id    TEXT REFERENCES requests(id) ON DELETE CASCADE,
    idx           INTEGER NOT NULL,
    node_key      TEXT,
    provider      TEXT NOT NULL,
    engine        TEXT,
    attempt_no    INTEGER NOT NULL DEFAULT 1,
    status        TEXT NOT NULL,
    kind          TEXT,
    http_status   INTEGER,
    error         TEXT,
    permanent     BOOLEAN NOT NULL DEFAULT FALSE,
    results_count INTEGER NOT NULL DEFAULT 0,
    wait_before_ms INTEGER NOT NULL DEFAULT 0,
    took_ms       INTEGER NOT NULL DEFAULT 0,
    started_at    TEXT,
    finished_at   TEXT,
    stage         TEXT
);
CREATE INDEX idx_steps_request ON request_steps(request_id);

CREATE TABLE logs (
    id      BIGSERIAL PRIMARY KEY,
    ts      TEXT NOT NULL,
    level   TEXT NOT NULL,
    rid     TEXT,
    api     TEXT,
    message TEXT NOT NULL
);
CREATE INDEX idx_logs_ts ON logs(ts);

CREATE TABLE stats_daily (
    date     TEXT PRIMARY KEY,
    requests INTEGER NOT NULL DEFAULT 0,
    ok       INTEGER NOT NULL DEFAULT 0,
    empty    INTEGER NOT NULL DEFAULT 0,
    fail     INTEGER NOT NULL DEFAULT 0,
    avg_ms   INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS stats_daily;
DROP TABLE IF EXISTS logs;
DROP TABLE IF EXISTS request_steps;
DROP TABLE IF EXISTS requests;
DROP TABLE IF EXISTS chain_edges;
DROP TABLE IF EXISTS chain_nodes;
DROP TABLE IF EXISTS chains;
DROP TABLE IF EXISTS providers;
DROP TABLE IF EXISTS passkeys;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS settings;
