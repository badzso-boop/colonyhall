-- name: UpsertChunk :exec
-- A part whose content hash is unchanged keeps its embedding.
INSERT INTO search_chunk_text (id, workspace_id, source_kind, source_id, source_ref, part, visibility, channel_id,
    thread_root_id, author_id, source_at, header, body, content_hash, indexed_at)
VALUES (@id, @workspace_id, @source_kind, @source_id, @source_ref, @part, @visibility, sqlc.narg(channel_id),
    sqlc.narg(thread_root_id), sqlc.narg(author_id), @source_at, @header, @body, @content_hash, @indexed_at)
ON CONFLICT (source_kind, source_id, part) DO UPDATE SET
    workspace_id   = EXCLUDED.workspace_id,
    source_ref     = EXCLUDED.source_ref,
    visibility     = EXCLUDED.visibility,
    channel_id     = EXCLUDED.channel_id,
    thread_root_id = EXCLUDED.thread_root_id,
    author_id      = EXCLUDED.author_id,
    source_at      = EXCLUDED.source_at,
    header         = EXCLUDED.header,
    body           = EXCLUDED.body,
    indexed_at     = EXCLUDED.indexed_at,
    embedding      = CASE WHEN search_chunk_text.content_hash = EXCLUDED.content_hash THEN search_chunk_text.embedding END,
    embed_model    = CASE WHEN search_chunk_text.content_hash = EXCLUDED.content_hash THEN search_chunk_text.embed_model END,
    embedded_at    = CASE WHEN search_chunk_text.content_hash = EXCLUDED.content_hash THEN search_chunk_text.embedded_at END,
    embed_attempts = CASE WHEN search_chunk_text.content_hash = EXCLUDED.content_hash THEN search_chunk_text.embed_attempts ELSE 0 END,
    embed_retry_at = CASE WHEN search_chunk_text.content_hash = EXCLUDED.content_hash THEN search_chunk_text.embed_retry_at END,
    content_hash   = EXCLUDED.content_hash;

-- name: DeleteSourcePartsFrom :exec
DELETE FROM search_chunk_text WHERE source_kind = @source_kind AND source_id = @source_id AND part >= @from_part;

-- name: DeleteSummaries :exec
DELETE FROM search_chunk_text
WHERE source_kind = 'summary' AND channel_id = @channel_id
  AND thread_root_id IS NOT DISTINCT FROM sqlc.narg(thread_root_id)::uuid;

-- name: PendingEmbeddings :many
SELECT id, header, body FROM search_chunk_text
WHERE embedding IS NULL AND (embed_retry_at IS NULL OR embed_retry_at <= @now)
ORDER BY indexed_at, id
LIMIT @lim;

-- name: SetEmbedding :exec
UPDATE search_chunk_text
SET embedding = (@embedding::text)::vector, embed_model = @embed_model, embedded_at = @embedded_at, embed_retry_at = NULL
WHERE id = @id;

-- name: EmbedFailed :exec
UPDATE search_chunk_text SET embed_attempts = embed_attempts + 1, embed_retry_at = @retry_at
WHERE id = ANY(@ids::uuid[]);

-- name: EmbedModels :many
SELECT DISTINCT embed_model::text FROM search_chunk_text WHERE embed_model IS NOT NULL ORDER BY 1;

-- name: ResetEmbeddings :execrows
UPDATE search_chunk_text
SET embedding = NULL, embed_model = NULL, embedded_at = NULL, embed_attempts = 0, embed_retry_at = NULL
WHERE embedding IS NOT NULL AND embed_model IS DISTINCT FROM @keep_model;

