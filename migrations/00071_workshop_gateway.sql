-- +goose Up
-- Round 56a (docs/25-onfejlesztes-terv.md, 13.2): the Workshop's MCP
-- gateway. Encrypted workspace secrets (brought forward from docs/24 2.3),
-- connectors to external MCP servers and HTTP APIs, per-agent grants, and
-- the append-only call log.

-- Secrets are sealed with secretbox (COLONY_SECRET_KEYS); the associated
-- data binds each value to its row and workspace. Never read back whole:
-- hint is the last 4 characters.
CREATE TABLE workspace_secret (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    kind         text NOT NULL CHECK (kind IN ('connector_token', 'connector_header', 'connector_basic',
                                               'ai_api_key', 'repo_deploy_key', 'repo_token')),
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    provider     text NOT NULL DEFAULT '',
    key_id       text NOT NULL,
    nonce        bytea NOT NULL,
    ciphertext   bytea NOT NULL,
    hint         text NOT NULL DEFAULT '',
    created_by   uuid NOT NULL REFERENCES actor(id),
    created_at   timestamptz NOT NULL,
    rotated_at   timestamptz,
    deleted_at   timestamptz
);
CREATE INDEX workspace_secret_ws_idx ON workspace_secret (workspace_id, created_at) WHERE deleted_at IS NULL;
ALTER TABLE workspace_secret ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_secret FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON workspace_secret USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- A connector: an external MCP server (its tools appear in the gateway as
-- <slug>__<tool>) or a plain HTTP API (used by the declarative tools of
-- round 56b). tools_cache holds the MCP server's tool list with the risk of
-- each tool; risk_overrides is the admin's per-tool correction.
CREATE TABLE connector (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    slug            text NOT NULL CHECK (slug ~ '^[a-z][a-z0-9_]{1,15}$' AND position('__' in slug) = 0),
    title           text NOT NULL CHECK (length(title) BETWEEN 1 AND 80),
    kind            text NOT NULL CHECK (kind IN ('mcp', 'http')),
    base_url        text NOT NULL CHECK (length(base_url) BETWEEN 1 AND 500),
    auth            text NOT NULL CHECK (auth IN ('none', 'bearer', 'header', 'basic')),
    header_name     text NOT NULL DEFAULT '',
    secret_id       uuid REFERENCES workspace_secret(id),
    approval_off    boolean NOT NULL DEFAULT false,
    risk_overrides  jsonb NOT NULL DEFAULT '{}'::jsonb,
    tools_cache     jsonb NOT NULL DEFAULT '[]'::jsonb,
    tools_cached_at timestamptz,
    state           text NOT NULL DEFAULT 'new' CHECK (state IN ('new', 'ok', 'error')),
    last_error      text NOT NULL DEFAULT '',
    created_by      uuid NOT NULL REFERENCES actor(id),
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    deleted_at      timestamptz
);
CREATE UNIQUE INDEX connector_slug_idx ON connector (workspace_id, slug) WHERE deleted_at IS NULL;
ALTER TABLE connector ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON connector USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- Which agent may use which connector (and, from 56b, which extension and
-- the Workshop's meta-tools). Without a grant an agent sees only the core
-- tools, as before. Revoking sets revoked_at; a new grant is a new row.
CREATE TABLE agent_tool_grant (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    agent_id     uuid NOT NULL REFERENCES actor(id),
    target_kind  text NOT NULL CHECK (target_kind IN ('connector', 'extension', 'workshop')),
    target_id    uuid,
    granted_by   uuid NOT NULL REFERENCES actor(id),
    created_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    CHECK ((target_kind = 'workshop') = (target_id IS NULL))
);
CREATE UNIQUE INDEX agent_tool_grant_live_idx ON agent_tool_grant (workspace_id, agent_id, target_kind, COALESCE(target_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE revoked_at IS NULL;
ALTER TABLE agent_tool_grant ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_tool_grant FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON agent_tool_grant USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- The call log: append-only (CLAUDE.md rule 4), without content: who
-- called which tool, how long, how big, and the outcome.
CREATE TABLE tool_call (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspace(id),
    actor_id     uuid NOT NULL REFERENCES actor(id),
    target_kind  text NOT NULL CHECK (target_kind IN ('connector', 'extension')),
    target_id    uuid NOT NULL,
    tool         text NOT NULL,
    risk         text NOT NULL CHECK (risk IN ('read', 'write', 'external')),
    approval_id  uuid,
    started_at   timestamptz NOT NULL,
    duration_ms  integer NOT NULL CHECK (duration_ms >= 0),
    in_bytes     integer NOT NULL CHECK (in_bytes >= 0),
    out_bytes    integer NOT NULL CHECK (out_bytes >= 0),
    outcome      text NOT NULL CHECK (outcome IN ('ok', 'error', 'denied', 'timeout', 'limit')),
    error        text NOT NULL DEFAULT ''
);
CREATE INDEX tool_call_ws_idx ON tool_call (workspace_id, started_at DESC, id DESC);
CREATE INDEX tool_call_target_idx ON tool_call (workspace_id, target_kind, target_id, started_at DESC);
ALTER TABLE tool_call ENABLE ROW LEVEL SECURITY;
ALTER TABLE tool_call FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON tool_call USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS tool_call;
DROP TABLE IF EXISTS agent_tool_grant;
DROP TABLE IF EXISTS connector;
DROP TABLE IF EXISTS workspace_secret;
