package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Conversation is what the scheduler needs from the conversation module.
type Conversation interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	SendMessage(ctx context.Context, by identity.Actor, in conversation.SendInput) (conversation.SendResult, error)
}

// Actors loads an actor with its current state (paused, stopped,
// emergency stop), because a schedule must not post for someone who may
// no longer write.
type Actors interface {
	Current(ctx context.Context, actorID ids.ID) (identity.Actor, error)
}

// Service holds the schedule use cases.
type Service struct {
	Store        Store
	Conversation Conversation
	Actors       Actors
	Events       eventlog.Store
	Bus          bus.Bus
	Tx           db.TxRunner
	IDs          ids.Generator
	Clock        clock.Clock
}

// CreateInput describes a scheduled message.
type CreateInput struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID
	Body         string
	At           *time.Time // one-off
	Cron         string     // repeating
	Timezone     string
	CatchUp      string
}

func human(by identity.Actor) error {
	if by.Kind != identity.KindHuman || by.Halted {
		return ErrForbidden
	}
	return nil
}

// Create schedules a message in the caller's name. Only humans schedule:
// the message will carry their authority (e.g. a task for a full-access
// agent), so an agent must not be able to create one.
func (s *Service) Create(ctx context.Context, by identity.Actor, in CreateInput) (Schedule, error) {
	if err := human(by); err != nil {
		return Schedule{}, err
	}
	body, err := conversation.ValidateBody(in.Body)
	if err != nil {
		return Schedule{}, ErrEmptyBody
	}
	if in.Timezone == "" {
		in.Timezone = DefaultTimezone
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return Schedule{}, ErrBadZone
	}
	if in.CatchUp == "" {
		in.CatchUp = CatchUpOnce
	}
	if in.CatchUp != CatchUpOnce && in.CatchUp != CatchUpSkip {
		return Schedule{}, ErrBadCatchUp
	}
	now := s.Clock.Now()
	var next time.Time
	switch {
	case in.At != nil && in.Cron == "":
		if in.At.Before(now.Add(-time.Minute)) {
			return Schedule{}, ErrBadTime
		}
		next = in.At.UTC()
	case in.At == nil && in.Cron != "":
		if next, err = Next(in.Cron, in.Timezone, now); err != nil {
			return Schedule{}, err
		}
	default:
		return Schedule{}, ErrBadTime
	}
	if ok, err := s.Conversation.CanRead(ctx, by, in.ChannelID); err != nil || !ok {
		if err == nil {
			err = ErrForbidden
		}
		return Schedule{}, err
	}
	sc := Schedule{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, CreatedBy: by.ID, ChannelID: in.ChannelID,
		ThreadRootID: in.ThreadRootID, Body: body, Cron: strings.TrimSpace(in.Cron), Timezone: in.Timezone,
		CatchUp: in.CatchUp, State: StateActive, NextRunAt: next, CreatedAt: now}
	err = s.record(ctx, by.WorkspaceID, by.ID, EventCreated, sc.ID,
		Event{ScheduleID: sc.ID.String(), ChannelID: sc.ChannelID.String(), Cron: sc.Cron, NextRunAt: next.Format(time.RFC3339)},
		func(ctx context.Context) error { return s.Store.Create(ctx, sc) })
	return sc, err
}

// PreviewRuns is how many runs Preview shows.
const PreviewRuns = 5

// Preview returns the next runs of a schedule as Create would make it,
// without storing anything: the schedule builder shows them before saving.
func (s *Service) Preview(_ context.Context, by identity.Actor, in CreateInput) ([]time.Time, CreateInput, error) {
	if err := human(by); err != nil {
		return nil, in, err
	}
	if in.Timezone == "" {
		in.Timezone = DefaultTimezone
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return nil, in, ErrBadZone
	}
	in.Cron = strings.TrimSpace(in.Cron)
	now := s.Clock.Now()
	switch {
	case in.At != nil && in.Cron == "":
		if in.At.Before(now.Add(-time.Minute)) {
			return nil, in, ErrBadTime
		}
		return []time.Time{in.At.UTC()}, in, nil
	case in.At == nil && in.Cron != "":
		runs, err := NextRuns(in.Cron, in.Timezone, now, PreviewRuns)
		return runs, in, err
	default:
		return nil, in, ErrBadTime
	}
}

// List returns the schedules the caller may see: admins all, others their own.
func (s *Service) List(ctx context.Context, by identity.Actor) ([]Schedule, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	all, err := s.Store.List(ctx, by.WorkspaceID)
	if err != nil || by.IsAdmin() {
		return all, err
	}
	var own []Schedule
	for _, sc := range all {
		if sc.CreatedBy == by.ID {
			own = append(own, sc)
		}
	}
	return own, nil
}

