package planner

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Calendar window cached per fetch, and the default refresh period.
const (
	CacheBefore    = 24 * time.Hour
	CacheAfter     = 60 * 24 * time.Hour
	DefaultRefresh = 15 * time.Minute
	MaxReadRange   = 14 * 24 * time.Hour
)

// Service implements the planner use cases. Only a human writes their own
// tasks and calendars; agents read through ReadTasks / ReadEvents.
type Service struct {
	Store     Store
	Directory identity.Directory
	Events    eventlog.Store
	Bus       bus.Bus
	Tx        db.TxRunner
	IDs       ids.Generator
	Clock     clock.Clock
	Fetcher   Fetcher        // the server sets fetch.Safe
	Location  *time.Location // default Europe/Budapest
}

func (s *Service) loc() *time.Location {
	if s.Location != nil {
		return s.Location
	}
	if l, err := time.LoadLocation("Europe/Budapest"); err == nil {
		return l
	}
	return time.UTC
}

type noFetcher struct{}

func (noFetcher) Fetch(context.Context, string) ([]byte, error) {
	return nil, errors.New("no fetcher configured")
}

func (s *Service) fetcher() Fetcher {
	if s.Fetcher != nil {
		return s.Fetcher
	}
	return noFetcher{}
}

func human(by identity.Actor) error {
	if by.Kind != identity.KindHuman {
		return ErrHumansOnly
	}
	return nil
}

// emit writes a planner event in the current transaction.
func (s *Service) emit(ctx context.Context, by identity.Actor, typ string, aggID ids.ID, payload any, at time.Time) (int64, error) {
	ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, typ, by.ID, ids.Nil, "planner", aggID, payload, at)
	if err != nil {
		return 0, err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return 0, err
	}
	return ev.Seq, nil
}

func checkTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" || strings.ContainsAny(t, "\r\n") || utf8.RuneCountInString(t) > MaxTitleLen {
		return "", ErrTitle
	}
	return t, nil
}

func checkDescription(d string) (string, error) {
	if utf8.RuneCountInString(d) > MaxDescriptionLen {
		return "", ErrDescription
	}
	return strings.TrimRight(d, " \t\r\n"), nil
}

// ensureAccess creates the default access row the first time an owner uses
// the planner, so agents can find whose planner they may read.
func (s *Service) ensureAccess(ctx context.Context, by identity.Actor) error {
	if _, err := s.Store.Access(ctx, by.ID); err == nil {
		return nil
	} else if !errors.Is(err, db.ErrNotFound) {
		return err
	}
	a := DefaultAccess(by.WorkspaceID, by.ID)
	a.UpdatedAt = s.Clock.Now()
	return s.Store.PutAccess(ctx, a)
}

// Tasks lists the caller's open tasks and those done in the last 14 days.
func (s *Service) Tasks(ctx context.Context, by identity.Actor) ([]Task, error) {
	if err := human(by); err != nil {
		return nil, err
	}
	return s.Store.Tasks(ctx, by.ID, s.Clock.Now().Add(-DoneListDays*24*time.Hour))
}

// ownTask loads a task of the caller (not found for anyone else's).
func (s *Service) ownTask(ctx context.Context, by identity.Actor, id ids.ID) (Task, error) {
	if err := human(by); err != nil {
		return Task{}, err
	}
	t, err := s.Store.TaskByID(ctx, id)
	if errors.Is(err, db.ErrNotFound) || (err == nil && (t.OwnerID != by.ID || t.DeletedAt != nil)) {
		return Task{}, ErrNotFound
	}
	return t, err
}

