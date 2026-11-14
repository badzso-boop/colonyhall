-- name: InsertFile :exec
INSERT INTO file (id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at, human_only)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: FileByID :one
SELECT *
FROM file
WHERE id = $1;

-- name: FilesByIDs :many
SELECT *
FROM file
WHERE id = ANY($1::uuid[]);

-- name: ListFilesByChannel :many
SELECT *
FROM file
WHERE channel_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: AttachFileToMessage :exec
INSERT INTO message_file (message_id, file_id, position)
VALUES ($1, $2, $3);

-- name: AttachmentsForMessage :many
SELECT f.id, f.name, f.mime, f.size, f.deleted_at
FROM message_file mf
JOIN file f ON f.id = mf.file_id
WHERE mf.message_id = $1
ORDER BY mf.position ASC;

-- name: AttachmentsForMessages :many
SELECT mf.message_id, f.id, f.name, f.mime, f.size, f.deleted_at
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

-- name: FileFetchRequestByAgentAndPath :one
SELECT id, workspace_id, message_id, channel_id, agent_id, requested_by, path, state, file_id, error_code, created_at, completed_at
FROM file_fetch_request
WHERE workspace_id = $1 AND agent_id = $2 AND path = $3
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

-- name: UpdateFileFetchRequestCompleted :execrows
UPDATE file_fetch_request
SET state = 'done', file_id = $2, completed_at = $3
WHERE id = $1 AND state = 'pending';

-- name: UpdateFileFetchRequestFailed :execrows
UPDATE file_fetch_request
SET state = 'failed', error_code = $2, completed_at = $3
WHERE id = $1 AND state = 'pending';

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

-- Round 54: deleting files and the workspace storage.

-- name: MarkFileDeleted :execrows
UPDATE file SET deleted_at = sqlc.arg(at) WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListMyFiles :many
-- A person's (or their agents') live files, newest first, with the message
-- each is attached to (the earliest by ID, UUIDv7: the first sent) and how
-- many of the same uploaders' other live files share its content. Only
-- those: counting everyone's copies would tell the caller whether some
-- other person (in a channel the caller may not read) holds a given file.
SELECT f.id, f.workspace_id, f.uploaded_by, f.channel_id, f.name, f.mime, f.size, f.sha256, f.created_at, f.human_only, f.deleted_at,
       coalesce(mf.message_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS message_id,
       (SELECT count(*) FROM file o
         WHERE o.workspace_id = f.workspace_id AND o.sha256 = f.sha256 AND o.id <> f.id AND o.deleted_at IS NULL
           AND o.uploaded_by = ANY(sqlc.arg(uploaders)::uuid[]))::int AS shared_count
FROM file f
LEFT JOIN LATERAL (SELECT x.message_id FROM message_file x WHERE x.file_id = f.id ORDER BY x.message_id LIMIT 1) mf ON true
WHERE f.workspace_id = sqlc.arg(workspace_id)
  AND f.uploaded_by = ANY(sqlc.arg(uploaders)::uuid[])
  AND f.deleted_at IS NULL
  AND (sqlc.narg(channel_id)::uuid IS NULL OR f.channel_id = sqlc.narg(channel_id)::uuid)
  AND (sqlc.arg(kind)::text = ''
       OR (sqlc.arg(kind)::text = 'image' AND f.mime LIKE 'image/%')
       OR (sqlc.arg(kind)::text = 'document' AND f.mime NOT LIKE 'image/%' AND (f.mime LIKE 'text/%' OR f.mime IN ('application/pdf', 'application/json', 'application/msword', 'application/rtf') OR f.mime LIKE 'application/vnd.%'))
       OR (sqlc.arg(kind)::text = 'other' AND f.mime NOT LIKE 'image/%' AND NOT (f.mime LIKE 'text/%' OR f.mime IN ('application/pdf', 'application/json', 'application/msword', 'application/rtf') OR f.mime LIKE 'application/vnd.%')))
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (f.created_at, f.id) < (sqlc.narg(before_at)::timestamptz, sqlc.arg(before_id)::uuid))
ORDER BY f.created_at DESC, f.id DESC
LIMIT sqlc.arg(max_rows)::int;

-- name: LiveFilesBytes :one
SELECT coalesce(sum(size), 0)::bigint FROM (
    SELECT DISTINCT ON (sha256) size FROM file WHERE workspace_id = $1 AND deleted_at IS NULL
) t;

-- name: WorkspaceHasLiveContent :one
SELECT EXISTS (SELECT 1 FROM file WHERE workspace_id = $1 AND sha256 = $2 AND deleted_at IS NULL);

-- name: EnsureWorkspaceStorage :exec
INSERT INTO workspace_storage (workspace_id, updated_at) VALUES ($1, $2) ON CONFLICT (workspace_id) DO NOTHING;

-- name: LockWorkspaceStorage :one
SELECT * FROM workspace_storage WHERE workspace_id = $1 FOR UPDATE;

-- name: WorkspaceStorage :one
SELECT * FROM workspace_storage WHERE workspace_id = $1;

-- name: SaveWorkspaceStorage :exec
INSERT INTO workspace_storage (workspace_id, quota_bytes, files_bytes, db_bytes, chat_bytes, db_measured_at, warned_level, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (workspace_id) DO UPDATE SET
    quota_bytes = EXCLUDED.quota_bytes,
    files_bytes = EXCLUDED.files_bytes,
    db_bytes = EXCLUDED.db_bytes,
    chat_bytes = EXCLUDED.chat_bytes,
    db_measured_at = EXCLUDED.db_measured_at,
    warned_level = EXCLUDED.warned_level,
    updated_at = EXCLUDED.updated_at;

-- name: LockFileContent :exec
-- One lock per content for every server: an upload holds it from the row
-- insert to the content write, the sweep from the check to the removal.
SELECT pg_advisory_xact_lock(hashtextextended('colonyhall.file_content:' || sqlc.arg(sha256)::text, 0));

-- name: FileContentLive :one
SELECT EXISTS (SELECT 1 FROM file WHERE sha256 = $1 AND deleted_at IS NULL);

-- name: RemovableFileContents :many
SELECT DISTINCT sha256 FROM file
WHERE deleted_at IS NOT NULL AND content_removed_at IS NULL
LIMIT sqlc.arg(max_rows)::int;

-- name: MarkFileContentRemoved :exec
UPDATE file SET content_removed_at = sqlc.arg(at)
WHERE sha256 = sqlc.arg(sha256) AND deleted_at IS NOT NULL AND content_removed_at IS NULL;
