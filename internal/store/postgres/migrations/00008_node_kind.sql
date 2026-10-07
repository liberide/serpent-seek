-- Logical chain blocks (PostgreSQL dialect).
-- See the sqlite 00008 migration for the semantics.

-- +goose Up
ALTER TABLE chain_nodes ADD COLUMN kind TEXT NOT NULL DEFAULT 'provider';
ALTER TABLE chain_nodes ADD CONSTRAINT chain_nodes_kind_check
    CHECK (kind IN ('provider', 'start', 'join'));

-- +goose Down
ALTER TABLE chain_nodes DROP CONSTRAINT IF EXISTS chain_nodes_kind_check;
ALTER TABLE chain_nodes DROP COLUMN kind;
