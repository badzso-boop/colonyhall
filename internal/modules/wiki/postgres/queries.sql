-- name: InsertPage :exec
INSERT INTO wiki_page (id, workspace_id, slug, title, tags, created_by, created_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: InsertRevision :exec
INSERT INTO wiki_revision (page_id, rev, body, kind, author_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: PageBySlug :one
SELECT id, workspace_id, slug, title, tags, created_by, created_at, deleted_at
FROM wiki_page
WHERE workspace_id = $1 AND slug = $2;

-- name: PageByID :one
SELECT id, workspace_id, slug, title, tags, created_by, created_at, deleted_at
FROM wiki_page
WHERE id = $1;

-- name: LatestRevision :one
SELECT page_id, rev, body, kind, author_id, created_at
FROM wiki_revision
WHERE page_id = $1
ORDER BY rev DESC
LIMIT 1;

-- name: ListRevisions :many
SELECT page_id, rev, body, kind, author_id, created_at
FROM wiki_revision
WHERE page_id = $1
ORDER BY rev ASC;

-- name: UpdatePageMeta :exec
UPDATE wiki_page
SET title = $2, slug = $3, tags = $4
WHERE id = $1;

-- name: SoftDeletePage :exec
UPDATE wiki_page
SET deleted_at = $2
WHERE id = $1;

-- name: RestorePage :exec
UPDATE wiki_page
SET deleted_at = NULL
WHERE id = $1;

-- name: ListPages :many
SELECT
    p.id, p.workspace_id, p.slug, p.title, p.tags, p.created_by, p.created_at, p.deleted_at,
    r.rev, r.body, r.created_at AS updated_at
FROM wiki_page p
JOIN wiki_revision r ON r.page_id = p.id AND r.rev = (
    SELECT MAX(rev) FROM wiki_revision WHERE page_id = p.id
)
WHERE p.workspace_id = $1
  AND (sqlc.arg(include_deleted)::boolean OR p.deleted_at IS NULL)
ORDER BY p.title ASC;

-- name: PostAuthors :many
SELECT r.page_id, r.author_id, count(*)::int AS posts, max(r.rev)::int AS last_rev
FROM wiki_revision r
JOIN wiki_page p ON p.id = r.page_id
WHERE p.workspace_id = $1 AND r.kind IN ('create', 'append')
GROUP BY r.page_id, r.author_id;

-- name: CheckSlugExists :one
SELECT EXISTS (
    SELECT 1 FROM wiki_page
    WHERE workspace_id = $1 AND slug = $2
);
