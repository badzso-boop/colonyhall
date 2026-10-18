-- +goose Up
CREATE TABLE decision (
    id UUID PRIMARY KEY,
    number INT NOT NULL,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    author_id UUID NOT NULL REFERENCES actor(id),
    channel_id UUID NOT NULL REFERENCES channel(id),
    thread_root_id UUID REFERENCES message(id),
    text VARCHAR(300) NOT NULL,
    rationale VARCHAR(500) NOT NULL,
    supersedes INT,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT decision_workspace_number_key UNIQUE (workspace_id, number)
);

CREATE INDEX decision_workspace_created_at_idx ON decision(workspace_id, created_at DESC);
CREATE INDEX decision_workspace_supersedes_idx ON decision(workspace_id, supersedes) WHERE supersedes IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS decision;