// CreateTask adds a scheduled task at the top of the caller's list.
func (s *Service) CreateTask(ctx context.Context, by identity.Actor, title, description string) (Task, error) {
	if err := human(by); err != nil {
		return Task{}, err
	}
	title, err := checkTitle(title)
	if err != nil {
		return Task{}, err
	}
	if description, err = checkDescription(description); err != nil {
		return Task{}, err
	}
	now := s.Clock.Now()
	t := Task{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, OwnerID: by.ID, Title: title, Description: description,
		Status: StatusScheduled, StatusChangedAt: now, CreatedAt: now, UpdatedAt: now}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.ensureAccess(ctx, by); err != nil {
			return err
		}
		minPos, ok, err := s.Store.MinPosition(ctx, by.ID)
		if err != nil {
			return err
		}
		if ok {
			t.Position = minPos - 1
		}
		if err := s.Store.InsertTask(ctx, t); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventTaskCreated, t.ID, TaskEvent{TaskID: t.ID.String(), OwnerID: by.ID.String(), NewStatus: t.Status}, now)
		return err
	})
	if err != nil {
		return Task{}, err
	}
	s.Bus.Notify(seq)
	return t, nil
}

// TaskPatch is a partial update; nil fields stay as they are.
type TaskPatch struct {
	Title       *string
	Description *string
	Status      *string
	// AfterID / BeforeID move the task between two neighbours (either may be
	// nil at the ends of the list). Move is true when either is given.
	Move     bool
	AfterID  ids.ID
	BeforeID ids.ID
}

