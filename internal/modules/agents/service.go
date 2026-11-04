package agents

import (
	"context"
	"errors"
	"unicode/utf8"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Identity is what the agents module needs from identity.
type Identity interface {
	CheckAgentHandle(ctx context.Context, by identity.Actor, handle string) (identity.HandleState, error)
	CreateAgentActor(ctx context.Context, by identity.Actor, in identity.AgentActorInput) (identity.Actor, string, error)
	ArchiveAgent(ctx context.Context, by identity.Actor, agentID ids.ID) (identity.Actor, error)
	ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
}

// Hosts is what the agents module needs from hosts.
type Hosts interface {
	ValidateNewAgent(ctx context.Context, by identity.Actor, in hosts.AssignInput, grants []hosts.GrantInput) (hosts.Assignment, []hosts.GrantInput, error)
	CreateAgentConfig(ctx context.Context, by identity.Actor, agent identity.Actor, a hosts.Assignment, grants []hosts.GrantInput) (hosts.AgentConfig, error)
	EndAssignment(ctx context.Context, by identity.Actor, agentID ids.ID) error
	// AssignmentByAgent returns db.ErrNotFound for an agent without one.
	AssignmentByAgent(ctx context.Context, agentID ids.ID) (hosts.Assignment, error)
}

// Channels is what the agents module needs from conversation.
type Channels interface {
	// ChannelInWorkspace reports whether the channel exists in the workspace.
	ChannelInWorkspace(ctx context.Context, workspaceID, channelID ids.ID) (bool, error)
	AddMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID, canWrite bool) error
	RemoveMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID) error
	ChannelIDsOf(ctx context.Context, by identity.Actor, actorID ids.ID) ([]ids.ID, error)
}

// Rules renders the shared rules (agentrules).
type Rules interface {
	RenderShared(ctx context.Context, workspaceID ids.ID, handle string) (int, string, error)
	RenderText(ctx context.Context, workspaceID ids.ID, text, handle string) string
}

// Service creates and archives agents (round 33).
type Service struct {
	Store    Store
	Identity Identity
	Hosts    Hosts
	Channels Channels
	Rules    Rules
	// RevokeDelegations ends the agent's active delegation grants
	// (delegation module); AckInbox acks its pending items without waking
	// it (delivery module). nil: skipped.
	RevokeDelegations func(ctx context.Context, by identity.Actor, agentID ids.ID) error
	AckInbox          func(ctx context.Context, agentID ids.ID) error

	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock
}

// CreateInput is a whole agent: profile, machine configuration, channels
// and instructions. APIKey: no machine, only a token.
type CreateInput struct {
	Profile      identity.AgentActorInput
	APIKey       bool
	Host         *hosts.AssignInput
	Grants       []hosts.GrantInput
	ChannelIDs   []ids.ID
	Instructions string
}

// Created is the new agent.
type Created struct {
	Actor  identity.Actor
	Config *hosts.AgentConfig
	Token  string // API-key agents only
}

func admin(by identity.Actor) error {
	if by.Kind != identity.KindHuman || !by.IsAdmin() || by.Halted {
		return ErrForbidden
	}
	return nil
}

