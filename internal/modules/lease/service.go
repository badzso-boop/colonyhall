package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// budapest is the time zone lease times are shown in (agents and humans
// read "until 14:30" as local time).
var budapest = mustLoadBudapest()

func mustLoadBudapest() *time.Location {
	loc, err := time.LoadLocation("Europe/Budapest")
	if err != nil {
		return time.UTC
	}
	return loc
}

// AcquireResult contains the outcome of an acquire call.
type AcquireResult struct {
	Taken   bool
	Lease   Lease
	Message string
}

// Service implements the business logic and lifecycle rules for leases.
type Service struct {
	store  Store
	actors identity.Directory
	events eventlog.Store
	bus    bus.Bus
	tx     db.TxRunner
	clock  clock.Clock
	ids    ids.Generator
}

// NewService creates a new lease Service.
func NewService(store Store, actors identity.Directory, events eventlog.Store, b bus.Bus, tx db.TxRunner, clk clock.Clock, gen ids.Generator) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	if gen == nil {
		gen = ids.V7{}
	}
	return &Service{
		store:  store,
		actors: actors,
		events: events,
		bus:    b,
		tx:     tx,
		clock:  clk,
		ids:    gen,
	}
}

// Acquire acquires a lease, extends an existing lease held by the caller, or returns taken info.
func (s *Service) Acquire(ctx context.Context, actor identity.Actor, rawResource string, ttlMinutes int, note string) (AcquireResult, error) {
	if actor.Halted {
		return AcquireResult{}, identity.ErrHalted
	}

	resource, err := NormalizeAndValidateResource(rawResource)
	if err != nil {
		return AcquireResult{}, err
	}

	if ttlMinutes <= 0 {
		ttlMinutes = 30
	}
	if ttlMinutes < 1 || ttlMinutes > 240 {
		return AcquireResult{}, ErrInvalidTTL
	}

	trimmedNote := strings.TrimSpace(note)
	if utf8.RuneCountInString(trimmedNote) > 200 {
		return AcquireResult{}, ErrNoteTooLong
	}

	var res AcquireResult
	var seq int64

	err = s.tx.InTx(ctx, func(txCtx context.Context) error {
		if err := s.store.LockResource(txCtx, actor.WorkspaceID, resource); err != nil {
			return err
		}

		now := s.clock.Now()
		active, err := s.store.GetActiveByResource(txCtx, actor.WorkspaceID, resource, now)
		if err == nil {
			// Active lease exists
			if active.HolderID != actor.ID {
				// Held by someone else -> return taken message, no error, no wait
				handle := active.HolderHandle
				if handle == "" {
					if act, err := s.actors.ActorByID(txCtx, active.HolderID); err == nil {
						handle = act.Handle
					}
				}
				timeStr := active.ExpiresAt.In(budapest).Format("15:04")
				msg := fmt.Sprintf("taken by @%s until %s", handle, timeStr)
				if active.Note != "" {
					msg = fmt.Sprintf("taken by @%s until %s (%s)", handle, timeStr, active.Note)
				}
				res = AcquireResult{
					Taken:   true,
					Lease:   active,
					Message: msg,
				}
				return nil
			}

			// Held by caller -> extend it
			newExpiresAt := now.Add(time.Duration(ttlMinutes) * time.Minute)
			var notePtr *string
			if trimmedNote != "" {
				notePtr = &trimmedNote
			}
			if err := s.store.Extend(txCtx, active.ID, newExpiresAt, notePtr); err != nil {
				return err
			}

			active.ExpiresAt = newExpiresAt
			if notePtr != nil {
				active.Note = *notePtr
			}
			msg := fmt.Sprintf("lease on %s extended until %s", resource, newExpiresAt.In(budapest).Format("15:04"))
			res = AcquireResult{
				Taken:   false,
				Lease:   active,
				Message: msg,
			}

			ev, err := eventlog.NewEvent(s.ids.New(), actor.WorkspaceID, EventExtended, actor.ID, ids.Nil, "lease", active.ID, ExtendedEvent{
				LeaseID:      active.ID,
				Resource:     resource,
				HolderID:     actor.ID,
				NewExpiresAt: newExpiresAt,
			}, now)
			if err != nil {
				return err
			}
			if s.events != nil {
				if err := s.events.Append(txCtx, &ev); err != nil {
					return err
				}
				seq = ev.Seq
			}
			return nil
		}

		if !errors.Is(err, db.ErrNotFound) {
			return err
		}

		// No active lease -> create new one
		expiresAt := now.Add(time.Duration(ttlMinutes) * time.Minute)
		l := Lease{
			ID:           s.ids.New(),
			WorkspaceID:  actor.WorkspaceID,
			Resource:     resource,
			HolderID:     actor.ID,
			HolderHandle: actor.Handle,
			Note:         trimmedNote,
			AcquiredAt:   now,
			ExpiresAt:    expiresAt,
		}

		if err := s.store.Create(txCtx, l); err != nil {
			return err
		}

		msg := fmt.Sprintf("lease on %s acquired until %s", resource, expiresAt.In(budapest).Format("15:04"))
		res = AcquireResult{
			Taken:   false,
			Lease:   l,
			Message: msg,
		}

		ev, err := eventlog.NewEvent(s.ids.New(), actor.WorkspaceID, EventAcquired, actor.ID, ids.Nil, "lease", l.ID, AcquiredEvent{
			LeaseID:   l.ID,
			Resource:  resource,
			HolderID:  actor.ID,
			ExpiresAt: expiresAt,
			Note:      trimmedNote,
		}, now)
		if err != nil {
			return err
		}
		if s.events != nil {
			if err := s.events.Append(txCtx, &ev); err != nil {
				return err
			}
			seq = ev.Seq
		}
		return nil
	})

	if err != nil {
		return AcquireResult{}, err
	}
	if seq > 0 && s.bus != nil {
		s.bus.Notify(seq)
	}
	return res, nil
}

