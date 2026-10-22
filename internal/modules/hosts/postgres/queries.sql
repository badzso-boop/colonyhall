-- name: CreatePairingCode :exec
INSERT INTO host_pairing_code (id, workspace_id, code_hash, created_by, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UsePairingCode :one
UPDATE host_pairing_code SET used_at = sqlc.arg(now)
WHERE code_hash = sqlc.arg(code_hash) AND used_at IS NULL AND expires_at > sqlc.arg(now)
RETURNING *;

-- name: CreateHost :exec
INSERT INTO host (id, workspace_id, name, token_hash, state, created_by, created_at, report)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: HostByID :one
SELECT * FROM host WHERE id = $1;

-- name: HostByTokenHash :one
SELECT * FROM host WHERE token_hash = $1;

-- name: ListHosts :many
SELECT * FROM host WHERE workspace_id = $1 ORDER BY name, id;

-- name: SetHostState :exec
UPDATE host SET state = $2 WHERE id = $1;

-- name: SaveReport :exec
UPDATE host SET report = $2, last_seen_at = $3 WHERE id = $1;

-- name: Touch :exec
UPDATE host SET last_seen_at = $2 WHERE id = $1;

-- name: SetAssignment :exec
INSERT INTO agent_host (agent_id, host_id, workspace_id, client, model, work_model, allow_work, permission_mode, mode, root_dir, worktree_repo, sandbox, approvals, max_session_tokens, daily_token_limit, allow_preview, allow_web_read, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
ON CONFLICT (agent_id) DO UPDATE SET host_id = EXCLUDED.host_id, client = EXCLUDED.client, model = EXCLUDED.model,
  work_model = EXCLUDED.work_model, allow_work = EXCLUDED.allow_work, permission_mode = EXCLUDED.permission_mode,
  mode = EXCLUDED.mode, root_dir = EXCLUDED.root_dir, worktree_repo = EXCLUDED.worktree_repo, sandbox = EXCLUDED.sandbox, approvals = EXCLUDED.approvals,
  max_session_tokens = EXCLUDED.max_session_tokens, daily_token_limit = EXCLUDED.daily_token_limit,
  allow_preview = EXCLUDED.allow_preview, allow_web_read = EXCLUDED.allow_web_read,
  created_by = EXCLUDED.created_by, created_at = EXCLUDED.created_at;

-- name: DeleteAssignment :exec
DELETE FROM agent_host WHERE agent_id = $1;

-- name: AssignmentByAgent :one
SELECT * FROM agent_host WHERE agent_id = $1;

-- name: AssignmentsForHost :many
SELECT * FROM agent_host WHERE host_id = $1 ORDER BY agent_id;

-- name: AddGrant :exec
INSERT INTO folder_grant (id, workspace_id, agent_id, host_id, path, mode, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GrantByID :one
SELECT * FROM folder_grant WHERE id = $1;

-- name: RevokeGrant :exec
UPDATE folder_grant SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: ActiveGrants :many
SELECT * FROM folder_grant WHERE host_id = $1 AND agent_id = $2 AND revoked_at IS NULL ORDER BY path, id;

-- name: RevokeGrantsForAgent :exec
UPDATE folder_grant SET revoked_at = $2 WHERE agent_id = $1 AND revoked_at IS NULL;
