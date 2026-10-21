-- +goose Up
ALTER TABLE actor
    ADD COLUMN description VARCHAR(300) NOT NULL DEFAULT '',
    ADD COLUMN avatar_emoji VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT 'Europe/Budapest',
    ADD COLUMN status_text VARCHAR(80) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE actor
    DROP COLUMN IF EXISTS status_text,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS avatar_emoji,
    DROP COLUMN IF EXISTS description;
