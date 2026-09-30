-- Analytics indexes: speed up the per-provider filter and provider breakdown,
-- which query request_steps by provider.

-- +goose Up
CREATE INDEX idx_steps_provider ON request_steps(provider);

-- +goose Down
DROP INDEX IF EXISTS idx_steps_provider;
