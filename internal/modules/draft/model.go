// Package draft keeps unsent messages (round 55, part A): one per person,
// conversation and thread. A draft is not a message: it never reaches
// agents, search or anyone else's event stream, and only its owner reads it.
package draft

import (
	"errors"
	"time"

	"colonyhall/internal/platform/ids"
)

// Draft is one person's unsent text (and uploads) in one place.
// ThreadRootID is ids.Nil for the main stream.
type Draft struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	ActorID      ids.ID
	ChannelID    ids.ID
	ThreadRootID ids.ID
	Body         string
	FileIDs      []ids.ID
	UpdatedAt    time.Time
	// Attachments is filled on reads: the FileIDs still usable here.
	Attachments []Attachment
}

// Attachment is an upload a draft holds, for the composer's tray.
type Attachment struct {
	ID   ids.ID
	Name string
	Mime string
	Size int64
}

// Limits.
const (
	MaxBodyLen = 20000
	MaxFiles   = 20
	MaxDrafts  = 500 // listed per person
)

// Errors.
var (
	ErrForbidden    = errors.New("drafts are for people only")
	ErrBodyTooLong  = errors.New("draft is too long")
	ErrTooManyFiles = errors.New("a draft holds at most 20 files")
	ErrBadThread    = errors.New("thread root must be a top-level message in the same channel")
	ErrBadFile      = errors.New("a draft can only hold your own uploads in this conversation")
)
