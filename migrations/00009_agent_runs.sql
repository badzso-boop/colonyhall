-- +goose Up
-- Append-only log of agent runs, tokens, and durations.
CREATE TABLE agent_run (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL REFERENCES workspace(id),
    agent_id            uuid NOT NULL REFERENCES actor(id),
    host_id             uuid REFERENCES host(id),
    phase               text NOT NULL,
    client              text NOT NULL,
    model               text NOT NULL,
    session_id          text NOT NULL,
    channel_id          uuid REFERENCES channel(id),
    thread_root_id      uuid,
    items               integer NOT NULL,
    input_tokens        bigint NOT NULL,
    output_tokens       bigint NOT NULL,
    cache_read_tokens   bigint NOT NULL,
    cache_write_tokens  bigint NOT NULL,
    duration_ms         integer NOT NULL,
    status              text NOT NULL,
    error_id            uuid REFERENCES run_error(id),
    created_at          timestamptz NOT NULL
);
CREATE INDEX agent_run_lookup_idx ON agent_run (workspace_id, agent_id, created_at);

-- +goose Down
DROP TABLE agent_run;
