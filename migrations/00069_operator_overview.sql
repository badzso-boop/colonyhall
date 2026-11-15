-- +goose Up
-- Round 51 (docs/24-hosztolt-ajanlat.md, 3. pont): the operator's overview
-- of the workspaces, read without leaving the operator scope. The function
-- runs only for the operator (colony.operator = 'on'; anything else is an
-- error), switches system mode on for its own query only and puts the
-- previous value back (the pattern of 00053's lookups), and returns account
-- data and numbers only: no channel, message, file or person names. Its
-- counting rules are the ones the PWA showed before: actors not archived,
-- hosts not revoked, a host online when seen since online_since, the last
-- activity the newest audit entry, the newest owner invite.
-- only_ws: one workspace, or NULL / the nil UUID for all.
-- +goose StatementBegin
CREATE FUNCTION colony_operator_overview(only_ws uuid, online_since timestamptz)
RETURNS TABLE (
    ws_id uuid, ws_name text, ws_slug text, ws_state text, ws_suspended_at timestamptz, ws_suspended_reason text,
    ws_created_at timestamptz, has_owner boolean, users_count integer, agents_count integer, hosts_count integer,
    hosts_online integer, last_activity timestamptz, quota_bytes bigint, files_bytes bigint, db_bytes bigint,
    invite_created_at timestamptz, invite_expires_at timestamptz, invite_used boolean, invite_revoked boolean
) LANGUAGE plpgsql AS $$
DECLARE prev text := coalesce(current_setting('colony.system', true), '');
BEGIN
    IF NOT colony_operator() THEN
        RAISE EXCEPTION 'colony_operator_overview: the operator scope only' USING ERRCODE = '42501';
    END IF;
    PERFORM set_config('colony.system', 'on', true);
    RETURN QUERY
    SELECT w.id, w.name, w.slug, w.state, w.suspended_at, w.suspended_reason, w.created_at,
           EXISTS (SELECT 1 FROM actor a WHERE a.workspace_id = w.id AND a.archived_at IS NULL
                   AND a.kind = 'human' AND a.role = 'owner'),
           (SELECT count(*) FROM actor a WHERE a.workspace_id = w.id AND a.archived_at IS NULL AND a.kind = 'human')::integer,
           (SELECT count(*) FROM actor a WHERE a.workspace_id = w.id AND a.archived_at IS NULL AND a.kind = 'agent')::integer,
           (SELECT count(*) FROM host h WHERE h.workspace_id = w.id AND h.state <> 'revoked')::integer,
           (SELECT count(*) FROM host h WHERE h.workspace_id = w.id AND h.state <> 'revoked'
                   AND h.last_seen_at IS NOT NULL AND h.last_seen_at >= online_since)::integer,
           (SELECT l.at FROM audit_log l WHERE l.workspace_id = w.id ORDER BY l.seq DESC LIMIT 1),
           coalesce(s.quota_bytes, 0), coalesce(s.files_bytes, 0), coalesce(s.db_bytes, 0),
           i.created_at, i.expires_at, i.used_at IS NOT NULL, i.revoked_at IS NOT NULL
    FROM workspace w
    LEFT JOIN workspace_storage s ON s.workspace_id = w.id
    LEFT JOIN LATERAL (SELECT x.created_at, x.expires_at, x.used_at, x.revoked_at FROM workspace_invite x
                       WHERE x.workspace_id = w.id ORDER BY x.created_at DESC, x.id DESC LIMIT 1) i ON true
    WHERE only_ws IS NULL OR only_ws = '00000000-0000-0000-0000-000000000000'::uuid OR w.id = only_ws
    ORDER BY w.created_at, w.id;
    PERFORM set_config('colony.system', prev, true);
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS colony_operator_overview(uuid, timestamptz);
