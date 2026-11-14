// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package wiki

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists wiki pages and revisions. Implementations: postgres, memory.
// Lookups return db.ErrNotFound when nothing matches.
type Store interface {
	CreatePage(ctx context.Context, page Page, initialRev Revision) error
	AddRevision(ctx context.Context, rev Revision) error
	UpdatePageMeta(ctx context.Context, id ids.ID, title string, slug string, tags []string) error
	SoftDelete(ctx context.Context, id ids.ID, at time.Time) error
	Restore(ctx context.Context, id ids.ID) error
	PageBySlug(ctx context.Context, workspaceID ids.ID, slug string) (Page, error)
	PageByID(ctx context.Context, id ids.ID) (Page, error)
	ListPages(ctx context.Context, workspaceID ids.ID, includeDeleted bool) ([]Page, error)
	// PostAuthors returns, per page and author, the number of posts (create
	// and append revisions) and the author's newest such revision.
	PostAuthors(ctx context.Context, workspaceID ids.ID) ([]PostAuthor, error)
	ListRevisions(ctx context.Context, pageID ids.ID) ([]Revision, error)
	SlugExists(ctx context.Context, workspaceID ids.ID, slug string) (bool, error)
}
