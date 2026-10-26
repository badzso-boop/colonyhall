-- +goose Up
CREATE TABLE terminal_event (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    agent_id        uuid NOT NULL REFERENCES actor(id),
    channel_id      uuid NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
    thread_root_id  uuid,
    name            text NOT NULL,
    exit_code       integer,
    duration_ms     bigint NOT NULL,
    tail            text NOT NULL,
    created_at      timestamptz NOT NULL
);

CREATE INDEX terminal_event_agent_idx ON terminal_event(agent_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS terminal_event;
