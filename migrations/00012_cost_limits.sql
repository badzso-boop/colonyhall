-- +goose Up
ALTER TABLE agent_host ADD COLUMN max_session_tokens int NOT NULL DEFAULT 150000;
ALTER TABLE agent_host ADD COLUMN daily_token_limit bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE agent_host DROP COLUMN daily_token_limit;
ALTER TABLE agent_host DROP COLUMN max_session_tokens;
