-- name: InsertError :exec
INSERT INTO run_error (id, workspace_id, agent_id, channel_id, thread_root_id, summary, detail, created_at)
VALUES ($1, $2, $3, $4, sqlc.narg(thread_root_id), $5, $6, $7);

-- name: ErrorByID :one
SELECT * FROM run_error WHERE id = $1;
