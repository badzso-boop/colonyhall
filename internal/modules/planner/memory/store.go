// Package memory is an in-memory planner.Store.
package memory

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/planner"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements planner.Store in memory.
type Store struct {
	mu        sync.Mutex
	tasks     map[ids.ID]planner.Task
	access    map[ids.ID]planner.Access
	calendars map[ids.ID]planner.Calendar
	events    map[ids.ID][]planner.Event
}

// New creates an empty Store.
func New() *Store {
	return &Store{tasks: map[ids.ID]planner.Task{}, access: map[ids.ID]planner.Access{},
		calendars: map[ids.ID]planner.Calendar{}, events: map[ids.ID][]planner.Event{}}
}

// InsertTask implements planner.Store.
func (s *Store) InsertTask(_ context.Context, t planner.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; ok {
		return db.ErrConflict
	}
	s.tasks[t.ID] = t
	return nil
}

// UpdateTask implements planner.Store.
func (s *Store) UpdateTask(_ context.Context, t planner.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return db.ErrNotFound
	}
	s.tasks[t.ID] = t
	return nil
}

// TaskByID implements planner.Store.
func (s *Store) TaskByID(_ context.Context, id ids.ID) (planner.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return planner.Task{}, db.ErrNotFound
	}
	return t, nil
}

// Tasks implements planner.Store.
func (s *Store) Tasks(_ context.Context, ownerID ids.ID, doneSince time.Time) ([]planner.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []planner.Task
	for _, t := range s.tasks {
		if t.OwnerID != ownerID || t.DeletedAt != nil {
			continue
		}
		if t.Status == planner.StatusDone && (t.DoneAt == nil || t.DoneAt.Before(doneSince)) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// MinPosition implements planner.Store.
func (s *Store) MinPosition(_ context.Context, ownerID ids.ID) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var minPos float64
	found := false
	for _, t := range s.tasks {
		if t.OwnerID == ownerID && t.DeletedAt == nil && (!found || t.Position < minPos) {
			minPos, found = t.Position, true
		}
	}
	return minPos, found, nil
}

// Access implements planner.Store.
func (s *Store) Access(_ context.Context, ownerID ids.ID) (planner.Access, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.access[ownerID]
	if !ok {
		return planner.Access{}, db.ErrNotFound
	}
	a.TaskAgentIDs = slices.Clone(a.TaskAgentIDs)
	a.CalAgentIDs = slices.Clone(a.CalAgentIDs)
	return a, nil
}

// PutAccess implements planner.Store.
func (s *Store) PutAccess(_ context.Context, a planner.Access) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.TaskAgentIDs = slices.Clone(a.TaskAgentIDs)
	a.CalAgentIDs = slices.Clone(a.CalAgentIDs)
	s.access[a.OwnerID] = a
	return nil
}

// AccessList implements planner.Store.
func (s *Store) AccessList(_ context.Context, workspaceID ids.ID) ([]planner.Access, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []planner.Access
	for _, a := range s.access {
		if a.WorkspaceID == workspaceID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OwnerID.String() < out[j].OwnerID.String() })
	return out, nil
}

// InsertCalendar implements planner.Store.
func (s *Store) InsertCalendar(_ context.Context, c planner.Calendar) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calendars[c.ID] = c
	return nil
}

// CalendarByID implements planner.Store.
func (s *Store) CalendarByID(_ context.Context, id ids.ID) (planner.Calendar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.calendars[id]
	if !ok {
		return planner.Calendar{}, db.ErrNotFound
	}
	return c, nil
}

// Calendars implements planner.Store.
func (s *Store) Calendars(_ context.Context, ownerID ids.ID) ([]planner.Calendar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []planner.Calendar
	for _, c := range s.calendars {
		if c.OwnerID == ownerID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// CalendarsDue implements planner.Store.
func (s *Store) CalendarsDue(_ context.Context, before time.Time) ([]planner.Calendar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []planner.Calendar
	for _, c := range s.calendars {
		if c.LastFetchedAt == nil || c.LastFetchedAt.Before(before) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// DeleteCalendar implements planner.Store.
func (s *Store) DeleteCalendar(_ context.Context, id ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.calendars, id)
	delete(s.events, id)
	return nil
}

// SetCalendarFetch implements planner.Store.
func (s *Store) SetCalendarFetch(_ context.Context, id ids.ID, at time.Time, fetchErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.calendars[id]
	if !ok {
		return db.ErrNotFound
	}
	c.LastFetchedAt, c.LastError = &at, fetchErr
	s.calendars[id] = c
	return nil
}

// ReplaceEvents implements planner.Store.
func (s *Store) ReplaceEvents(_ context.Context, calendarID ids.ID, events []planner.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[calendarID] = slices.Clone(events)
	return nil
}

// Events implements planner.Store.
func (s *Store) Events(_ context.Context, ownerID ids.ID, from, to time.Time) ([]planner.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []planner.Event
	for calID, evs := range s.events {
		if c, ok := s.calendars[calID]; !ok || c.OwnerID != ownerID {
			continue
		}
		for _, e := range evs {
			if e.End.After(from) && e.Start.Before(to) || (e.Start.Equal(e.End) && !e.Start.Before(from) && e.Start.Before(to)) {
				out = append(out, e)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}
