package runs

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Hosts provides host assignment info for agents.
type Hosts interface {
	AssignmentByAgent(ctx context.Context, agentID ids.ID) (hosts.Assignment, error)
}

// Agents tells the runs module whose agents a person may see (identity).
type Agents interface {
	ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
	// OwnedAgentIDs lists the agents (archived too) the person owns.
	OwnedAgentIDs(ctx context.Context, workspaceID, ownerID ids.ID) ([]ids.ID, error)
}

// Service holds the runs use cases.
type Service struct {
	Store        Store
	Agents       Agents
	Conversation conversation.Reader
	Hosts        Hosts
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
	// Another person's agent's errors are not found (round 38).
	if err := s.seesAgent(ctx, by, e.AgentID); err != nil {
		return Error{}, err
	}
	return e, nil
}

// ReportRunInput is a finished run reported by an agent.
type ReportRunInput struct {
	HostID              ids.ID
	Phase               string
	Client              string
	Model               string
	SessionID           string
	ChannelID           ids.ID
	ThreadRootID        ids.ID
	Items               int
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheWriteTokens    int64
	ContextTokens       int64
	InstructionsVersion int
	DurationMS          int
	Status              string
	ErrorID             ids.ID
	WebReads            []string
	Steps               []Step
	ReplyMessageID      ids.ID
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
	if in.ContextTokens < 0 || in.ContextTokens > MaxTokens {
		return Run{}, ErrInvalidRun
	}
	if in.DurationMS < 0 || in.DurationMS > MaxDurationMS {
		return Run{}, ErrInvalidRun
	}
	if strings.TrimSpace(in.Client) == "" {
		return Run{}, ErrInvalidRun
	}
	// References must point at the caller's own things: an error it
	// reported, a channel it can read. The host is not taken from the
	// request at all (this module cannot verify it).
	if in.ErrorID != ids.Nil {
		e, err := s.Store.ErrorByID(ctx, in.ErrorID)
		if err != nil || e.AgentID != by.ID {
			return Run{}, ErrInvalidRun
		}
	}
	if in.ChannelID != ids.Nil {
		if ok, err := s.Conversation.CanRead(ctx, by, in.ChannelID); err != nil || !ok {
			return Run{}, ErrInvalidRun
		}
	}
	in.HostID = ids.Nil
	in.Client, in.Model, in.SessionID = clip(strings.TrimSpace(in.Client), 40), clip(strings.TrimSpace(in.Model), 100), clip(strings.TrimSpace(in.SessionID), 100)
	if len(in.Steps) > 200 {
		in.Steps = in.Steps[:200]
	}

	r := Run{
		ID:                  s.IDs.New(),
		WorkspaceID:         by.WorkspaceID,
		AgentID:             by.ID,
		HostID:              in.HostID,
		Phase:               in.Phase,
		Client:              strings.TrimSpace(in.Client),
		Model:               strings.TrimSpace(in.Model),
		SessionID:           strings.TrimSpace(in.SessionID),
		ChannelID:           in.ChannelID,
		ThreadRootID:        in.ThreadRootID,
		Items:               in.Items,
		InputTokens:         in.InputTokens,
		OutputTokens:        in.OutputTokens,
		CacheReadTokens:     in.CacheReadTokens,
		CacheWriteTokens:    in.CacheWriteTokens,
		ContextTokens:       in.ContextTokens,
		InstructionsVersion: max(in.InstructionsVersion, 0),
		DurationMS:          in.DurationMS,
		Status:              in.Status,
		ErrorID:             in.ErrorID,
		CreatedAt:           s.Clock.Now(),
		WebReads:            in.WebReads,
		Steps:               in.Steps,
		ReplyMessageID:      in.ReplyMessageID,
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
	if !by.IsAdmin() {
		// A person lists one of their own agents' runs at a time.
		if agentID == ids.Nil {
			return nil, ErrForbidden
		}
		if err := s.seesAgent(ctx, by, agentID); err != nil {
			return nil, err
		}
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
	scope, err := s.agentScope(ctx, by, ids.Nil)
	if err != nil {
		return nil, err
	}
	list, err := s.Store.Usage(ctx, by.WorkspaceID, since)
	if err != nil || scope == nil {
		return list, err
	}
	return slices.DeleteFunc(list, func(d DayUsage) bool { return !slices.Contains(scope, d.AgentID) }), nil
}

// AgentBudget returns the daily token budget and usage for the calling agent.
// Only agents may call this; humans get ErrForbidden.
func (s *Service) AgentBudget(ctx context.Context, by identity.Actor) (AgentBudget, error) {
	if by.Kind != identity.KindAgent {
		return AgentBudget{}, ErrForbidden
	}
	var limit int64
	if s.Hosts != nil {
		a, err := s.Hosts.AssignmentByAgent(ctx, by.ID)
		if err == nil {
			limit = a.DailyTokenLimit
		} else if !errors.Is(err, db.ErrNotFound) {
			return AgentBudget{}, err
		}
	}
	now := s.Clock.Now().In(Budapest)
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Budapest)
	resetsAt := todayMidnight.AddDate(0, 0, 1)

	usedToday, err := s.Store.TokensSince(ctx, by.WorkspaceID, by.ID, todayMidnight)
	if err != nil {
		return AgentBudget{}, err
	}

	var remaining int64
	if limit > 0 {
		remaining = limit - usedToday
		if remaining < 0 {
			remaining = 0
		}
	}

	return AgentBudget{
		Limit:     limit,
		UsedToday: usedToday,
		Remaining: remaining,
		ResetsAt:  resetsAt,
	}, nil
}

// TokensToday returns the total tokens consumed by the agent today in Europe/Budapest.
func (s *Service) TokensToday(ctx context.Context, workspaceID, agentID ids.ID) (int64, error) {
	now := s.Clock.Now().In(Budapest)
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Budapest)
	return s.Store.TokensSince(ctx, workspaceID, agentID, todayMidnight)
}

// seesAgent answers db.ErrNotFound unless by may see the agent's runs,
// steps and errors: admins see every agent, a person their own (round 38).
func (s *Service) seesAgent(ctx context.Context, by identity.Actor, agentID ids.ID) error {
	if by.IsAdmin() || s.Agents == nil {
		return nil
	}
	a, err := s.Agents.ActorByID(ctx, agentID)
	if err != nil {
		return err
	}
	if !by.SeesAgent(a) {
		return db.ErrNotFound
	}
	return nil
}

// agentScope is the set of agents whose runs by may see under an owner
// filter; nil means every agent (an admin without an owner filter).
func (s *Service) agentScope(ctx context.Context, by identity.Actor, ownerID ids.ID) ([]ids.ID, error) {
	if !by.IsAdmin() {
		if ownerID != ids.Nil && ownerID != by.ID {
			return []ids.ID{}, nil
		}
		ownerID = by.ID
	}
	if ownerID == ids.Nil {
		return nil, nil
	}
	if s.Agents == nil {
		return []ids.ID{}, nil
	}
	list, err := s.Agents.OwnedAgentIDs(ctx, by.WorkspaceID, ownerID)
	if list == nil {
		list = []ids.ID{}
	}
	return list, err
}
