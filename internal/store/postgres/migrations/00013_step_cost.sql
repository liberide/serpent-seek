-- Per-attempt cost of the provider call, snapshotted from the provider
-- instance's `cost_per_request` param at execution time. 0 (the default) means
-- no cost was configured, so the value is not counted or displayed.

-- +goose Up
ALTER TABLE request_steps ADD COLUMN cost DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE request_steps DROP COLUMN cost;
