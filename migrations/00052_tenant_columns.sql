-- +goose Up
-- Round 45 (docs/23-elkulonites.md): every table gets a workspace_id that
-- row-level security can check without a join. Eleven tables reached their
-- workspace through a parent row; they get a copy, filled from the parent,
-- and new rows take it from the connection's scope (colony.workspace_id,
-- set by the server's pool), so no INSERT changes. Composite foreign keys
-- make a reference to another workspace's row impossible, even for a
-- program error (a foreign key check ignores row-level security).

-- The connection's workspace; NULL when unset (the NOT NULL columns then
-- refuse the row, nothing lands without a workspace).
CREATE FUNCTION colony_ws() RETURNS uuid LANGUAGE sql STABLE
    AS $$ SELECT nullif(current_setting('colony.workspace_id', true), '')::uuid $$;

-- Parents: (id, workspace_id) as a key the children can reference.
ALTER TABLE actor ADD CONSTRAINT actor_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE channel ADD CONSTRAINT channel_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE message ADD CONSTRAINT message_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE file ADD CONSTRAINT file_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE sidebar_section ADD CONSTRAINT sidebar_section_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE planner_calendar ADD CONSTRAINT planner_calendar_id_workspace_key UNIQUE (id, workspace_id);
ALTER TABLE wiki_page ADD CONSTRAINT wiki_page_id_workspace_key UNIQUE (id, workspace_id);

-- Expand.
ALTER TABLE api_token ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE session ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE credential ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE channel_member ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE channel_mute ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE reaction ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE thread_read ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE message_file ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE sidebar_item ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE planner_calendar_event ADD COLUMN workspace_id uuid DEFAULT colony_ws();
ALTER TABLE wiki_revision ADD COLUMN workspace_id uuid DEFAULT colony_ws();

-- Backfill from the parents.
UPDATE api_token t SET workspace_id = a.workspace_id FROM actor a WHERE a.id = t.actor_id;
UPDATE session t SET workspace_id = a.workspace_id FROM actor a WHERE a.id = t.actor_id;
UPDATE credential t SET workspace_id = a.workspace_id FROM actor a WHERE a.id = t.actor_id;
UPDATE channel_member t SET workspace_id = c.workspace_id FROM channel c WHERE c.id = t.channel_id;
UPDATE channel_mute t SET workspace_id = c.workspace_id FROM channel c WHERE c.id = t.channel_id;
UPDATE reaction t SET workspace_id = m.workspace_id FROM message m WHERE m.id = t.message_id;
UPDATE thread_read t SET workspace_id = m.workspace_id FROM message m WHERE m.id = t.thread_root_id;
UPDATE message_file t SET workspace_id = m.workspace_id FROM message m WHERE m.id = t.message_id;
UPDATE sidebar_item t SET workspace_id = s.workspace_id FROM sidebar_section s WHERE s.id = t.section_id;
UPDATE planner_calendar_event t SET workspace_id = c.workspace_id FROM planner_calendar c WHERE c.id = t.calendar_id;
UPDATE wiki_revision t SET workspace_id = p.workspace_id FROM wiki_page p WHERE p.id = t.page_id;

-- Contract: required, and every reference stays inside the workspace.
ALTER TABLE api_token ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT api_token_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id);
ALTER TABLE session ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT session_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id);
ALTER TABLE credential ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT credential_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id);
ALTER TABLE channel_member ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT channel_member_channel_ws_fkey FOREIGN KEY (channel_id, workspace_id) REFERENCES channel (id, workspace_id),
    ADD CONSTRAINT channel_member_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id);
ALTER TABLE channel_mute ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT channel_mute_channel_ws_fkey FOREIGN KEY (channel_id, workspace_id) REFERENCES channel (id, workspace_id),
    ADD CONSTRAINT channel_mute_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id);
ALTER TABLE reaction ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT reaction_message_ws_fkey FOREIGN KEY (message_id, workspace_id) REFERENCES message (id, workspace_id),
    ADD CONSTRAINT reaction_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id);
ALTER TABLE thread_read ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT thread_read_message_ws_fkey FOREIGN KEY (thread_root_id, workspace_id) REFERENCES message (id, workspace_id),
    ADD CONSTRAINT thread_read_actor_ws_fkey FOREIGN KEY (actor_id, workspace_id) REFERENCES actor (id, workspace_id);
ALTER TABLE message_file ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT message_file_message_ws_fkey FOREIGN KEY (message_id, workspace_id) REFERENCES message (id, workspace_id),
    ADD CONSTRAINT message_file_file_ws_fkey FOREIGN KEY (file_id, workspace_id) REFERENCES file (id, workspace_id);
ALTER TABLE sidebar_item ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT sidebar_item_section_ws_fkey FOREIGN KEY (section_id, workspace_id) REFERENCES sidebar_section (id, workspace_id),
    ADD CONSTRAINT sidebar_item_channel_ws_fkey FOREIGN KEY (channel_id, workspace_id) REFERENCES channel (id, workspace_id);
ALTER TABLE planner_calendar_event ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT planner_calendar_event_calendar_ws_fkey FOREIGN KEY (calendar_id, workspace_id) REFERENCES planner_calendar (id, workspace_id);
ALTER TABLE wiki_revision ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT wiki_revision_page_ws_fkey FOREIGN KEY (page_id, workspace_id) REFERENCES wiki_page (id, workspace_id);

-- The git link cache becomes per workspace: a private repository's title
-- must not show in another customer's workspace. It is only a cache, so
-- the old rows go and are fetched again.
DELETE FROM git_link;
ALTER TABLE git_link DROP CONSTRAINT git_link_pkey;
ALTER TABLE git_link ADD COLUMN workspace_id uuid NOT NULL DEFAULT colony_ws() REFERENCES workspace (id),
    ADD PRIMARY KEY (workspace_id, url);

-- +goose Down
DELETE FROM git_link;
ALTER TABLE git_link DROP CONSTRAINT git_link_pkey;
ALTER TABLE git_link DROP COLUMN workspace_id;
ALTER TABLE git_link ADD PRIMARY KEY (url);
ALTER TABLE wiki_revision DROP COLUMN workspace_id;
ALTER TABLE planner_calendar_event DROP COLUMN workspace_id;
ALTER TABLE sidebar_item DROP COLUMN workspace_id;
ALTER TABLE message_file DROP COLUMN workspace_id;
ALTER TABLE thread_read DROP COLUMN workspace_id;
ALTER TABLE reaction DROP COLUMN workspace_id;
ALTER TABLE channel_mute DROP COLUMN workspace_id;
ALTER TABLE channel_member DROP COLUMN workspace_id;
ALTER TABLE credential DROP COLUMN workspace_id;
ALTER TABLE session DROP COLUMN workspace_id;
ALTER TABLE api_token DROP COLUMN workspace_id;
ALTER TABLE wiki_page DROP CONSTRAINT wiki_page_id_workspace_key;
ALTER TABLE planner_calendar DROP CONSTRAINT planner_calendar_id_workspace_key;
ALTER TABLE sidebar_section DROP CONSTRAINT sidebar_section_id_workspace_key;
ALTER TABLE file DROP CONSTRAINT file_id_workspace_key;
ALTER TABLE message DROP CONSTRAINT message_id_workspace_key;
ALTER TABLE channel DROP CONSTRAINT channel_id_workspace_key;
ALTER TABLE actor DROP CONSTRAINT actor_id_workspace_key;
DROP FUNCTION colony_ws();
