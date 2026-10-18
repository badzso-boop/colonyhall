-- name: InsertNote :one
INSERT INTO agent_note (id, workspace_id, agent_id, thread_root_id, text, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetLatestNote :one
SELECT * FROM agent_note
WHERE workspace_id = $1 AND agent_id = $2 AND thread_root_id = $3
ORDER BY created_at DESC
LIMIT 1;

-- name: ListNotesByAgent :many
SELECT * FROM agent_note
WHERE workspace_id = $1 AND agent_id = $2
  AND (sqlc.narg('thread_root_id')::uuid IS NULL OR thread_root_id = sqlc.narg('thread_root_id'))
ORDER BY created_at DESC;
