package planner

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists tasks, access settings and calendars.
// Lookups return db.ErrNotFound when nothing matches.
type Store interface {
	InsertTask(ctx context.Context, t Task) error
	// UpdateTask writes every mutable field of t (title, description,
	// status and its times, position, deleted_at, updated_at).
	UpdateTask(ctx context.Context, t Task) error
	TaskByID(ctx context.Context, id ids.ID) (Task, error)
	// Tasks returns the owner's not deleted tasks that are open, or done at
	// or after doneSince, ordered by position.
	Tasks(ctx context.Context, ownerID ids.ID, doneSince time.Time) ([]Task, error)
	// MinPosition returns the smallest position of the owner's tasks (0, false if none).
	MinPosition(ctx context.Context, ownerID ids.ID) (float64, bool, error)

	Access(ctx context.Context, ownerID ids.ID) (Access, error)
	PutAccess(ctx context.Context, a Access) error
	// AccessGrantedTo returns the access rows of the workspace that let
	// agentID read tasks (task_all or listed) or calendars (listed).
	AccessList(ctx context.Context, workspaceID ids.ID) ([]Access, error)

	InsertCalendar(ctx context.Context, c Calendar) error
	CalendarByID(ctx context.Context, id ids.ID) (Calendar, error)
	Calendars(ctx context.Context, ownerID ids.ID) ([]Calendar, error)
	// CalendarsDue returns calendars never fetched or last fetched before t.
	CalendarsDue(ctx context.Context, before time.Time) ([]Calendar, error)
	DeleteCalendar(ctx context.Context, id ids.ID) error
	SetCalendarFetch(ctx context.Context, id ids.ID, at time.Time, fetchErr string) error
	// ReplaceEvents swaps the cached occurrences of one calendar.
	ReplaceEvents(ctx context.Context, calendarID ids.ID, events []Event) error
	// Events returns the owner's occurrences overlapping [from, to), by start.
	Events(ctx context.Context, ownerID ids.ID, from, to time.Time) ([]Event, error)
}
