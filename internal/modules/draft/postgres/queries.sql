-- name: UpsertDraft :one
INSERT INTO draft (id, workspace_id, actor_id, channel_id, thread_root_id, body, file_ids, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT ON CONSTRAINT draft_place_key DO UPDATE
    SET body = EXCLUDED.body, file_ids = EXCLUDED.file_ids, updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetDraft :one
SELECT * FROM draft
WHERE actor_id = $1 AND channel_id = $2 AND thread_root_id IS NOT DISTINCT FROM sqlc.narg('thread_root_id')::uuid;

-- name: DeleteDraft :execrows
DELETE FROM draft
WHERE actor_id = $1 AND channel_id = $2 AND thread_root_id IS NOT DISTINCT FROM sqlc.narg('thread_root_id')::uuid
  AND (sqlc.narg('not_after')::timestamptz IS NULL OR updated_at <= sqlc.narg('not_after'));

-- name: ListDrafts :many
SELECT * FROM draft
WHERE workspace_id = $1 AND actor_id = $2
ORDER BY updated_at DESC
LIMIT $3;
