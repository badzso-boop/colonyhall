-- name: CreateWorkspace :exec
INSERT INTO workspace (id, name, created_at) VALUES ($1, $2, $3);

-- name: CreateActor :exec
INSERT INTO actor (id, workspace_id, kind, handle, display_name, role, state, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ActorByID :one
SELECT * FROM actor WHERE id = $1;

-- name: ActorByHandle :one
SELECT * FROM actor WHERE workspace_id = $1 AND handle = $2;

-- name: ActorsByIDs :many
SELECT * FROM actor WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY handle;

-- name: ListActors :many
SELECT * FROM actor WHERE workspace_id = $1 ORDER BY handle;

-- name: CreateToken :exec
INSERT INTO api_token (id, actor_id, token_hash, label, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: ActorByTokenHash :one
SELECT a.* FROM api_token t JOIN actor a ON a.id = t.actor_id
WHERE t.token_hash = $1 AND t.revoked_at IS NULL;

-- name: ActorsByHandle :many
SELECT * FROM actor WHERE handle = $1 ORDER BY workspace_id;

-- name: WorkspaceByName :one
SELECT * FROM workspace WHERE name = $1;

-- name: Credential :one
SELECT password_hash FROM credential WHERE actor_id = $1;

-- name: SetCredential :exec
INSERT INTO credential (actor_id, password_hash, updated_at) VALUES ($1, $2, $3)
ON CONFLICT (actor_id) DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = EXCLUDED.updated_at;

-- name: CreateSession :exec
INSERT INTO session (id, actor_id, token_hash, csrf_token, user_agent, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: SessionByTokenHash :one
SELECT sqlc.embed(s), sqlc.embed(a)
FROM session s JOIN actor a ON a.id = s.actor_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE session SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;