// Cancel stops a schedule (its creator or an admin).
func (s *Service) Cancel(ctx context.Context, by identity.Actor, id ids.ID) error {
	if err := human(by); err != nil {
		return err
	}
	sc, err := s.Store.ByID(ctx, id)
	if err != nil {
		return err
	}
	if sc.WorkspaceID != by.WorkspaceID {
		return db.ErrNotFound
	}
	if sc.CreatedBy != by.ID && !by.IsAdmin() {
		return ErrForbidden
	}
	return s.record(ctx, by.WorkspaceID, by.ID, EventCanceled, sc.ID, Event{ScheduleID: sc.ID.String()},
		func(ctx context.Context) error { return s.Store.SetState(ctx, sc.ID, StateCanceled) })
}

// RunDue posts every due message once and returns how many it posted.
// Each run is first claimed with a conditional update, so two scheduler
// loops (or a retry) never post the same run twice.
func (s *Service) RunDue(ctx context.Context) (int, error) {
	now := s.Clock.Now()
	due, err := s.Store.Due(ctx, now, 50)
	if err != nil {
		return 0, err
	}
	posted := 0
	for _, sc := range due {
		ok, err := s.fire(ctx, sc, now)
		if err != nil {
			return posted, err
		}
		if ok {
			posted++
		}
	}
	return posted, nil
}

func (s *Service) fire(ctx context.Context, sc Schedule, now time.Time) (bool, error) {
	state, next := StateDone, sc.NextRunAt
	if sc.Cron != "" {
		n, err := Next(sc.Cron, sc.Timezone, now) // missed runs collapse into this one
		if err != nil {
			return false, err
		}
		state, next = StateActive, n
	}
	claimed, err := s.Store.Claim(ctx, sc.ID, sc.NextRunAt, next, state, now)
	if err != nil || !claimed {
		return false, err
	}
	late := now.Sub(sc.NextRunAt)
	if late > MaxLate || (sc.CatchUp == CatchUpSkip && late > 2*time.Minute) {
		return false, s.outcome(ctx, sc, ids.Nil, fmt.Sprintf("missed: due %s, the server was not running", sc.NextRunAt.Format(time.RFC3339)), late)
	}
	creator, err := s.Actors.Current(ctx, sc.CreatedBy)
	if err != nil {
		return false, s.outcome(ctx, sc, ids.Nil, "the creator cannot post: "+err.Error(), late)
	}
	body := "⏰ " + sc.Body
	if late > 2*time.Minute {
		body = "⏰ (delayed) " + sc.Body
	}
	res, err := s.Conversation.SendMessage(ctx, creator, conversation.SendInput{
		ChannelID: sc.ChannelID, ThreadRootID: sc.ThreadRootID, Body: body,
		ClientMsgID: fmt.Sprintf("s-%s-%d", sc.ID, sc.NextRunAt.Unix()), // a retried run does not post twice
	})
	if err != nil {
		return false, s.outcome(ctx, sc, ids.Nil, err.Error(), late)
	}
	msgID := ids.Nil
	if res.Message != nil {
		msgID = res.Message.ID
	}
	return true, s.outcome(ctx, sc, msgID, "", late)
}

func (s *Service) outcome(ctx context.Context, sc Schedule, msgID ids.ID, runErr string, late time.Duration) error {
	ev := Event{ScheduleID: sc.ID.String(), Late: late > 2*time.Minute, Error: runErr}
	if msgID != ids.Nil {
		ev.MessageID = msgID.String()
	}
	return s.record(ctx, sc.WorkspaceID, sc.CreatedBy, EventFired, sc.ID, ev,
		func(ctx context.Context) error { return s.Store.Record(ctx, sc.ID, msgID, runErr) })
}

func (s *Service) record(ctx context.Context, workspaceID, actorID ids.ID, typ string, aggID ids.ID, payload any, change func(ctx context.Context) error) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := change(ctx); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, actorID, ids.Nil, "schedule", aggID, payload, s.Clock.Now())
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

// Loop runs RunDue every interval until ctx ends.
func (s *Service) Loop(ctx context.Context, interval time.Duration, logf func(msg string, args ...any)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := s.RunDue(ctx); err != nil && ctx.Err() == nil {
			logf("scheduler: run failed", "err", err)
		} else if n > 0 {
			logf("scheduler: posted", "messages", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
