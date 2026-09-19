-- Chain execution mode + explicit start block.
-- chains.mode: first_success (default, stop at first usable result) or
--              full_chain (walk every reachable block and merge results).
-- chain_nodes.is_start: exactly one node per chain is the entry point.
-- requests: full-chain merge summary and the persisted final rows (history view).
-- No backfill: the project is in development, fresh DBs seed is_start explicitly.

-- +goose Up
ALTER TABLE chains ADD COLUMN mode TEXT NOT NULL DEFAULT 'first_success'
    CHECK (mode IN ('first_success', 'full_chain'));

ALTER TABLE chain_nodes ADD COLUMN is_start INTEGER NOT NULL DEFAULT 0;

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
ALTER TABLE chains DROP COLUMN mode;
