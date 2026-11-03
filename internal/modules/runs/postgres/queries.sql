-- name: InsertError :exec
INSERT INTO run_error (id, workspace_id, agent_id, channel_id, thread_root_id, summary, detail, created_at)
VALUES ($1, $2, $3, $4, sqlc.narg(thread_root_id), $5, $6, $7);

-- name: ErrorByID :one
SELECT * FROM run_error WHERE id = $1;

-- name: InsertRun :exec
INSERT INTO agent_run (
    id, workspace_id, agent_id, host_id, phase, client, model, session_id,
    channel_id, thread_root_id, items, input_tokens, output_tokens,
    cache_read_tokens, cache_write_tokens, duration_ms, status, error_id, created_at,
    web_reads, steps, reply_message_id, context_tokens, instructions_version
) VALUES (
    $1, $2, $3, sqlc.narg(host_id), $4, $5, $6, $7,
    sqlc.narg(channel_id), sqlc.narg(thread_root_id), $8, $9, $10,
    $11, $12, $13, $14, sqlc.narg(error_id), $15,
    $16, $17, sqlc.narg(reply_message_id), $18, sqlc.narg(instructions_version)
);

-- name: ListRuns :many
SELECT * FROM agent_run
WHERE workspace_id = $1
  AND (sqlc.narg(agent_id)::uuid IS NULL OR agent_id = sqlc.narg(agent_id)::uuid)
ORDER BY created_at DESC
LIMIT $2;

-- name: Usage :many
SELECT
    agent_id,
    TO_CHAR(created_at AT TIME ZONE 'Europe/Budapest', 'YYYY-MM-DD')::text AS day,
    COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
    COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(cache_read_tokens), 0)::bigint AS cache_read_tokens,
    COALESCE(SUM(cache_write_tokens), 0)::bigint AS cache_write_tokens,
    COUNT(*)::int AS run_count,
    COUNT(*) FILTER (WHERE status = 'error')::int AS error_count
FROM agent_run
WHERE workspace_id = $1
  AND created_at >= $2
GROUP BY agent_id, day
ORDER BY day DESC, agent_id ASC;

-- name: AgentTokensSince :one
SELECT COALESCE(SUM(input_tokens + output_tokens + cache_read_tokens + cache_write_tokens), 0)::bigint AS total_tokens
FROM agent_run
WHERE workspace_id = $1
  AND agent_id = $2
  AND created_at >= $3;


-- name: SearchRuns :many
SELECT * FROM agent_run
WHERE workspace_id = $1
  AND (sqlc.narg(agent_id)::uuid IS NULL OR agent_id = sqlc.narg(agent_id)::uuid)
  AND (sqlc.narg(host_id)::uuid IS NULL OR host_id = sqlc.narg(host_id)::uuid)
  AND (sqlc.narg(channel_id)::uuid IS NULL OR channel_id = sqlc.narg(channel_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at < sqlc.narg(to_at)::timestamptz)
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: UsageRuns :many
SELECT agent_id, host_id, model, client, status, duration_ms, input_tokens, output_tokens,
       cache_read_tokens, cache_write_tokens, created_at
FROM agent_run
WHERE workspace_id = $1 AND created_at >= $2;

-- name: InsertPrice :exec
INSERT INTO model_price (id, workspace_id, model, input_per_mtok, output_per_mtok, cache_read_per_mtok,
                         cache_write_per_mtok, valid_from, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: Prices :many
SELECT * FROM model_price WHERE workspace_id = $1 ORDER BY model, valid_from, id;

-- name: Currency :one
SELECT * FROM workspace_currency WHERE workspace_id = $1;

-- name: PutCurrency :exec
INSERT INTO workspace_currency (workspace_id, usd_huf, updated_by, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id) DO UPDATE
SET usd_huf = EXCLUDED.usd_huf, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at;
