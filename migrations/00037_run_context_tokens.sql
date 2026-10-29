-- +goose Up
-- The context size of the session after a run, as the client measured it.
-- The summed usage of a run counts the same context once per tool call, so
-- it cannot tell how close a session is to its limit. 0 means unknown.
ALTER TABLE agent_run
    ADD COLUMN context_tokens bigint NOT NULL DEFAULT 0 CHECK (context_tokens >= 0);

-- +goose Down
ALTER TABLE agent_run DROP COLUMN IF EXISTS context_tokens;
