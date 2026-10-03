-- Logical chain blocks: an explicit Start entry point that can fan out to
-- several providers, and a Join block that merges branches and removes
-- duplicate URLs.
-- chain_nodes.kind: provider (default, runs a provider instance), start
--   (single logical entry point) or join (branch merge/dedupe).
-- Logical nodes have no provider_id and cannot be answer nodes.
-- is_start is kept for backward compatibility; kind='start' takes precedence.

-- +goose Up
ALTER TABLE chain_nodes ADD COLUMN kind TEXT NOT NULL DEFAULT 'provider'
    CHECK (kind IN ('provider', 'start', 'join'));

-- +goose Down
ALTER TABLE chain_nodes DROP COLUMN kind;
