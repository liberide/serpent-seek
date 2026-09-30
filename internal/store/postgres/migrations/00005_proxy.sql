-- Named outbound proxies and per-provider proxy selection.
-- proxies: user-managed list of HTTP(S)/SOCKS5 proxies (password is write-only).
-- providers.proxy_id: optional reference to the proxy an instance routes through.

-- +goose Up
CREATE TABLE proxies (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    type       TEXT NOT NULL DEFAULT 'http',
    host       TEXT NOT NULL,
    port       TEXT,
    username   TEXT,
    password   TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_proxies_name ON proxies(name);

ALTER TABLE providers ADD COLUMN proxy_id TEXT;

-- +goose Down
ALTER TABLE providers DROP COLUMN proxy_id;
DROP TABLE proxies;
