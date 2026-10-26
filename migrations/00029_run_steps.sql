-- +goose Up
ALTER TABLE agent_run ADD COLUMN steps jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE agent_run ADD COLUMN reply_message_id uuid REFERENCES message(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE agent_run DROP COLUMN IF EXISTS reply_message_id;
ALTER TABLE agent_run DROP COLUMN IF EXISTS steps;
