// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of wiki.Store.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements wiki.Store in memory.
type Store struct {
	mu        sync.Mutex
	pages     map[ids.ID]wiki.Page
	revisions map[ids.ID][]wiki.Revision
}

// New returns an empty in-memory wiki store.
func New() *Store {
	return &Store{
		pages:     make(map[ids.ID]wiki.Page),
		revisions: make(map[ids.ID][]wiki.Revision),
	}
}

// CreatePage stores a new page and its initial revision.
func (s *Store) CreatePage(_ context.Context, page wiki.Page, initialRev wiki.Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.pages {
		if p.WorkspaceID == page.WorkspaceID && p.Slug == page.Slug {
			return db.ErrConflict
		}
	}

	tagsCopy := make([]string, len(page.Tags))
	copy(tagsCopy, page.Tags)
	page.Tags = tagsCopy

	s.pages[page.ID] = page
	s.revisions[page.ID] = []wiki.Revision{initialRev}
	return nil
}

// AddRevision appends a revision to a page.
func (s *Store) AddRevision(_ context.Context, rev wiki.Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.pages[rev.PageID]; !ok {
		return db.ErrNotFound
	}

	s.revisions[rev.PageID] = append(s.revisions[rev.PageID], rev)
	return nil
}

// UpdatePageMeta updates the title, slug, and tags of a page.
func (s *Store) UpdatePageMeta(_ context.Context, id ids.ID, title string, slug string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.pages[id]
	if !ok {
		return db.ErrNotFound
	}

	for otherID, other := range s.pages {
		if otherID != id && other.WorkspaceID == p.WorkspaceID && other.Slug == slug {
			return db.ErrConflict
		}
	}

	tagsCopy := make([]string, len(tags))
	copy(tagsCopy, tags)
	p.Title = title
	p.Slug = slug
	p.Tags = tagsCopy
	s.pages[id] = p
	return nil
}

// SoftDelete marks a page as deleted.
func (s *Store) SoftDelete(_ context.Context, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.pages[id]
	if !ok {
		return db.ErrNotFound
	}

	del := at
	p.DeletedAt = &del
	s.pages[id] = p
	return nil
}

// Restore removes the deleted_at timestamp from a page.
func (s *Store) Restore(_ context.Context, id ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.pages[id]
	if !ok {
		return db.ErrNotFound
	}

	p.DeletedAt = nil
	s.pages[id] = p
	return nil
}

// PageBySlug loads a page by workspace ID and slug, populated with latest revision details.
func (s *Store) PageBySlug(_ context.Context, workspaceID ids.ID, slug string) (wiki.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.pages {
		if p.WorkspaceID == workspaceID && p.Slug == slug {
			return s.populateLatest(p), nil
		}
	}
	return wiki.Page{}, db.ErrNotFound
}

// PageByID loads a page by ID, populated with latest revision details.
func (s *Store) PageByID(_ context.Context, id ids.ID) (wiki.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.pages[id]
	if !ok {
		return wiki.Page{}, db.ErrNotFound
	}
	return s.populateLatest(p), nil
}

func (s *Store) populateLatest(p wiki.Page) wiki.Page {
	revs := s.revisions[p.ID]
	if len(revs) > 0 {
		latest := revs[len(revs)-1]
		p.Rev = latest.Rev
		p.Body = latest.Body
		p.UpdatedAt = latest.CreatedAt
	}
	tagsCopy := make([]string, len(p.Tags))
	copy(tagsCopy, p.Tags)
	p.Tags = tagsCopy
	return p
}

// PostAuthors summarises the posts per page and author.
func (s *Store) PostAuthors(_ context.Context, workspaceID ids.ID) ([]wiki.PostAuthor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type key struct{ page, author ids.ID }
	agg := map[key]*wiki.PostAuthor{}
	var order []key
	for pageID, revs := range s.revisions {
		if p, ok := s.pages[pageID]; !ok || p.WorkspaceID != workspaceID {
			continue
		}
		for _, r := range revs {
			if r.Kind != wiki.RevisionCreate && r.Kind != wiki.RevisionAppend {
				continue
			}
			k := key{pageID, r.AuthorID}
			a, ok := agg[k]
			if !ok {
				a = &wiki.PostAuthor{PageID: pageID, AuthorID: r.AuthorID}
				agg[k] = a
				order = append(order, k)
			}
			a.Posts++
			if r.Rev > a.LastRev {
				a.LastRev = r.Rev
			}
		}
	}
	out := make([]wiki.PostAuthor, 0, len(order))
	for _, k := range order {
		out = append(out, *agg[k])
	}
	return out, nil
}

// ListPages lists all pages in a workspace, ordered by title ascending.
func (s *Store) ListPages(_ context.Context, workspaceID ids.ID, includeDeleted bool) ([]wiki.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []wiki.Page
	for _, p := range s.pages {
		if p.WorkspaceID != workspaceID {
			continue
		}
		if !includeDeleted && p.DeletedAt != nil {
			continue
		}
		out = append(out, s.populateLatest(p))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Title < out[j].Title
	})
	return out, nil
}

// ListRevisions lists all revisions for a page, ordered by rev ascending.
func (s *Store) ListRevisions(_ context.Context, pageID ids.ID) ([]wiki.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.pages[pageID]; !ok {
		return nil, db.ErrNotFound
	}

	revs := s.revisions[pageID]
	out := make([]wiki.Revision, len(revs))
	copy(out, revs)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Rev < out[j].Rev
	})
	return out, nil
}

// SlugExists reports whether a page with the given slug exists in the workspace.
func (s *Store) SlugExists(_ context.Context, workspaceID ids.ID, slug string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.pages {
		if p.WorkspaceID == workspaceID && p.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}
