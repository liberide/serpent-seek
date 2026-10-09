-- Align provider driver codes with the documented API contract:
--   perplexity_search -> perplexity
--   kimi              -> kimi_search
--   kimi_pro          -> kimi_search_pro
-- Only the driver code changes; provider instance ids (referenced by
-- chain_nodes.provider_id) stay untouched.

-- +goose Up
UPDATE providers SET code = 'perplexity' WHERE code = 'perplexity_search';
UPDATE providers SET code = 'kimi_search' WHERE code = 'kimi';
UPDATE providers SET code = 'kimi_search_pro' WHERE code = 'kimi_pro';

-- +goose Down
UPDATE providers SET code = 'perplexity_search' WHERE code = 'perplexity';
UPDATE providers SET code = 'kimi' WHERE code = 'kimi_search';
UPDATE providers SET code = 'kimi_pro' WHERE code = 'kimi_search_pro';
