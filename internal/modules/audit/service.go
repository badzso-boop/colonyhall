package audit

import (
	"bytes"
	"context"
	"errors"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
)

// ConsumerName is the event-log consumer that fills the audit log.
const ConsumerName = "audit"

// ErrForbidden: only admins read the audit log.
var ErrForbidden = errors.New("only admins can read the audit log")

// Service holds the audit use cases.
type Service struct {
	Store Store
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
