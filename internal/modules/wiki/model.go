package wiki

import (
	"errors"
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
