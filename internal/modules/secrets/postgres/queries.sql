-- name: InsertSecret :exec
INSERT INTO workspace_secret (id, workspace_id, kind, name, provider, key_id, nonce, ciphertext, hint, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetSecret :one
SELECT * FROM workspace_secret WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: ReplaceSecret :execrows
UPDATE workspace_secret SET key_id = $3, nonce = $4, ciphertext = $5, hint = $6, rotated_at = $7
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: DeleteSecret :execrows
UPDATE workspace_secret SET deleted_at = $3 WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: ListSecretsForRotation :many
SELECT * FROM workspace_secret WHERE deleted_at IS NULL AND key_id <> $1 ORDER BY created_at, id;

-- name: ResealSecret :execrows
UPDATE workspace_secret SET key_id = $3, nonce = $4, ciphertext = $5
WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL;