// ReleaseByResource releases a lease held by the caller on the specified resource.
func (s *Service) ReleaseByResource(ctx context.Context, actor identity.Actor, rawResource string) (string, error) {
	if actor.Halted {
		return "", identity.ErrHalted
	}

	resource, err := NormalizeAndValidateResource(rawResource)
	if err != nil {
		return "", err
	}

	var seq int64
	err = s.tx.InTx(ctx, func(txCtx context.Context) error {
		if err := s.store.LockResource(txCtx, actor.WorkspaceID, resource); err != nil {
			return err
		}

		now := s.clock.Now()
		active, err := s.store.GetActiveByResource(txCtx, actor.WorkspaceID, resource, now)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return ErrLeaseNotFound
			}
			return err
		}

		if active.HolderID != actor.ID {
			return ErrNotHolder
		}

		if err := s.store.Release(txCtx, active.ID, actor.ID, now); err != nil {
			return err
		}

		ev, err := eventlog.NewEvent(s.ids.New(), actor.WorkspaceID, EventReleased, actor.ID, ids.Nil, "lease", active.ID, ReleasedEvent{
			LeaseID:    active.ID,
			Resource:   resource,
			HolderID:   active.HolderID,
			ReleasedBy: actor.ID,
		}, now)
		if err != nil {
			return err
		}
		if s.events != nil {
			if err := s.events.Append(txCtx, &ev); err != nil {
				return err
			}
			seq = ev.Seq
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if seq > 0 && s.bus != nil {
		s.bus.Notify(seq)
	}
	return fmt.Sprintf("lease on %s released", resource), nil
}

// ReleaseByID releases any lease by its ID (humans only).
func (s *Service) ReleaseByID(ctx context.Context, actor identity.Actor, id ids.ID) (Lease, error) {
	if actor.Kind != identity.KindHuman || actor.Halted {
		return Lease{}, ErrForbidden
	}

	var l Lease
	var seq int64

	err := s.tx.InTx(ctx, func(txCtx context.Context) error {
		existing, err := s.store.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if existing.WorkspaceID != actor.WorkspaceID {
			return ErrLeaseNotFound
		}

		now := s.clock.Now()
		if existing.ReleasedAt != nil || !existing.ExpiresAt.After(now) {
			// Already released or expired
			l = existing
			return nil
		}

		if err := s.store.LockResource(txCtx, existing.WorkspaceID, existing.Resource); err != nil {
			return err
		}

		if err := s.store.Release(txCtx, existing.ID, actor.ID, now); err != nil {
			return err
		}

		existing.ReleasedAt = &now
		existing.ReleasedBy = &actor.ID
		l = existing

		ev, err := eventlog.NewEvent(s.ids.New(), actor.WorkspaceID, EventReleased, actor.ID, ids.Nil, "lease", existing.ID, ReleasedEvent{
			LeaseID:    existing.ID,
			Resource:   existing.Resource,
			HolderID:   existing.HolderID,
			ReleasedBy: actor.ID,
			Reason:     "human release",
		}, now)
		if err != nil {
			return err
		}
		if s.events != nil {
			if err := s.events.Append(txCtx, &ev); err != nil {
				return err
			}
			seq = ev.Seq
		}
		return nil
	})

	if err != nil {
		return Lease{}, err
	}
	if seq > 0 && s.bus != nil {
		s.bus.Notify(seq)
	}
	return l, nil
}

// ListActive lists active leases in the workspace for humans.
func (s *Service) ListActive(ctx context.Context, actor identity.Actor) ([]Lease, error) {
	if actor.Kind != identity.KindHuman || actor.Halted {
		return nil, ErrForbidden
	}
	list, err := s.store.ListActive(ctx, actor.WorkspaceID, s.clock.Now())
	if err != nil {
		return nil, err
	}
	return s.visible(ctx, actor, list)
}

// visible drops the leases of agents the person may not see (round 38).
func (s *Service) visible(ctx context.Context, actor identity.Actor, list []Lease) ([]Lease, error) {
	if actor.IsAdmin() || len(list) == 0 {
		return list, nil
	}
	holders := make([]ids.ID, 0, len(list))
	for _, l := range list {
		holders = append(holders, l.HolderID)
	}
	actors, err := s.actors.ActorsByIDs(ctx, holders)
	if err != nil {
		return nil, err
	}
	byID := make(map[ids.ID]identity.Actor, len(actors))
	for _, a := range actors {
		byID[a.ID] = a
	}
	out := list[:0:0]
	for _, l := range list {
		if a, ok := byID[l.HolderID]; ok && actor.SeesAgent(a) {
			out = append(out, l)
		}
	}
	return out, nil
}

// History is how far back state=all may look.
const History = 7 * 24 * time.Hour

// List returns the active leases (state "active"), or every lease still
// running or ended since `since` (state "all"; default the last 24 hours,
// at most History back). Humans only.
func (s *Service) List(ctx context.Context, actor identity.Actor, state string, since *time.Time) ([]Lease, error) {
	switch state {
	case "", "active":
		return s.ListActive(ctx, actor)
	case "all":
	default:
		return nil, ErrBadState
	}
	if actor.Kind != identity.KindHuman || actor.Halted {
		return nil, ErrForbidden
	}
	now := s.clock.Now()
	from := now.Add(-24 * time.Hour)
	if since != nil {
		from = *since
	}
	if from.Before(now.Add(-History)) || from.After(now) {
		return nil, ErrBadSince
	}
	list, err := s.store.ListSince(ctx, actor.WorkspaceID, from)
	if err != nil {
		return nil, err
	}
	return s.visible(ctx, actor, list)
}

// HandleEvent handles background events: agent halted/deleted or emergency stop.
func (s *Service) HandleEvent(ctx context.Context, ev eventlog.Event) error {
	now := s.clock.Now()
	switch ev.Type {
	case identity.EventEmergencyActivated:
		return s.autoRelease(ctx, ev, "emergency stop", func(txCtx context.Context) ([]Lease, error) {
			return s.store.ReleaseActiveByWorkspace(txCtx, ev.WorkspaceID, ev.ActorID, now)
		})
	case identity.EventActorStateChanged:
		var sc identity.StateChanged
		if err := json.Unmarshal(ev.Payload, &sc); err != nil || sc.State == identity.StateActive {
			return nil
		}
		holderID, err := ids.Parse(sc.ActorID)
		if err != nil {
			return nil
		}
		return s.autoRelease(ctx, ev, "agent halted/deleted", func(txCtx context.Context) ([]Lease, error) {
			return s.store.ReleaseActiveByHolder(txCtx, ev.WorkspaceID, holderID, ev.ActorID, now)
		})
	}
	return nil
}

// autoRelease releases leases and records their events in one transaction
// (transactional outbox: a release without its event must not commit).
func (s *Service) autoRelease(ctx context.Context, ev eventlog.Event, reason string, release func(context.Context) ([]Lease, error)) error {
	now := s.clock.Now()
	var last int64
	err := s.tx.InTx(ctx, func(txCtx context.Context) error {
		released, err := release(txCtx)
		if err != nil {
			return err
		}
		for _, l := range released {
			seq, err := s.recordRelease(txCtx, ev.WorkspaceID, ev.ActorID, l, reason, now)
			if err != nil {
				return err
			}
			last = seq
		}
		return nil
	})
	if err != nil {
		return err
	}
	if last > 0 && s.bus != nil {
		s.bus.Notify(last)
	}
	return nil
}

func (s *Service) recordRelease(ctx context.Context, workspaceID, actorID ids.ID, l Lease, reason string, now time.Time) (int64, error) {
	if s.events == nil {
		return 0, nil
	}
	ev, err := eventlog.NewEvent(s.ids.New(), workspaceID, EventReleased, actorID, ids.Nil, "lease", l.ID, ReleasedEvent{
		LeaseID:    l.ID,
		Resource:   l.Resource,
		HolderID:   l.HolderID,
		ReleasedBy: actorID,
		Reason:     reason,
	}, now)
	if err != nil {
		return 0, err
	}
	if err := s.events.Append(ctx, &ev); err != nil {
		return 0, err
	}
	return ev.Seq, nil
}
