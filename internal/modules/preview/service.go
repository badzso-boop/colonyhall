// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// ConsumerName is the name of the preview eventlog consumer.
const ConsumerName = "preview"

// Hosts is what preview needs from the hosts module.
type Hosts interface {
	Assignment(ctx context.Context, agentID ids.ID) (hosts.Assignment, error)
}

// Conversation is what preview needs from conversation.
type Conversation interface {
	CanWrite(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	SendMessage(ctx context.Context, by identity.Actor, in conversation.SendInput) (conversation.SendResult, error)
}

// Directory is what preview needs to look up actors.
type Directory interface {
	ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
}

// Service manages previews.
type Service struct {
	Store        Store
	Hosts        Hosts
	Conversation Conversation
	Directory    Directory
	Events       eventlog.Store
	Bus          bus.Bus
	Tx           db.TxRunner
	IDs          ids.Generator
	Clock        clock.Clock
}

// StartInput is the input for starting a preview via MCP.
type StartInput struct {
	Name         string
	ComposePath  string
	TTL          time.Duration
	ChannelID    ids.ID
	ThreadRootID ids.ID
}

// Start creates a new requested preview for an agent.
func (s *Service) Start(ctx context.Context, by identity.Actor, in StartInput) (Preview, error) {
	if by.Kind != identity.KindAgent || by.Halted {
		return Preview{}, ErrForbidden
	}
	if err := ValidateName(in.Name); err != nil {
		return Preview{}, err
	}
	if err := ValidateComposePath(in.ComposePath); err != nil {
		return Preview{}, err
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if ttl < time.Minute || ttl > MaxTTL {
		return Preview{}, ErrBadTTL
	}

	// The preview's link is posted into the channel: the agent must be able to write there.
	if in.ChannelID != ids.Nil {
		ok, err := s.Conversation.CanWrite(ctx, by, in.ChannelID)
		if err != nil || !ok {
			if err == nil {
				err = ErrForbidden
			}
			return Preview{}, err
		}
	}

	as, err := s.Hosts.Assignment(ctx, by.ID)
	if err != nil {
		return Preview{}, ErrNotAllowed
	}
	if !as.AllowPreview || as.Mode != hosts.AgentModeDirect || as.Sandbox {
		return Preview{}, ErrNotAllowed
	}

	// Check if already active with the same name.
	if _, err := s.Store.ActiveByAgentAndName(ctx, by.ID, in.Name); err == nil {
		return Preview{}, ErrAlreadyRunning
	} else if !errors.Is(err, db.ErrNotFound) {
		return Preview{}, err
	}

	now := s.Clock.Now()
	var threadRootPtr *ids.ID
	if in.ThreadRootID != ids.Nil {
		threadRootPtr = &in.ThreadRootID
	}

	p := Preview{
		ID:           s.IDs.New(),
		WorkspaceID:  by.WorkspaceID,
		AgentID:      by.ID,
		HostID:       as.HostID,
		ChannelID:    in.ChannelID,
		ThreadRootID: threadRootPtr,
		Name:         strings.TrimSpace(in.Name),
		ComposePath:  strings.TrimSpace(in.ComposePath),
		State:        StateRequested,
		ExpiresAt:    now.Add(ttl),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.Create(ctx, p); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventPreviewRequested, by.ID, in.ChannelID,
			"preview", p.ID, PreviewEvent{
				PreviewID:   p.ID.String(),
				AgentID:     p.AgentID.String(),
				HostID:      p.HostID.String(),
				Name:        p.Name,
				ComposePath: p.ComposePath,
				State:       p.State,
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
		return Preview{}, err
	}
	s.Bus.Notify(seq)
	return p, nil
}

// Stop stops an active preview for the calling agent by name.
func (s *Service) Stop(ctx context.Context, by identity.Actor, name string) (Preview, error) {
	if by.Kind != identity.KindAgent || by.Halted {
		return Preview{}, ErrForbidden
	}
	p, err := s.Store.ActiveByAgentAndName(ctx, by.ID, name)
	if err != nil {
		return Preview{}, ErrNotFound
	}
	now := s.Clock.Now()
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.StopActive(ctx, p.ID, now); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventPreviewStopped, by.ID, p.ChannelID,
			"preview", p.ID, PreviewEvent{
				PreviewID: p.ID.String(),
				AgentID:   p.AgentID.String(),
				HostID:    p.HostID.String(),
				Name:      p.Name,
				State:     StateStopped,
				StoppedBy: by.ID.String(),
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
		return Preview{}, err
	}
	s.Bus.Notify(seq)
	p.State = StateStopped
	p.UpdatedAt = now
	return p, nil
}

// StopByID stops an active preview by ID (for humans / admins).
func (s *Service) StopByID(ctx context.Context, by identity.Actor, id ids.ID) (Preview, error) {
	if by.Halted {
		return Preview{}, ErrForbidden
	}
	p, err := s.Store.ByID(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	if p.WorkspaceID != by.WorkspaceID {
		return Preview{}, db.ErrNotFound
	}
	if by.Kind != identity.KindHuman {
		return Preview{}, ErrForbidden
	}
	if !by.IsAdmin() {
		agentActor, err := s.Directory.ActorByID(ctx, p.AgentID)
		if err != nil || !by.CanManage(agentActor) {
			return Preview{}, ErrForbidden
		}
	}
	if p.State == StateStopped || p.State == StateFailed {
		return p, nil
	}
	now := s.Clock.Now()
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.StopActive(ctx, p.ID, now); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventPreviewStopped, by.ID, p.ChannelID,
			"preview", p.ID, PreviewEvent{
				PreviewID: p.ID.String(),
				AgentID:   p.AgentID.String(),
				HostID:    p.HostID.String(),
				Name:      p.Name,
				State:     StateStopped,
				StoppedBy: by.ID.String(),
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
		return Preview{}, err
	}
	s.Bus.Notify(seq)
	p.State = StateStopped
	p.UpdatedAt = now
	return p, nil
}

// Get loads a preview by ID.
func (s *Service) Get(ctx context.Context, by identity.Actor, id ids.ID) (Preview, error) {
	p, err := s.Store.ByID(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	if p.WorkspaceID != by.WorkspaceID || !s.seen(ctx, by, p.AgentID) {
		return Preview{}, db.ErrNotFound
	}
	return p, nil
}

// ProxyTarget is the running preview a person opens through the server's
// reverse proxy (round 57). Same visibility as Get; agents never use it
// (the proxy would let them reach other machines' ports through the
// server), and a preview that is not running is ErrOffline.
func (s *Service) ProxyTarget(ctx context.Context, by identity.Actor, id ids.ID) (Preview, error) {
	if by.Kind != identity.KindHuman {
		return Preview{}, ErrForbidden
	}
	p, err := s.Get(ctx, by, id)
	if err != nil {
		return Preview{}, err
	}
	if p.State != StateRunning || p.URL == "" || !s.Clock.Now().Before(p.ExpiresAt) {
		return p, ErrOffline
	}
	return p, nil
}

// List returns previews for a workspace, optionally filtered by agentID. A
// person sees only the previews of agents they may see (round 38).
func (s *Service) List(ctx context.Context, by identity.Actor, agentID ids.ID) ([]Preview, error) {
	var list []Preview
	var err error
	if agentID != ids.Nil {
		list, err = s.Store.ListByAgent(ctx, by.WorkspaceID, agentID)
	} else {
		list, err = s.Store.ListByWorkspace(ctx, by.WorkspaceID)
	}
	if err != nil || by.Kind != identity.KindHuman || by.IsAdmin() {
		return list, err
	}
	out := list[:0:0]
	for _, p := range list {
		if s.seen(ctx, by, p.AgentID) {
			out = append(out, p)
		}
	}
	return out, nil
}

// seen: agents (MCP) keep their old view; a person sees only the previews of
// agents they may see.
func (s *Service) seen(ctx context.Context, by identity.Actor, agentID ids.ID) bool {
	if by.Kind != identity.KindHuman || by.IsAdmin() {
		return true
	}
	a, err := s.Directory.ActorByID(ctx, agentID)
	return err == nil && by.SeesAgent(a)
}

// ReportStatus updates the state of a preview as reported by a guard.
func (s *Service) ReportStatus(ctx context.Context, hostID, previewID ids.ID, state, url, errStr string) error {
	p, err := s.Store.ByID(ctx, previewID)
	if err != nil {
		return err
	}
	if p.HostID != hostID {
		return ErrForbidden
	}
	now := s.Clock.Now()

	var eventType string
	switch state {
	case StateRunning:
		eventType = EventPreviewRunning
	case StateFailed:
		eventType = EventPreviewFailed
	case StateStopped:
		eventType = EventPreviewStopped
	default:
		return errors.New("invalid reported preview state")
	}

	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.UpdateStatus(ctx, previewID, state, url, errStr, now); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), p.WorkspaceID, eventType, ids.Nil, p.ChannelID,
			"preview", p.ID, PreviewEvent{
				PreviewID: p.ID.String(),
				AgentID:   p.AgentID.String(),
				HostID:    p.HostID.String(),
				Name:      p.Name,
				State:     state,
				URL:       url,
				Error:     errStr,
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

	// Post announcement in thread.
	if p.ChannelID != ids.Nil && s.Conversation != nil && s.Directory != nil {
		actor, err := s.Directory.ActorByID(ctx, p.AgentID)
		if err == nil {
			var threadRoot ids.ID
			if p.ThreadRootID != nil {
				threadRoot = *p.ThreadRootID
			}
			var body string
			switch state {
			case StateRunning:
				remaining := int(p.ExpiresAt.Sub(s.Clock.Now()).Minutes())
				if remaining < 0 {
					remaining = 0
				}
				body = fmt.Sprintf("Preview '%s' is running: %s (expires in %d min)", p.Name, url, remaining)
			case StateFailed:
				// The reason (compose output, paths) is for humans only:
				// it stays on the preview, shown in the Previews panel.
				body = fmt.Sprintf("Preview '%s' failed to start; the reason is in the Previews panel.", p.Name)
			}
			if body != "" {
				_, _ = s.Conversation.SendMessage(ctx, actor, conversation.SendInput{
					ChannelID:    p.ChannelID,
					ThreadRootID: threadRoot,
					Body:         body,
				})
			}
		}
	}

	return nil
}

// HandleEvent handles background events (e.g. emergency stop or actor state changed).
func (s *Service) HandleEvent(ctx context.Context, ev eventlog.Event) error {
	now := s.Clock.Now()
	switch ev.Type {
	case identity.EventEmergencyActivated:
		return s.Store.StopActiveByWorkspace(ctx, ev.WorkspaceID, now)
	case identity.EventActorStateChanged:
		var sc identity.StateChanged
		if err := json.Unmarshal(ev.Payload, &sc); err == nil && sc.State != identity.StateActive {
			agentID, err := ids.Parse(sc.ActorID)
			if err == nil {
				return s.Store.StopActiveByAgent(ctx, agentID, now)
			}
		}
	}
	return nil
}

// HostPreviews implements hosts.PreviewProvider.
func (s *Service) HostPreviews(ctx context.Context, hostID ids.ID) ([]hosts.HostPreview, error) {
	active, err := s.Store.ActiveByHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	out := make([]hosts.HostPreview, 0, len(active))
	for _, p := range active {
		out = append(out, hosts.HostPreview{
			ID:          p.ID,
			AgentID:     p.AgentID,
			Name:        p.Name,
			ComposePath: p.ComposePath,
			State:       p.State,
			URL:         p.URL,
			Error:       p.Error,
			ExpiresAt:   p.ExpiresAt,
		})
	}
	return out, nil
}
