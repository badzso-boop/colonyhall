-- +goose Up
-- A human's personal task list and subscribed ICS calendars. Only the owner
-- writes them; agents the owner allowed may read them.
CREATE TABLE planner_task (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspace(id),
    owner_id          uuid NOT NULL REFERENCES actor(id),
    title             text NOT NULL,
    description       text NOT NULL DEFAULT '',
    status            text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'started', 'done')),
    position          double precision NOT NULL,
    started_at        timestamptz,
    done_at           timestamptz,
    status_changed_at timestamptz NOT NULL,
    deleted_at        timestamptz,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX planner_task_owner_idx ON planner_task (owner_id, position) WHERE deleted_at IS NULL;

CREATE TABLE planner_access (
    owner_id       uuid PRIMARY KEY REFERENCES actor(id),
    workspace_id   uuid NOT NULL REFERENCES workspace(id),
    task_all       boolean NOT NULL DEFAULT true,
    task_agent_ids uuid[] NOT NULL DEFAULT '{}',
    cal_agent_ids  uuid[] NOT NULL DEFAULT '{}',
    updated_at     timestamptz NOT NULL
);

-- The URL is a secret (it carries an access token): it is never returned.
CREATE TABLE planner_calendar (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    owner_id        uuid NOT NULL REFERENCES actor(id),
    name            text NOT NULL,
    url             text NOT NULL,
    colour          text NOT NULL,
    last_fetched_at timestamptz,
    last_error      text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL
);

-- A cache of parsed occurrences, replaced on each successful fetch.
CREATE TABLE planner_calendar_event (
    calendar_id uuid NOT NULL REFERENCES planner_calendar(id) ON DELETE CASCADE,
    owner_id    uuid NOT NULL REFERENCES actor(id),
    title       text NOT NULL,
    starts_at   timestamptz NOT NULL,
    ends_at     timestamptz NOT NULL,
    all_day     boolean NOT NULL,
    location    text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT ''
);
CREATE INDEX planner_calendar_event_owner_idx ON planner_calendar_event (owner_id, starts_at);

-- +goose Down
DROP TABLE planner_calendar_event;
DROP TABLE planner_calendar;
DROP TABLE planner_access;
DROP TABLE planner_task;
