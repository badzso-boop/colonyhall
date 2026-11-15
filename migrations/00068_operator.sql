-- +goose Up
-- Round 51 (docs/24-hosztolt-ajanlat.md): the platform operator. Operators
-- are not actors: their own tables, under their own row-level security
-- scope (colony.operator = 'on'), which sees these tables and no
-- workspace's rows. A workspace is created by the operator and gets its
-- owner through a one-time invite; the operator can suspend and restore it.

CREATE FUNCTION colony_operator() RETURNS boolean LANGUAGE sql STABLE
    AS $$ SELECT coalesce(current_setting('colony.operator', true), '') = 'on' $$;

CREATE TABLE operator (
    id              uuid PRIMARY KEY,
    handle          text NOT NULL UNIQUE CHECK (handle ~ '^[a-z0-9][a-z0-9_-]{1,31}$'),
    display_name    text NOT NULL,
    password_hash   text NOT NULL,
    -- The TOTP secret, encrypted (internal/platform/secretbox): the key id,
    -- the nonce and the ciphertext. The key is never in the database.
    totp_key_id     text NOT NULL,
    totp_nonce      bytea NOT NULL,
    totp_ciphertext bytea NOT NULL,
    -- The last accepted TOTP time step: a code is accepted once.
    totp_last_step  bigint NOT NULL DEFAULT 0,
    -- pending: created, the first code not confirmed yet; active; disabled.
    state           text NOT NULL CHECK (state IN ('pending', 'active', 'disabled')),
    created_at      timestamptz NOT NULL,
    activated_at    timestamptz,
    last_login_at   timestamptz
);

CREATE TABLE operator_backup_code (
    id          uuid PRIMARY KEY,
    operator_id uuid NOT NULL REFERENCES operator(id),
    -- SHA-256 of the code (10 random characters).
    code_hash   bytea NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL,
    used_at     timestamptz
);
CREATE INDEX operator_backup_code_operator_idx ON operator_backup_code (operator_id) WHERE used_at IS NULL;

CREATE TABLE operator_session (
    id           uuid PRIMARY KEY,
    operator_id  uuid NOT NULL REFERENCES operator(id),
    token_hash   bytea NOT NULL UNIQUE,
    csrf_token   text NOT NULL,
    created_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    -- Absolute end (8 hours); the idle limit (30 minutes) uses last_seen_at.
    expires_at   timestamptz NOT NULL,
    remote_addr  text NOT NULL DEFAULT '',
    user_agent   text NOT NULL DEFAULT '',
    revoked_at   timestamptz
);
CREATE INDEX operator_session_operator_idx ON operator_session (operator_id);

-- Every operator action; append-only. workspace_id names the workspace it
-- concerns, when there is one (no foreign key: the row is the operator's).
CREATE TABLE operator_audit (
    id           uuid PRIMARY KEY,
    -- NULL for a command run on the server (operator-create …).
    operator_id  uuid REFERENCES operator(id),
    action       text NOT NULL,
    workspace_id uuid,
    payload      jsonb NOT NULL DEFAULT '{}',
    remote_addr  text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL
);
CREATE INDEX operator_audit_created_idx ON operator_audit (created_at DESC, id DESC);

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['operator', 'operator_backup_code', 'operator_session', 'operator_audit']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY operator ON %I USING (colony_operator() OR colony_system()) '
                       'WITH CHECK (colony_operator() OR colony_system())', t);
    END LOOP;
END $$;
-- +goose StatementEnd

-- The owner's one-time invite (7 days). Only the hash of the token is
-- stored; the link is shown once, to the operator.
CREATE TABLE workspace_invite (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL REFERENCES workspace(id),
    token_hash          bytea NOT NULL UNIQUE,
    role                text NOT NULL CHECK (role IN ('owner')),
    -- The operator who made it (no foreign key: a platform row).
    created_by_operator uuid,
    created_at          timestamptz NOT NULL,
    expires_at          timestamptz NOT NULL,
    used_at             timestamptz,
    used_by             uuid REFERENCES actor(id),
    revoked_at          timestamptz
);
CREATE INDEX workspace_invite_workspace_idx ON workspace_invite (workspace_id, created_at DESC);
ALTER TABLE workspace_invite ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_invite FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON workspace_invite USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- Suspension: when and why (the reason is for the operator; the customer
-- sees only that the workspace is paused).
ALTER TABLE workspace ADD COLUMN suspended_at timestamptz;
ALTER TABLE workspace ADD COLUMN suspended_reason text NOT NULL DEFAULT '';

-- The operator's login has its own limit counters.
ALTER TABLE auth_failure DROP CONSTRAINT auth_failure_scope_check;
ALTER TABLE auth_failure ADD CONSTRAINT auth_failure_scope_check
    CHECK (scope IN ('login', 'emergency_clear', 'password', 'claude_login', 'operator_login'));

-- +goose Down
SET LOCAL colony.system = 'on';
ALTER TABLE auth_failure DROP CONSTRAINT auth_failure_scope_check;
DELETE FROM auth_failure WHERE scope = 'operator_login';
ALTER TABLE auth_failure ADD CONSTRAINT auth_failure_scope_check
    CHECK (scope IN ('login', 'emergency_clear', 'password', 'claude_login'));
ALTER TABLE workspace DROP COLUMN suspended_reason;
ALTER TABLE workspace DROP COLUMN suspended_at;
DROP TABLE IF EXISTS workspace_invite;
DROP TABLE IF EXISTS operator_audit;
DROP TABLE IF EXISTS operator_session;
DROP TABLE IF EXISTS operator_backup_code;
DROP TABLE IF EXISTS operator;
DROP FUNCTION IF EXISTS colony_operator();
