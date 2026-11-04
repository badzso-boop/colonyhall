-- +goose Up
-- A docs-only planner agent (round 38): write_scope 'docs' lets a
-- full-access Claude Code agent write only Markdown; partner_id is the
-- sandboxed agent it hands code changes to. Both are set at creation and
-- never changed (the agent configuration is immutable).
ALTER TABLE agent_host ADD COLUMN write_scope text NOT NULL DEFAULT 'all'
    CHECK (write_scope IN ('all', 'docs'));
ALTER TABLE agent_host ADD COLUMN partner_id uuid;

-- +goose Down
ALTER TABLE agent_host DROP COLUMN IF EXISTS partner_id;
ALTER TABLE agent_host DROP COLUMN IF EXISTS write_scope;
