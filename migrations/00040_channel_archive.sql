-- +goose Up
-- Round 34: an archived channel is read-only for everyone; members and
-- history stay. DMs are never archived (the service refuses it).
ALTER TABLE channel
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD COLUMN archived_by UUID REFERENCES actor(id);

-- +goose Down
ALTER TABLE channel DROP COLUMN IF EXISTS archived_by, DROP COLUMN IF EXISTS archived_at;
