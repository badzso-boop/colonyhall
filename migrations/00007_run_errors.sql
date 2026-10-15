-- +goose Up
-- Error details of agent runs. The chat only says "I ran into an error"
-- with a reference; the details are for humans (append-only).
CREATE TABLE run_error (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    agent_id        uuid NOT NULL REFERENCES actor(id),
    channel_id      uuid NOT NULL REFERENCES channel(id),
    thread_root_id  uuid,
    summary         text NOT NULL,
    detail          text NOT NULL,
    created_at      timestamptz NOT NULL
);
CREATE INDEX run_error_agent_idx ON run_error (agent_id, created_at);

-- +goose Down
DROP TABLE run_error;
