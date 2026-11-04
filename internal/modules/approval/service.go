package approval

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

// ConsumerName is the name of the approval eventlog consumer.
const ConsumerName = "approval"

// Conversation is what the approval module needs from conversation.
type Conversation interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	CanWrite(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	SendMessage(ctx context.Context, by identity.Actor, in conversation.SendInput) (conversation.SendResult, error)
}

// Service holds the approval use cases.
type Service struct {
	Store        Store
	Conversation Conversation
	// Agents loads the requesting agent: only its owner or an admin sees and
	// decides its requests (round 38). nil: anyone who reads the channel.
	Agents interface {
		ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
	}
	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock
}

// CreateInput is the input for creating a tool approval request.
type CreateInput struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID
	ToolName     string
	Input        string
	Timeout      time.Duration
}

// CreateRequest creates and stores an approval request, posting a notice in the thread as the agent.
func (s *Service) CreateRequest(ctx context.Context, by identity.Actor, in CreateInput) (ApprovalRequest, error) {
	if by.Kind != identity.KindAgent {
		return ApprovalRequest{}, ErrForbidden
	}
	toolName := clip(strings.TrimSpace(in.ToolName), 120)
	if toolName == "" {
		return ApprovalRequest{}, ErrBadToolName
	}
	// The request posts a notice in the thread: the agent must be able to write there.
	if ok, err := s.Conversation.CanWrite(ctx, by, in.ChannelID); err != nil || !ok {
		if err == nil {
			err = ErrForbidden
		}
		return ApprovalRequest{}, err
	}
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	now := s.Clock.Now()
	req := ApprovalRequest{
		ID:           s.IDs.New(),
		WorkspaceID:  by.WorkspaceID,
		AgentID:      by.ID,
		ChannelID:    in.ChannelID,
		ThreadRootID: in.ThreadRootID,
		ToolName:     toolName,
		Input:        clip(Redact(in.Input), MaxInput),
		State:        StatePending,
		CreatedAt:    now,
		ExpiresAt:    now.Add(timeout),
	}

	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.Create(ctx, req); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), req.WorkspaceID, EventApprovalRequested, by.ID, req.ChannelID,
			"approval", req.ID, ApprovalRequested{
				ApprovalID: req.ID.String(),
				AgentID:    by.ID.String(),
				ChannelID:  req.ChannelID.String(),
				ToolName:   req.ToolName,
			}, now)
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
		return ApprovalRequest{}, err
	}

	// Post message in the chat thread as the agent.
	msgBody := fmt.Sprintf("🔐 Approval needed: %s [approval:%s]", req.ToolName, req.ID)
	_, _ = s.Conversation.SendMessage(ctx, by, conversation.SendInput{
		ChannelID:    in.ChannelID,
		ThreadRootID: in.ThreadRootID,
		Body:         msgBody,
	})

	s.Bus.Notify(seq)
	return req, nil
}

// WaitForDecision waits until the request is decided, expires, or the agent is halted.
func (s *Service) WaitForDecision(ctx context.Context, req ApprovalRequest, current func(ctx context.Context) (identity.Actor, error)) (ApprovalRequest, error) {
	signals, unsub := s.Bus.Subscribe()
	defer unsub()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		// 1. Check if agent is halted / paused / stopped.
		if current != nil {
			actor, err := current(ctx)
			if err != nil || actor.Halted || actor.State != identity.StateActive {
				now := s.Clock.Now()
				_ = s.decideInternal(ctx, req.ID, StateDenied, ids.Nil, "agent halted", now)
				return s.Store.ByID(ctx, req.ID)
			}
		}

		// 2. Check store state.
		cur, err := s.Store.ByID(ctx, req.ID)
		if err != nil {
			return ApprovalRequest{}, err
		}
		if cur.State != StatePending {
			return cur, nil
		}

		// 3. Check expiration.
		now := s.Clock.Now()
		if !now.Before(req.ExpiresAt) {
			_ = s.decideInternal(ctx, req.ID, StateExpired, ids.Nil, "no answer in time", now)
			return s.Store.ByID(ctx, req.ID)
		}

		select {
		case <-ctx.Done():
			return req, ctx.Err()
		case <-signals:
		case <-ticker.C:
		}
	}
}

