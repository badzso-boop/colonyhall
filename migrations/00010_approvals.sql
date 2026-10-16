-- +goose Up
-- Approvals from the chat: agents can ask humans for permission before running tools.
CREATE TABLE approval_request (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL REFERENCES workspace(id),
    agent_id         uuid NOT NULL REFERENCES actor(id),
    channel_id       uuid NOT NULL REFERENCES channel(id),
    thread_root_id   uuid,
    tool_name        text NOT NULL,
    input            text NOT NULL,
    state            text NOT NULL,
    decided_by       uuid REFERENCES actor(id),
    decision_message text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL,
    decided_at       timestamptz,
    expires_at       timestamptz NOT NULL
);
CREATE INDEX approval_request_workspace_state_idx ON approval_request (workspace_id, state);
CREATE INDEX approval_request_channel_idx ON approval_request (channel_id, created_at);

ALTER TABLE agent_host ADD COLUMN approvals text NOT NULL DEFAULT 'none';

-- +goose Down
DROP TABLE approval_request;
ALTER TABLE agent_host DROP COLUMN approvals;
