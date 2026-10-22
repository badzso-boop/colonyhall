-- +goose Up
ALTER TABLE agent_run ADD COLUMN web_reads text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE agent_run DROP COLUMN IF EXISTS web_reads;
