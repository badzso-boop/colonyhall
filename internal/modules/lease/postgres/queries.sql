-- name: AdvisoryLockResource :exec
SELECT pg_advisory_xact_lock(hashtext(sqlc.arg(workspace_id)::text || ':' || sqlc.arg(resource)::text));

-- name: InsertLease :one
INSERT INTO lease (
    id, workspace_id, resource, holder_id, note,
    acquired_at, expires_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7
) RETURNING *;

-- name: GetActiveLeaseByResource :one
SELECT l.id, l.workspace_id, l.resource, l.holder_id, l.note, l.acquired_at, l.expires_at, l.released_at, l.released_by, a.handle AS holder_handle
FROM lease l
JOIN actor a ON a.id = l.holder_id
WHERE l.workspace_id = $1 
  AND l.resource = $2 
  AND l.released_at IS NULL 
  AND l.expires_at > $3;

-- name: GetLeaseByID :one
SELECT l.id, l.workspace_id, l.resource, l.holder_id, l.note, l.acquired_at, l.expires_at, l.released_at, l.released_by, a.handle AS holder_handle
FROM lease l
JOIN actor a ON a.id = l.holder_id
WHERE l.id = $1;

-- name: ExtendLease :one
UPDATE lease
SET expires_at = $2,
    note = COALESCE(sqlc.narg('note'), note)
WHERE id = $1 AND released_at IS NULL
RETURNING *;

-- name: ReleaseLease :one
UPDATE lease
SET released_at = $2,
    released_by = sqlc.narg('released_by')
WHERE id = $1 AND released_at IS NULL
RETURNING *;

-- name: ListActiveLeases :many
SELECT l.id, l.workspace_id, l.resource, l.holder_id, l.note, l.acquired_at, l.expires_at, l.released_at, l.released_by, a.handle AS holder_handle
FROM lease l
JOIN actor a ON a.id = l.holder_id
WHERE l.workspace_id = $1 
  AND l.released_at IS NULL 
  AND l.expires_at > $2
ORDER BY l.resource ASC;

-- name: ListLeasesSince :many
-- Leases still running, or that ended (released or expired) at or after since; newest first.
SELECT l.id, l.workspace_id, l.resource, l.holder_id, l.note, l.acquired_at, l.expires_at, l.released_at, l.released_by, a.handle AS holder_handle
FROM lease l
JOIN actor a ON a.id = l.holder_id
WHERE l.workspace_id = $1
  AND COALESCE(l.released_at, l.expires_at) >= sqlc.arg(since)::timestamptz
ORDER BY l.acquired_at DESC, l.id DESC;

-- name: ReleaseActiveLeasesByWorkspace :many
UPDATE lease
SET released_at = $2,
    released_by = sqlc.narg('released_by')
WHERE workspace_id = $1
  AND released_at IS NULL
  AND expires_at > $2
RETURNING *;

-- name: ReleaseActiveLeasesByHolder :many
UPDATE lease
SET released_at = $3,
    released_by = sqlc.narg('released_by')
WHERE workspace_id = $1
  AND holder_id = $2
  AND released_at IS NULL
  AND expires_at > $3
RETURNING *;