// Get returns the approval request details to a human who can read the channel.
func (s *Service) Get(ctx context.Context, by identity.Actor, id ids.ID) (ApprovalRequest, error) {
	if by.Kind != identity.KindHuman {
		return ApprovalRequest{}, ErrForbidden
	}
	r, err := s.Store.ByID(ctx, id)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if r.WorkspaceID != by.WorkspaceID {
		return ApprovalRequest{}, db.ErrNotFound
	}
	if ok, err := s.Conversation.CanRead(ctx, by, r.ChannelID); err != nil || !ok {
		if err == nil {
			err = ErrForbidden
		}
		return ApprovalRequest{}, err
	}
	if err := s.seesAgent(ctx, by, r.AgentID); err != nil {
		return ApprovalRequest{}, err
	}
	return r, nil
}

// Decide records a human's decision on a pending approval request.
func (s *Service) Decide(ctx context.Context, by identity.Actor, id ids.ID, decision string, message string) (ApprovalRequest, error) {
	if by.Kind != identity.KindHuman {
		return ApprovalRequest{}, ErrForbidden
	}
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != DecisionAllow && decision != DecisionDeny {
		return ApprovalRequest{}, ErrBadDecision
	}
	r, err := s.Store.ByID(ctx, id)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if r.WorkspaceID != by.WorkspaceID {
		return ApprovalRequest{}, db.ErrNotFound
	}
	if by.ID == r.AgentID {
		return ApprovalRequest{}, ErrForbidden
	}
	// Deciding acts in the channel: a read-only member may watch, not decide.
	if ok, err := s.Conversation.CanWrite(ctx, by, r.ChannelID); err != nil || !ok {
		if err == nil {
			err = ErrForbidden
		}
		return ApprovalRequest{}, err
	}
	if err := s.seesAgent(ctx, by, r.AgentID); err != nil {
		return ApprovalRequest{}, err
	}
	if r.State != StatePending {
		return ApprovalRequest{}, ErrAlreadyDecided
	}

	now := s.Clock.Now()
	if !now.Before(r.ExpiresAt) {
		_ = s.decideInternal(ctx, id, StateExpired, ids.Nil, "no answer in time", now)
		return ApprovalRequest{}, ErrAlreadyDecided
	}

	message = clip(strings.TrimSpace(message), 1000)
	newState := StateAllowed
	if decision == DecisionDeny {
		newState = StateDenied
	}
	if err := s.decideInternal(ctx, id, newState, by.ID, message, now); err != nil {
		return ApprovalRequest{}, err
	}
	return s.Store.ByID(ctx, id)
}

func (s *Service) decideInternal(ctx context.Context, id ids.ID, state string, decidedBy ids.ID, message string, now time.Time) error {
	r, err := s.Store.ByID(ctx, id)
	if err != nil {
		return err
	}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.Decide(ctx, id, state, decidedBy, message, now); err != nil {
			return err
		}
		dec := "allow"
		switch state {
		case StateDenied:
			dec = "deny"
		case StateExpired:
			dec = "expire"
		}
		byStr := ""
		if decidedBy != ids.Nil {
			byStr = decidedBy.String()
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), r.WorkspaceID, EventApprovalDecided, decidedBy, r.ChannelID,
			"approval", r.ID, ApprovalDecided{
				ApprovalID: r.ID.String(),
				DecidedBy:  byStr,
				Decision:   dec,
				State:      state,
			}, now)
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

// HandleEvent expires all pending approval requests when emergency stop is activated.
func (s *Service) HandleEvent(ctx context.Context, ev eventlog.Event) error {
	if ev.Type == identity.EventEmergencyActivated {
		_, err := s.Store.ExpirePendingByWorkspace(ctx, ev.WorkspaceID, s.Clock.Now())
		return err
	}
	return nil
}

// seesAgent answers db.ErrNotFound when the request belongs to another
// person's agent: approving its tool use is giving it a task (round 38).
func (s *Service) seesAgent(ctx context.Context, by identity.Actor, agentID ids.ID) error {
	if s.Agents == nil || by.IsAdmin() {
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
