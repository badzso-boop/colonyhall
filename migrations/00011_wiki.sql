-- +goose Up
CREATE TABLE wiki_page (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    slug VARCHAR(80) NOT NULL,
    title VARCHAR(120) NOT NULL,
    tags TEXT[] NOT NULL DEFAULT '{}',
    created_by UUID NOT NULL REFERENCES actor(id),
    created_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT wiki_page_workspace_slug_key UNIQUE (workspace_id, slug)
);

CREATE INDEX wiki_page_workspace_idx ON wiki_page(workspace_id);
CREATE INDEX wiki_page_slug_idx ON wiki_page(slug);

CREATE TABLE wiki_revision (
    page_id UUID NOT NULL REFERENCES wiki_page(id),
    rev INT NOT NULL,
    body TEXT NOT NULL,
    kind VARCHAR(20) NOT NULL,
    author_id UUID NOT NULL REFERENCES actor(id),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (page_id, rev)
);

CREATE INDEX wiki_revision_page_id_rev_desc ON wiki_revision(page_id, rev DESC);

-- +goose Down
DROP TABLE IF EXISTS wiki_revision;
DROP TABLE IF EXISTS wiki_page;
