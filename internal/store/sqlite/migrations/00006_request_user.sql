-- Per-user ownership of search requests so non-admin users only see their own
-- history and live traces.

-- +goose Up
ALTER TABLE requests ADD COLUMN user_id TEXT;
CREATE INDEX idx_requests_user_id ON requests(user_id);

-- +goose Down
DROP INDEX idx_requests_user_id;
ALTER TABLE requests DROP COLUMN user_id;
