-- +goose Up
ALTER TABLE file_fetch_request ALTER COLUMN message_id DROP NOT NULL;
CREATE INDEX IF NOT EXISTS file_fetch_request_agent_path_idx ON file_fetch_request(workspace_id, agent_id, path);

-- +goose Down
DROP INDEX IF EXISTS file_fetch_request_agent_path_idx;
ALTER TABLE file_fetch_request ALTER COLUMN message_id SET NOT NULL;
