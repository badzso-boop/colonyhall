-- +goose Up
-- Soft retract of a message: author (humans only) can retract their message.
-- deleted_at timestamptz was defined in 00001; this adds deleted_by.
ALTER TABLE message
    ADD COLUMN deleted_by uuid REFERENCES actor(id);

CREATE INDEX message_deleted_at_idx ON message (deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS message_deleted_at_idx;
ALTER TABLE message
    DROP COLUMN IF EXISTS deleted_by;
