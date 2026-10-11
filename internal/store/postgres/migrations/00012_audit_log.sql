-- Administrative audit trail. Every mutating admin action (settings, providers,
-- chains, API keys, users, proxies, maintenance) appends a row here. Details is
-- a compact JSON object and never contains secret values.

-- +goose Up
CREATE TABLE audit_log (
    id TEXT PRIMARY KEY,
    ts TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    user_name TEXT NOT NULL DEFAULT '',
    via TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_audit_log_ts ON audit_log(ts);
CREATE INDEX idx_audit_log_action ON audit_log(action);

-- +goose Down
DROP INDEX idx_audit_log_action;
DROP INDEX idx_audit_log_ts;
DROP TABLE audit_log;
