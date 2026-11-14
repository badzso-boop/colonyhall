-- +goose Up
-- Round 53 reverted (Norbi, 2026-11-14): the third, unaccented keyword
-- vector made the keyword leg worse on the real evaluation set (recall@10
-- 0.597 -> 0.463, hybrid without rerank 0.827 -> 0.767; docs/11-rag-terv.md).
-- 00057 already ran on servers, so it stays; this removes what it added.
DROP INDEX IF EXISTS search_chunk_text_tsv_unaccent_idx;
ALTER TABLE search_chunk_text DROP COLUMN IF EXISTS tsv_unaccent;
DROP FUNCTION IF EXISTS colony_unaccent(text);

-- +goose Down
-- +goose StatementBegin
CREATE FUNCTION colony_unaccent(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    AS $$ SELECT translate($1,
        'áàâäãåéèêëíìîïóòôöőõúùûüűçñýžščřÁÀÂÄÃÅÉÈÊËÍÌÎÏÓÒÔÖŐÕÚÙÛÜŰÇÑÝŽŠČŘ',
        'aaaaaaeeeeiiiioooooouuuuucnyzscrAAAAAAEEEEIIIIOOOOOOUUUUUCNYZSCR') $$;
-- +goose StatementEnd
ALTER TABLE search_chunk_text
    ADD COLUMN tsv_unaccent tsvector GENERATED ALWAYS AS (to_tsvector('simple', colony_unaccent(header || ' ' || body))) STORED;
CREATE INDEX search_chunk_text_tsv_unaccent_idx ON search_chunk_text USING gin (tsv_unaccent);
