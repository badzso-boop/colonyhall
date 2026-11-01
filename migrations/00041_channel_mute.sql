-- +goose Up
-- Round 34: per-viewer muting. It only changes what that human is bothered
-- by, never what agents receive. muted_until NULL = until unmuted; an
-- expired row counts as unmuted (no job removes it).
CREATE TABLE channel_mute (
    actor_id    UUID NOT NULL REFERENCES actor(id),
    channel_id  UUID NOT NULL REFERENCES channel(id),
    muted_until TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (actor_id, channel_id)
);

-- +goose Down
DROP TABLE IF EXISTS channel_mute;
