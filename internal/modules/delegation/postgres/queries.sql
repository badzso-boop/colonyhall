-- name: InsertDelegation :execrows
INSERT INTO delegation (
    id, workspace_id, message_id, channel_id, thread_root_id,
    from_agent_id, to_agent_id, question_id, state, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10
)
ON CONFLICT (message_id, to_agent_id) DO NOTHING;

-- name: GetDelegationByID :one
SELECT * FROM delegation WHERE id = $1;

-- name: GetDelegationByQuestionID :one
SELECT * FROM delegation WHERE question_id = $1;

-- name: GetDelegationByMessageAndTarget :one
SELECT * FROM delegation WHERE message_id = $1 AND to_agent_id = $2;

-- name: UpdateDelegationStateConditional :execrows
UPDATE delegation
SET state = $3,
    decided_by = $4,
    decided_at = $5
WHERE id = $1 AND state = $2;

-- name: ListDelegationsByWorkspace :many
SELECT * FROM delegation
WHERE workspace_id = $1
  AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state'))
ORDER BY created_at DESC;
