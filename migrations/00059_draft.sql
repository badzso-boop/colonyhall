-- +goose Up
-- Round 55, part A: unsent messages, one per person, conversation and
-- thread (thread_root_id NULL = the main stream). A draft is not a message:
-- it never reaches agents, search or other people's event streams, and
-- only its owner reads it (the service filters by actor; the tenant policy
-- is the second line). file_ids are uploads not yet sent.
CREATE TABLE draft (
    id             uuid PRIMARY KEY,
    workspace_id   uuid NOT NULL,
    actor_id       uuid NOT NULL,
    channel_id     uuid NOT NULL,
    thread_root_id uuid REFERENCES message(id),
    body           text NOT NULL CHECK (length(body) <= 20000),
    file_ids       uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(file_ids) <= 20),
    updated_at     timestamptz NOT NULL,
    CONSTRAINT draft_channel_ws_fkey FOREIGN KEY (channel_id, workspace_id) REFERENCES channel (id, workspace_id),
    CONSTRAINT draft_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id),
    CONSTRAINT draft_place_key UNIQUE NULLS NOT DISTINCT (actor_id, channel_id, thread_root_id)
);

CREATE INDEX draft_actor_idx ON draft (workspace_id, actor_id, updated_at DESC);

ALTER TABLE draft ENABLE ROW LEVEL SECURITY;
ALTER TABLE draft FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON draft USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS draft;
