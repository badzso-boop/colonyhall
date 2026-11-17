-- +goose Up
-- Round 56c (docs/25-onfejlesztes-terv.md 13.8, 13.11): routines and
-- interactive messages.

-- An interactive message carries blocks (buttons, a select, a table, text)
-- next to its text body; the body stays for notifications, search and
-- agents. NULL: a plain message.
ALTER TABLE message ADD COLUMN blocks jsonb;

-- A click or a choice on an interactive message (append-only).
CREATE TABLE message_action (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    message_id   uuid NOT NULL REFERENCES message(id),
    actor_id     uuid NOT NULL REFERENCES actor(id),
    action_id    text NOT NULL CHECK (length(action_id) BETWEEN 1 AND 64),
    value        text NOT NULL DEFAULT '' CHECK (length(value) <= 200),
    created_at   timestamptz NOT NULL
);
CREATE INDEX message_action_message_idx ON message_action (message_id, created_at);
ALTER TABLE message_action ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_action FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON message_action USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- A routine's schedule and webhook (the manifest is the source; this is
-- the state). One row per extension and routine name.
CREATE TABLE routine_state (
    id                 uuid PRIMARY KEY,
    workspace_id       uuid NOT NULL REFERENCES workspace(id),
    extension_id       uuid NOT NULL REFERENCES extension(id),
    routine            text NOT NULL,
    next_run_at        timestamptz,
    last_run_at        timestamptz,
    webhook_token_hash bytea,
    UNIQUE (extension_id, routine)
);
CREATE INDEX routine_state_due_idx ON routine_state (next_run_at) WHERE next_run_at IS NOT NULL;
ALTER TABLE routine_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE routine_state FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON routine_state USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- The queue of routine runs: a cron tick, a message, a webhook or a click
-- adds one; the worker runs it outside any transaction.
CREATE TABLE routine_run (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    extension_id uuid NOT NULL REFERENCES extension(id),
    routine      text NOT NULL,
    trigger      text NOT NULL CHECK (trigger IN ('cron', 'message', 'webhook', 'action')),
    input        jsonb NOT NULL,
    state        text NOT NULL CHECK (state IN ('pending', 'running', 'done', 'failed')),
    error        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL,
    started_at   timestamptz,
    finished_at  timestamptz
);
CREATE INDEX routine_run_pending_idx ON routine_run (created_at) WHERE state = 'pending';
CREATE INDEX routine_run_ext_idx ON routine_run (workspace_id, extension_id, created_at DESC);
ALTER TABLE routine_run ENABLE ROW LEVEL SECURITY;
ALTER TABLE routine_run FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON routine_run USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS routine_run;
DROP TABLE IF EXISTS routine_state;
DROP TABLE IF EXISTS message_action;
ALTER TABLE message DROP COLUMN IF EXISTS blocks;
