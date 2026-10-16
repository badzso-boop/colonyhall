-- +goose Up
CREATE TABLE git_link (
    url VARCHAR PRIMARY KEY,
    kind VARCHAR(20) NOT NULL,
    title TEXT NOT NULL,
    state VARCHAR(50) NOT NULL,
    author VARCHAR(100) NOT NULL DEFAULT '',
    fetched_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX git_link_fetched_at_idx ON git_link(fetched_at);

CREATE TABLE git_repo_channel (
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    repo VARCHAR(150) NOT NULL,
    channel_id UUID NOT NULL REFERENCES channel(id),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (workspace_id, repo)
);

CREATE INDEX git_repo_channel_channel_idx ON git_repo_channel(channel_id);

CREATE TABLE git_webhook_delivery (
    delivery_id VARCHAR(128) PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS git_webhook_delivery;
DROP TABLE IF EXISTS git_repo_channel;
DROP TABLE IF EXISTS git_link;
