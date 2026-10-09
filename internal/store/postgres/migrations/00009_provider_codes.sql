-- Align provider driver codes with the documented API contract.
-- See the sqlite 00009 migration for the semantics.

-- +goose Up
UPDATE providers SET code = 'perplexity' WHERE code = 'perplexity_search';
UPDATE providers SET code = 'kimi_search' WHERE code = 'kimi';
UPDATE providers SET code = 'kimi_search_pro' WHERE code = 'kimi_pro';

-- +goose Down
UPDATE providers SET code = 'perplexity_search' WHERE code = 'perplexity';
UPDATE providers SET code = 'kimi' WHERE code = 'kimi_search';
UPDATE providers SET code = 'kimi_pro' WHERE code = 'kimi_search_pro';
