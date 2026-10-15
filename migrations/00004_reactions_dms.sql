-- +goose Up
-- Reactions (soft-removed, never deleted) and the sender of inbox items.

CREATE TABLE reaction (
    message_id  uuid NOT NULL REFERENCES message(id),
    actor_id    uuid NOT NULL REFERENCES actor(id),
    emoji       text NOT NULL,
    created_at  timestamptz NOT NULL,
    removed_at  timestamptz,
    PRIMARY KEY (message_id, actor_id, emoji)
);

-- Who caused the inbox item (message author, reactor, …).
ALTER TABLE delivery ADD COLUMN from_actor_id uuid REFERENCES actor(id);

-- +goose Down
ALTER TABLE delivery DROP COLUMN from_actor_id;
DROP TABLE reaction;
