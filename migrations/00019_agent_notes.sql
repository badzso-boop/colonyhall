-- +goose Up
CREATE TABLE agent_note (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    agent_id UUID NOT NULL REFERENCES actor(id),
    thread_root_id UUID NOT NULL REFERENCES message(id),
    text VARCHAR(2000) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX agent_note_lookup_idx ON agent_note(workspace_id, agent_id, thread_root_id, created_at DESC);
CREATE INDEX agent_note_list_idx ON agent_note(workspace_id, agent_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS agent_note;
