-- +goose Up
-- Round 55, part D: pinned messages per conversation, and the "important"
-- mark an agent can set when it sends (MCP send important: true). A pin is
-- never deleted: unpinning marks the row, so the agent pin limit counts
-- every pin of the last day and the history stays for the audit.
ALTER TABLE message ADD COLUMN important boolean NOT NULL DEFAULT false;

CREATE TABLE message_pin (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    channel_id   uuid NOT NULL,
    message_id   uuid NOT NULL REFERENCES message(id),
    pinned_by    uuid NOT NULL,
    pinned_at    timestamptz NOT NULL,
    unpinned_by  uuid,
    unpinned_at  timestamptz,
    CONSTRAINT message_pin_channel_ws_fkey FOREIGN KEY (channel_id, workspace_id) REFERENCES channel (id, workspace_id),
    CONSTRAINT message_pin_actor_ws_fkey FOREIGN KEY (pinned_by, workspace_id) REFERENCES actor (id, workspace_id)
);

CREATE UNIQUE INDEX message_pin_active_idx ON message_pin (message_id) WHERE unpinned_at IS NULL;
CREATE INDEX message_pin_channel_idx ON message_pin (channel_id, pinned_at DESC) WHERE unpinned_at IS NULL;
CREATE INDEX message_pin_by_idx ON message_pin (channel_id, pinned_by, pinned_at);

ALTER TABLE message_pin ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_pin FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON message_pin USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS message_pin;
ALTER TABLE message DROP COLUMN IF EXISTS important;
