-- +goose Up
-- Round 35: in a locked channel agents cannot write and are not woken; humans
-- keep talking. DMs are never locked (the service refuses it).
ALTER TABLE channel
    ADD COLUMN agent_locked_at TIMESTAMPTZ,
    ADD COLUMN agent_locked_by UUID REFERENCES actor(id);

CREATE INDEX channel_agent_locked ON channel (workspace_id) WHERE agent_locked_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS channel_agent_locked;
ALTER TABLE channel DROP COLUMN IF EXISTS agent_locked_by, DROP COLUMN IF EXISTS agent_locked_at;
