-- Provider instances: one row per configured provider *instance*.
-- `code` is the driver type (apiserpent, searxng, ...) and is no longer unique,
-- so several instances of the same driver can coexist with different names and
-- credentials. `id` is the identity referenced by chain nodes.

-- +goose Up
CREATE TABLE providers_v2 (
    id               TEXT PRIMARY KEY,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    base_url         TEXT,
    credentials_json TEXT,
    params_json      TEXT,
    updated_at       TEXT NOT NULL
);

-- Preserve existing rows: the old primary key (`code`) becomes the new instance id,
-- so chain node references stored as provider_code stay valid.
INSERT INTO providers_v2 (id, code, name, enabled, base_url, credentials_json, params_json, updated_at)
    SELECT code, code, name, enabled, base_url, credentials_json, params_json, updated_at FROM providers;

DROP TABLE providers;
ALTER TABLE providers_v2 RENAME TO providers;
CREATE INDEX idx_providers_code ON providers(code);
CREATE UNIQUE INDEX idx_providers_name ON providers(name);

ALTER TABLE chain_nodes RENAME COLUMN provider_code TO provider_id;

-- +goose Down
ALTER TABLE chain_nodes RENAME COLUMN provider_id TO provider_code;

CREATE TABLE providers_v1 (
    code             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    base_url         TEXT,
    credentials_json TEXT,
    params_json      TEXT,
    updated_at       TEXT NOT NULL
);
INSERT OR IGNORE INTO providers_v1 (code, name, enabled, base_url, credentials_json, params_json, updated_at)
    SELECT code, name, enabled, base_url, credentials_json, params_json, updated_at FROM providers ORDER BY id;
DROP TABLE providers;
ALTER TABLE providers_v1 RENAME TO providers;
