-- name: InsertConnector :exec
INSERT INTO connector (id, workspace_id, slug, title, kind, base_url, auth, header_name, secret_id, approval_off,
                       risk_overrides, tools_cache, state, last_error, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);

-- name: UpdateConnector :execrows
UPDATE connector SET title = $3, base_url = $4, auth = $5, header_name = $6, secret_id = $7, approval_off = $8,
                     risk_overrides = $9, updated_at = $10
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: SetConnectorTools :execrows
UPDATE connector SET tools_cache = $3, tools_cached_at = $4, state = $5, last_error = $6, updated_at = $4
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: GetConnector :one
SELECT * FROM connector WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: ListConnectors :many
SELECT * FROM connector WHERE workspace_id = $1 AND deleted_at IS NULL ORDER BY slug;

-- name: DeleteConnector :execrows
UPDATE connector SET deleted_at = $3, updated_at = $3 WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: InsertGrant :exec
INSERT INTO agent_tool_grant (id, workspace_id, agent_id, target_kind, target_id, granted_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: RevokeGrant :execrows
UPDATE agent_tool_grant SET revoked_at = $3 WHERE workspace_id = $1 AND id = $2 AND revoked_at IS NULL;

-- name: RevokeGrantsForTarget :exec
UPDATE agent_tool_grant SET revoked_at = $4
WHERE workspace_id = $1 AND target_kind = $2 AND target_id = $3 AND revoked_at IS NULL;

-- name: ListGrants :many
SELECT * FROM agent_tool_grant WHERE workspace_id = $1 AND revoked_at IS NULL ORDER BY created_at, id;

-- name: GrantsForAgent :many
SELECT * FROM agent_tool_grant WHERE workspace_id = $1 AND agent_id = $2 AND revoked_at IS NULL ORDER BY created_at, id;

-- name: InsertToolCall :exec
INSERT INTO tool_call (id, workspace_id, actor_id, target_kind, target_id, tool, risk, approval_id, started_at,
                       duration_ms, in_bytes, out_bytes, outcome, error)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: ListToolCalls :many
SELECT * FROM tool_call
WHERE workspace_id = $1
  AND (sqlc.narg('target_id')::uuid IS NULL OR target_id = sqlc.narg('target_id'))
  AND (sqlc.narg('before')::timestamptz IS NULL OR started_at < sqlc.narg('before'))
ORDER BY started_at DESC, id DESC
LIMIT $2;
