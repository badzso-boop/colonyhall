-- +goose Up
-- Round 36a: hybrid search over chat, wiki, decisions and compaction
-- summaries (docs/11-rag-terv.md). Numbered after round 45's tenant
-- isolation: both tables are tenant tables with row-level security. The search tables are a projection that
-- can be rebuilt from the source data, not primary data: rows are replaced
-- and removed when their source changes.

-- pgvector is not a trusted extension: on an existing Postgres a superuser
-- creates it once (docs/16-telepites.md). Where the
-- migration runs as a superuser (dev, CI) it creates it itself.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        BEGIN
            CREATE EXTENSION vector;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE EXCEPTION 'the vector extension is missing: run CREATE EXTENSION vector as superuser in this database (see docs/16-telepites.md)';
        END;
    END IF;
END
$$;
-- +goose StatementEnd

CREATE TABLE search_chunk_text (
    id             uuid PRIMARY KEY,
    workspace_id   uuid NOT NULL REFERENCES workspace(id),
    source_kind    text NOT NULL CHECK (source_kind IN ('message', 'wiki', 'decision', 'summary')),
    source_id      uuid NOT NULL,
    -- A human-readable reference: the wiki slug, D-12; empty otherwise.
    source_ref     text NOT NULL DEFAULT '',
    part           integer NOT NULL,
    visibility     text NOT NULL CHECK (visibility IN ('channel', 'workspace')),
    channel_id     uuid,
    thread_root_id uuid,
    author_id      uuid,
    source_at      timestamptz NOT NULL,
    header         text NOT NULL,
    body           text NOT NULL,
    content_hash   text NOT NULL,
    tsv_hu         tsvector GENERATED ALWAYS AS (to_tsvector('hungarian', header || ' ' || body)) STORED,
    tsv_simple     tsvector GENERATED ALWAYS AS (to_tsvector('simple', header || ' ' || body)) STORED,
    embedding      vector(384),
    embed_model    text,
    embedded_at    timestamptz,
    embed_attempts integer NOT NULL DEFAULT 0,
    embed_retry_at timestamptz,
    indexed_at     timestamptz NOT NULL,
    UNIQUE (source_kind, source_id, part),
    CHECK (visibility <> 'channel' OR channel_id IS NOT NULL),
    -- Composite: a chunk can only point at a channel of its own workspace
    -- (round 45, docs/23-elkulonites.md).
    CONSTRAINT search_chunk_text_channel_ws_fkey FOREIGN KEY (channel_id, workspace_id) REFERENCES channel (id, workspace_id)
);

CREATE INDEX search_chunk_text_tsv_hu_idx ON search_chunk_text USING gin (tsv_hu);
CREATE INDEX search_chunk_text_tsv_simple_idx ON search_chunk_text USING gin (tsv_simple);
CREATE INDEX search_chunk_text_embedding_idx ON search_chunk_text
    USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
CREATE INDEX search_chunk_text_channel_idx ON search_chunk_text (workspace_id, channel_id, source_at);
CREATE INDEX search_chunk_text_pending_idx ON search_chunk_text (id) WHERE embedding IS NULL;

CREATE TABLE search_log (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    actor_id     uuid NOT NULL,
    at           timestamptz NOT NULL,
    query        text NOT NULL,
    mode         text NOT NULL,
    result_count integer NOT NULL,
    result_refs  text[] NOT NULL DEFAULT '{}',
    timings_ms   jsonb NOT NULL DEFAULT '{}',
    degraded     text[] NOT NULL DEFAULT '{}',
    CONSTRAINT search_log_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id)
);

CREATE INDEX search_log_at_idx ON search_log (workspace_id, at);

-- Row-level security like every tenant table (00053_row_level_security.sql).
ALTER TABLE search_chunk_text ENABLE ROW LEVEL SECURITY;
ALTER TABLE search_chunk_text FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON search_chunk_text USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());
ALTER TABLE search_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE search_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON search_log USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS search_log;
DROP TABLE IF EXISTS search_chunk_text;
