-- +goose Up
-- Origin channel and thread for agent requests, enabling answers to route back
-- to the origin thread.
ALTER TABLE message
    ADD COLUMN origin_channel_id uuid REFERENCES channel(id),
    ADD COLUMN origin_thread_id  uuid REFERENCES message(id),
    -- The DM reply an answer in the origin thread responds to.
    ADD COLUMN answers_message_id uuid REFERENCES message(id);

CREATE INDEX message_origin_idx ON message (origin_channel_id, origin_thread_id)
    WHERE origin_channel_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS message_origin_idx;
ALTER TABLE message
    DROP COLUMN IF EXISTS origin_channel_id,
    DROP COLUMN IF EXISTS origin_thread_id,
    DROP COLUMN IF EXISTS answers_message_id;
