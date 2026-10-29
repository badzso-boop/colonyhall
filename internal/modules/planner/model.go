// Package planner is a human's personal task list and subscribed ICS
// calendars. Only the owner writes them; agents the owner allowed may read
// them through MCP (never write).
package planner

import (
	"errors"
	"time"

	"colonyhall/internal/platform/ids"
)

// Task statuses, set only by the owner.
const (
	StatusScheduled = "scheduled"
	StatusStarted   = "started"
	StatusDone      = "done"
)

// Limits.
const (
	MaxTitleLen       = 300
	MaxDescriptionLen = 20000
	MaxCalendarName   = 100
	DoneListDays      = 14
)

// Errors.
var (
	ErrHumansOnly    = errors.New("only a human manages a task list or calendars")
	ErrNotAllowed    = errors.New("not allowed")
	ErrNotFound      = errors.New("not found")
	ErrTitle         = errors.New("title must be 1 to 300 characters on one line")
	ErrDescription   = errors.New("description must be at most 20000 characters")
	ErrStatus        = errors.New("status must be scheduled, started or done")
	ErrCalendarName  = errors.New("calendar name must be 1 to 100 characters")
	ErrCalendarColor = errors.New("colour must look like #1a2b3c")
	ErrCalendarURL   = errors.New("calendar URL must be an https or webcal link to a public host")
	ErrOwnerRequired = errors.New("more than one person shared their planner with you: pass owner")
	ErrBadRange      = errors.New("to must be after from, at most 14 days later")
)

// Task is one item of a person's task list.
type Task struct {
	ID              ids.ID
	WorkspaceID     ids.ID
	OwnerID         ids.ID
	Title           string
	Description     string
	Status          string
	Position        float64 // the owner's order, ascending
	StartedAt       *time.Time
	DoneAt          *time.Time
	StatusChangedAt time.Time
	DeletedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Access says which agents may read an owner's tasks and calendars.
type Access struct {
	OwnerID      ids.ID
	WorkspaceID  ids.ID
	TaskAll      bool // every agent of the workspace may read the tasks
	TaskAgentIDs []ids.ID
	CalAgentIDs  []ids.ID
	UpdatedAt    time.Time
}

// DefaultAccess: all agents may read the tasks, none the calendars.
func DefaultAccess(workspaceID, ownerID ids.ID) Access {
	return Access{OwnerID: ownerID, WorkspaceID: workspaceID, TaskAll: true}
}

// Calendar is a subscribed ICS calendar. URL is a secret and never leaves
// the server; MaskedURL is what the owner sees.
type Calendar struct {
	ID            ids.ID
	WorkspaceID   ids.ID
	OwnerID       ids.ID
	Name          string
	URL           string
	Colour        string
	LastFetchedAt *time.Time
	LastError     string
	CreatedAt     time.Time
}

// Event is one occurrence of a calendar entry (recurrences expanded).
type Event struct {
	CalendarID  ids.ID
	OwnerID     ids.ID
	Title       string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Location    string
	Description string
}
