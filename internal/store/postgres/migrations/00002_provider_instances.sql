-- Provider instances: one row per configured provider *instance*.
-- `code` is the driver type (apiserpent, searxng, ...) and is no longer unique,
-- so several instances of the same driver can coexist with different names and
-- credentials. `id` is the identity referenced by chain nodes.

-- +goose Up
ALTER TABLE providers RENAME TO providers_old;

CREATE TABLE providers (
    id               TEXT PRIMARY KEY,
    code             TEXT NOT NULL,
    name             TEXT NOT NULL,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    base_url         TEXT,
    credentials_json TEXT,
    params_json      TEXT,
    updated_at       TEXT NOT NULL
);

-- Preserve existing rows: the old primary key (`code`) becomes the new instance id,
-- so chain node references stored as provider_code stay valid.
INSERT INTO providers (id, code, name, enabled, base_url, credentials_json, params_json, updated_at)
    SELECT code, code, name, enabled, base_url, credentials_json, params_json, updated_at FROM providers_old;

DROP TABLE providers_old;
CREATE INDEX idx_providers_code ON providers(code);
CREATE UNIQUE INDEX idx_providers_name ON providers(name);

ALTER TABLE chain_nodes RENAME COLUMN provider_code TO provider_id;

-- +goose Down
ALTER TABLE chain_nodes RENAME COLUMN provider_id TO provider_code;

ALTER TABLE providers RENAME TO providers_old;
CREATE TABLE providers (
    code             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    base_url         TEXT,
    credentials_json TEXT,
    params_json      TEXT,
    updated_at       TEXT NOT NULL
);
INSERT INTO providers (code, name, enabled, base_url, credentials_json, params_json, updated_at)
    SELECT DISTINCT ON (code) code, name, enabled, base_url, credentials_json, params_json, updated_at
    FROM providers_old ORDER BY code, id;
DROP TABLE providers_old;
