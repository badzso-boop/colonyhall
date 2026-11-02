package identity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/palette"
)

// Event types for account control. The workspace.* events are shown to
// everyone in the workspace (e.g. the emergency-stop banner).
const (
	EventTokenIssued        = "token.issued"
	EventTokenRevoked       = "token.revoked"
	EventActorStateChanged  = "actor.state_changed"
	EventActorRoleChanged   = "actor.role_changed"
	EventPasswordReset      = "actor.password_reset"
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

// RoleChanged is the payload of EventActorRoleChanged.
type RoleChanged struct {
	ActorID string `json:"actor_id"`
	Old     string `json:"old"`
	New     string `json:"new"`
}

// PasswordReset is the payload of EventPasswordReset: who, never the password.
type PasswordReset struct {
	ActorID string `json:"actor_id"`
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
	if target.Kind == KindHuman {
		// Only the owner disables people, and never themself.
		if by.Role != RoleOwner || target.ID == by.ID {
			return ErrForbidden
		}
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

// SetActorRole changes a person's role. Only the owner may, and not their own;
// nobody can be made owner, and agents have no role to change.
func (s *Service) SetActorRole(ctx context.Context, by Actor, actorID ids.ID, role Role) error {
	if by.Kind != KindHuman || by.Role != RoleOwner || by.Halted {
		return ErrForbidden
	}
	if role != RoleAdmin && role != RoleMember && role != RoleObserver {
		return ErrInvalidRole
	}
	target, err := s.Store.ActorByID(ctx, actorID)
	if err != nil {
		return err
	}
	if target.WorkspaceID != by.WorkspaceID {
		return db.ErrNotFound
	}
	if target.Kind != KindHuman || target.ID == by.ID {
		return ErrForbidden
	}
	if target.Role == role {
		return nil
	}
	return s.record(ctx, by, EventActorRoleChanged, "actor", target.ID, RoleChanged{ActorID: target.ID.String(), Old: string(target.Role), New: string(role)},
		func(ctx context.Context) error { return s.Store.SetActorRole(ctx, target.ID, role) })
}

// UserRow is a person with their login activity, for the owner's users page.
type UserRow struct {
	Actor          Actor
	ActiveSessions int
	LastLoginAt    *time.Time
}

// ListUsers lists the people of the workspace (the owner only).
func (s *Service) ListUsers(ctx context.Context, by Actor) ([]UserRow, error) {
	if by.Kind != KindHuman || by.Role != RoleOwner {
		return nil, ErrForbidden
	}
	actors, err := s.Store.ListActors(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	stats, err := s.Store.SessionStats(ctx, by.WorkspaceID, s.Clock.Now())
	if err != nil {
		return nil, err
	}
	out := []UserRow{}
	for _, a := range actors {
		if a.Kind != KindHuman {
			continue
		}
		st := stats[a.ID]
		out = append(out, UserRow{Actor: a, ActiveSessions: st.Active, LastLoginAt: st.LastLogin})
	}
	return out, nil
}

// UpdateProfileInput specifies fields to update in an actor's profile.
type UpdateProfileInput struct {
	DisplayName *string
	Description *string
	AvatarEmoji *string
	Timezone    *string
	StatusText  *string
	// Color: admins only, agents only; "" clears it.
	Color *string
}

// validDisplayName refuses what could pose as something else where the name
// is shown: invalid UTF-8, control, format (bidi, zero-width) and line or
// paragraph separator characters. At least one visible character is needed.
func validDisplayName(n string) bool {
	if n == "" || !utf8.ValidString(n) || utf8.RuneCountInString(n) > MaxDisplayNameLen {
		return false
	}
	visible := false
	for _, r := range n {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return false
		}
		if !unicode.IsSpace(r) {
			visible = true
		}
	}
	return visible
}

// UpdateProfile updates an actor's profile.
// A human edits their own profile; admins edit anyone's (including agents); agents cannot edit profiles.
func (s *Service) UpdateProfile(ctx context.Context, by Actor, actorID ids.ID, in UpdateProfileInput) (Actor, error) {
	if by.Kind != KindHuman || by.Halted {
		return Actor{}, ErrForbidden
	}
	target, err := s.Store.ActorByID(ctx, actorID)
	if err != nil {
		return Actor{}, err
	}
	if target.WorkspaceID != by.WorkspaceID {
		return Actor{}, db.ErrNotFound
	}
	if target.ID != by.ID && !by.IsAdmin() {
		return Actor{}, ErrForbidden
	}

	newName := target.DisplayName
	if in.DisplayName != nil {
		// Someone else's name needs the rank check too: an admin must not
		// rename the owner.
		if target.ID != by.ID && !by.CanManage(target) {
			return Actor{}, ErrForbidden
		}
		n := strings.TrimSpace(*in.DisplayName)
		if !validDisplayName(n) {
			return Actor{}, ErrInvalidDisplayName
		}
		newName = n
	}

	newDesc := target.Description
	if in.Description != nil {
		d := *in.Description
		if len([]rune(d)) > MaxDescriptionLen {
			return Actor{}, ErrInvalidDescription
		}
		newDesc = d
	}

	newStatus := target.StatusText
	if in.StatusText != nil {
		st := *in.StatusText
		if len([]rune(st)) > MaxStatusTextLen {
			return Actor{}, ErrInvalidStatusText
		}
		newStatus = st
	}

	newTz := target.Timezone
	if in.Timezone != nil {
		tz := *in.Timezone
		if err := ValidateTimezone(tz); err != nil {
			return Actor{}, err
		}
		newTz = tz
	}
	if newTz == "" {
		newTz = DefaultTimezone
	}

	newEmoji := target.AvatarEmoji
	if in.AvatarEmoji != nil {
		em := *in.AvatarEmoji
		if !ValidAvatarEmoji(em) {
			return Actor{}, ErrInvalidAvatarEmoji
		}
		newEmoji = em
	}

	newColor := target.Color
	if in.Color != nil {
		c := *in.Color
		if !by.IsAdmin() {
			return Actor{}, ErrForbidden
		}
		if c != "" && (target.Kind != KindAgent || !palette.Valid(c)) {
			return Actor{}, ErrInvalidColor
		}
		newColor = c
	}

	payload := ActorProfileUpdated{
		ActorID:     target.ID.String(),
		DisplayName: newName,
		Description: newDesc,
		AvatarEmoji: newEmoji,
		Timezone:    newTz,
		StatusText:  newStatus,
		Color:       newColor,
	}

	err = s.record(ctx, by, EventActorProfileUpdated, "actor", target.ID, payload, func(ctx context.Context) error {
		return s.Store.UpdateActorProfile(ctx, target.ID, newName, newDesc, newEmoji, newTz, newStatus, newColor)
	})
	if err != nil {
		return Actor{}, err
	}

	target.DisplayName = newName
	target.Description = newDesc
	target.AvatarEmoji = newEmoji
	target.Timezone = newTz
	target.StatusText = newStatus
	target.Color = newColor
	if err := s.colour(ctx, target.WorkspaceID, []*Actor{&target}); err != nil {
		return Actor{}, err
	}
	return s.load(ctx, target)
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
