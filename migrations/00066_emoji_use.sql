-- +goose Up
-- Round 55, part C: the emoji picker's "recent" and "frequent", per person.
-- One row per person and emoji; the service keeps each person's newest 100.
CREATE TABLE emoji_use (
    workspace_id uuid NOT NULL,
    actor_id     uuid NOT NULL,
    emoji        text NOT NULL CHECK (length(emoji) <= 64),
    uses         integer NOT NULL CHECK (uses > 0),
    last_used_at timestamptz NOT NULL,
    PRIMARY KEY (actor_id, emoji),
    CONSTRAINT emoji_use_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id)
);

CREATE INDEX emoji_use_recent_idx ON emoji_use (actor_id, last_used_at DESC);

ALTER TABLE emoji_use ENABLE ROW LEVEL SECURITY;
ALTER TABLE emoji_use FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON emoji_use USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS emoji_use;
