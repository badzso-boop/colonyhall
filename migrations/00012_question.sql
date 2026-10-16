-- +goose Up
CREATE TABLE question (
    id UUID PRIMARY KEY,
    number INT NOT NULL,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    asked_by UUID NOT NULL REFERENCES actor(id),
    to_actor_id UUID REFERENCES actor(id),
    to_human BOOLEAN NOT NULL DEFAULT false,
    channel_id UUID NOT NULL REFERENCES channel(id),
    thread_root_id UUID NOT NULL REFERENCES message(id),
    text TEXT NOT NULL,
    options TEXT[] NOT NULL DEFAULT '{}',
    default_option TEXT,
    deadline TIMESTAMPTZ,
    state VARCHAR(20) NOT NULL DEFAULT 'open',
    answer TEXT,
    answered_by UUID REFERENCES actor(id),
    created_at TIMESTAMPTZ NOT NULL,
    answered_at TIMESTAMPTZ,
    escalated_at TIMESTAMPTZ,
    withdrawn_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    CONSTRAINT question_workspace_number_key UNIQUE (workspace_id, number)
);

CREATE INDEX question_workspace_idx ON question(workspace_id);
CREATE INDEX question_workspace_state_idx ON question(workspace_id, state);
CREATE INDEX question_thread_root_idx ON question(thread_root_id);

-- +goose Down
DROP TABLE IF EXISTS question;
