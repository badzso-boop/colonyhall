-- +goose Up
ALTER TABLE file ADD COLUMN human_only BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE file_fetch_request (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    message_id UUID NOT NULL REFERENCES message(id),
    channel_id UUID NOT NULL REFERENCES channel(id),
    agent_id UUID NOT NULL REFERENCES actor(id),
    requested_by UUID NOT NULL REFERENCES actor(id),
    path TEXT NOT NULL,
    state VARCHAR(20) NOT NULL, -- pending, done, failed
    file_id UUID REFERENCES file(id),
    error_code VARCHAR(50), -- host_offline, not_found, denied, too_large, not_a_file, timeout
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE INDEX file_fetch_request_lookup_idx ON file_fetch_request(workspace_id, message_id, path);
CREATE INDEX file_fetch_request_agent_idx ON file_fetch_request(workspace_id, agent_id, state);
CREATE INDEX file_fetch_request_pending_idx ON file_fetch_request(state) WHERE state = 'pending';

CREATE TABLE workspace_file_fetch_settings (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id),
    max_image_mb INT NOT NULL DEFAULT 200 CHECK (max_image_mb >= 1 AND max_image_mb <= 2048),
    max_file_mb INT NOT NULL DEFAULT 100 CHECK (max_file_mb >= 1 AND max_file_mb <= 2048),
    updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS workspace_file_fetch_settings;
DROP TABLE IF EXISTS file_fetch_request;
ALTER TABLE file DROP COLUMN IF EXISTS human_only;