// Create validates everything first, then writes the actor, the
// assignment, the grants, the memberships and the instructions in one
// transaction, so a half-made agent cannot exist.
func (s *Service) Create(ctx context.Context, by identity.Actor, in CreateInput) (Created, error) {
	if err := admin(by); err != nil {
		return Created{}, err
	}
	// --- validation: nothing is written before it passes ---
	if err := identity.ValidateAgentActor(in.Profile); err != nil {
		return Created{}, err
	}
	st, err := s.Identity.CheckAgentHandle(ctx, by, in.Profile.Handle)
	if err != nil {
		return Created{}, err
	}
	if st.Taken {
		return Created{}, ErrHandleTaken
	}
	if in.APIKey && (in.Host != nil || len(in.Grants) > 0) {
		return Created{}, ErrAPIKeyWithHost
	}
	if !in.APIKey && in.Host == nil {
		return Created{}, ErrHostRequired
	}
	var assignment hosts.Assignment
	var grants []hosts.GrantInput
	if in.Host != nil {
		if assignment, grants, err = s.Hosts.ValidateNewAgent(ctx, by, *in.Host, in.Grants); err != nil {
			return Created{}, err
		}
	}
	if err := ValidateInstructions(in.Instructions); err != nil {
		return Created{}, err
	}
	own := in.Instructions
	if assignment.WriteScope == hosts.WriteScopeDocs {
		p, err := s.Identity.ActorByID(ctx, assignment.PartnerID)
		if err != nil {
			return Created{}, err
		}
		own = withDocsOnly(own, p.Handle)
	}
	if own != "" && s.Rules != nil {
		_, shared, err := s.Rules.RenderShared(ctx, by.WorkspaceID, in.Profile.Handle)
		if err != nil {
			return Created{}, err
		}
		if utf8.RuneCountInString(Effective(shared, own, in.Profile.Handle)) > MaxEffective {
			return Created{}, ErrEffectiveTooLong
		}
	}
	channels := make([]ids.ID, 0, len(in.ChannelIDs))
	seen := map[ids.ID]bool{}
	for _, c := range in.ChannelIDs {
		if seen[c] {
			continue
		}
		seen[c] = true
		ok, err := s.Channels.ChannelInWorkspace(ctx, by.WorkspaceID, c)
		if err != nil {
			return Created{}, err
		}
		if !ok {
			return Created{}, ErrChannelNotFound
		}
		channels = append(channels, c)
	}

	// --- writes ---
	in.Profile.WithToken = in.APIKey
	var out Created
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		a, secret, err := s.Identity.CreateAgentActor(ctx, by, in.Profile)
		if err != nil {
			if errors.Is(err, db.ErrConflict) {
				return ErrHandleTaken
			}
			return err
		}
		out.Actor, out.Token = a, secret
		ev := AgentEvent{AgentID: a.ID.String(), Handle: a.Handle}
		if in.Host != nil {
			cfg, err := s.Hosts.CreateAgentConfig(ctx, by, a, assignment, grants)
			if err != nil {
				return err
			}
			out.Config = &cfg
			ev.HostID = cfg.HostID.String()
		}
		for _, c := range channels {
			if err := s.Channels.AddMember(ctx, by, c, a.ID, true); err != nil {
				return err
			}
		}
		if in.Instructions != "" {
			if err := s.Store.CreateInstructions(ctx, Instructions{AgentID: a.ID, WorkspaceID: by.WorkspaceID, Version: 1,
				Text: in.Instructions, CreatedBy: by.ID, CreatedAt: s.Clock.Now()}); err != nil {
				return err
			}
		}
		seq, err = s.append(ctx, by, EventAgentCreated, a.ID, ev)
		return err
	})
	if err != nil {
		return Created{}, err
	}
	// The modules' own notifications ran before the commit; this one, with
	// the newest sequence, makes every event of the creation visible.
	s.Bus.Notify(seq)
	return out, nil
}

// Archive deletes an agent the only way the plan allows: it is stopped
// and archived, its tokens, assignment, grants, memberships and active
// delegation grants end, and its inbox is acked without waking it. Its
// messages stay. Archiving twice is a no-op.
func (s *Service) Archive(ctx context.Context, by identity.Actor, agentID ids.ID) error {
	if err := admin(by); err != nil {
		return err
	}
	a, err := s.Identity.ActorByID(ctx, agentID)
	if err != nil {
		return err
	}
	if a.WorkspaceID != by.WorkspaceID {
		return db.ErrNotFound
	}
	if a.Kind != identity.KindAgent {
		return identity.ErrNotAnAgent
	}
	if a.ArchivedAt != nil {
		return nil
	}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.Identity.ArchiveAgent(ctx, by, a.ID); err != nil {
			return err
		}
		if err := s.Hosts.EndAssignment(ctx, by, a.ID); err != nil {
			return err
		}
		channels, err := s.Channels.ChannelIDsOf(ctx, by, a.ID)
		if err != nil {
			return err
		}
		for _, c := range channels {
			if err := s.Channels.RemoveMember(ctx, by, c, a.ID); err != nil {
				return err
			}
		}
		if s.RevokeDelegations != nil {
			if err := s.RevokeDelegations(ctx, by, a.ID); err != nil {
				return err
			}
		}
		if s.AckInbox != nil {
			if err := s.AckInbox(ctx, a.ID); err != nil {
				return err
			}
		}
		seq, err = s.append(ctx, by, EventAgentArchived, a.ID, AgentEvent{AgentID: a.ID.String(), Handle: a.Handle})
		return err
	})
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}