// UpdateTask changes the caller's task. A status change records its time;
// started_at keeps the first start, done_at is cleared when reopened.
func (s *Service) UpdateTask(ctx context.Context, by identity.Actor, id ids.ID, p TaskPatch) (Task, error) {
	t, err := s.ownTask(ctx, by, id)
	if err != nil {
		return Task{}, err
	}
	now := s.Clock.Now()
	type ev struct {
		typ     string
		payload TaskEvent
	}
	var evs []ev
	base := TaskEvent{TaskID: t.ID.String(), OwnerID: by.ID.String()}
	if p.Title != nil || p.Description != nil {
		if p.Title != nil {
			if t.Title, err = checkTitle(*p.Title); err != nil {
				return Task{}, err
			}
		}
		if p.Description != nil {
			if t.Description, err = checkDescription(*p.Description); err != nil {
				return Task{}, err
			}
		}
		evs = append(evs, ev{EventTaskUpdated, base})
	}
	if p.Status != nil && *p.Status != t.Status {
		st := *p.Status
		if st != StatusScheduled && st != StatusStarted && st != StatusDone {
			return Task{}, ErrStatus
		}
		e := base
		e.OldStatus, e.NewStatus = t.Status, st
		t.Status, t.StatusChangedAt = st, now
		if st == StatusStarted && t.StartedAt == nil {
			t.StartedAt = &now
		}
		if st == StatusDone {
			t.DoneAt = &now
		} else {
			t.DoneAt = nil
		}
		evs = append(evs, ev{EventTaskStatusChanged, e})
	}
	if p.Move {
		pos, err := s.between(ctx, by, t.ID, p.AfterID, p.BeforeID)
		if err != nil {
			return Task{}, err
		}
		t.Position = pos
		evs = append(evs, ev{EventTaskMoved, base})
	}
	if len(evs) == 0 {
		return t, nil
	}
	t.UpdatedAt = now
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.UpdateTask(ctx, t); err != nil {
			return err
		}
		for _, e := range evs {
			if seq, err = s.emit(ctx, by, e.typ, t.ID, e.payload, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	s.Bus.Notify(seq)
	return t, nil
}

// between returns a position between two of the caller's tasks. Positions
// are floats: a move writes one row; after ~50 moves into the same gap the
// gap is too small and the list is renumbered (rare for a personal list).
func (s *Service) between(ctx context.Context, by identity.Actor, self, afterID, beforeID ids.ID) (float64, error) {
	pos := func(id ids.ID) (float64, bool, error) {
		if id == ids.Nil || id == self {
			return 0, false, nil
		}
		t, err := s.ownTask(ctx, by, id)
		if err != nil {
			return 0, false, err
		}
		return t.Position, true, nil
	}
	a, hasA, err := pos(afterID)
	if err != nil {
		return 0, err
	}
	b, hasB, err := pos(beforeID)
	if err != nil {
		return 0, err
	}
	switch {
	case hasA && hasB:
		if b-a > 1e-9 {
			return (a + b) / 2, nil
		}
		if err := s.renumber(ctx, by); err != nil {
			return 0, err
		}
		return s.between(ctx, by, self, afterID, beforeID)
	case hasA:
		return a + 1, nil
	case hasB:
		return b - 1, nil
	}
	minPos, ok, err := s.Store.MinPosition(ctx, by.ID)
	if err != nil || !ok {
		return 0, err
	}
	return minPos - 1, nil
}

func (s *Service) renumber(ctx context.Context, by identity.Actor) error {
	all, err := s.Store.Tasks(ctx, by.ID, time.Time{})
	if err != nil {
		return err
	}
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		for i, t := range all {
			t.Position = float64(i)
			if err := s.Store.UpdateTask(ctx, t); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteTask soft-deletes the caller's task.
func (s *Service) DeleteTask(ctx context.Context, by identity.Actor, id ids.ID) error {
	t, err := s.ownTask(ctx, by, id)
	if err != nil {
		return err
	}
	now := s.Clock.Now()
	t.DeletedAt, t.UpdatedAt = &now, now
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.UpdateTask(ctx, t); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventTaskDeleted, t.ID, TaskEvent{TaskID: t.ID.String(), OwnerID: by.ID.String()}, now)
		return err
	})
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}

// GetAccess returns who may read the caller's planner.
func (s *Service) GetAccess(ctx context.Context, by identity.Actor) (Access, error) {
	if err := human(by); err != nil {
		return Access{}, err
	}
	a, err := s.Store.Access(ctx, by.ID)
	if errors.Is(err, db.ErrNotFound) {
		return DefaultAccess(by.WorkspaceID, by.ID), nil
	}
	return a, err
}

// SetAccess replaces who may read the caller's planner. Listed ids must be
// agents of the caller's workspace.
func (s *Service) SetAccess(ctx context.Context, by identity.Actor, taskAll bool, taskAgents, calAgents []ids.ID) (Access, error) {
	if err := human(by); err != nil {
		return Access{}, err
	}
	clean := func(list []ids.ID) ([]ids.ID, error) {
		out := []ids.ID{}
		if len(list) == 0 {
			return out, nil
		}
		actors, err := s.Directory.ActorsByIDs(ctx, list)
		if err != nil {
			return nil, err
		}
		for _, a := range actors {
			if a.Kind == identity.KindAgent && a.WorkspaceID == by.WorkspaceID && !slices.Contains(out, a.ID) {
				out = append(out, a.ID)
			}
		}
		if len(out) != len(dedupe(list)) {
			return nil, ErrNotAllowed
		}
		return out, nil
	}
	ta, err := clean(taskAgents)
	if err != nil {
		return Access{}, err
	}
	ca, err := clean(calAgents)
	if err != nil {
		return Access{}, err
	}
	a := Access{OwnerID: by.ID, WorkspaceID: by.WorkspaceID, TaskAll: taskAll, TaskAgentIDs: ta, CalAgentIDs: ca, UpdatedAt: s.Clock.Now()}
	if err := s.Store.PutAccess(ctx, a); err != nil {
		return Access{}, err
	}
	return a, nil
}

func dedupe(list []ids.ID) []ids.ID {
	var out []ids.ID
	for _, id := range list {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

var colourRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Calendars lists the caller's subscriptions.
func (s *Service) Calendars(ctx context.Context, by identity.Actor) ([]Calendar, error) {
	if err := human(by); err != nil {
		return nil, err
	}
	return s.Store.Calendars(ctx, by.ID)
}

// AddCalendar subscribes the caller to an ICS URL and fetches it once.
func (s *Service) AddCalendar(ctx context.Context, by identity.Actor, name, rawURL, colour string) (Calendar, error) {
	if err := human(by); err != nil {
		return Calendar{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxCalendarName {
		return Calendar{}, ErrCalendarName
	}
	if colour == "" {
		colour = "#3b82f6"
	}
	if !colourRe.MatchString(colour) {
		return Calendar{}, ErrCalendarColor
	}
	u, err := NormalizeCalendarURL(rawURL)
	if err != nil {
		return Calendar{}, err
	}
	now := s.Clock.Now()
	c := Calendar{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, OwnerID: by.ID, Name: name, URL: u, Colour: colour, CreatedAt: now}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.ensureAccess(ctx, by); err != nil {
			return err
		}
		if err := s.Store.InsertCalendar(ctx, c); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventCalendarsChanged, c.ID, CalendarsEvent{OwnerID: by.ID.String(), CalendarID: c.ID.String()}, now)
		return err
	})
	if err != nil {
		return Calendar{}, err
	}
	s.Bus.Notify(seq)
	s.refresh(ctx, c)
	return s.Store.CalendarByID(ctx, c.ID)
}

func (s *Service) ownCalendar(ctx context.Context, by identity.Actor, id ids.ID) (Calendar, error) {
	if err := human(by); err != nil {
		return Calendar{}, err
	}
	c, err := s.Store.CalendarByID(ctx, id)
	if errors.Is(err, db.ErrNotFound) || (err == nil && c.OwnerID != by.ID) {
		return Calendar{}, ErrNotFound
	}
	return c, err
}

// RefreshCalendar fetches one of the caller's calendars now.
func (s *Service) RefreshCalendar(ctx context.Context, by identity.Actor, id ids.ID) (Calendar, error) {
	c, err := s.ownCalendar(ctx, by, id)
	if err != nil {
		return Calendar{}, err
	}
	s.refresh(ctx, c)
	return s.Store.CalendarByID(ctx, c.ID)
}

// DeleteCalendar removes a subscription and its cached events.
func (s *Service) DeleteCalendar(ctx context.Context, by identity.Actor, id ids.ID) error {
	c, err := s.ownCalendar(ctx, by, id)
	if err != nil {
		return err
	}
	now := s.Clock.Now()
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.DeleteCalendar(ctx, c.ID); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventCalendarsChanged, c.ID, CalendarsEvent{OwnerID: by.ID.String(), CalendarID: c.ID.String()}, now)
		return err
	})
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}

