package runs

import (
	"context"
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

// ReportRunInput is a finished run reported by an agent.
type ReportRunInput struct {
	HostID           ids.ID
	Phase            string
	Client           string
	Model            string
	SessionID        string
	ChannelID        ids.ID
	ThreadRootID     ids.ID
	Items            int
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	DurationMS       int
	Status           string
	ErrorID          ids.ID
}

// ReportRun records a finished run for the calling agent.
func (s *Service) ReportRun(ctx context.Context, by identity.Actor, in ReportRunInput) (Run, error) {
	if by.Kind != identity.KindAgent {
		return Run{}, ErrForbidden
	}
	if in.Phase != PhaseDecide && in.Phase != PhaseWork && in.Phase != PhaseDirect {
		return Run{}, ErrInvalidRun
	}
	if in.Status != StatusOk && in.Status != StatusError && in.Status != StatusHalted {
		return Run{}, ErrInvalidRun
	}
	if in.Items < 0 {
		return Run{}, ErrInvalidRun
	}
	if in.InputTokens < 0 || in.InputTokens > MaxTokens {
		return Run{}, ErrInvalidRun
	}
	if in.OutputTokens < 0 || in.OutputTokens > MaxTokens {
		return Run{}, ErrInvalidRun
	}
	if in.CacheReadTokens < 0 || in.CacheReadTokens > MaxTokens {
		return Run{}, ErrInvalidRun
	}
	if in.CacheWriteTokens < 0 || in.CacheWriteTokens > MaxTokens {
		return Run{}, ErrInvalidRun
	}
	if in.DurationMS < 0 || in.DurationMS > MaxDurationMS {
		return Run{}, ErrInvalidRun
	}
	if strings.TrimSpace(in.Client) == "" {
		return Run{}, ErrInvalidRun
	}

	r := Run{
		ID:               s.IDs.New(),
		WorkspaceID:      by.WorkspaceID,
		AgentID:          by.ID,
		HostID:           in.HostID,
		Phase:            in.Phase,
		Client:           strings.TrimSpace(in.Client),
		Model:            strings.TrimSpace(in.Model),
		SessionID:        strings.TrimSpace(in.SessionID),
		ChannelID:        in.ChannelID,
		ThreadRootID:     in.ThreadRootID,
		Items:            in.Items,
		InputTokens:      in.InputTokens,
		OutputTokens:     in.OutputTokens,
		CacheReadTokens:  in.CacheReadTokens,
		CacheWriteTokens: in.CacheWriteTokens,
		DurationMS:       in.DurationMS,
		Status:           in.Status,
		ErrorID:          in.ErrorID,
		CreatedAt:        s.Clock.Now(),
	}

	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.InsertRun(ctx, r); err != nil {
			return err
		}
		totalTokens := r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens
		ev, err := eventlog.NewEvent(
			s.IDs.New(),
			r.WorkspaceID,
			EventRunReported,
			by.ID,
			ids.Nil,
			"agent_run",
			r.ID,
			RunReported{
				RunID:       r.ID.String(),
				AgentID:     by.ID.String(),
				Phase:       r.Phase,
				Client:      r.Client,
				Model:       r.Model,
				Items:       r.Items,
				TotalTokens: totalTokens,
				DurationMS:  r.DurationMS,
				Status:      r.Status,
			},
			r.CreatedAt,
		)
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
		return Run{}, err
	}
	s.Bus.Notify(seq)
	return r, nil
}

// ListRuns returns recent runs for humans of the workspace, newest first.
func (s *Service) ListRuns(ctx context.Context, by identity.Actor, agentID ids.ID, limit int) ([]Run, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.Store.ListRuns(ctx, by.WorkspaceID, agentID, limit)
}

// Usage returns aggregated token usage per agent and day in Europe/Budapest for humans of the workspace.
func (s *Service) Usage(ctx context.Context, by identity.Actor, days int) ([]DayUsage, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	if days < 1 || days > 90 {
		return nil, ErrInvalidDays
	}
	now := s.Clock.Now().In(Budapest)
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Budapest)
	since := todayMidnight.AddDate(0, 0, -(days - 1))
	return s.Store.Usage(ctx, by.WorkspaceID, since)
}
