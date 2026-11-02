-- name: InsertEntry :exec
INSERT INTO audit_log (seq, workspace_id, actor_id, action, target_type, target_id, payload, at, prev_hash, hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (seq) DO NOTHING;

-- name: LastEntry :one
SELECT * FROM audit_log WHERE workspace_id = $1 ORDER BY seq DESC LIMIT 1;

-- name: PageEntries :many
SELECT * FROM audit_log
WHERE workspace_id = sqlc.arg(workspace_id) AND (sqlc.arg(before)::bigint = 0 OR seq < sqlc.arg(before)::bigint)
ORDER BY seq DESC LIMIT sqlc.arg(page_size);

-- name: ForwardEntries :many
SELECT * FROM audit_log WHERE workspace_id = $1 AND seq > $2 ORDER BY seq LIMIT $3;

-- name: SearchEntries :many
SELECT * FROM audit_log
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.arg(before)::bigint = 0 OR seq < sqlc.arg(before)::bigint)
  AND (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id)::uuid)
  AND (sqlc.arg(actions)::text[] = '{}' OR action = ANY(sqlc.arg(actions)::text[]))
  AND (sqlc.arg(target_type)::text = '' OR target_type = sqlc.arg(target_type)::text)
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR at < sqlc.narg(to_at)::timestamptz)
ORDER BY seq DESC
LIMIT sqlc.arg(row_limit);

-- name: CountEntries :one
SELECT count(*) FROM audit_log
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id)::uuid)
  AND (sqlc.arg(actions)::text[] = '{}' OR action = ANY(sqlc.arg(actions)::text[]))
  AND (sqlc.arg(target_type)::text = '' OR target_type = sqlc.arg(target_type)::text)
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR at < sqlc.narg(to_at)::timestamptz);

-- name: DistinctActions :many
SELECT DISTINCT action FROM audit_log WHERE workspace_id = $1 ORDER BY action;
