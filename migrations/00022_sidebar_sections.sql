-- +goose Up
CREATE TABLE sidebar_section (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES actor(id) ON DELETE CASCADE,
    name VARCHAR(40) NOT NULL,
    position INT NOT NULL DEFAULT 0,
    collapsed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_sidebar_section_owner ON sidebar_section(workspace_id, owner_id) WHERE deleted_at IS NULL;

CREATE TABLE sidebar_item (
    section_id UUID NOT NULL REFERENCES sidebar_section(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
    position INT NOT NULL DEFAULT 0,
    PRIMARY KEY (section_id, channel_id)
);

CREATE INDEX idx_sidebar_item_channel ON sidebar_item(channel_id);

-- +goose Down
DROP TABLE IF EXISTS sidebar_item;
DROP TABLE IF EXISTS sidebar_section;
