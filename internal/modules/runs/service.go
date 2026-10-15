package runs

import (
	"context"
	"strings"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Service holds the runs use cases.
type Service struct {
	Store        Store
	Conversation conversation.Reader
	Events       eventlog.Store
	Bus          bus.Bus
	Tx           db.TxRunner
	IDs          ids.Generator
	Clock        clock.Clock
}

// ReportInput is an error an agent's run produced.
type ReportInput struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID
	Summary      string
	Detail       string
}

// ReportError stores an error of the calling agent's run. The agent must be
// able to read the channel it reports about.
func (s *Service) ReportError(ctx context.Context, by identity.Actor, in ReportInput) (Error, error) {
	if by.Kind != identity.KindAgent {
		return Error{}, ErrForbidden
	}
	summary := clip(strings.TrimSpace(Redact(in.Summary)), MaxSummary)
	if summary == "" {
		return Error{}, ErrEmpty
	}
	ch, err := s.Conversation.ChannelByID(ctx, in.ChannelID)
	if err != nil {
		return Error{}, err
	}
	if ch.WorkspaceID != by.WorkspaceID {
		return Error{}, db.ErrNotFound
	}
	if ok, err := s.Conversation.CanRead(ctx, by, ch.ID); err != nil || !ok {
		if err == nil {
			err = ErrForbidden
		}
		return Error{}, err
	}
	e := Error{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, AgentID: by.ID, ChannelID: ch.ID, ThreadRootID: in.ThreadRootID,
		Summary: summary, Detail: clip(Redact(in.Detail), MaxDetail), CreatedAt: s.Clock.Now()}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.InsertError(ctx, e); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), e.WorkspaceID, EventErrorReported, by.ID, ids.Nil, "run_error", e.ID,
			ErrorReported{ErrorID: e.ID.String(), AgentID: by.ID.String(), Summary: summary}, e.CreatedAt)
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
		return Error{}, err
	}
	s.Bus.Notify(seq)
	return e, nil
}

// GetError returns an error's details to a human of the workspace. Agents
// never get the details: the reason is for the people who run them.
func (s *Service) GetError(ctx context.Context, by identity.Actor, id ids.ID) (Error, error) {
	if by.Kind != identity.KindHuman {
		return Error{}, ErrForbidden
	}
	e, err := s.Store.ErrorByID(ctx, id)
	if err != nil {
		return Error{}, err
	}
	if e.WorkspaceID != by.WorkspaceID {
		return Error{}, db.ErrNotFound
	}
	// Only someone who can read the conversation it happened in.
	if ok, err := s.Conversation.CanRead(ctx, by, e.ChannelID); err != nil || !ok {
		if err == nil {
			err = ErrForbidden
		}
		return Error{}, err
	}
	return e, nil
}
