-- name: CreateApprovalRequest :exec
INSERT INTO approval_request (id, workspace_id, agent_id, channel_id, thread_root_id, tool_name, input, state, decided_by, decision_message, created_at, decided_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: ApprovalRequestByID :one
SELECT * FROM approval_request WHERE id = $1;

-- name: DecideApprovalRequest :execrows
UPDATE approval_request
SET state = $2, decided_by = $3, decision_message = $4, decided_at = $5
WHERE id = $1 AND state = 'pending';

-- name: ExpirePendingByWorkspace :execrows
UPDATE approval_request
SET state = 'expired', decision_message = 'emergency stop', decided_at = $2
WHERE workspace_id = $1 AND state = 'pending';

-- name: ListPendingByWorkspace :many
SELECT * FROM approval_request
WHERE workspace_id = $1 AND state = 'pending'
ORDER BY created_at;
