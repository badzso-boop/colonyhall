-- +goose Up
-- Round 55, part I: a per-conversation notification level, unified with the
-- mute of round 34. A row's level is 'none' (muted, until muted_until or
-- forever: every existing row), 'mentions' or 'all'; no row means the
-- default (all for DMs, mentions for channels). The level decides the
-- viewer's unread badge, the activity view and later push notifications;
-- never what agents receive.
ALTER TABLE channel_mute ADD COLUMN level text NOT NULL DEFAULT 'none'
    CHECK (level IN ('all', 'mentions', 'none'));

-- +goose Down
ALTER TABLE channel_mute DROP COLUMN IF EXISTS level;
