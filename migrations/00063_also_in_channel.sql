-- +goose Up
-- Round 55, part G: a thread reply that is also shown in the channel's main
-- stream ("also send to #channel"). One message with a flag, not a copy.
ALTER TABLE message ADD COLUMN also_in_channel boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE message DROP COLUMN IF EXISTS also_in_channel;
