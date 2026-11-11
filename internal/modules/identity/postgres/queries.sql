-- name: CreateWorkspace :exec
INSERT INTO workspace (id, name, created_at, slug, state) VALUES ($1, $2, $3, $4, $5);

-- name: CreateActor :exec
INSERT INTO actor (id, workspace_id, kind, handle, display_name, role, state, created_at, description, avatar_emoji, timezone, status_text, color, owner_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, sqlc.narg(color), sqlc.narg(owner_id));

-- name: UpdateActorProfile :exec
UPDATE actor
SET description = $2,
    avatar_emoji = $3,
    timezone = $4,
    status_text = $5,
    display_name = $6,
    color = sqlc.narg(color)
WHERE id = $1;

-- name: ActorByID :one
SELECT * FROM actor WHERE id = $1;

-- name: ActorByHandle :one
SELECT * FROM actor WHERE workspace_id = $1 AND handle = $2 AND archived_at IS NULL;

-- name: ActorsByIDs :many
SELECT * FROM actor WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY handle;

-- name: ListActors :many
SELECT * FROM actor WHERE workspace_id = $1 AND archived_at IS NULL ORDER BY handle;

-- name: ListAllActors :many
SELECT * FROM actor WHERE workspace_id = $1 ORDER BY handle, created_at;

-- name: ArchiveActor :exec
UPDATE actor SET archived_at = sqlc.arg(at), archived_by = sqlc.arg(by), state = 'stopped'
WHERE id = sqlc.arg(id) AND archived_at IS NULL;

-- name: RevokeActorTokens :exec
UPDATE api_token SET revoked_at = sqlc.arg(at) WHERE actor_id = sqlc.arg(actor_id) AND revoked_at IS NULL;

-- name: CreateToken :exec
-- The workspace comes from the actor (round 45): tokens are also made by
-- the CLI and the seed, which do not run in one workspace's scope.
INSERT INTO api_token (id, actor_id, token_hash, label, created_at, host_id, expires_at, prefix, workspace_id)
SELECT $1, $2, $3, $4, $5, sqlc.narg(host_id), sqlc.narg(expires_at), sqlc.narg(prefix), a.workspace_id
FROM actor a WHERE a.id = $2;

-- name: TouchToken :exec
UPDATE api_token SET last_used_at = sqlc.arg(at)
WHERE token_hash = sqlc.arg(token_hash) AND (last_used_at IS NULL OR last_used_at < sqlc.arg(not_before));

-- name: TouchSession :exec
UPDATE session SET last_seen_at = sqlc.arg(at)
WHERE id = sqlc.arg(id) AND (last_seen_at IS NULL OR last_seen_at < sqlc.arg(not_before));

-- name: ActorByTokenHash :one
SELECT a.* FROM api_token t JOIN actor a ON a.id = t.actor_id
WHERE t.token_hash = sqlc.arg(token_hash) AND t.revoked_at IS NULL
  AND (t.expires_at IS NULL OR t.expires_at > sqlc.arg(now)::timestamptz);

-- name: RevokeHostTokens :exec
UPDATE api_token SET revoked_at = sqlc.arg(at)
WHERE host_id = sqlc.arg(host_id) AND revoked_at IS NULL
  AND (sqlc.narg(agent_id)::uuid IS NULL OR actor_id = sqlc.narg(agent_id)::uuid);

-- name: ActorsByHandle :many
SELECT * FROM actor WHERE handle = $1 AND archived_at IS NULL ORDER BY workspace_id;

-- name: WorkspaceByName :one
SELECT * FROM workspace WHERE name = $1;

-- name: Credential :one
SELECT password_hash FROM credential WHERE actor_id = $1;

-- name: SetCredential :exec
INSERT INTO credential (actor_id, password_hash, updated_at, workspace_id)
SELECT $1, $2, $3, a.workspace_id FROM actor a WHERE a.id = $1
ON CONFLICT (actor_id) DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = EXCLUDED.updated_at;

-- name: CreateSession :exec
INSERT INTO session (id, actor_id, token_hash, csrf_token, user_agent, created_at, expires_at, workspace_id)
SELECT $1, $2, $3, $4, $5, $6, $7, a.workspace_id FROM actor a WHERE a.id = $2;

