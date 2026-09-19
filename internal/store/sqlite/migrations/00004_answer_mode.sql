-- Answer-node support.
-- chain_nodes.mode: search (default, Row source) or answer (generative node).
--   Answer nodes do not participate in the Row merge and cannot be the start
--   block; their generated text is stored in requests.answer.
-- requests.answer: the generated answer text produced by the last answer node.

-- +goose Up
ALTER TABLE chain_nodes ADD COLUMN mode TEXT NOT NULL DEFAULT 'search'
    CHECK (mode IN ('search', 'answer'));

ALTER TABLE requests ADD COLUMN answer TEXT;

-- +goose Down
ALTER TABLE requests DROP COLUMN answer;
ALTER TABLE chain_nodes DROP COLUMN mode;
