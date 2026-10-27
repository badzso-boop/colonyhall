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

-- name: ListPendingByPairAndConversation :many
SELECT * FROM delegation
WHERE workspace_id = $1
  AND (
      (from_agent_id = $2 AND to_agent_id = $3)
      OR (from_agent_id = $3 AND to_agent_id = $2)
  )
  AND channel_id = $4
  AND (
      (sqlc.narg('thread_root_id')::uuid IS NULL AND thread_root_id IS NULL)
      OR (thread_root_id = sqlc.narg('thread_root_id')::uuid)
  )
  AND state = 'pending'
ORDER BY created_at ASC;

-- name: InsertGrant :exec
INSERT INTO delegation_grant (
    id, workspace_id, channel_id, thread_root_id,
    agent_a_id, agent_b_id, delegation_id,
    approved_by, approved_at, last_used_at, expires_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9, $10, $11
);

-- name: GetGrantByID :one
SELECT * FROM delegation_grant WHERE id = $1;

-- name: GetActiveGrantByPairAndConversation :one
SELECT * FROM delegation_grant
WHERE workspace_id = $1
  AND agent_a_id = $2
  AND agent_b_id = $3
  AND channel_id = $4
  AND (
      (sqlc.narg('thread_root_id')::uuid IS NULL AND thread_root_id IS NULL)
      OR (thread_root_id = sqlc.narg('thread_root_id')::uuid)
  )
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg('now')::timestamptz
ORDER BY approved_at DESC
LIMIT 1;

-- name: UpdateGrantLastUsed :execrows
UPDATE delegation_grant
SET last_used_at = $2
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeGrantConditional :execrows
UPDATE delegation_grant
SET revoked_by = $2,
    revoked_at = sqlc.arg('revoked_at')::timestamptz
WHERE id = $1
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg('revoked_at')::timestamptz;

-- name: ListGrantsByWorkspace :many
SELECT * FROM delegation_grant
WHERE workspace_id = $1
  AND (
      NOT sqlc.arg('active_only')::boolean
      OR (revoked_at IS NULL AND expires_at > sqlc.arg('now')::timestamptz)
  )
ORDER BY approved_at DESC;

-- name: GetSettings :one
SELECT * FROM delegation_settings WHERE workspace_id = $1;

-- name: UpsertSettings :exec
INSERT INTO delegation_settings (workspace_id, grant_idle_minutes, grant_max_hours, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id) DO UPDATE
SET grant_idle_minutes = EXCLUDED.grant_idle_minutes,
    grant_max_hours = EXCLUDED.grant_max_hours,
    updated_at = EXCLUDED.updated_at;

