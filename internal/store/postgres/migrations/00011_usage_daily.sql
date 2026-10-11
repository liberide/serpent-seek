-- Daily per-subject search counters used to enforce SEARCH_DAILY_QUOTA.
-- subject is "key:<api_key id>", "user:<user id>" or "ip:<address>"; day is a
-- UTC calendar date (YYYY-MM-DD). Rows older than the retention window are
-- removed by the cleanup job.

-- +goose Up
CREATE TABLE usage_daily (
    subject TEXT NOT NULL,
    day TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (subject, day)
);
CREATE INDEX idx_usage_daily_day ON usage_daily(day);

-- +goose Down
DROP INDEX idx_usage_daily_day;
DROP TABLE usage_daily;
