-- +goose Up
-- Round 51b (docs/24-hosztolt-ajanlat.md, 6. és 7. pont): the operator's
-- plans and break-glass access.

-- Plans: the operator edits them (platform table, operator scope only). A
-- plan limits the storage; the CPU cores and memory are what a hosted
-- guard will get (stored and shown now, enforced later).
CREATE TABLE plan (
    id            uuid PRIMARY KEY,
    name          text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 60),
    storage_bytes bigint NOT NULL CHECK (storage_bytes > 0),
    cpu_cores     integer NOT NULL CHECK (cpu_cores > 0),
    memory_bytes  bigint NOT NULL CHECK (memory_bytes > 0),
    archived_at   timestamptz,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL
);
ALTER TABLE plan ENABLE ROW LEVEL SECURITY;
ALTER TABLE plan FORCE ROW LEVEL SECURITY;
CREATE POLICY operator ON plan USING (colony_operator() OR colony_system())
    WITH CHECK (colony_operator() OR colony_system());

-- A workspace's plan, as the storage module needs it: a copy of the plan's
-- numbers next to the owner's quota (round 54), so the effective quota is
-- computed inside the workspace without reading the platform table. The
-- operator's assignment and plan edits write it. NULL plan_id: no plan,
-- only the owner's quota, as before.
ALTER TABLE workspace_storage ADD COLUMN plan_id uuid;
ALTER TABLE workspace_storage ADD COLUMN plan_name text NOT NULL DEFAULT '';
ALTER TABLE workspace_storage ADD COLUMN plan_storage_bytes bigint NOT NULL DEFAULT 0 CHECK (plan_storage_bytes >= 0);
ALTER TABLE workspace_storage ADD COLUMN plan_cpu_cores integer NOT NULL DEFAULT 0 CHECK (plan_cpu_cores >= 0);
ALTER TABLE workspace_storage ADD COLUMN plan_memory_bytes bigint NOT NULL DEFAULT 0 CHECK (plan_memory_bytes >= 0);

-- Break-glass: the operator's read-only look into one workspace, with a
-- reason, for at most 4 hours (12 in development mode). The customer's
-- audit records it and every read; the owner is notified and may close it.
-- The handoff token (60 s, once) opens the read-only session on the
-- customer's own host; actor_id is the workspace's operator actor.
CREATE TABLE break_glass (
    id                 uuid PRIMARY KEY,
    workspace_id       uuid NOT NULL REFERENCES workspace(id),
    operator_id        uuid NOT NULL,
    operator_handle    text NOT NULL,
    actor_id           uuid NOT NULL REFERENCES actor(id),
    reason             text NOT NULL,
    dev_mode           boolean NOT NULL DEFAULT false,
    opened_at          timestamptz NOT NULL,
    expires_at         timestamptz NOT NULL,
    handoff_hash       bytea NOT NULL UNIQUE,
    handoff_expires_at timestamptz NOT NULL,
    handoff_used_at    timestamptz,
    closed_at          timestamptz,
    closed_by          text CHECK (closed_by IN ('operator', 'owner', 'expired')),
    CHECK (expires_at > opened_at)
);
CREATE INDEX break_glass_workspace_idx ON break_glass (workspace_id, opened_at DESC);
CREATE INDEX break_glass_open_idx ON break_glass (expires_at) WHERE closed_at IS NULL;
ALTER TABLE break_glass ENABLE ROW LEVEL SECURITY;
ALTER TABLE break_glass FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON break_glass USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- The owners' notification of an opened access (round 55's feed).
ALTER TABLE activity_item DROP CONSTRAINT activity_item_kind_check;
ALTER TABLE activity_item ADD CONSTRAINT activity_item_kind_check
    CHECK (kind IN ('mention', 'thread_reply', 'reaction', 'approval', 'question', 'important', 'reminder', 'operator_access'));

-- A session opened by a break-glass handoff (read-only, the operator actor).
ALTER TABLE session ADD COLUMN break_glass_id uuid REFERENCES break_glass(id);

-- The overview (00069) also returns the plan.
DROP FUNCTION colony_operator_overview(uuid, timestamptz);
-- +goose StatementBegin
CREATE FUNCTION colony_operator_overview(only_ws uuid, online_since timestamptz)
RETURNS TABLE (
    ws_id uuid, ws_name text, ws_slug text, ws_state text, ws_suspended_at timestamptz, ws_suspended_reason text,
    ws_created_at timestamptz, has_owner boolean, users_count integer, agents_count integer, hosts_count integer,
    hosts_online integer, last_activity timestamptz, quota_bytes bigint, files_bytes bigint, db_bytes bigint,
    invite_created_at timestamptz, invite_expires_at timestamptz, invite_used boolean, invite_revoked boolean,
    plan_id uuid, plan_storage_bytes bigint, break_glass_open boolean
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
           (SELECT count(*) FROM actor a WHERE a.workspace_id = w.id AND a.archived_at IS NULL AND a.kind = 'human'
                   AND NOT EXISTS (SELECT 1 FROM break_glass g WHERE g.actor_id = a.id))::integer,
           (SELECT count(*) FROM actor a WHERE a.workspace_id = w.id AND a.archived_at IS NULL AND a.kind = 'agent')::integer,
           (SELECT count(*) FROM host h WHERE h.workspace_id = w.id AND h.state <> 'revoked')::integer,
           (SELECT count(*) FROM host h WHERE h.workspace_id = w.id AND h.state <> 'revoked'
                   AND h.last_seen_at IS NOT NULL AND h.last_seen_at >= online_since)::integer,
           -- The operator's own break-glass entries are not the customer's activity.
           (SELECT l.at FROM audit_log l WHERE l.workspace_id = w.id AND l.action NOT LIKE 'operator.break_glass_%'
                   ORDER BY l.seq DESC LIMIT 1),
           coalesce(s.quota_bytes, 0), coalesce(s.files_bytes, 0), coalesce(s.db_bytes, 0),
           i.created_at, i.expires_at, i.used_at IS NOT NULL, i.revoked_at IS NOT NULL,
           s.plan_id, coalesce(s.plan_storage_bytes, 0),
           EXISTS (SELECT 1 FROM break_glass g WHERE g.workspace_id = w.id AND g.closed_at IS NULL)
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
SET LOCAL colony.system = 'on';
DROP FUNCTION colony_operator_overview(uuid, timestamptz);
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
-- A break-glass session must not outlive its rules as an ordinary admin session.
UPDATE session SET revoked_at = now() WHERE break_glass_id IS NOT NULL AND revoked_at IS NULL;
ALTER TABLE session DROP COLUMN break_glass_id;
DELETE FROM activity_item WHERE kind = 'operator_access';
ALTER TABLE activity_item DROP CONSTRAINT activity_item_kind_check;
ALTER TABLE activity_item ADD CONSTRAINT activity_item_kind_check
    CHECK (kind IN ('mention', 'thread_reply', 'reaction', 'approval', 'question', 'important', 'reminder'));
DROP TABLE IF EXISTS break_glass;
ALTER TABLE workspace_storage DROP COLUMN plan_memory_bytes;
ALTER TABLE workspace_storage DROP COLUMN plan_cpu_cores;
ALTER TABLE workspace_storage DROP COLUMN plan_storage_bytes;
ALTER TABLE workspace_storage DROP COLUMN plan_name;
ALTER TABLE workspace_storage DROP COLUMN plan_id;
DROP TABLE IF EXISTS plan;
