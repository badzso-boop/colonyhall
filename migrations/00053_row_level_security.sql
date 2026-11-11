-- +goose Up
-- Round 45 (docs/23-elkulonites.md): row-level security on every table.
-- A row is visible and writable when its workspace is the connection's
-- (colony.workspace_id) or the connection is in system mode (colony.system
-- = 'on', background code only). Without a setting nothing is visible.
-- FORCE makes it apply to the tables' owner too (the server runs the
-- migrations, so it owns the tables); a superuser or a BYPASSRLS role
-- still bypasses it, which the server reports at start-up.

CREATE FUNCTION colony_system() RETURNS boolean LANGUAGE sql STABLE
    AS $$ SELECT coalesce(current_setting('colony.system', true), '') = 'on' $$;

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'actor', 'agent_host', 'agent_instructions', 'agent_note', 'agent_rules', 'agent_run', 'api_token',
        'approval_request', 'audit_log', 'channel', 'channel_member', 'channel_mute', 'compaction',
        'compaction_unit', 'credential', 'decision', 'delegation', 'delegation_grant', 'delegation_settings',
        'delivery', 'event', 'file', 'file_fetch_request', 'folder_grant', 'git_link', 'git_repo_channel',
        'host', 'host_pairing_code', 'lease', 'message', 'message_file', 'model_price', 'planner_access',
        'planner_calendar', 'planner_calendar_event', 'planner_task', 'preview', 'question', 'reaction',
        'run_error', 'schedule', 'session', 'sidebar_item', 'sidebar_section', 'terminal_event',
        'thread_read', 'wiki_page', 'wiki_revision', 'workspace_compaction_settings', 'workspace_currency',
        'workspace_file_fetch_settings']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY tenant ON %I USING (workspace_id = colony_ws() OR colony_system()) '
                       'WITH CHECK (workspace_id = colony_ws() OR colony_system())', t);
    END LOOP;
END $$;
-- +goose StatementEnd

-- The workspace row itself: its own id.
ALTER TABLE workspace ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON workspace USING (id = colony_ws() OR colony_system())
    WITH CHECK (id = colony_ws() OR colony_system());

-- Webhook delivery ids (repetition filter, no customer data): system only,
-- written through colony_record_webhook_delivery.
ALTER TABLE git_webhook_delivery ENABLE ROW LEVEL SECURITY;
ALTER TABLE git_webhook_delivery FORCE ROW LEVEL SECURITY;
CREATE POLICY system_only ON git_webhook_delivery USING (colony_system()) WITH CHECK (colony_system());

-- consumer_offset (consumer names and sequence numbers) has no customer
-- data and is written inside each event's own workspace scope: no policy.

-- Lookups that run before the workspace is known. Each answers one narrow
-- question (which workspace does this credential belong to); the real
-- query then runs in that workspace's scope. They switch system mode on
-- for their own query only and put the previous value back (a function's
-- SET clause would do this, but Postgres allows it on a custom setting to
-- superusers only). If the query fails, the transaction is aborted anyway.
-- +goose StatementBegin
CREATE FUNCTION colony_token_workspace(h bytea) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), ''); r uuid;
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    SELECT workspace_id INTO r FROM api_token WHERE token_hash = h;
    PERFORM set_config('colony.system', prev, true);
    RETURN r;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_session_workspace(h bytea) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), ''); r uuid;
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    SELECT workspace_id INTO r FROM session WHERE token_hash = h;
    PERFORM set_config('colony.system', prev, true);
    RETURN r;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_host_workspace(h bytea) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), ''); r uuid;
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    SELECT workspace_id INTO r FROM host WHERE token_hash = h;
    PERFORM set_config('colony.system', prev, true);
    RETURN r;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_pairing_workspace(h bytea) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), ''); r uuid;
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    SELECT workspace_id INTO r FROM host_pairing_code WHERE code_hash = h;
    PERFORM set_config('colony.system', prev, true);
    RETURN r;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_handle_workspaces(h text) RETURNS TABLE (id uuid, name text) LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    RETURN QUERY SELECT w.id, w.name FROM actor a JOIN workspace w ON w.id = a.workspace_id
                 WHERE a.handle = h AND a.archived_at IS NULL ORDER BY w.id;
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_workspace_by_slug(s text) RETURNS SETOF workspace LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    RETURN QUERY SELECT * FROM workspace WHERE slug = s;
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_repo_channels(r text) RETURNS SETOF git_repo_channel LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    RETURN QUERY SELECT * FROM git_repo_channel WHERE repo = r ORDER BY workspace_id, channel_id;
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_record_webhook_delivery(d text, at timestamptz) RETURNS boolean LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), ''); n integer;
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    INSERT INTO git_webhook_delivery (delivery_id, received_at) VALUES (d, at) ON CONFLICT (delivery_id) DO NOTHING;
    GET DIAGNOSTICS n = ROW_COUNT;
    PERFORM set_config('colony.system', prev, true);
    RETURN n > 0;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION colony_event_head() RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), ''); r bigint;
BEGIN
    PERFORM set_config('colony.system', 'on', true);
    SELECT coalesce(max(seq), 0) INTO r FROM event;
    PERFORM set_config('colony.system', prev, true);
    RETURN r;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION colony_event_head();
DROP FUNCTION colony_record_webhook_delivery(text, timestamptz);
DROP FUNCTION colony_repo_channels(text);
DROP FUNCTION colony_pairing_workspace(bytea);
DROP FUNCTION colony_host_workspace(bytea);
DROP FUNCTION colony_workspace_by_slug(text);
DROP FUNCTION colony_handle_workspaces(text);
DROP FUNCTION colony_session_workspace(bytea);
DROP FUNCTION colony_token_workspace(bytea);
DROP POLICY system_only ON git_webhook_delivery;
ALTER TABLE git_webhook_delivery NO FORCE ROW LEVEL SECURITY;
ALTER TABLE git_webhook_delivery DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant ON workspace;
ALTER TABLE workspace NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workspace DISABLE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_policies WHERE schemaname = 'public' AND policyname = 'tenant' LOOP
        EXECUTE format('DROP POLICY tenant ON %I', t);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;
-- +goose StatementEnd
DROP FUNCTION colony_system();
