-- +goose Up
ALTER TABLE agent_host ADD COLUMN worktree_repo text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agent_host DROP COLUMN worktree_repo;
