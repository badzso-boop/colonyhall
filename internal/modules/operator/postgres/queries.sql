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
