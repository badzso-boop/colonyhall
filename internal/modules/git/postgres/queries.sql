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

-- name: InsertWebhookDelivery :execrows
INSERT INTO git_webhook_delivery (delivery_id, received_at)
VALUES ($1, $2)
ON CONFLICT (delivery_id) DO NOTHING;

-- name: GetWebhookDelivery :one
SELECT delivery_id, received_at
FROM git_webhook_delivery
WHERE delivery_id = $1;

-- name: GetGitRepoChannelsByRepo :many
SELECT workspace_id, repo, channel_id, created_at
FROM git_repo_channel
WHERE repo = $1;