// MyEvents returns the caller's calendar occurrences in [from, to).
func (s *Service) MyEvents(ctx context.Context, by identity.Actor, from, to time.Time) ([]Event, error) {
	if err := human(by); err != nil {
		return nil, err
	}
	if !to.After(from) || to.Sub(from) > MaxReadRange {
		return nil, ErrBadRange
	}
	return s.Store.Events(ctx, by.ID, from, to)
}

// refresh fetches and parses a calendar; a failure keeps the last good
// cache and stores a message without the URL.
func (s *Service) refresh(ctx context.Context, c Calendar) {
	now := s.Clock.Now()
	fctx, cancel := context.WithTimeout(ctx, FetchTimeout+5*time.Second)
	defer cancel()
	data, err := s.fetcher().Fetch(fctx, c.URL)
	if err != nil {
		_ = s.Store.SetCalendarFetch(ctx, c.ID, now, FetchError(err))
		return
	}
	evs, err := ParseCalendar(data, s.loc(), now.Add(-CacheBefore), now.Add(CacheAfter))
	if err != nil {
		_ = s.Store.SetCalendarFetch(ctx, c.ID, now, "not a valid calendar (ICS)")
		return
	}
	for i := range evs {
		evs[i].CalendarID, evs[i].OwnerID = c.ID, c.OwnerID
	}
	_ = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.ReplaceEvents(ctx, c.ID, evs); err != nil {
			return err
		}
		return s.Store.SetCalendarFetch(ctx, c.ID, now, "")
	})
}

// RefreshDue fetches every calendar not fetched within every.
func (s *Service) RefreshDue(ctx context.Context, every time.Duration) {
	due, err := s.Store.CalendarsDue(ctx, s.Clock.Now().Add(-every))
	if err != nil {
		return
	}
	for _, c := range due {
		if ctx.Err() != nil {
			return
		}
		s.refresh(ctx, c)
	}
}

