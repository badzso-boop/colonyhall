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

-- name: CountCallsSince :one
SELECT count(*) FROM tool_call WHERE workspace_id = $1 AND started_at >= $2 AND outcome <> 'denied';

-- name: InsertExtension :exec
INSERT INTO extension (id, workspace_id, slug, title, description, maintainer_id, bot_id, state, live_version, approval_off,
                       disabled_reason, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: UpdateExtension :execrows
UPDATE extension SET title = $3, description = $4, state = $5, live_version = $6, approval_off = $7, disabled_reason = $8,
                     maintainer_id = $9, updated_at = $10
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: SetExtensionErrors :execrows
UPDATE extension SET consecutive_errors = $3, recent_errors = $4
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: GetExtension :one
SELECT * FROM extension WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: ListExtensions :many
SELECT * FROM extension WHERE workspace_id = $1 AND deleted_at IS NULL ORDER BY slug;

-- name: DeleteExtension :execrows
UPDATE extension SET deleted_at = $3, updated_at = $3, state = 'disabled' WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: InsertVersion :exec
INSERT INTO extension_version (id, workspace_id, extension_id, version, manifest, source, bundle, bundle_sha256, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetVersion :one
SELECT * FROM extension_version WHERE workspace_id = $1 AND extension_id = $2 AND version = $3;

-- name: ListVersions :many
SELECT id, workspace_id, extension_id, version, manifest, bundle_sha256, created_by, created_at
FROM extension_version WHERE workspace_id = $1 AND extension_id = $2 ORDER BY version DESC;

-- name: InsertApproval :exec
INSERT INTO extension_approval (id, workspace_id, extension_id, version, decided_by, diff, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListApprovals :many
SELECT * FROM extension_approval WHERE workspace_id = $1 AND extension_id = $2 ORDER BY created_at DESC, id DESC;

-- name: GetDoc :one
SELECT * FROM ext_store_doc WHERE workspace_id = $1 AND extension_id = $2 AND collection = $3 AND doc_id = $4;

-- name: PutDoc :exec
INSERT INTO ext_store_doc (workspace_id, extension_id, collection, doc_id, data, size_bytes, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (extension_id, collection, doc_id) DO UPDATE SET data = EXCLUDED.data, size_bytes = EXCLUDED.size_bytes,
    updated_at = EXCLUDED.updated_at;

-- name: DeleteDoc :exec
DELETE FROM ext_store_doc WHERE workspace_id = $1 AND extension_id = $2 AND collection = $3 AND doc_id = $4;

-- name: ListDocs :many
SELECT * FROM ext_store_doc WHERE workspace_id = $1 AND extension_id = $2 AND collection = $3 AND doc_id > $4
ORDER BY doc_id LIMIT $5;

-- name: StoreUsage :one
SELECT count(*)::integer AS docs, COALESCE(sum(size_bytes), 0)::integer AS bytes
FROM ext_store_doc WHERE workspace_id = $1 AND extension_id = $2;

-- name: UpsertRoutineState :exec
INSERT INTO routine_state (id, workspace_id, extension_id, routine, next_run_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (extension_id, routine) DO UPDATE SET next_run_at = EXCLUDED.next_run_at;

-- name: ListRoutineStates :many
SELECT * FROM routine_state WHERE workspace_id = $1 AND extension_id = $2 ORDER BY routine;

-- name: GetRoutineState :one
SELECT * FROM routine_state WHERE workspace_id = $1 AND id = $2;

-- name: SetRoutineNext :exec
UPDATE routine_state SET next_run_at = $3, last_run_at = $4 WHERE workspace_id = $1 AND id = $2;

-- name: SetRoutineWebhook :exec
UPDATE routine_state SET webhook_token_hash = $3 WHERE workspace_id = $1 AND id = $2;

-- name: DueRoutines :many
SELECT * FROM routine_state WHERE next_run_at IS NOT NULL AND next_run_at <= $1 ORDER BY next_run_at LIMIT $2;

-- name: InsertRoutineRun :exec
INSERT INTO routine_run (id, workspace_id, extension_id, routine, trigger, input, state, created_at)
VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7);

-- name: ClaimRoutineRuns :many
UPDATE routine_run SET state = 'running', started_at = $1
WHERE id IN (SELECT id FROM routine_run WHERE state = 'pending' ORDER BY created_at LIMIT $2 FOR UPDATE SKIP LOCKED)
RETURNING *;

-- name: FinishRoutineRun :exec
UPDATE routine_run SET state = $3, error = $4, finished_at = $5 WHERE workspace_id = $1 AND id = $2;

-- name: ListRoutineRuns :many
SELECT * FROM routine_run WHERE workspace_id = $1 AND extension_id = $2 ORDER BY created_at DESC LIMIT $3;
