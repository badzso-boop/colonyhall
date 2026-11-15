-- Round 51: the platform operator (migration 00068). Every query runs in the
-- operator scope (colony.operator = 'on').

-- name: CreateOperator :exec
INSERT INTO operator (id, handle, display_name, password_hash, totp_key_id, totp_nonce, totp_ciphertext, state, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: OperatorByHandle :one
SELECT * FROM operator WHERE handle = $1;

-- name: OperatorByID :one
SELECT * FROM operator WHERE id = $1;

-- name: ListOperators :many
SELECT * FROM operator ORDER BY handle;

-- name: SetTOTP :execrows
UPDATE operator SET totp_key_id = sqlc.arg(key_id), totp_nonce = sqlc.arg(nonce), totp_ciphertext = sqlc.arg(ciphertext),
    totp_last_step = 0, state = sqlc.arg(state)
WHERE id = sqlc.arg(id);

-- name: ResealTOTP :execrows
UPDATE operator SET totp_key_id = sqlc.arg(key_id), totp_nonce = sqlc.arg(nonce), totp_ciphertext = sqlc.arg(ciphertext)
WHERE id = sqlc.arg(id);

-- name: AdvanceTOTPStep :execrows
UPDATE operator SET totp_last_step = sqlc.arg(step) WHERE id = sqlc.arg(id) AND totp_last_step < sqlc.arg(step);

-- name: SetOperatorState :execrows
UPDATE operator SET state = sqlc.arg(state), activated_at = sqlc.narg(activated_at) WHERE id = sqlc.arg(id);

-- name: SetLastLogin :exec
UPDATE operator SET last_login_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: DeleteBackupCodes :exec
DELETE FROM operator_backup_code WHERE operator_id = $1;

-- name: InsertBackupCode :exec
INSERT INTO operator_backup_code (id, operator_id, code_hash, created_at) VALUES ($1, $2, $3, $4);

-- name: UseBackupCode :execrows
UPDATE operator_backup_code SET used_at = sqlc.arg(at)
WHERE operator_id = sqlc.arg(operator_id) AND code_hash = sqlc.arg(code_hash) AND used_at IS NULL;

-- name: UnusedBackupCodes :one
SELECT count(*) FROM operator_backup_code WHERE operator_id = $1 AND used_at IS NULL;

-- name: CreateSession :exec
INSERT INTO operator_session (id, operator_id, token_hash, csrf_token, created_at, last_seen_at, expires_at, remote_addr, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: SessionByTokenHash :one
SELECT * FROM operator_session WHERE token_hash = $1;

-- name: TouchSession :exec
UPDATE operator_session SET last_seen_at = sqlc.arg(at) WHERE id = sqlc.arg(id) AND last_seen_at < sqlc.arg(at);

-- name: RevokeSession :exec
UPDATE operator_session SET revoked_at = coalesce(revoked_at, sqlc.arg(at)) WHERE id = sqlc.arg(id);

-- name: RevokeSessions :exec
UPDATE operator_session SET revoked_at = sqlc.arg(at) WHERE operator_id = sqlc.arg(operator_id) AND revoked_at IS NULL;

-- name: DeleteEndedSessions :execrows
DELETE FROM operator_session
WHERE expires_at < sqlc.arg(before) OR revoked_at < sqlc.arg(before) OR last_seen_at < sqlc.arg(idle_before);

-- name: AppendAudit :exec
INSERT INTO operator_audit (id, operator_id, action, workspace_id, payload, remote_addr, created_at)
VALUES ($1, sqlc.narg(operator_id), $2, sqlc.narg(workspace_id), $3, $4, $5);

-- name: ListAudit :many
SELECT * FROM operator_audit
WHERE sqlc.narg(before)::timestamptz IS NULL OR created_at < sqlc.narg(before)::timestamptz
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(lim);

-- name: Overview :many
-- The workspaces' account data and numbers, in the operator scope
-- (colony_operator_overview refuses any other). only_ws: one workspace, or
-- the nil UUID for all. A missing time is 0001-01-01 (sqlc cannot type a
-- nullable function column); the store turns it into nil.
SELECT o.ws_id::uuid AS id, o.ws_name::text AS name, o.ws_slug::text AS slug, o.ws_state::text AS state,
       coalesce(o.ws_suspended_at, '0001-01-01Z')::timestamptz AS suspended_at, o.ws_suspended_reason::text AS suspended_reason,
       o.ws_created_at::timestamptz AS created_at, o.has_owner::boolean AS has_owner,
       o.users_count::integer AS users, o.agents_count::integer AS agents, o.hosts_count::integer AS hosts,
       o.hosts_online::integer AS hosts_online, coalesce(o.last_activity, '0001-01-01Z')::timestamptz AS last_activity,
       o.quota_bytes::bigint AS quota_bytes, o.files_bytes::bigint AS files_bytes, o.db_bytes::bigint AS db_bytes,
       coalesce(o.invite_created_at, '0001-01-01Z')::timestamptz AS invite_created_at,
       coalesce(o.invite_expires_at, '0001-01-01Z')::timestamptz AS invite_expires_at,
       coalesce(o.invite_used, false)::boolean AS invite_used, coalesce(o.invite_revoked, false)::boolean AS invite_revoked,
       coalesce(o.plan_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS plan_id,
       o.plan_storage_bytes::bigint AS plan_storage_bytes, o.break_glass_open::boolean AS break_glass_open
FROM colony_operator_overview(sqlc.arg(only_ws)::uuid, sqlc.arg(online_since)::timestamptz) AS o;

-- Round 51b: plans (operator scope).

-- name: CreatePlan :exec
INSERT INTO plan (id, name, storage_bytes, cpu_cores, memory_bytes, archived_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: PlanByID :one
SELECT * FROM plan WHERE id = $1;

-- name: ListPlans :many
SELECT * FROM plan ORDER BY archived_at IS NOT NULL, storage_bytes, name;

-- name: UpdatePlan :exec
UPDATE plan SET name = $2, storage_bytes = $3, cpu_cores = $4, memory_bytes = $5, archived_at = $6, updated_at = $7
WHERE id = $1;
