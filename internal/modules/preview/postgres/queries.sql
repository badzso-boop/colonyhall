-- name: CreatePreview :exec
INSERT INTO preview (
    id, workspace_id, agent_id, host_id, channel_id, thread_root_id,
    name, compose_path, state, url, error, expires_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
);

-- name: GetPreviewByID :one
SELECT * FROM preview WHERE id = $1;

-- name: ListPreviewsByWorkspace :many
SELECT * FROM preview
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: ListPreviewsByAgent :many
SELECT * FROM preview
WHERE workspace_id = $1 AND agent_id = $2
ORDER BY created_at DESC;

-- name: GetActivePreviewByAgentAndName :one
SELECT * FROM preview
WHERE agent_id = $1 AND name = $2 AND state IN ('requested', 'running')
LIMIT 1;

-- name: ListActivePreviewsByHost :many
SELECT * FROM preview
WHERE host_id = $1 AND state IN ('requested', 'running')
ORDER BY created_at ASC;

-- name: UpdatePreviewStatus :execrows
UPDATE preview
SET state = $2, url = $3, error = $4, updated_at = $5
WHERE id = $1;

-- name: StopActivePreview :execrows
UPDATE preview
SET state = 'stopped', updated_at = $2
WHERE id = $1 AND state IN ('requested', 'running');

-- name: StopActivePreviewsByAgent :execrows
UPDATE preview
SET state = 'stopped', updated_at = $2
WHERE agent_id = $1 AND state IN ('requested', 'running');

-- name: StopActivePreviewsByWorkspace :execrows
UPDATE preview
SET state = 'stopped', updated_at = $2
WHERE workspace_id = $1 AND state IN ('requested', 'running');
