-- +goose Up
-- Round 45: a workspace may use its own Gitea instead of the server's
-- shared one (docs/23-elkulonites.md, 12/4). When enabled, its links and
-- statuses are fetched from it and its webhook comes to the workspace's
-- own subdomain with its own secret; the shared webhook skips it.
-- The token is stored as given until the secret store of phase 5: only a
-- read-only token may be set. It is never returned by the API.
CREATE TABLE workspace_gitea (
    workspace_id    uuid PRIMARY KEY REFERENCES workspace (id),
    enabled         boolean NOT NULL,
    url             text NOT NULL,
    public_url      text NOT NULL DEFAULT '',
    token           text NOT NULL,
    webhook_secret  text NOT NULL,
    updated_at      timestamptz NOT NULL
);
ALTER TABLE workspace_gitea ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_gitea FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON workspace_gitea USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE workspace_gitea;
