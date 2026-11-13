-- +goose Up
-- Round 40: durable login and release limits. Failed password checks (and
-- the Claude-login relay's attempts) are counted per handle and per client
-- IP over a sliding window, so a restart no longer forgets them.
--
-- workspace_id is NULLABLE on purpose (a justified exception to rule 10):
-- a failure is recorded before anyone is authenticated, and a guessed handle
-- may belong to no workspace (or to several in single-tenant mode). A row
-- gets the workspace when it is known (the handle's actor, or the
-- subdomain's workspace in multi-tenant mode). The tenant policy hides rows
-- without one from every workspace scope; the pre-auth paths read and write
-- through the narrow functions below, like the other lookups of round 45.
CREATE TABLE auth_failure (
    id           uuid PRIMARY KEY,
    workspace_id uuid REFERENCES workspace(id),
    -- login, emergency_clear, password, claude_login
    scope        text NOT NULL CHECK (scope IN ('login', 'emergency_clear', 'password', 'claude_login')),
    -- Lower-cased, at most 64 characters; empty when not given.
    handle       text NOT NULL CHECK (length(handle) <= 64),
    -- The client IP (canonical text form); empty when unknown.
    ip           text NOT NULL CHECK (length(ip) <= 64),
    failed_at    timestamptz NOT NULL,
    -- Set on the failure that reached a limit: until when it locks.
    locked_until timestamptz,
    -- Set when a later success cleared the handle's count (the row stays for
    -- the lockout history until the hourly cleanup).
    cleared_at   timestamptz
);

CREATE INDEX auth_failure_handle_idx ON auth_failure (scope, handle, failed_at);
CREATE INDEX auth_failure_ip_idx ON auth_failure (scope, ip, failed_at);
CREATE INDEX auth_failure_lockout_idx ON auth_failure (workspace_id, failed_at) WHERE locked_until IS NOT NULL;

ALTER TABLE auth_failure ENABLE ROW LEVEL SECURITY;
ALTER TABLE auth_failure FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON auth_failure USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose StatementBegin
CREATE FUNCTION colony_record_auth_failure(fid uuid, ws uuid, sc text, h text, addr text, at timestamptz, until timestamptz)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    INSERT INTO auth_failure (id, workspace_id, scope, handle, ip, failed_at, locked_until)
    VALUES (fid, ws, sc, h, addr, at, until);
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
-- The counted failures since a time, for a handle and for an IP (an empty
-- key matches nothing). Only kind and time leave the function.
CREATE FUNCTION colony_auth_failures(sc text, h text, addr text, since timestamptz)
RETURNS TABLE (by_ip boolean, failed_at timestamptz) LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    RETURN QUERY
        SELECT false, f.failed_at FROM auth_failure f
         WHERE h <> '' AND f.scope = sc AND f.handle = h AND f.failed_at > since AND f.cleared_at IS NULL
        UNION ALL
        SELECT true, f.failed_at FROM auth_failure f
         WHERE addr <> '' AND f.scope = sc AND f.ip = addr AND f.failed_at > since
        ORDER BY 2 LIMIT 1000;
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
-- A success clears the handle's count (never the IP's: one valid account
-- must not reset the counter of an address that guesses others).
CREATE FUNCTION colony_clear_auth_failures(sc text, h text, at timestamptz)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    UPDATE auth_failure SET cleared_at = at WHERE scope = sc AND handle = h AND cleared_at IS NULL;
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
-- Lockouts that belong to no workspace (an unknown handle), for the system
-- status card in single-tenant mode only.
CREATE FUNCTION colony_unattributed_lockouts(since timestamptz)
RETURNS TABLE (scope text, handle text, ip text, failed_at timestamptz, locked_until timestamptz) LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    RETURN QUERY SELECT f.scope, f.handle, f.ip, f.failed_at, f.locked_until FROM auth_failure f
                 WHERE f.workspace_id IS NULL AND f.locked_until IS NOT NULL AND f.failed_at > since
                 ORDER BY f.failed_at DESC LIMIT 100;
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION colony_unattributed_lockouts(timestamptz);
DROP FUNCTION colony_clear_auth_failures(text, text, timestamptz);
DROP FUNCTION colony_auth_failures(text, text, text, timestamptz);
DROP FUNCTION colony_record_auth_failure(uuid, uuid, text, text, text, timestamptz, timestamptz);
DROP TABLE IF EXISTS auth_failure;
