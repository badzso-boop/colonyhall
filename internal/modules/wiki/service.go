// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package wiki

import (
	"context"
	"fmt"
	"strings"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Service implements the wiki use cases and authorization rules.
type Service struct {
	Store  Store
	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock
}

// CreateInput describes a new wiki page.
type CreateInput struct {
	Title string
	Body  string
	Tags  []string
}

// EditInput describes a human page edit.
type EditInput struct {
	BaseRev int
	Body    string
	Title   string
	Tags    *[]string
}

// Create creates a new wiki page with initial rev 1.
func (s *Service) Create(ctx context.Context, by identity.Actor, in CreateInput) (Page, error) {
	if by.Halted {
		return Page{}, identity.ErrHalted
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Page{}, ErrTitleEmpty
	}
	if len([]rune(title)) > MaxTitleLen {
		return Page{}, ErrTitleTooLong
	}

	if strings.TrimSpace(in.Body) == "" {
		return Page{}, ErrBodyEmpty
	}
	if len([]rune(in.Body)) > MaxBodyLen {
		return Page{}, ErrBodyTooLong
	}

	baseSlug := Slugify(title)
	slug, err := s.uniqueSlug(ctx, by.WorkspaceID, baseSlug, ids.Nil)
	if err != nil {
		return Page{}, err
	}

	now := s.Clock.Now()
	pageID := s.IDs.New()

	tags := in.Tags
	if tags == nil {
		tags = []string{}
	}

	page := Page{
		ID:          pageID,
		WorkspaceID: by.WorkspaceID,
		Slug:        slug,
		Title:       title,
		Tags:        tags,
		Body:        in.Body,
		Rev:         1,
		CreatedBy:   by.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	rev := Revision{
		PageID:    pageID,
		Rev:       1,
		Body:      in.Body,
		Kind:      RevisionCreate,
		AuthorID:  by.ID,
		CreatedAt: now,
	}

	evPayload := Event{
		PageID: pageID,
		Slug:   slug,
		Title:  title,
		Rev:    1,
		Kind:   RevisionCreate,
	}

	err = s.record(ctx, by.WorkspaceID, by.ID, EventCreated, pageID, evPayload, func(ctx context.Context) error {
		return s.Store.CreatePage(ctx, page, rev)
	})
	if err != nil {
		return Page{}, err
	}

	return page, nil
}

// Append appends text to an existing wiki page.
func (s *Service) Append(ctx context.Context, by identity.Actor, slug string, text string) (Page, error) {
	if by.Halted {
		return Page{}, identity.ErrHalted
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Page{}, ErrAppendEmpty
	}
	if len([]rune(text)) > MaxAppendLen {
		return Page{}, ErrAppendTooLong
	}

	page, err := s.Store.PageBySlug(ctx, by.WorkspaceID, slug)
	if err != nil {
		return Page{}, err
	}
	if page.DeletedAt != nil {
		return Page{}, ErrNotFound
	}

	now := s.Clock.Now()
	header := fmt.Sprintf("\n\n---\n*@%s, %s:*\n\n", by.Handle, now.UTC().Format("2006-01-02 15:04"))
	newBody := page.Body + header + trimmed
	if len([]rune(newBody)) > MaxBodyLen {
		return Page{}, ErrBodyTooLong
	}

	newRev := page.Rev + 1
	rev := Revision{
		PageID:    page.ID,
		Rev:       newRev,
		Body:      newBody,
		Kind:      RevisionAppend,
		AuthorID:  by.ID,
		CreatedAt: now,
	}

	evPayload := Event{
		PageID: page.ID,
		Slug:   page.Slug,
		Title:  page.Title,
		Rev:    newRev,
		Kind:   RevisionAppend,
	}

	err = s.record(ctx, by.WorkspaceID, by.ID, EventAppended, page.ID, evPayload, func(ctx context.Context) error {
		return s.Store.AddRevision(ctx, rev)
	})
	if err != nil {
		return Page{}, err
	}

	page.Body = newBody
	page.Rev = newRev
	page.UpdatedAt = now
	return page, nil
}

// Edit replaces the body and optionally updates title/tags. Humans only.
func (s *Service) Edit(ctx context.Context, by identity.Actor, slug string, in EditInput) (Page, error) {
	if by.Kind != identity.KindHuman || by.Halted {
		return Page{}, ErrForbidden
	}

	if strings.TrimSpace(in.Body) == "" {
		return Page{}, ErrBodyEmpty
	}
	if len([]rune(in.Body)) > MaxBodyLen {
		return Page{}, ErrBodyTooLong
	}

	page, err := s.Store.PageBySlug(ctx, by.WorkspaceID, slug)
	if err != nil {
		return Page{}, err
	}
	if page.DeletedAt != nil {
		return Page{}, ErrNotFound
	}

	if in.BaseRev != page.Rev {
		return Page{}, ErrConflict
	}

	newTitle := page.Title
	newSlug := page.Slug
	if trimmedTitle := strings.TrimSpace(in.Title); trimmedTitle != "" && trimmedTitle != page.Title {
		if len([]rune(trimmedTitle)) > MaxTitleLen {
			return Page{}, ErrTitleTooLong
		}
		newTitle = trimmedTitle
		baseSlug := Slugify(newTitle)
		resolvedSlug, err := s.uniqueSlug(ctx, by.WorkspaceID, baseSlug, page.ID)
		if err != nil {
			return Page{}, err
		}
		newSlug = resolvedSlug
	}

	newTags := page.Tags
	if in.Tags != nil {
		newTags = *in.Tags
	}

	now := s.Clock.Now()
	newRev := page.Rev + 1
	rev := Revision{
		PageID:    page.ID,
		Rev:       newRev,
		Body:      in.Body,
		Kind:      RevisionEdit,
		AuthorID:  by.ID,
		CreatedAt: now,
	}

	evPayload := Event{
		PageID: page.ID,
		Slug:   newSlug,
		Title:  newTitle,
		Rev:    newRev,
		Kind:   RevisionEdit,
	}

	err = s.record(ctx, by.WorkspaceID, by.ID, EventEdited, page.ID, evPayload, func(ctx context.Context) error {
		if err := s.Store.UpdatePageMeta(ctx, page.ID, newTitle, newSlug, newTags); err != nil {
			return err
		}
		return s.Store.AddRevision(ctx, rev)
	})
	if err != nil {
		return Page{}, err
	}

	page.Title = newTitle
	page.Slug = newSlug
	page.Tags = newTags
	page.Body = in.Body
	page.Rev = newRev
	page.UpdatedAt = now
	return page, nil
}

// Delete soft-deletes a wiki page. Humans only.
func (s *Service) Delete(ctx context.Context, by identity.Actor, slug string) error {
	if by.Kind != identity.KindHuman || by.Halted {
		return ErrForbidden
	}

	page, err := s.Store.PageBySlug(ctx, by.WorkspaceID, slug)
	if err != nil {
		return err
	}
	if page.DeletedAt != nil {
		return nil
	}

	now := s.Clock.Now()
	newRev := page.Rev + 1
	rev := Revision{
		PageID:    page.ID,
		Rev:       newRev,
		Body:      page.Body,
		Kind:      RevisionDelete,
		AuthorID:  by.ID,
		CreatedAt: now,
	}

	evPayload := Event{
		PageID: page.ID,
		Slug:   page.Slug,
		Title:  page.Title,
		Rev:    newRev,
		Kind:   RevisionDelete,
	}

	return s.record(ctx, by.WorkspaceID, by.ID, EventDeleted, page.ID, evPayload, func(ctx context.Context) error {
		if err := s.Store.SoftDelete(ctx, page.ID, now); err != nil {
			return err
		}
		return s.Store.AddRevision(ctx, rev)
	})
}

// Restore restores a soft-deleted wiki page. Humans only.
func (s *Service) Restore(ctx context.Context, by identity.Actor, slug string) (Page, error) {
	if by.Kind != identity.KindHuman || by.Halted {
		return Page{}, ErrForbidden
	}

	page, err := s.Store.PageBySlug(ctx, by.WorkspaceID, slug)
	if err != nil {
		return Page{}, err
	}
	if page.DeletedAt == nil {
		return page, nil
	}

	now := s.Clock.Now()
	newRev := page.Rev + 1
	rev := Revision{
		PageID:    page.ID,
		Rev:       newRev,
		Body:      page.Body,
		Kind:      RevisionRestore,
		AuthorID:  by.ID,
		CreatedAt: now,
	}

	evPayload := Event{
		PageID: page.ID,
		Slug:   page.Slug,
		Title:  page.Title,
		Rev:    newRev,
		Kind:   RevisionRestore,
	}

	err = s.record(ctx, by.WorkspaceID, by.ID, EventRestored, page.ID, evPayload, func(ctx context.Context) error {
		if err := s.Store.Restore(ctx, page.ID); err != nil {
			return err
		}
		return s.Store.AddRevision(ctx, rev)
	})
	if err != nil {
		return Page{}, err
	}

	page.DeletedAt = nil
	page.Rev = newRev
	page.UpdatedAt = now
	return page, nil
}

// Get returns a page by slug. Agents cannot access deleted pages.
func (s *Service) Get(ctx context.Context, by identity.Actor, slug string) (Page, error) {
	page, err := s.Store.PageBySlug(ctx, by.WorkspaceID, slug)
	if err != nil {
		return Page{}, err
	}
	if page.DeletedAt != nil && by.Kind != identity.KindHuman {
		return Page{}, ErrNotFound
	}
	return page, nil
}

// List returns the index of wiki pages.
func (s *Service) List(ctx context.Context, by identity.Actor, query string, includeDeleted bool) ([]PageIndex, error) {
	if by.Kind != identity.KindHuman {
		includeDeleted = false
	}

	pages, err := s.Store.ListPages(ctx, by.WorkspaceID, includeDeleted)
	if err != nil {
		return nil, err
	}

	authors, err := s.Store.PostAuthors(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	byPage := map[ids.ID][]PostAuthor{}
	for _, a := range authors {
		byPage[a.PageID] = append(byPage[a.PageID], a)
	}

	qLower := strings.ToLower(strings.TrimSpace(query))
	var out []PageIndex
	for _, p := range pages {
		if qLower != "" && !matchesQuery(p, qLower) {
			continue
		}
		count, last, participants := PostSummary(byPage[p.ID])
		out = append(out, PageIndex{
			Slug:           p.Slug,
			Title:          p.Title,
			Tags:           p.Tags,
			Excerpt:        Excerpt(p.Body),
			CreatedAt:      p.CreatedAt,
			UpdatedAt:      p.UpdatedAt,
			DeletedAt:      p.DeletedAt,
			CreatedBy:      p.CreatedBy,
			PostCount:      count,
			LastAuthorID:   last,
			ParticipantIDs: participants,
		})
	}

	return out, nil
}

func matchesQuery(p Page, q string) bool {
	if strings.Contains(strings.ToLower(p.Title), q) ||
		strings.Contains(strings.ToLower(p.Slug), q) ||
		strings.Contains(strings.ToLower(p.Body), q) {
		return true
	}
	for _, tag := range p.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

// Revisions lists all revisions of a wiki page.
func (s *Service) Revisions(ctx context.Context, by identity.Actor, slug string) ([]Revision, error) {
	page, err := s.Store.PageBySlug(ctx, by.WorkspaceID, slug)
	if err != nil {
		return nil, err
	}
	if page.DeletedAt != nil && by.Kind != identity.KindHuman {
		return nil, ErrNotFound
	}
	return s.Store.ListRevisions(ctx, page.ID)
}

func (s *Service) uniqueSlug(ctx context.Context, workspaceID ids.ID, baseSlug string, excludeID ids.ID) (string, error) {
	exists, err := s.Store.SlugExists(ctx, workspaceID, baseSlug)
	if err != nil {
		return "", err
	}
	if !exists {
		return baseSlug, nil
	}
	if excludeID != ids.Nil {
		p, err := s.Store.PageBySlug(ctx, workspaceID, baseSlug)
		if err == nil && p.ID == excludeID {
			return baseSlug, nil
		}
	}

	for i := 2; ; i++ {
		suffix := fmt.Sprintf("-%d", i)
		maxBase := MaxSlugLen - len(suffix)
		runes := []rune(baseSlug)
		if len(runes) > maxBase {
			runes = runes[:maxBase]
		}
		candidate := strings.TrimRight(string(runes), "-")
		if candidate == "" {
			candidate = "page"
		}
		candidate += suffix

		exists, err := s.Store.SlugExists(ctx, workspaceID, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		if excludeID != ids.Nil {
			p, err := s.Store.PageBySlug(ctx, workspaceID, candidate)
			if err == nil && p.ID == excludeID {
				return candidate, nil
			}
		}
	}
}

func (s *Service) record(ctx context.Context, workspaceID, actorID ids.ID, typ string, pageID ids.ID, payload Event, change func(ctx context.Context) error) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := change(ctx); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, actorID, ids.Nil, "wiki", pageID, payload, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err == nil {
		s.Bus.Notify(seq)
	}
	return err
}

// PageForIndex returns a page's current state, deleted or not, for the
// search indexer (no actor: the caller is the server itself).
func (s *Service) PageForIndex(ctx context.Context, workspaceID ids.ID, slug string) (Page, error) {
	return s.Store.PageBySlug(ctx, workspaceID, slug)
}
