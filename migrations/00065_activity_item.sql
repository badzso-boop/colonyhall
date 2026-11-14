-- +goose Up
-- Round 55, part H: the activity feed, one row per person and thing that
-- concerns them (a mention, a reply in their thread, a reaction to their
-- message, an approval or question waiting for them, an agent's important
-- message, a due reminder). Built by an event consumer, so it is
-- independent of the UI; push notifications (round 43) will read it.
-- (recipient_id, source_seq, kind) makes a replayed event a no-op.
CREATE TABLE activity_item (
    id             uuid PRIMARY KEY,
    workspace_id   uuid NOT NULL,
    recipient_id   uuid NOT NULL,
    kind           text NOT NULL CHECK (kind IN ('mention', 'thread_reply', 'reaction', 'approval', 'question', 'important', 'reminder')),
    channel_id     uuid,
    message_id     uuid,
    thread_root_id uuid,
    actor_id       uuid,
    ref_id         uuid,
    emoji          text NOT NULL DEFAULT '' CHECK (length(emoji) <= 64),
    source_seq     bigint NOT NULL,
    created_at     timestamptz NOT NULL,
    read_at        timestamptz,
    CONSTRAINT activity_item_recipient_ws_fkey FOREIGN KEY (recipient_id, workspace_id) REFERENCES actor (id, workspace_id),
    CONSTRAINT activity_item_source_key UNIQUE (recipient_id, source_seq, kind)
);

CREATE INDEX activity_item_feed_idx ON activity_item (workspace_id, recipient_id, created_at DESC, id DESC);
CREATE INDEX activity_item_unread_idx ON activity_item (recipient_id) WHERE read_at IS NULL;

ALTER TABLE activity_item ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_item FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON activity_item USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS activity_item;
