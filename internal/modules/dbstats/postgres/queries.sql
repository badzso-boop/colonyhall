-- name: DatabaseSize :one
SELECT pg_database_size(current_database())::bigint AS size_bytes;

-- name: LargestTables :many
-- Sizes include indexes and TOAST; the row count is the planner's estimate
-- (-1 before the first ANALYZE, shown as 0).
SELECT c.relname::text AS name,
       pg_total_relation_size(c.oid)::bigint AS total_bytes,
       GREATEST(c.reltuples, 0)::bigint AS rows_estimate
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p') AND n.nspname = current_schema()
ORDER BY total_bytes DESC, name
LIMIT sqlc.arg(max_rows)::int;

-- name: RecordSizeSample :exec
INSERT INTO db_size_sample (sampled_on, size_bytes) VALUES (sqlc.arg(sampled_on)::date, sqlc.arg(size_bytes))
ON CONFLICT (sampled_on) DO NOTHING;

-- name: SizeSamples :many
SELECT * FROM db_size_sample WHERE sampled_on >= sqlc.arg(since)::date ORDER BY sampled_on;

-- name: PruneSizeSamples :execrows
DELETE FROM db_size_sample WHERE sampled_on < sqlc.arg(before)::date;

-- Round 54: the workspace storage's database share.

-- name: WorkspaceIDs :many
-- System mode: every workspace (the hourly usage refresh).
SELECT id FROM workspace ORDER BY id;

-- name: WorkspaceRowBytes :one
-- The size of a workspace's rows in the large tables (pg_column_size of the
-- whole row, TOAST compressed size included), without indexes or page
-- overhead: the caller multiplies by dbstats.OverheadFactor. Run with the
-- connection scoped to the workspace (row-level security narrows it too).
SELECT
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM message t WHERE t.workspace_id = $1)::bigint AS message,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM event t WHERE t.workspace_id = $1)::bigint AS event,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM search_chunk_text t WHERE t.workspace_id = $1)::bigint AS search_chunk,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM search_log t WHERE t.workspace_id = $1)::bigint AS search_log,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM delivery t WHERE t.workspace_id = $1)::bigint AS delivery,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM agent_run t WHERE t.workspace_id = $1)::bigint AS agent_run,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM audit_log t WHERE t.workspace_id = $1)::bigint AS audit_log,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM file t WHERE t.workspace_id = $1)::bigint AS file,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM wiki_revision t WHERE t.workspace_id = $1)::bigint AS wiki_revision,
    (SELECT coalesce(sum(pg_column_size(t.*)), 0) FROM terminal_event t WHERE t.workspace_id = $1)::bigint AS terminal_event;

-- name: ChatTablesSize :one
-- The chat and search tables with indexes and TOAST (single-workspace
-- breakdown).
SELECT (pg_total_relation_size('message') + pg_total_relation_size('event')
      + pg_total_relation_size('search_chunk_text') + pg_total_relation_size('search_log')
      + pg_total_relation_size('delivery'))::bigint AS size_bytes;
