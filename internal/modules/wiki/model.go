// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package wiki

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"

	"colonyhall/internal/platform/ids"
)

// Limits as specified.
const (
	MaxTitleLen  = 120
	MaxBodyLen   = 50000
	MaxAppendLen = 8000
	MaxSlugLen   = 80
)

// Revision kinds.
const (
	RevisionCreate  = "create"
	RevisionAppend  = "append"
	RevisionEdit    = "edit"
	RevisionDelete  = "delete"
	RevisionRestore = "restore"
)

// Errors.
var (
	ErrNotFound      = errors.New("wiki page not found")
	ErrForbidden     = errors.New("forbidden")
	ErrConflict      = errors.New("conflict: page was modified by someone else")
	ErrTitleEmpty    = errors.New("title is required")
	ErrTitleTooLong  = errors.New("title exceeds maximum length of 120 characters")
	ErrBodyEmpty     = errors.New("body is required")
	ErrBodyTooLong   = errors.New("body exceeds maximum length of 50000 characters")
	ErrAppendEmpty   = errors.New("append text is required")
	ErrAppendTooLong = errors.New("append text exceeds maximum length of 8000 characters")
	ErrDeleted       = errors.New("wiki page is deleted")
	ErrNotDeleted    = errors.New("wiki page is not deleted")
)

// Page represents a wiki page with its latest revision body.
type Page struct {
	ID          ids.ID
	WorkspaceID ids.ID
	Slug        string
	Title       string
	Tags        []string
	Body        string
	Rev         int
	CreatedBy   ids.ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// Revision is one immutable version of a wiki page.
type Revision struct {
	PageID    ids.ID
	Rev       int
	Body      string
	Kind      string
	AuthorID  ids.ID
	CreatedAt time.Time
}

// PageIndex is an entry in the wiki index.
type PageIndex struct {
	Slug      string
	Title     string
	Tags      []string
	Excerpt   string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
	// The forum view (round 32): who opened it, how many posts (the
	// opening one and the appends), the newest post's author, and up to
	// MaxParticipants authors, the most recent first.
	CreatedBy      ids.ID
	PostCount      int
	LastAuthorID   ids.ID
	ParticipantIDs []ids.ID
}

// MaxParticipants is how many post authors the index lists per page.
const MaxParticipants = 5

// PostAuthor is one author's posts (create and append revisions) on a page.
type PostAuthor struct {
	PageID   ids.ID
	AuthorID ids.ID
	Posts    int
	LastRev  int
}

// PostSummary folds the post authors of one page into the index fields.
func PostSummary(authors []PostAuthor) (count int, last ids.ID, participants []ids.ID) {
	sorted := append([]PostAuthor(nil), authors...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].LastRev > sorted[j].LastRev })
	for _, a := range sorted {
		count += a.Posts
		if len(participants) < MaxParticipants {
			participants = append(participants, a.AuthorID)
		}
	}
	if len(sorted) > 0 {
		last = sorted[0].AuthorID
	}
	return count, last, participants
}

// FoldAccents replaces Hungarian and common accented vowels with their ASCII base.
func FoldAccents(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'á', 'à', 'ä', 'â', 'Á', 'À', 'Ä', 'Â':
			b.WriteRune('a')
		case 'é', 'è', 'ë', 'ê', 'É', 'È', 'Ë', 'Ê':
			b.WriteRune('e')
		case 'í', 'ì', 'ï', 'î', 'Í', 'Ì', 'Ï', 'Î':
			b.WriteRune('i')
		case 'ó', 'ò', 'ö', 'ő', 'ô', 'Ó', 'Ò', 'Ö', 'Ő', 'Ô':
			b.WriteRune('o')
		case 'ú', 'ù', 'ü', 'ű', 'û', 'Ú', 'Ù', 'Ü', 'Ű', 'Û':
			b.WriteRune('u')
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// Slugify generates a URL-friendly slug from a page title.
func Slugify(title string) string {
	folded := FoldAccents(title)
	var b strings.Builder
	lastHyphen := true
	for _, r := range folded {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
		} else if !lastHyphen {
			b.WriteRune('-')
			lastHyphen = true
		}
	}
	res := strings.TrimRight(b.String(), "-")
	if res == "" {
		res = "page"
	}
	runes := []rune(res)
	if len(runes) > MaxSlugLen {
		res = string(runes[:MaxSlugLen])
		res = strings.TrimRight(res, "-")
		if res == "" {
			res = "page"
		}
	}
	return res
}

// Excerpt returns the first line of the body up to 120 runes.
func Excerpt(body string) string {
	body = strings.TrimSpace(body)
	if idx := strings.IndexByte(body, '\n'); idx >= 0 {
		body = strings.TrimSpace(body[:idx])
	}
	runes := []rune(body)
	if len(runes) > 120 {
		return string(runes[:120])
	}
	return string(runes)
}
