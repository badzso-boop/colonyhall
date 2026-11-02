package audit

import (
	"bytes"
	"context"
	"errors"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// ConsumerName is the event-log consumer that fills the audit log.
const ConsumerName = "audit"

// ErrForbidden: only admins read the audit log.
var ErrForbidden = errors.New("only admins can read the audit log")

// Service holds the audit use cases.
type Service struct {
	Store Store
	// Events, Bus, Tx, IDs and Clock record the exports themselves.
	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock
}

// HandleEvent appends one event to its workspace's chain. It runs inside
// the consumer transaction; events arrive in seq order, one at a time.
func (s *Service) HandleEvent(ctx context.Context, e eventlog.Event) error {
	prev := []byte{}
	last, err := s.Store.Last(ctx, e.WorkspaceID)
	switch {
	case err == nil:
		if last.Seq >= e.Seq {
			return nil // already recorded (replay)
		}
		prev = last.Hash
	case errors.Is(err, db.ErrNotFound):
	default:
		return err
	}
	entry := Entry{Seq: e.Seq, WorkspaceID: e.WorkspaceID, ActorID: e.ActorID, Action: e.Type,
		TargetType: e.AggregateType, TargetID: e.AggregateID, Payload: e.Payload, At: e.At, PrevHash: prev}
	entry.Hash = entry.ComputeHash()
	return s.Store.Insert(ctx, entry)
}

// List returns a page of the caller's workspace audit log, newest first.
func (s *Service) List(ctx context.Context, by identity.Actor, before int64, limit int) ([]Entry, error) {
	if !by.IsAdmin() {
		return nil, ErrForbidden
	}
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	return s.Store.Page(ctx, by.WorkspaceID, before, limit)
}

// Verify walks the whole chain of the caller's workspace.
func (s *Service) Verify(ctx context.Context, by identity.Actor) (VerifyResult, error) {
	if !by.IsAdmin() {
		return VerifyResult{}, ErrForbidden
	}
	res := VerifyResult{OK: true}
	prev := []byte{}
	var after int64
	for {
		batch, err := s.Store.Forward(ctx, by.WorkspaceID, after, 500)
		if err != nil {
			return VerifyResult{}, err
		}
		if len(batch) == 0 {
			return res, nil
		}
		for _, e := range batch {
			res.Checked++
			if !bytes.Equal(e.PrevHash, prev) || !bytes.Equal(e.ComputeHash(), e.Hash) {
				return VerifyResult{OK: false, Checked: res.Checked, BrokenAt: e.Seq}, nil
			}
			prev = e.Hash
			after = e.Seq
		}
	}
}

// MaxExportRows bounds one CSV export.
const MaxExportRows = 100_000

// EventExported is written when someone exports the audit log.
const EventExported = "audit.exported"

// Exported is the payload of EventExported: what was asked for.
type Exported struct {
	Rows       int64    `json:"rows"`
	Actions    []string `json:"actions,omitempty"`
	TargetType string   `json:"target_type,omitempty"`
}

// Search lists a filtered page, newest first (admins).
func (s *Service) Search(ctx context.Context, by identity.Actor, f Filter) ([]Entry, error) {
	if !by.IsAdmin() {
		return nil, ErrForbidden
	}
	if f.Limit <= 0 {
		f.Limit = DefaultPageSize
	}
	f.Limit = min(f.Limit, MaxPageSize)
	return s.Store.Search(ctx, by.WorkspaceID, f)
}

// Actions lists the actions that occur, for the filter (admins).
func (s *Service) Actions(ctx context.Context, by identity.Actor) ([]string, error) {
	if !by.IsAdmin() {
		return nil, ErrForbidden
	}
	return s.Store.Actions(ctx, by.WorkspaceID)
}

// Export walks the entries a filter selects, newest first, up to
// MaxExportRows, and records that it happened. begin gets the total first, so
// the caller can say when the output is cut.
func (s *Service) Export(ctx context.Context, by identity.Actor, f Filter, begin func(total int64) error, row func(Entry) error) error {
	if !by.IsAdmin() {
		return ErrForbidden
	}
	total, err := s.Store.Count(ctx, by.WorkspaceID, f)
	if err != nil {
		return err
	}
	if err := s.recordExport(ctx, by, f, total); err != nil {
		return err
	}
	if err := begin(total); err != nil {
		return err
	}
	written := 0
	f.Limit = 1000
	for written < MaxExportRows {
		page, err := s.Store.Search(ctx, by.WorkspaceID, f)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		for _, e := range page {
			if written == MaxExportRows {
				return nil
			}
			if err := row(e); err != nil {
				return err
			}
			written++
		}
		f.Before = page[len(page)-1].Seq
	}
	return nil
}

func (s *Service) recordExport(ctx context.Context, by identity.Actor, f Filter, total int64) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventExported, by.ID, ids.Nil, "audit", by.WorkspaceID,
			Exported{Rows: min(total, MaxExportRows), Actions: f.Actions, TargetType: f.TargetType}, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}