// Loop refreshes due calendars until ctx ends.
func (s *Service) Loop(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = DefaultRefresh
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		s.RefreshDue(ctx, every)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// resolveOwner finds whose planner agent may read: the named owner, or
// the only owner who allowed it (calendar=true: the calendar permission).
func (s *Service) resolveOwner(ctx context.Context, agent identity.Actor, ownerHandle string, calendar bool) (ids.ID, error) {
	if agent.Kind != identity.KindAgent {
		return ids.Nil, ErrNotAllowed
	}
	allowed := func(a Access) bool {
		if a.WorkspaceID != agent.WorkspaceID {
			return false
		}
		if calendar {
			return slices.Contains(a.CalAgentIDs, agent.ID)
		}
		return a.TaskAll || slices.Contains(a.TaskAgentIDs, agent.ID)
	}
	if h := strings.TrimPrefix(strings.TrimSpace(ownerHandle), "@"); h != "" {
		owner, err := s.Directory.ActorByHandle(ctx, agent.WorkspaceID, h)
		if err != nil || owner.Kind != identity.KindHuman {
			return ids.Nil, ErrNotAllowed
		}
		a, err := s.Store.Access(ctx, owner.ID)
		if err != nil || !allowed(a) {
			return ids.Nil, ErrNotAllowed
		}
		return owner.ID, nil
	}
	list, err := s.Store.AccessList(ctx, agent.WorkspaceID)
	if err != nil {
		return ids.Nil, err
	}
	var owners []ids.ID
	for _, a := range list {
		if allowed(a) {
			owners = append(owners, a.OwnerID)
		}
	}
	switch len(owners) {
	case 0:
		return ids.Nil, ErrNotAllowed
	case 1:
		return owners[0], nil
	}
	return ids.Nil, ErrOwnerRequired
}

// TaskView is what an agent reads: the open tasks in the owner's order and
// what changed since a time.
type TaskView struct {
	OwnerID ids.ID
	Open    []Task // started first, then scheduled, each in the owner's order
	Changed []Task // created, started or done at or after Since
	Since   time.Time
}

// ReadTasks is the agent's read-only view of an owner's tasks.
func (s *Service) ReadTasks(ctx context.Context, agent identity.Actor, ownerHandle string, since time.Time) (TaskView, error) {
	owner, err := s.resolveOwner(ctx, agent, ownerHandle, false)
	if err != nil {
		return TaskView{}, err
	}
	if since.IsZero() {
		now := s.Clock.Now().In(s.loc())
		since = time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, s.loc())
	}
	all, err := s.Store.Tasks(ctx, owner, since)
	if err != nil {
		return TaskView{}, err
	}
	v := TaskView{OwnerID: owner, Since: since}
	for _, st := range []string{StatusStarted, StatusScheduled} {
		for _, t := range all {
			if t.Status == st {
				v.Open = append(v.Open, t)
			}
		}
	}
	after := func(p *time.Time) bool { return p != nil && !p.Before(since) }
	for _, t := range all {
		if !t.CreatedAt.Before(since) || after(t.StartedAt) || after(t.DoneAt) {
			v.Changed = append(v.Changed, t)
		}
	}
	return v, nil
}

// ReadEvents is the agent's read-only view of an owner's calendar.
func (s *Service) ReadEvents(ctx context.Context, agent identity.Actor, ownerHandle string, from, to time.Time) ([]Event, []Calendar, error) {
	owner, err := s.resolveOwner(ctx, agent, ownerHandle, true)
	if err != nil {
		return nil, nil, err
	}
	if !to.After(from) || to.Sub(from) > MaxReadRange {
		return nil, nil, ErrBadRange
	}
	evs, err := s.Store.Events(ctx, owner, from, to)
	if err != nil {
		return nil, nil, err
	}
	cals, err := s.Store.Calendars(ctx, owner)
	return evs, cals, err
}

// Now returns the service clock's time in the planner's location.
func (s *Service) Now() time.Time { return s.Clock.Now().In(s.loc()) }

// Loc is the planner's time zone (Europe/Budapest by default).
func (s *Service) Loc() *time.Location { return s.loc() }
