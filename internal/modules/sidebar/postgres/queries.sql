-- name: ListSectionsByOwner :many
SELECT id, workspace_id, owner_id, name, position, collapsed, created_at, updated_at, deleted_at
FROM sidebar_section
WHERE workspace_id = $1 AND owner_id = $2 AND deleted_at IS NULL
ORDER BY position ASC, created_at ASC;

-- name: ListItemsBySectionIDs :many
SELECT section_id, channel_id, position
FROM sidebar_item
WHERE section_id = ANY($1::uuid[])
ORDER BY position ASC;

-- name: GetSectionByID :one
SELECT id, workspace_id, owner_id, name, position, collapsed, created_at, updated_at, deleted_at
FROM sidebar_section
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListItemsBySectionID :many
SELECT section_id, channel_id, position
FROM sidebar_item
WHERE section_id = $1
ORDER BY position ASC;

-- name: CreateSection :exec
INSERT INTO sidebar_section (id, workspace_id, owner_id, name, position, collapsed, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateSection :one
UPDATE sidebar_section
SET name = COALESCE(sqlc.narg('name'), name),
    position = COALESCE(sqlc.narg('position'), position),
    collapsed = COALESCE(sqlc.narg('collapsed'), collapsed),
    updated_at = $1
WHERE id = $2 AND deleted_at IS NULL
RETURNING id, workspace_id, owner_id, name, position, collapsed, created_at, updated_at, deleted_at;

-- name: SoftDeleteSection :exec
UPDATE sidebar_section
SET deleted_at = $2
WHERE id = $1 AND deleted_at IS NULL;

-- name: DeleteItemsBySectionID :exec
DELETE FROM sidebar_item
WHERE section_id = $1;

-- name: UpdateSectionPosition :exec
UPDATE sidebar_section
SET position = $2
WHERE id = $1 AND owner_id = $3 AND deleted_at IS NULL;

-- name: DeleteItemFromOwnerSections :exec
DELETE FROM sidebar_item
WHERE channel_id = $1
  AND section_id IN (
      SELECT id FROM sidebar_section WHERE owner_id = $2 AND deleted_at IS NULL
  );

-- name: UpsertItem :exec
INSERT INTO sidebar_item (section_id, channel_id, position)
VALUES ($1, $2, $3)
ON CONFLICT (section_id, channel_id)
DO UPDATE SET position = EXCLUDED.position;

-- name: DeleteItem :exec
DELETE FROM sidebar_item
WHERE section_id = $1 AND channel_id = $2;

-- name: UpdateItemPosition :exec
UPDATE sidebar_item
SET position = $3
WHERE section_id = $1 AND channel_id = $2;
