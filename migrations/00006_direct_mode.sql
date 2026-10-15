-- +goose Up
ALTER TABLE agent_host ADD COLUMN mode text NOT NULL DEFAULT 'compact';
ALTER TABLE agent_host ADD COLUMN root_dir text NOT NULL DEFAULT '';
ALTER TABLE agent_host ADD COLUMN sandbox boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE agent_host DROP COLUMN sandbox;
ALTER TABLE agent_host DROP COLUMN root_dir;
ALTER TABLE agent_host DROP COLUMN mode;
