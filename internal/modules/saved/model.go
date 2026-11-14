// Package saved keeps a person's saved messages and reminders (round 55,
// part E). Personal: nobody else sees them. A due reminder becomes a
// system message in the person's own DM, sent once by the background loop.
package saved

import (
	"errors"
	"time"

	"colonyhall/internal/platform/ids"
)

// Item is a saved message, a reminder, or both. MessageID is ids.Nil for a
// reminder without a message (/remind <when> <text>).
type Item struct {
	ID          ids.ID
	WorkspaceID ids.ID
	ActorID     ids.ID
	MessageID   ids.ID
	ChannelID   ids.ID
	Note        string
	RemindAt    *time.Time
	RemindedAt  *time.Time
	DoneAt      *time.Time
	CreatedAt   time.Time
}

// Cursor pages the list: items older than (At, ID).
type Cursor struct {
	At time.Time
	ID ids.ID
}

// Filter narrows the list.
type Filter string

// Filters.
const (
	FilterAll       Filter = "all"
	FilterReminders Filter = "reminders" // with a reminder, not done
	FilterDone      Filter = "done"
)

// Limits.
const (
	MaxNote       = 500
	MaxItems      = 2000
	MaxAhead      = 366 * 24 * time.Hour
	ExcerptLength = 300
	PageSize      = 50
)

// Errors.
var (
	ErrForbidden   = errors.New("saved items are for people only")
	ErrNoteTooLong = errors.New("a reminder text is at most 500 characters")
	ErrBadRemindAt = errors.New("remind_at must be in the future, within a year")
	ErrEmpty       = errors.New("give a message, a reminder time, or both")
	ErrTooMany     = errors.New("at most 2000 saved items; mark some done or remove them")
	ErrBadFilter   = errors.New("filter must be all, reminders or done")
)
