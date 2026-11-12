-- name: GetGitLink :one
SELECT url, kind, title, state, author, fetched_at
FROM git_link
WHERE workspace_id = $1 AND url = $2;

-- name: GetGitLinks :many
SELECT url, kind, title, state, author, fetched_at
FROM git_link
WHERE workspace_id = sqlc.arg(workspace_id) AND url = ANY(sqlc.arg(urls)::text[]);

-- name: UpsertGitLink :exec
INSERT INTO git_link (workspace_id, url, kind, title, state, author, fetched_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (workspace_id, url) DO UPDATE
SET kind = EXCLUDED.kind,
    title = EXCLUDED.title,
    state = EXCLUDED.state,
    author = EXCLUDED.author,
    fetched_at = EXCLUDED.fetched_at;

-- name: ListGitRepoChannels :many
SELECT workspace_id, repo, channel_id, created_at
FROM git_repo_channel
WHERE workspace_id = $1
ORDER BY repo ASC;

-- name: GetGitRepoChannel :one
SELECT workspace_id, repo, channel_id, created_at
FROM git_repo_channel
WHERE workspace_id = $1 AND repo = $2;

-- name: UpsertGitRepoChannel :exec
INSERT INTO git_repo_channel (workspace_id, repo, channel_id, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, repo) DO UPDATE
SET channel_id = EXCLUDED.channel_id,
    created_at = EXCLUDED.created_at;

-- name: DeleteGitRepoChannel :exec
DELETE FROM git_repo_channel
WHERE workspace_id = $1 AND repo = $2;

-- name: InsertWebhookDelivery :one
-- Round 45: webhooks arrive before any workspace is known; the function
-- (migration 00053) records the id in system mode for this call only.
SELECT colony_record_webhook_delivery(sqlc.arg(delivery_id)::text, sqlc.arg(received_at)::timestamptz)::boolean AS recorded;

-- name: GetWebhookDelivery :one
SELECT delivery_id, received_at
FROM git_webhook_delivery
WHERE delivery_id = $1;

-- name: GetGitRepoChannelsByRepo :many
-- Every workspace's mapping of the repository (the webhook fans out).
SELECT r.workspace_id::uuid AS workspace_id, r.repo::text AS repo, r.channel_id::uuid AS channel_id, r.created_at::timestamptz AS created_at
FROM colony_repo_channels(sqlc.arg(repo)::text) AS r;


-- name: GetWorkspaceGitea :one
SELECT * FROM workspace_gitea WHERE workspace_id = $1;

-- name: SetWorkspaceGitea :exec
INSERT INTO workspace_gitea (workspace_id, enabled, url, public_url, token, webhook_secret, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (workspace_id) DO UPDATE
SET enabled = EXCLUDED.enabled, url = EXCLUDED.url, public_url = EXCLUDED.public_url,
    token = EXCLUDED.token, webhook_secret = EXCLUDED.webhook_secret, updated_at = EXCLUDED.updated_at;
