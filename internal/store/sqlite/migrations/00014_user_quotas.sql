-- Per-user search quotas. An admin assigns a request allowance and a reset
-- cadence to a user; the guard (internal/httpd/guard.go) enforces it on every
-- search endpoint. requests = 0 means unlimited (the default: no row).
--
-- period is one of: daily | weekly | monthly | rolling.
--   daily   — resets at midnight, shifted by tz_offset minutes
--   weekly  — resets Monday 00:00, shifted by tz_offset
--   monthly — resets on the 1st of the month 00:00, shifted by tz_offset
--   rolling — a sliding window of window_hours; counted from usage_rolling hits
-- tz_offset is a fixed offset in minutes (-720..840); 0 = pure UTC. A fixed
-- offset is used instead of an IANA name to avoid a tzdata dependency.
--
-- Timestamps are RFC3339 strings in UTC, matching the rest of the schema.

-- +goose Up
CREATE TABLE user_quotas (
    user_id      TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    requests     INTEGER NOT NULL DEFAULT 0,
    period       TEXT NOT NULL DEFAULT 'daily'
                 CHECK (period IN ('daily', 'weekly', 'monthly', 'rolling')),
    window_hours INTEGER NOT NULL DEFAULT 24,
    tz_offset    INTEGER NOT NULL DEFAULT 0,
    updated_at   TEXT NOT NULL
);

-- Sliding-window hits for period = 'rolling'. One row per admitted request;
-- the guard counts rows newer than (now - window_hours). Rows are pruned by
-- the cleanup job. The composite PK also dedupes identical-timestamp inserts.
CREATE TABLE usage_rolling (
    subject TEXT NOT NULL,
    ts      TEXT NOT NULL,
    PRIMARY KEY (subject, ts)
);
CREATE INDEX idx_usage_rolling_ts ON usage_rolling(ts);

-- +goose Down
DROP INDEX idx_usage_rolling_ts;
DROP TABLE usage_rolling;
DROP TABLE user_quotas;