-- name: SessionByTokenHash :one
SELECT sqlc.embed(s), sqlc.embed(a)
FROM session s JOIN actor a ON a.id = s.actor_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE session SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeSessionsForActor :exec
UPDATE session SET revoked_at = sqlc.arg(at)::timestamptz
WHERE actor_id = sqlc.arg(actor_id) AND id <> sqlc.arg(keep) AND revoked_at IS NULL;

-- name: ListTokens :many
SELECT * FROM api_token WHERE actor_id = $1 ORDER BY created_at, id;

-- name: TokenByID :one
SELECT * FROM api_token WHERE id = $1;

-- name: RevokeToken :exec
UPDATE api_token SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: SetActorState :exec
UPDATE actor SET state = $2 WHERE id = $1;

-- name: SetWorkspaceSlug :execrows
UPDATE workspace SET slug = sqlc.arg(slug) WHERE id = sqlc.arg(id);

-- name: WorkspaceByID :one
SELECT * FROM workspace WHERE id = $1;

-- name: SetEmergencyStop :exec
UPDATE workspace SET emergency_stop_at = sqlc.narg(at), emergency_stop_by = sqlc.narg(by) WHERE id = sqlc.arg(id);

-- name: SessionStats :many
SELECT s.actor_id,
       COUNT(*) FILTER (WHERE s.revoked_at IS NULL AND s.expires_at > sqlc.arg(now_at)::timestamptz)::int AS active,
       MAX(s.created_at)::timestamptz AS last_login,
       -- A login is activity too; sessions from before last_seen_at existed have only it.
       COALESCE(MAX(s.last_seen_at), MAX(s.created_at))::timestamptz AS last_seen
FROM session s JOIN actor a ON a.id = s.actor_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
GROUP BY s.actor_id;

-- name: SetActorRole :exec
UPDATE actor SET role = $2 WHERE id = $1;

-- name: SetActorOwner :exec
UPDATE actor SET owner_id = sqlc.narg(owner_id) WHERE id = sqlc.arg(id);

-- name: DeleteExpiredHostTokens :execrows
-- One batch of short-lived guard tokens that expired or were revoked before
-- the cut-off. SKIP LOCKED lets several servers sweep at once without waiting.
DELETE FROM api_token WHERE id IN (
    SELECT t.id FROM api_token t
    WHERE t.host_id IS NOT NULL
      AND (t.expires_at < sqlc.arg(before)::timestamptz OR t.revoked_at < sqlc.arg(before)::timestamptz)
    LIMIT sqlc.arg(batch)::int
    FOR UPDATE SKIP LOCKED
);

-- name: DeleteEndedSessions :execrows
DELETE FROM session WHERE id IN (
    SELECT s.id FROM session s
    WHERE s.expires_at < sqlc.arg(before)::timestamptz OR s.revoked_at < sqlc.arg(before)::timestamptz
    LIMIT sqlc.arg(batch)::int
    FOR UPDATE SKIP LOCKED
);

-- Round 45: lookups before the workspace is known (migration 00053).

-- name: WorkspaceOfToken :one
SELECT ws::uuid AS workspace_id FROM colony_token_workspace(sqlc.arg(hash)::bytea) AS ws WHERE ws IS NOT NULL;

-- name: WorkspaceOfSession :one
SELECT ws::uuid AS workspace_id FROM colony_session_workspace(sqlc.arg(hash)::bytea) AS ws WHERE ws IS NOT NULL;

-- name: HandleWorkspaces :many
SELECT h.id::uuid AS id, h.name::text AS name FROM colony_handle_workspaces(sqlc.arg(handle)::text) AS h;

-- name: WorkspaceBySlugAnywhere :one
SELECT w.id::uuid AS id, w.name::text AS name, w.created_at::timestamptz AS created_at, w.slug::text AS slug, w.state::text AS state
FROM colony_workspace_by_slug(sqlc.arg(slug)::text) AS w;

-- name: RevokeTokenByHash :one
UPDATE api_token SET revoked_at = coalesce(revoked_at, sqlc.arg(at)::timestamptz)
WHERE token_hash = sqlc.arg(token_hash) RETURNING *;

-- name: SetWorkspaceState :execrows
UPDATE workspace SET state = sqlc.arg(state) WHERE id = sqlc.arg(id);