// OwnInstructions returns an agent's own instructions ("" when none);
// for the shared-rules delivery (no caller check: the agent's own, or a
// human's request already checked). A docs-only planner's fixed block
// follows them.
func (s *Service) OwnInstructions(ctx context.Context, agentID ids.ID) (string, error) {
	var text string
	in, err := s.Store.InstructionsByAgent(ctx, agentID)
	if err == nil {
		text = in.Text
	} else if !errors.Is(err, db.ErrNotFound) {
		return "", err
	}
	return s.withFixedBlocks(ctx, agentID, text)
}

// withFixedBlocks appends the blocks the configuration dictates (the
// docs-only planner's, round 38) to an agent's own instructions.
func (s *Service) withFixedBlocks(ctx context.Context, agentID ids.ID, own string) (string, error) {
	if s.Hosts == nil {
		return own, nil
	}
	a, err := s.Hosts.AssignmentByAgent(ctx, agentID)
	if errors.Is(err, db.ErrNotFound) {
		return own, nil
	}
	if err != nil {
		return "", err
	}
	if a.WriteScope != hosts.WriteScopeDocs {
		return own, nil
	}
	partner := "your-sandboxed-partner"
	if a.PartnerID != ids.Nil {
		if p, err := s.Identity.ActorByID(ctx, a.PartnerID); err == nil {
			partner = p.Handle
		}
	}
	return withDocsOnly(own, partner), nil
}

func withDocsOnly(own, partner string) string {
	if own == "" {
		return DocsOnlyBlock(partner)
	}
	return own + "\n\n" + DocsOnlyBlock(partner)
}

// InstructionsView is an agent's own instructions and what it receives.
type InstructionsView struct {
	Instructions
	Effective string
}

// Instructions shows a human an agent's own instructions. Agents never
// read other agents' instructions.
func (s *Service) Instructions(ctx context.Context, by identity.Actor, agentID ids.ID) (InstructionsView, error) {
	if by.Kind != identity.KindHuman {
		return InstructionsView{}, ErrForbidden
	}
	a, err := s.Identity.ActorByID(ctx, agentID)
	if err != nil {
		return InstructionsView{}, err
	}
	if a.WorkspaceID != by.WorkspaceID || a.Kind != identity.KindAgent {
		return InstructionsView{}, db.ErrNotFound
	}
	var v InstructionsView
	in, err := s.Store.InstructionsByAgent(ctx, agentID)
	if err == nil {
		v.Instructions = in
	} else if !errors.Is(err, db.ErrNotFound) {
		return InstructionsView{}, err
	}
	if s.Rules != nil {
		_, shared, err := s.Rules.RenderShared(ctx, by.WorkspaceID, a.Handle)
		if err != nil {
			return InstructionsView{}, err
		}
		own, err := s.withFixedBlocks(ctx, agentID, v.Text)
		if err != nil {
			return InstructionsView{}, err
		}
		v.Effective = Effective(shared, s.Rules.RenderText(ctx, by.WorkspaceID, own, a.Handle), a.Handle)
	}
	return v, nil
}

func (s *Service) append(ctx context.Context, by identity.Actor, typ string, agentID ids.ID, payload AgentEvent) (int64, error) {
	ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, typ, by.ID, ids.Nil, "actor", agentID, payload, s.Clock.Now())
	if err != nil {
		return 0, err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return 0, err
	}
	return ev.Seq, nil
}
