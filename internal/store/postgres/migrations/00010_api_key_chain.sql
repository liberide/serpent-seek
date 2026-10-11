-- Bind an API key (token) to a specific chain.
-- api_keys.chain_id: optional reference to chains(id). NULL means "use the
-- globally active chain". ON DELETE SET NULL keeps a token usable (it falls
-- back to the active chain) when its bound chain is removed.

-- +goose Up
ALTER TABLE api_keys ADD COLUMN chain_id TEXT REFERENCES chains(id) ON DELETE SET NULL;
CREATE INDEX idx_api_keys_chain ON api_keys(chain_id);

-- +goose Down
DROP INDEX idx_api_keys_chain;
ALTER TABLE api_keys DROP COLUMN chain_id;
