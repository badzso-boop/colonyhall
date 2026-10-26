-- +goose Up
CREATE TABLE workspace_compaction_settings (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id),
    compact_threshold_tokens INT NOT NULL DEFAULT 350000 CHECK (compact_threshold_tokens >= 20000 AND compact_threshold_tokens <= 2000000),
    compactor_agent_id UUID REFERENCES actor(id),
    updated_at TIMESTAMPTZ NOT NULL
);

ALTER TABLE channel ADD COLUMN compact_threshold_tokens INT CHECK (compact_threshold_tokens IS NULL OR (compact_threshold_tokens >= 20000 AND compact_threshold_tokens <= 2000000));

CREATE TABLE compaction (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    channel_id UUID NOT NULL REFERENCES channel(id),
    thread_root_id UUID REFERENCES message(id),
    up_to_message_id UUID REFERENCES message(id),
    previous_compaction_id UUID REFERENCES compaction(id),
    trigger VARCHAR(20) NOT NULL, -- auto, manual, regenerate, edit
    requested_by UUID REFERENCES actor(id),
    compactor_agent_id UUID REFERENCES actor(id),
    state VARCHAR(20) NOT NULL, -- pending, done, failed
    summary_file_id UUID REFERENCES file(id),
    system_message_id UUID REFERENCES message(id),
    messages_count INT NOT NULL DEFAULT 0,
    approx_tokens_in INT NOT NULL DEFAULT 0,
    summary_tokens INT NOT NULL DEFAULT 0,
    error_code VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE INDEX compaction_channel_thread_idx ON compaction (channel_id, thread_root_id, created_at DESC);
CREATE INDEX compaction_pending_idx ON compaction (state, created_at) WHERE state = 'pending';
CREATE INDEX compaction_sys_msg_idx ON compaction (system_message_id) WHERE system_message_id IS NOT NULL;

CREATE TABLE compaction_unit (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    channel_id UUID NOT NULL REFERENCES channel(id),
    thread_root_id UUID REFERENCES message(id),
    approx_tokens_since INT NOT NULL DEFAULT 0,
    messages_since INT NOT NULL DEFAULT 0,
    last_compaction_id UUID REFERENCES compaction(id),
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX compaction_unit_thread_idx ON compaction_unit (channel_id, thread_root_id) WHERE thread_root_id IS NOT NULL;
CREATE UNIQUE INDEX compaction_unit_main_idx ON compaction_unit (channel_id) WHERE thread_root_id IS NULL;

-- +goose Down
DROP TABLE IF EXISTS compaction_unit;
DROP TABLE IF EXISTS compaction;
ALTER TABLE channel DROP COLUMN IF EXISTS compact_threshold_tokens;
DROP TABLE IF EXISTS workspace_compaction_settings;
