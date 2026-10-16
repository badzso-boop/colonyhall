-- name: InsertFile :exec
INSERT INTO file (id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: FileByID :one
SELECT id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at
FROM file
WHERE id = $1;

-- name: FilesByIDs :many
SELECT id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at
FROM file
WHERE id = ANY($1::uuid[]);

-- name: ListFilesByChannel :many
SELECT id, workspace_id, uploaded_by, channel_id, name, mime, size, sha256, created_at
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
