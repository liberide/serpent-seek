-- Per-endpoint permissions for developer tokens.
--
-- api_keys.permissions holds a JSON array of endpoint ids ("METHOD /path").
-- Only developer tokens (scope 'developer') consult it:
--   * NULL or an empty value  -> no restriction (all endpoints)
--   * ["*"]                   -> explicit full access
--   * ["GET /api/chains", …]  -> only the listed endpoints are allowed
--
-- Regular (admin/search) keys do not carry the developer scope and do not
-- consult the column.

-- +goose Up
ALTER TABLE api_keys ADD COLUMN permissions TEXT;

-- +goose Down
ALTER TABLE api_keys DROP COLUMN permissions;
