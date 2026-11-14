-- +goose Up
-- Round 55, part E: a person's saved messages and reminders. Personal:
-- only the owner reads them (the service filters by actor; the tenant
-- policy is the second line). message_id is NULL for a reminder made with
-- /remind <when> <text>. A due reminder is claimed by setting reminded_at
-- in the transaction that posts it, so it is sent once even with several
-- server instances.
CREATE TABLE saved_item (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    actor_id     uuid NOT NULL,
    message_id   uuid REFERENCES message(id),
    channel_id   uuid,
    note         text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    remind_at    timestamptz,
    reminded_at  timestamptz,
    done_at      timestamptz,
    created_at   timestamptz NOT NULL,
    CONSTRAINT saved_item_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id),
    CONSTRAINT saved_item_channel_ws_fkey FOREIGN KEY (channel_id, workspace_id) REFERENCES channel (id, workspace_id),
    CONSTRAINT saved_item_has_something CHECK (message_id IS NOT NULL OR remind_at IS NOT NULL)
);

CREATE UNIQUE INDEX saved_item_message_idx ON saved_item (actor_id, message_id) WHERE message_id IS NOT NULL;
CREATE INDEX saved_item_actor_idx ON saved_item (workspace_id, actor_id, created_at DESC);
CREATE INDEX saved_item_due_idx ON saved_item (remind_at) WHERE reminded_at IS NULL AND done_at IS NULL AND remind_at IS NOT NULL;

ALTER TABLE saved_item ENABLE ROW LEVEL SECURITY;
ALTER TABLE saved_item FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON saved_item USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS saved_item;
