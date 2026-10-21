-- +goose Up
ALTER TABLE channel ADD COLUMN description VARCHAR(250) NOT NULL DEFAULT '';
ALTER TABLE channel_member ADD COLUMN left_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE channel_member DROP COLUMN IF EXISTS left_at;
ALTER TABLE channel DROP COLUMN IF EXISTS description;
