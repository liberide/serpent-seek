-- Answer-node support (PostgreSQL dialect).
-- See the sqlite 00004 migration for the semantics.

-- +goose Up
ALTER TABLE chain_nodes ADD COLUMN mode TEXT NOT NULL DEFAULT 'search';
ALTER TABLE chain_nodes ADD CONSTRAINT chain_nodes_mode_check
    CHECK (mode IN ('search', 'answer'));

ALTER TABLE requests ADD COLUMN answer TEXT;

-- +goose Down
ALTER TABLE requests DROP COLUMN answer;
ALTER TABLE chain_nodes DROP CONSTRAINT IF EXISTS chain_nodes_mode_check;
ALTER TABLE chain_nodes DROP COLUMN mode;
