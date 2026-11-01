-- +goose Up
-- Immutable agents (round 33): deleting an agent archives it. Its messages
-- keep pointing at it, and its handle becomes free for a new agent.
ALTER TABLE actor
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD COLUMN archived_by UUID REFERENCES actor(id);
ALTER TABLE actor DROP CONSTRAINT actor_workspace_id_handle_key;
CREATE UNIQUE INDEX actor_handle_active ON actor (workspace_id, handle) WHERE archived_at IS NULL;

-- An agent's own instructions, written once at creation. `version` is for
-- the later version picker; nothing adds a second version yet.
CREATE TABLE agent_instructions (
    agent_id     UUID NOT NULL REFERENCES actor(id),
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    version      INT NOT NULL DEFAULT 1,
    text         TEXT NOT NULL CHECK (char_length(text) <= 8000),
    created_by   UUID NOT NULL REFERENCES actor(id),
    created_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (agent_id, version)
);

-- +goose Down
DROP TABLE IF EXISTS agent_instructions;
DROP INDEX IF EXISTS actor_handle_active;
ALTER TABLE actor ADD CONSTRAINT actor_workspace_id_handle_key UNIQUE (workspace_id, handle);
ALTER TABLE actor DROP COLUMN IF EXISTS archived_by, DROP COLUMN IF EXISTS archived_at;
