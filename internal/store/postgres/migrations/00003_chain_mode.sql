-- Chain execution mode + explicit start block (PostgreSQL dialect).
-- See the sqlite 00003 migration for the semantics.

-- +goose Up
ALTER TABLE chains ADD COLUMN mode TEXT NOT NULL DEFAULT 'first_success';
ALTER TABLE chains ADD CONSTRAINT chains_mode_check
    CHECK (mode IN ('first_success', 'full_chain'));

ALTER TABLE chain_nodes ADD COLUMN is_start BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE requests ADD COLUMN merge_collected INTEGER;
ALTER TABLE requests ADD COLUMN merge_unique INTEGER;
ALTER TABLE requests ADD COLUMN merge_duplicates INTEGER;
ALTER TABLE requests ADD COLUMN results_json TEXT;

-- +goose Down
ALTER TABLE requests DROP COLUMN results_json;
ALTER TABLE requests DROP COLUMN merge_duplicates;
ALTER TABLE requests DROP COLUMN merge_unique;
ALTER TABLE requests DROP COLUMN merge_collected;
ALTER TABLE chain_nodes DROP COLUMN is_start;
ALTER TABLE chains DROP CONSTRAINT IF EXISTS chains_mode_check;
ALTER TABLE chains DROP COLUMN mode;
