-- +goose Up
CREATE TABLE file (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    uploaded_by UUID NOT NULL REFERENCES actor(id),
    channel_id UUID NOT NULL REFERENCES channel(id),
    name VARCHAR(200) NOT NULL,
    mime VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL,
    sha256 VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX file_workspace_idx ON file(workspace_id);
CREATE INDEX file_channel_idx ON file(channel_id);
CREATE INDEX file_sha256_idx ON file(sha256);

CREATE TABLE message_file (
    message_id UUID NOT NULL REFERENCES message(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES file(id),
    position INT NOT NULL DEFAULT 0,
    PRIMARY KEY (message_id, file_id)
);

CREATE INDEX message_file_file_id_idx ON message_file(file_id);

-- +goose Down
DROP TABLE IF EXISTS message_file;
DROP TABLE IF EXISTS file;
