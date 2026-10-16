-- +goose Up
ALTER TABLE agent_host ADD COLUMN allow_preview BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE preview (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    agent_id UUID NOT NULL REFERENCES actor(id),
    host_id UUID NOT NULL REFERENCES host(id),
    channel_id UUID NOT NULL REFERENCES channel(id),
    thread_root_id UUID REFERENCES message(id),
    name TEXT NOT NULL,
    compose_path TEXT NOT NULL,
    state VARCHAR(20) NOT NULL DEFAULT 'requested',
    url TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT preview_state_check CHECK (state IN ('requested', 'running', 'stopped', 'failed'))
);

CREATE UNIQUE INDEX preview_agent_active_name_idx ON preview (agent_id, name) WHERE state IN ('requested', 'running');
CREATE INDEX preview_workspace_idx ON preview(workspace_id);
CREATE INDEX preview_host_idx ON preview(host_id);
CREATE INDEX preview_agent_idx ON preview(agent_id);
CREATE INDEX preview_state_idx ON preview(state);

-- +goose Down
DROP TABLE IF EXISTS preview;
ALTER TABLE agent_host DROP COLUMN IF EXISTS allow_preview;
