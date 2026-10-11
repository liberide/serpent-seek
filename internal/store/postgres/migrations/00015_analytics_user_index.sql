-- Analytics index for the per-user views: the users leaderboard groups
-- requests by owner over a date range and the per-user analytics report
-- filters by (user_id, created_at). The existing single-column index
-- idx_requests_user_id cannot serve the range part of those queries.

-- +goose Up
CREATE INDEX idx_requests_user_created ON requests(user_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_requests_user_created;