-- +goose Up
-- Round 54: workspace storage. A person can delete the files they uploaded
-- (soft delete: the row and its events stay; messages show "File deleted";
-- the content leaves the store once no live row references its hash). A
-- workspace has a usage (its live files plus its share of the database), an
-- optional quota (0 = unlimited, the default) and the warning level it last
-- announced, so each crossing of 80% / 95% / full warns once.
ALTER TABLE file ADD COLUMN deleted_at timestamptz;
-- Set by the sweep when no live row referenced the hash any more and the
-- content was removed from the store (so the sweep does not look again).
ALTER TABLE file ADD COLUMN content_removed_at timestamptz;

-- "My files": a person's live files, newest first.
CREATE INDEX file_uploader_idx ON file (workspace_id, uploaded_by, created_at DESC, id DESC) WHERE deleted_at IS NULL;
-- The sweep: deleted rows whose content may still be in the store.
CREATE INDEX file_sweep_idx ON file (sha256) WHERE deleted_at IS NOT NULL AND content_removed_at IS NULL;

CREATE TABLE workspace_storage (
    workspace_id   uuid PRIMARY KEY REFERENCES workspace(id),
    -- 0 is unlimited.
    quota_bytes    bigint NOT NULL DEFAULT 0 CHECK (quota_bytes >= 0),
    -- Live files, each content once per workspace; kept exact on every
    -- upload and delete.
    files_bytes    bigint NOT NULL DEFAULT 0 CHECK (files_bytes >= 0),
    -- The workspace's share of the database (the hourly estimate), and the
    -- part of it that is chat and search data (messages, events, search).
    db_bytes       bigint NOT NULL DEFAULT 0 CHECK (db_bytes >= 0),
    chat_bytes     bigint NOT NULL DEFAULT 0 CHECK (chat_bytes >= 0),
    db_measured_at timestamptz,
    -- The warning level last announced: 0, 80, 95 or 100 (full).
    warned_level   smallint NOT NULL DEFAULT 0 CHECK (warned_level IN (0, 80, 95, 100)),
    updated_at     timestamptz NOT NULL
);

ALTER TABLE workspace_storage ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_storage FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON workspace_storage USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS workspace_storage;
DROP INDEX IF EXISTS file_sweep_idx;
DROP INDEX IF EXISTS file_uploader_idx;
ALTER TABLE file DROP COLUMN IF EXISTS content_removed_at;
ALTER TABLE file DROP COLUMN IF EXISTS deleted_at;
