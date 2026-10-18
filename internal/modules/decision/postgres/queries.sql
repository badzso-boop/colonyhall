-- name: InsertDecision :one
INSERT INTO decision (
    id, number, workspace_id, author_id, channel_id, thread_root_id,
    text, rationale, supersedes, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: NextDecisionNumber :one
SELECT COALESCE(MAX(number), 0) + 1 AS next_number
FROM decision
WHERE workspace_id = $1;

-- name: GetDecisionByNumber :one
SELECT 
    d.id, d.number, d.workspace_id, d.author_id, d.channel_id, d.thread_root_id,
    d.text, d.rationale, d.supersedes, d.created_at,
    COALESCE((SELECT s.number FROM decision s WHERE s.workspace_id = d.workspace_id AND s.supersedes = d.number LIMIT 1), 0)::int AS superseded_by
FROM decision d
WHERE d.workspace_id = $1 AND d.number = $2;

-- name: ListDecisions :many
SELECT 
    d.id, d.number, d.workspace_id, d.author_id, d.channel_id, d.thread_root_id,
    d.text, d.rationale, d.supersedes, d.created_at,
    COALESCE((SELECT s.number FROM decision s WHERE s.workspace_id = d.workspace_id AND s.supersedes = d.number LIMIT 1), 0)::int AS superseded_by
FROM decision d
WHERE d.workspace_id = $1
  AND (
    sqlc.arg(query)::text = '' OR 
    d.text ILIKE '%' || sqlc.arg(query) || '%' OR 
    d.rationale ILIKE '%' || sqlc.arg(query) || '%'
  )
  AND (
    NOT sqlc.arg(valid_only)::boolean OR 
    NOT EXISTS (SELECT 1 FROM decision s WHERE s.workspace_id = d.workspace_id AND s.supersedes = d.number)
  )
ORDER BY d.created_at DESC, d.number DESC
LIMIT sqlc.arg('limit');
