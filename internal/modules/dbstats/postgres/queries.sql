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
