-- +goose Up
-- Scheduled messages: at the given time the server posts the message in
-- the creator's name, so the mentioned agent wakes up as if the person had
-- written it then. One-off (cron = '') or repeating (cron expression).
CREATE TABLE schedule (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    created_by      uuid NOT NULL REFERENCES actor(id),
    channel_id      uuid NOT NULL REFERENCES channel(id),
    thread_root_id  uuid,
    body            text NOT NULL,
    cron            text NOT NULL,
    timezone        text NOT NULL,
    catch_up        text NOT NULL,          -- once | skip
    state           text NOT NULL,          -- active | done | canceled
    next_run_at     timestamptz NOT NULL,
    last_run_at     timestamptz,
    last_message_id uuid,
    last_error      text NOT NULL DEFAULT '',
    run_count       integer NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL
);
CREATE INDEX schedule_due_idx ON schedule (next_run_at) WHERE state = 'active';

-- +goose Down
DROP TABLE schedule;
