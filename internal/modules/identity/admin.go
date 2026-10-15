package identity

import (
	"context"
	"errors"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Event types for account control. The workspace.* events are shown to
// everyone in the workspace (e.g. the emergency-stop banner).
const (
	EventTokenIssued        = "token.issued"
	EventTokenRevoked       = "token.revoked"
	EventActorStateChanged  = "actor.state_changed"
	EventEmergencyActivated = "workspace.emergency_activated"
	EventEmergencyCleared   = "workspace.emergency_cleared"
	EventLogin              = "auth.login"
	EventLoginFailed        = "auth.login_failed"
)

// TokenEvent is the payload of token events. It never contains the secret.
type TokenEvent struct {
	TokenID string `json:"token_id"`
	ActorID string `json:"actor_id"`
	Label   string `json:"label,omitempty"`
}

// StateChanged is the payload of EventActorStateChanged.
type StateChanged struct {
	ActorID string `json:"actor_id"`
	State   string `json:"state"`
}

// EmergencyEvent is the payload of the workspace emergency events.
type EmergencyEvent struct {
	By string `json:"by"`
}

// LoginEvent is the payload of the auth events.
type LoginEvent struct {
	ActorID   string `json:"actor_id"`
	UserAgent string `json:"user_agent,omitempty"`
}

// load fills the computed Halted flag. Stopped actors are rejected.
func (s *Service) load(ctx context.Context, a Actor) (Actor, error) {
	if a.State == StateStopped {
		return Actor{}, ErrUnauthorized
	}
	a.Halted = a.State == StatePaused
	if a.Kind == KindAgent && !a.Halted {
		ws, err := s.Store.WorkspaceByID(ctx, a.WorkspaceID)
		if err != nil {
			return Actor{}, err
		}
		a.Halted = ws.EmergencyStopAt != nil
	}
	return a, nil
}

// Current reloads an actor with its up-to-date state. Long-lived sessions
// (MCP) call it on every tool call so a pause or an emergency stop applies
// immediately, not only to new connections.
func (s *Service) Current(ctx context.Context, actorID ids.ID) (Actor, error) {
	a, err := s.Store.ActorByID(ctx, actorID)
	if errors.Is(err, db.ErrNotFound) {
		return Actor{}, ErrUnauthorized
	}
	if err != nil {
		return Actor{}, err
	}
	return s.load(ctx, a)
}

// ListTokens lists an actor's tokens (yours, or of someone you manage).
func (s *Service) ListTokens(ctx context.Context, by Actor, actorID ids.ID) ([]Token, error) {
	if _, err := s.managed(ctx, by, actorID, true); err != nil {
		return nil, err
	}
	return s.Store.ListTokens(ctx, actorID)
}

// IssueToken creates a new token for an actor you manage; the secret is
// returned once.
func (s *Service) IssueToken(ctx context.Context, by Actor, actorID ids.ID, label string) (Token, string, error) {
	target, err := s.managed(ctx, by, actorID, false)
	if err != nil {
		return Token{}, "", err
	}
	secret, err := NewSecret()
	if err != nil {
		return Token{}, "", err
	}
	if label == "" {
		label = "token"
	}
	t := Token{ID: s.IDs.New(), ActorID: target.ID, Hash: HashSecret(secret), Label: truncate(label, 64), CreatedAt: s.Clock.Now()}
	err = s.record(ctx, by, EventTokenIssued, "token", t.ID, TokenEvent{TokenID: t.ID.String(), ActorID: target.ID.String(), Label: t.Label},
		func(ctx context.Context) error { return s.Store.CreateToken(ctx, t) })
	if err != nil {
		return Token{}, "", err
	}
	return t, secret, nil
}

// RevokeToken revokes one of your tokens or of someone you manage.
func (s *Service) RevokeToken(ctx context.Context, by Actor, tokenID ids.ID) error {
	t, err := s.Store.TokenByID(ctx, tokenID)
	if err != nil {
		return err
	}
	if _, err := s.managed(ctx, by, t.ActorID, true); err != nil {
		return err
	}
	return s.record(ctx, by, EventTokenRevoked, "token", t.ID, TokenEvent{TokenID: t.ID.String(), ActorID: t.ActorID.String()},
		func(ctx context.Context) error { return s.Store.RevokeToken(ctx, t.ID, s.Clock.Now()) })
}

// SetActorState pauses, stops or reactivates an actor you manage. Stopping
// also ends the actor's browser sessions.
func (s *Service) SetActorState(ctx context.Context, by Actor, actorID ids.ID, state string) error {
	if state != StateActive && state != StatePaused && state != StateStopped {
		return ErrInvalidState
	}
	target, err := s.managed(ctx, by, actorID, false)
	if err != nil {
		return err
	}
	return s.record(ctx, by, EventActorStateChanged, "actor", target.ID, StateChanged{ActorID: target.ID.String(), State: state},
		func(ctx context.Context) error {
			if err := s.Store.SetActorState(ctx, target.ID, state); err != nil {
				return err
			}
			if state == StateStopped {
				return s.Store.RevokeSessionsForActor(ctx, target.ID, ids.Nil, s.Clock.Now())
			}
			return nil
		})
}

// Workspace returns the caller's workspace (emergency-stop status).
func (s *Service) Workspace(ctx context.Context, by Actor) (Workspace, error) {
	return s.Store.WorkspaceByID(ctx, by.WorkspaceID)
}

// EmergencyStop halts every agent in the workspace. Human admins only.
func (s *Service) EmergencyStop(ctx context.Context, by Actor) error {
	if by.Kind != KindHuman || !by.IsAdmin() {
		return ErrForbidden
	}
	now := s.Clock.Now()
	return s.record(ctx, by, EventEmergencyActivated, "workspace", by.WorkspaceID, EmergencyEvent{By: by.ID.String()},
		func(ctx context.Context) error { return s.Store.SetEmergencyStop(ctx, by.WorkspaceID, &now, by.ID) })
}

// ClearEmergency lifts the emergency stop. It requires the admin's
// password again, so a hijacked browser session alone cannot restart agents.
// A paused admin cannot lift it. (Turning the stop on stays allowed for
// every human admin: it only ever reduces what agents can do.)
func (s *Service) ClearEmergency(ctx context.Context, by Actor, password string) error {
	if by.Kind != KindHuman || !by.IsAdmin() || by.Halted {
		return ErrForbidden
	}
	hash, err := s.Store.Credential(ctx, by.ID)
	if errors.Is(err, db.ErrNotFound) {
		return ErrBadCredentials
	}
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil {
		return err
	}
	if !ok {
		return ErrBadCredentials
	}
	return s.record(ctx, by, EventEmergencyCleared, "workspace", by.WorkspaceID, EmergencyEvent{By: by.ID.String()},
		func(ctx context.Context) error { return s.Store.SetEmergencyStop(ctx, by.WorkspaceID, nil, ids.Nil) })
}

// managed loads the target and checks that by may control it. With
// allowSelf, acting on your own account is allowed too.
func (s *Service) managed(ctx context.Context, by Actor, actorID ids.ID, allowSelf bool) (Actor, error) {
	target, err := s.Store.ActorByID(ctx, actorID)
	if err != nil {
		return Actor{}, err
	}
	if target.WorkspaceID != by.WorkspaceID {
		return Actor{}, db.ErrNotFound
	}
	if allowSelf && target.ID == by.ID {
		return target, nil
	}
	if !by.CanManage(target) {
		return Actor{}, ErrForbidden
	}
	return target, nil
}

// record runs change and appends the event in one transaction, then
// announces it.
func (s *Service) record(ctx context.Context, by Actor, typ, aggType string, aggID ids.ID, payload any, change func(ctx context.Context) error) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := change(ctx); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, typ, by.ID, ids.Nil, aggType, aggID, payload, s.Clock.Now())
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
