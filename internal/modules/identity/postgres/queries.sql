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
