-- +goose Up
-- Round 56b (docs/25-onfejlesztes-terv.md 13.2): the Workshop's
-- extensions. An extension has a bot (its own agent actor, named at
-- creation, so its messages and calls are traceable), immutable versions
-- (manifest + source + compiled bundle), approvals of versions, and a small
-- JSON store.

CREATE TABLE extension (
    id                 uuid PRIMARY KEY,
    workspace_id       uuid NOT NULL REFERENCES workspace(id),
    slug               text NOT NULL CHECK (slug ~ '^[a-z][a-z0-9_]{1,15}$' AND position('__' in slug) = 0),
    title              text NOT NULL CHECK (length(title) BETWEEN 1 AND 80),
    description        text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    maintainer_id      uuid NOT NULL REFERENCES actor(id),
    bot_id             uuid NOT NULL REFERENCES actor(id),
    state              text NOT NULL CHECK (state IN ('draft', 'live', 'disabled')),
    live_version       integer,
    approval_off       boolean NOT NULL DEFAULT false,
    disabled_reason    text NOT NULL DEFAULT '',
    -- The automatic switch-off (docs/25 13.4): failed calls in a row, and
    -- the times of the recent failures.
    consecutive_errors integer NOT NULL DEFAULT 0,
    recent_errors      jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    deleted_at         timestamptz
);
CREATE UNIQUE INDEX extension_slug_idx ON extension (workspace_id, slug) WHERE deleted_at IS NULL;
ALTER TABLE extension ENABLE ROW LEVEL SECURITY;
ALTER TABLE extension FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON extension USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- Versions never change (CLAUDE.md rule 4).
CREATE TABLE extension_version (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspace(id),
    extension_id  uuid NOT NULL REFERENCES extension(id),
    version       integer NOT NULL CHECK (version > 0),
    manifest      jsonb NOT NULL,
    source        text NOT NULL DEFAULT '',
    bundle        text NOT NULL DEFAULT '',
    bundle_sha256 text NOT NULL DEFAULT '',
    created_by    uuid NOT NULL REFERENCES actor(id),
    created_at    timestamptz NOT NULL,
    UNIQUE (extension_id, version)
);
ALTER TABLE extension_version ENABLE ROW LEVEL SECURITY;
ALTER TABLE extension_version FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON extension_version USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- An approval of a version, with what it adds (append-only).
CREATE TABLE extension_approval (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    extension_id uuid NOT NULL REFERENCES extension(id),
    version      integer NOT NULL,
    decided_by   uuid NOT NULL REFERENCES actor(id),
    diff         jsonb NOT NULL,
    created_at   timestamptz NOT NULL
);
CREATE INDEX extension_approval_idx ON extension_approval (extension_id, version);
ALTER TABLE extension_approval ENABLE ROW LEVEL SECURITY;
ALTER TABLE extension_approval FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON extension_approval USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- The extension's store: JSON documents by collection (counts towards the
-- storage quota through the database share).
CREATE TABLE ext_store_doc (
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    extension_id uuid NOT NULL REFERENCES extension(id),
    collection   text NOT NULL CHECK (collection ~ '^[a-z][a-z0-9_]{0,31}$'),
    doc_id       text NOT NULL CHECK (length(doc_id) BETWEEN 1 AND 128),
    data         jsonb NOT NULL,
    size_bytes   integer NOT NULL CHECK (size_bytes >= 0),
    updated_at   timestamptz NOT NULL,
    PRIMARY KEY (extension_id, collection, doc_id)
);
ALTER TABLE ext_store_doc ENABLE ROW LEVEL SECURITY;
ALTER TABLE ext_store_doc FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON ext_store_doc USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- The monthly call budget of a plan (NULL = unlimited, the default;
-- decision 2026-11-17), copied next to the workspace's storage like the
-- other plan numbers.
ALTER TABLE plan ADD COLUMN ext_calls_month integer CHECK (ext_calls_month IS NULL OR ext_calls_month > 0);
ALTER TABLE workspace_storage ADD COLUMN plan_ext_calls_month integer CHECK (plan_ext_calls_month IS NULL OR plan_ext_calls_month > 0);
CREATE INDEX tool_call_month_idx ON tool_call (workspace_id, started_at) WHERE outcome <> 'denied';

-- +goose Down
DROP INDEX IF EXISTS tool_call_month_idx;
ALTER TABLE workspace_storage DROP COLUMN IF EXISTS plan_ext_calls_month;
ALTER TABLE plan DROP COLUMN IF EXISTS ext_calls_month;
DROP TABLE IF EXISTS ext_store_doc;
DROP TABLE IF EXISTS extension_approval;
DROP TABLE IF EXISTS extension_version;
DROP TABLE IF EXISTS extension;