-- name: KeywordSearch :many
-- Access first: workspace-visible chunks, or chunks of a channel the
-- reader may search. Then the filters, then ranking. The words are OR-ed
-- (a natural-language question rarely has all its words in the answer);
-- ts_rank_cd puts the chunks with more of them first. Phrases ("…") and
-- exclusions (-word) keep their meaning. The cast to tsquery does not
-- stem again.
WITH q AS (
    SELECT regexp_replace(websearch_to_tsquery('hungarian', @query)::text, ' & (?!!)', ' | ', 'g')::tsquery AS hu,
           regexp_replace(websearch_to_tsquery('simple', @query)::text, ' & (?!!)', ' | ', 'g')::tsquery AS si
)
SELECT c.id, c.workspace_id, c.source_kind, c.source_id, c.source_ref, c.part, c.visibility, c.channel_id, c.thread_root_id,
    c.author_id, c.source_at, c.header, c.body, c.content_hash, COALESCE(c.embed_model, '')::text AS embed_model,
    c.indexed_at,
    (ts_rank_cd(c.tsv_hu, q.hu, 32) + ts_rank_cd(c.tsv_simple, q.si, 32))::float8 AS score,
    ts_headline('hungarian', c.body, q.hu || q.si, 'StartSel=«, StopSel=», MaxWords=35, MinWords=12, MaxFragments=2, FragmentDelimiter=" … "')::text AS snippet
FROM search_chunk_text c, q
WHERE c.workspace_id = @workspace_id
  AND (c.visibility = 'workspace' OR c.channel_id = ANY(@channel_ids::uuid[]))
  AND (c.tsv_hu @@ q.hu OR c.tsv_simple @@ q.si)
  AND (cardinality(@kinds::text[]) = 0 OR c.source_kind = ANY(@kinds::text[]))
  AND (sqlc.narg(channel_id)::uuid IS NULL OR c.channel_id = sqlc.narg(channel_id)::uuid)
  AND (sqlc.narg(author_id)::uuid IS NULL OR c.author_id = sqlc.narg(author_id)::uuid)
  AND (sqlc.narg(since)::timestamptz IS NULL OR c.source_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR c.source_at <= sqlc.narg(until)::timestamptz)
ORDER BY score DESC, c.source_at DESC
LIMIT @lim;

-- name: SemanticSearch :many
-- Same access rule and filters as KeywordSearch, inside the query.
SELECT c.id, c.workspace_id, c.source_kind, c.source_id, c.source_ref, c.part, c.visibility, c.channel_id, c.thread_root_id,
    c.author_id, c.source_at, c.header, c.body, c.content_hash, COALESCE(c.embed_model, '')::text AS embed_model,
    c.indexed_at,
    (1 - (c.embedding <=> (@embedding::text)::vector))::float8 AS score
FROM search_chunk_text c
WHERE c.workspace_id = @workspace_id
  AND (c.visibility = 'workspace' OR c.channel_id = ANY(@channel_ids::uuid[]))
  AND c.embedding IS NOT NULL AND c.embed_model = @embed_model
  AND (cardinality(@kinds::text[]) = 0 OR c.source_kind = ANY(@kinds::text[]))
  AND (sqlc.narg(channel_id)::uuid IS NULL OR c.channel_id = sqlc.narg(channel_id)::uuid)
  AND (sqlc.narg(author_id)::uuid IS NULL OR c.author_id = sqlc.narg(author_id)::uuid)
  AND (sqlc.narg(since)::timestamptz IS NULL OR c.source_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR c.source_at <= sqlc.narg(until)::timestamptz)
ORDER BY c.embedding <=> (@embedding::text)::vector
LIMIT @lim;

-- name: InsertLog :exec
INSERT INTO search_log (id, workspace_id, actor_id, at, query, mode, result_count, result_refs, timings_ms, degraded)
VALUES (@id, @workspace_id, @actor_id, @at, @query, @mode, @result_count, @result_refs, @timings_ms, @degraded);

-- name: PurgeLogs :execrows
DELETE FROM search_log WHERE at < @before;

-- name: Stats :one
SELECT count(*)::int AS chunks,
    (count(*) FILTER (WHERE embedding IS NULL))::int AS pending,
    COALESCE(max(embedded_at), 'epoch'::timestamptz)::timestamptz AS last_embed_at,
    COALESCE(array_agg(DISTINCT embed_model) FILTER (WHERE embed_model IS NOT NULL), '{}')::text[] AS models
FROM search_chunk_text WHERE workspace_id = @workspace_id;
