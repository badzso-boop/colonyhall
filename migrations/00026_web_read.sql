-- +goose Up
ALTER TABLE agent_host ADD COLUMN allow_web_read boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE agent_host DROP COLUMN IF EXISTS allow_web_read;
