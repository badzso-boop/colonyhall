-- name: InsertFile :exec
INSERT INTO file (id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at, human_only)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: FileByID :one
SELECT id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at, human_only
FROM file
WHERE id = $1;

-- name: FilesByIDs :many
SELECT id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at, human_only
FROM file
WHERE id = ANY($1::uuid[]);

-- name: ListFilesByChannel :many
SELECT id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at, human_only
FROM file
WHERE channel_id = $1
ORDER BY created_at DESC;

-- name: AttachFileToMessage :exec
INSERT INTO message_file (message_id, file_id, position)
VALUES ($1, $2, $3);

-- name: AttachmentsForMessage :many
SELECT f.id, f.name, f.mime, f.size
FROM message_file mf
JOIN file f ON f.id = mf.file_id
WHERE mf.message_id = $1
ORDER BY mf.position ASC;

-- name: AttachmentsForMessages :many
SELECT mf.message_id, f.id, f.name, f.mime, f.size
FROM message_file mf
JOIN file f ON f.id = mf.file_id
WHERE mf.message_id = ANY($1::uuid[])
ORDER BY mf.message_id, mf.position ASC;

-- name: InsertFileFetchRequest :exec
INSERT INTO file_fetch_request (
    id, workspace_id, message_id, channel_id, agent_id, requested_by, path, state, file_id, error_code, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
);

-- name: FileFetchRequestByID :one
SELECT id, workspace_id, message_id, channel_id, agent_id, requested_by, path, state, file_id, error_code, created_at, completed_at
FROM file_fetch_request
WHERE id = $1;

-- name: FileFetchRequestByMessageAndPath :one
SELECT id, workspace_id, message_id, channel_id, agent_id, requested_by, path, state, file_id, error_code, created_at, completed_at
FROM file_fetch_request
WHERE workspace_id = $1 AND message_id = $2 AND path = $3
ORDER BY created_at DESC
LIMIT 1;

-- name: ListFileFetchRequestsByAgent :many
SELECT id, workspace_id, message_id, channel_id, agent_id, requested_by, path, state, file_id, error_code, created_at, completed_at
FROM file_fetch_request
WHERE workspace_id = $1 AND agent_id = $2 AND state = $3
ORDER BY created_at ASC;

-- name: ListPendingFileFetchRequestsBefore :many
SELECT id, workspace_id, message_id, channel_id, agent_id, requested_by, path, state, file_id, error_code, created_at, completed_at
FROM file_fetch_request
WHERE state = 'pending' AND created_at <= $1
ORDER BY created_at ASC;

-- name: UpdateFileFetchRequestCompleted :exec
UPDATE file_fetch_request
SET state = 'done', file_id = $2, completed_at = $3
WHERE id = $1;

-- name: UpdateFileFetchRequestFailed :exec
UPDATE file_fetch_request
SET state = 'failed', error_code = $2, completed_at = $3
WHERE id = $1;

-- name: GetFileFetchSettings :one
SELECT workspace_id, max_image_mb, max_file_mb, updated_at
FROM workspace_file_fetch_settings
WHERE workspace_id = $1;

-- name: UpsertFileFetchSettings :exec
INSERT INTO workspace_file_fetch_settings (workspace_id, max_image_mb, max_file_mb, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id) DO UPDATE SET
    max_image_mb = EXCLUDED.max_image_mb,
    max_file_mb = EXCLUDED.max_file_mb,
    updated_at = EXCLUDED.updated_at;
