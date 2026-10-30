// Package postgres implements wiki.Store on Postgres. SQL lives in queries.sql.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/modules/wiki/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements wiki.Store on Postgres.
type Store struct {
	DB *db.Postgres
}

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(s.DB.Q(ctx))
}

func toPage(p sqlcgen.WikiPage) wiki.Page {
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	return wiki.Page{
		ID:          p.ID,
		WorkspaceID: p.WorkspaceID,
		Slug:        p.Slug,
		Title:       p.Title,
		Tags:        tags,
		CreatedBy:   p.CreatedBy,
		CreatedAt:   p.CreatedAt,
		DeletedAt:   p.DeletedAt,
	}
}

// CreatePage stores a new page and its initial revision.
func (s *Store) CreatePage(ctx context.Context, page wiki.Page, initialRev wiki.Revision) error {
	queries := s.q(ctx)
	err := queries.InsertPage(ctx, sqlcgen.InsertPageParams{
		ID:          page.ID,
		WorkspaceID: page.WorkspaceID,
		Slug:        page.Slug,
		Title:       page.Title,
		Tags:        page.Tags,
		CreatedBy:   page.CreatedBy,
		CreatedAt:   page.CreatedAt,
		DeletedAt:   page.DeletedAt,
	})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return db.ErrConflict
		}
		return err
	}
	return queries.InsertRevision(ctx, sqlcgen.InsertRevisionParams{
		PageID:    initialRev.PageID,
		Rev:       int32(initialRev.Rev),
		Body:      initialRev.Body,
		Kind:      initialRev.Kind,
		AuthorID:  initialRev.AuthorID,
		CreatedAt: initialRev.CreatedAt,
	})
}

// AddRevision appends a revision to a page.
func (s *Store) AddRevision(ctx context.Context, rev wiki.Revision) error {
	return s.q(ctx).InsertRevision(ctx, sqlcgen.InsertRevisionParams{
		PageID:    rev.PageID,
		Rev:       int32(rev.Rev),
		Body:      rev.Body,
		Kind:      rev.Kind,
		AuthorID:  rev.AuthorID,
		CreatedAt: rev.CreatedAt,
	})
}

// UpdatePageMeta updates the title, slug, and tags of a page.
func (s *Store) UpdatePageMeta(ctx context.Context, id ids.ID, title string, slug string, tags []string) error {
	err := s.q(ctx).UpdatePageMeta(ctx, sqlcgen.UpdatePageMetaParams{
		ID:    id,
		Title: title,
		Slug:  slug,
		Tags:  tags,
	})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return db.ErrConflict
		}
		return err
	}
	return nil
}

// SoftDelete marks a page as deleted.
func (s *Store) SoftDelete(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).SoftDeletePage(ctx, sqlcgen.SoftDeletePageParams{
		ID:        id,
		DeletedAt: &at,
	})
}

// Restore removes the deleted_at timestamp from a page.
func (s *Store) Restore(ctx context.Context, id ids.ID) error {
	return s.q(ctx).RestorePage(ctx, id)
}

// PageBySlug loads a page by workspace ID and slug.
func (s *Store) PageBySlug(ctx context.Context, workspaceID ids.ID, slug string) (wiki.Page, error) {
	queries := s.q(ctx)
	p, err := queries.PageBySlug(ctx, sqlcgen.PageBySlugParams{
		WorkspaceID: workspaceID,
		Slug:        slug,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return wiki.Page{}, db.ErrNotFound
	}
	if err != nil {
		return wiki.Page{}, err
	}
	rev, err := queries.LatestRevision(ctx, p.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return wiki.Page{}, err
	}
	res := toPage(p)
	if err == nil {
		res.Rev = int(rev.Rev)
		res.Body = rev.Body
		res.UpdatedAt = rev.CreatedAt
	}
	return res, nil
}

// PageByID loads a page by ID.
func (s *Store) PageByID(ctx context.Context, id ids.ID) (wiki.Page, error) {
	queries := s.q(ctx)
	p, err := queries.PageByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return wiki.Page{}, db.ErrNotFound
	}
	if err != nil {
		return wiki.Page{}, err
	}
	rev, err := queries.LatestRevision(ctx, p.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return wiki.Page{}, err
	}
	res := toPage(p)
	if err == nil {
		res.Rev = int(rev.Rev)
		res.Body = rev.Body
		res.UpdatedAt = rev.CreatedAt
	}
	return res, nil
}

// PostAuthors summarises the posts per page and author.
func (s *Store) PostAuthors(ctx context.Context, workspaceID ids.ID) ([]wiki.PostAuthor, error) {
	rows, err := s.q(ctx).PostAuthors(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]wiki.PostAuthor, len(rows))
	for i, r := range rows {
		out[i] = wiki.PostAuthor{PageID: r.PageID, AuthorID: r.AuthorID, Posts: int(r.Posts), LastRev: int(r.LastRev)}
	}
	return out, nil
}

// ListPages lists all pages in a workspace, ordered by title ascending.
func (s *Store) ListPages(ctx context.Context, workspaceID ids.ID, includeDeleted bool) ([]wiki.Page, error) {
	rows, err := s.q(ctx).ListPages(ctx, sqlcgen.ListPagesParams{
		WorkspaceID:    workspaceID,
		IncludeDeleted: includeDeleted,
	})
	if err != nil {
		return nil, err
	}
	out := make([]wiki.Page, 0, len(rows))
	for _, r := range rows {
		tags := r.Tags
		if tags == nil {
			tags = []string{}
		}
		out = append(out, wiki.Page{
			ID:          r.ID,
			WorkspaceID: r.WorkspaceID,
			Slug:        r.Slug,
			Title:       r.Title,
			Tags:        tags,
			Body:        r.Body,
			Rev:         int(r.Rev),
			CreatedBy:   r.CreatedBy,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			DeletedAt:   r.DeletedAt,
		})
	}
	return out, nil
}

// ListRevisions lists all revisions for a page, ordered by rev ascending.
func (s *Store) ListRevisions(ctx context.Context, pageID ids.ID) ([]wiki.Revision, error) {
	_, err := s.q(ctx).PageByID(ctx, pageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.q(ctx).ListRevisions(ctx, pageID)
	if err != nil {
		return nil, err
	}
	out := make([]wiki.Revision, 0, len(rows))
	for _, r := range rows {
		out = append(out, wiki.Revision{
			PageID:    r.PageID,
			Rev:       int(r.Rev),
			Body:      r.Body,
			Kind:      r.Kind,
			AuthorID:  r.AuthorID,
			CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// SlugExists reports whether a page with the given slug exists in the workspace.
func (s *Store) SlugExists(ctx context.Context, workspaceID ids.ID, slug string) (bool, error) {
	return s.q(ctx).CheckSlugExists(ctx, sqlcgen.CheckSlugExistsParams{
		WorkspaceID: workspaceID,
		Slug:        slug,
	})
}
