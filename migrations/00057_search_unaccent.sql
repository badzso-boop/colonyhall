-- +goose Up
-- Round 53: accent-insensitive keyword search (docs/11-rag-terv.md). People
-- type without accents ("konyv" for "könyv"); the hungarian and simple
-- vectors keep the accents, so such a question matched no keyword at all.
-- A third vector over unaccented text, queried with an unaccented query.
--
-- Not the unaccent extension: the application role may not create
-- extensions, so the migration would stop an existing server from starting.
-- translate() is built in and IMMUTABLE; the letters are the same as the
-- memory adapter's (internal/modules/search/memory).
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

-- +goose Down
DROP INDEX IF EXISTS search_chunk_text_tsv_unaccent_idx;
ALTER TABLE search_chunk_text DROP COLUMN IF EXISTS tsv_unaccent;
DROP FUNCTION IF EXISTS colony_unaccent(text);
