// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/tenant"
)

// Round 51 (docs/24-hosztolt-ajanlat.md): the platform operator creates a
// workspace without an owner and hands a one-time invite link to the
// customer; the link's holder becomes the owner. The operator can suspend
// and restore a workspace. These calls carry no actor: the operator module
// calls them after it authenticated the operator.

// InviteTTL is how long an invite link is valid.
const InviteTTL = 7 * 24 * time.Hour

// Workspace events of round 51.
const (
	EventWorkspaceCreated   = "workspace.created"
	EventWorkspaceSuspended = "workspace.suspended"
	EventWorkspaceRestored  = "workspace.restored"
	EventInviteCreated      = "workspace.invite_created"
	EventInviteAccepted     = "workspace.invite_accepted"
)

// Invite errors.
var (
	// ErrInviteInvalid: unknown, used, revoked or expired; one answer for all.
	ErrInviteInvalid = errors.New("this invite link is not valid any more; ask the operator for a new one")
	// ErrHasOwner: the workspace already has an owner; it needs no invite.
	ErrHasOwner = errors.New("the workspace already has an owner")
	// ErrInvalidWorkspaceName: 1–80 characters.
	ErrInvalidWorkspaceName = errors.New("the workspace name must be 1–80 characters")
	// ErrSlugTaken: another workspace has this subdomain.
	ErrSlugTaken = errors.New("another workspace already uses this subdomain")
	// ErrInvalidReason: the suspension reason is 1–500 characters.
	ErrInvalidReason = errors.New("give a reason of 1–500 characters")
)

// Invite is a one-time owner invite. Only the hash of its token is kept.
type Invite struct {
	ID                ids.ID
	WorkspaceID       ids.ID
	TokenHash         []byte
	Role              Role
	CreatedByOperator ids.ID
	CreatedAt         time.Time
	ExpiresAt         time.Time
	UsedAt            *time.Time
	UsedBy            ids.ID
	RevokedAt         *time.Time
}

// Open reports whether the invite can still be used at now.
func (i Invite) Open(now time.Time) bool {
	return i.UsedAt == nil && i.RevokedAt == nil && now.Before(i.ExpiresAt)
}

// WorkspaceCreated is the payload of EventWorkspaceCreated.
type WorkspaceCreated struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	// By: "operator" (the platform operator).
	By string `json:"by"`
}

// WorkspaceStateChanged is the payload of the suspend and restore events.
// The reason stays in the operator's audit.
type WorkspaceStateChanged struct {
	State string `json:"state"`
	By    string `json:"by"`
}

// InviteEvent is the payload of the invite events. Never the token.
type InviteEvent struct {
	InviteID  string    `json:"invite_id"`
	ExpiresAt time.Time `json:"expires_at"`
	OwnerID   string    `json:"owner_id,omitempty"`
}

const byOperator = "operator"

// CreateWorkspace creates an active workspace without an owner (the
// operator; an invite brings the owner).
func (s *Service) CreateWorkspace(ctx context.Context, name, slug string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return Workspace{}, ErrInvalidWorkspaceName
	}
	if !ValidSlug(slug) {
		return Workspace{}, ErrInvalidSlug
	}
	ws := Workspace{ID: s.IDs.New(), Name: name, Slug: slug, State: WorkspaceActive, CreatedAt: s.Clock.Now()}
	if _, err := s.Store.WorkspaceBySlug(ctx, slug); err == nil {
		return Workspace{}, ErrSlugTaken
	} else if !errors.Is(err, db.ErrNotFound) {
		return Workspace{}, err
	}
	err := s.record(db.WithWorkspace(ctx, ws.ID), Actor{WorkspaceID: ws.ID}, EventWorkspaceCreated, "workspace", ws.ID,
		WorkspaceCreated{Name: name, Slug: slug, By: byOperator},
		func(ctx context.Context) error {
			err := s.Store.CreateWorkspace(ctx, ws)
			if errors.Is(err, db.ErrConflict) {
				return ErrSlugTaken
			}
			return err
		})
	if err != nil {
		return Workspace{}, err
	}
	return ws, nil
}

// SetWorkspaceSuspended suspends (with a reason) or restores a workspace.
// The data stays; while suspended, every request, guard and agent of the
// workspace is refused (the tenant layer) and its schedules wait.
func (s *Service) SetWorkspaceSuspended(ctx context.Context, workspaceID ids.ID, suspend bool, reason string) (Workspace, error) {
	reason = strings.TrimSpace(reason)
	if suspend && (reason == "" || utf8.RuneCountInString(reason) > 500) {
		return Workspace{}, ErrInvalidReason
	}
	ctx = db.WithWorkspace(ctx, workspaceID)
	ws, err := s.Store.WorkspaceByID(ctx, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	state, typ := WorkspaceActive, EventWorkspaceRestored
	var at *time.Time
	if suspend {
		now := s.Clock.Now()
		state, typ, at = WorkspaceSuspended, EventWorkspaceSuspended, &now
	} else {
		reason = ""
	}
	if ws.State == state {
		return ws, nil
	}
	err = s.record(ctx, Actor{WorkspaceID: workspaceID}, typ, "workspace", workspaceID, WorkspaceStateChanged{State: state, By: byOperator},
		func(ctx context.Context) error { return s.Store.SetWorkspaceState(ctx, workspaceID, state, reason, at) })
	if err != nil {
		return Workspace{}, err
	}
	ws.State, ws.SuspendedAt, ws.SuspendedReason = state, at, reason
	return ws, nil
}

// HasOwner reports whether the workspace has an active owner.
func (s *Service) HasOwner(ctx context.Context, workspaceID ids.ID) (bool, error) {
	actors, err := s.Store.ListActors(db.WithWorkspace(ctx, workspaceID), workspaceID)
	if err != nil {
		return false, err
	}
	for _, a := range actors {
		if a.Kind == KindHuman && a.Role == RoleOwner {
			return true, nil
		}
	}
	return false, nil
}

// IssueInvite makes a new owner invite for a workspace without an owner;
// earlier open invites are revoked. It returns the invite and its token
// (shown once).
func (s *Service) IssueInvite(ctx context.Context, workspaceID, operatorID ids.ID) (Invite, string, error) {
	ctx = db.WithWorkspace(ctx, workspaceID)
	if _, err := s.Store.WorkspaceByID(ctx, workspaceID); err != nil {
		return Invite{}, "", err
	}
	owned, err := s.HasOwner(ctx, workspaceID)
	if err != nil {
		return Invite{}, "", err
	}
	if owned {
		return Invite{}, "", ErrHasOwner
	}
	secret, err := NewSecret()
	if err != nil {
		return Invite{}, "", err
	}
	now := s.Clock.Now()
	inv := Invite{ID: s.IDs.New(), WorkspaceID: workspaceID, TokenHash: HashSecret(secret), Role: RoleOwner,
		CreatedByOperator: operatorID, CreatedAt: now, ExpiresAt: now.Add(InviteTTL)}
	err = s.record(ctx, Actor{WorkspaceID: workspaceID}, EventInviteCreated, "workspace", workspaceID,
		InviteEvent{InviteID: inv.ID.String(), ExpiresAt: inv.ExpiresAt},
		func(ctx context.Context) error {
			if err := s.Store.RevokeOpenInvites(ctx, workspaceID, now); err != nil {
				return err
			}
			return s.Store.CreateInvite(ctx, inv)
		})
	if err != nil {
		return Invite{}, "", err
	}
	return inv, secret, nil
}

// LatestInvite is the workspace's newest invite (db.ErrNotFound: none).
func (s *Service) LatestInvite(ctx context.Context, workspaceID ids.ID) (Invite, error) {
	return s.Store.LatestInvite(db.WithWorkspace(ctx, workspaceID), workspaceID)
}

// openInvite finds an open invite of the request's subdomain. Invites exist
// only in multi-tenant mode; anything else is ErrInviteInvalid.
func (s *Service) openInvite(ctx context.Context, secret string) (Invite, error) {
	t, ok := tenant.From(ctx)
	if !ok || secret == "" {
		return Invite{}, ErrInviteInvalid
	}
	inv, err := s.Store.InviteByTokenHash(ctx, HashSecret(secret))
	if errors.Is(err, db.ErrNotFound) || (err == nil && (inv.WorkspaceID != t.Workspace || !inv.Open(s.Clock.Now()))) {
		return Invite{}, ErrInviteInvalid
	}
	return inv, err
}

// InviteInfo returns the workspace an open invite is for (its name and the
// invite's expiry), for the acceptance page.
func (s *Service) InviteInfo(ctx context.Context, secret string) (Workspace, Invite, error) {
	inv, err := s.openInvite(ctx, secret)
	if err != nil {
		return Workspace{}, Invite{}, err
	}
	ws, err := s.Store.WorkspaceByID(ctx, inv.WorkspaceID)
	return ws, inv, err
}

// AcceptInviteInput is the new owner.
type AcceptInviteInput struct {
	Handle      string
	DisplayName string
	Password    string
	UserAgent   string
}

// AcceptInvite uses an open invite: the owner is created with the password
// and logged in. It returns the owner, the session and its cookie secret.
func (s *Service) AcceptInvite(ctx context.Context, secret string, in AcceptInviteInput) (Actor, Session, string, error) {
	inv, err := s.openInvite(ctx, secret)
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	if err := ValidatePassword(in.Password); err != nil {
		return Actor{}, Session{}, "", err
	}
	// Validate before anything is written: the in-memory store does not
	// roll back.
	if err := ValidateHandle(in.Handle); err != nil {
		return Actor{}, Session{}, "", err
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		name = in.Handle
	}
	var owner Actor
	var seq, aseq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if owned, err := s.HasOwner(ctx, inv.WorkspaceID); err != nil {
			return err
		} else if owned {
			return ErrInviteInvalid
		}
		if _, err := s.Store.ActorByHandle(ctx, inv.WorkspaceID, in.Handle); err == nil {
			return fmt.Errorf("handle @%s is taken: %w", in.Handle, db.ErrConflict)
		}
		// The conditional update is the lock: of two acceptances at once,
		// one gets the invite.
		ok, err := s.Store.UseInvite(ctx, inv.ID, s.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return ErrInviteInvalid // used or revoked in the meantime
		}
		owner, _, aseq, err = s.createActor(ctx, ids.Nil, NewActorInput{WorkspaceID: inv.WorkspaceID, Kind: KindHuman,
			Handle: in.Handle, DisplayName: name, Role: RoleOwner, NoToken: true})
		if err != nil {
			return err
		}
		if err := s.ResetPassword(ctx, owner.ID, in.Password); err != nil {
			return err
		}
		if err := s.Store.SetInviteUsedBy(ctx, inv.ID, owner.ID); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), inv.WorkspaceID, EventInviteAccepted, owner.ID, ids.Nil, "workspace", inv.WorkspaceID,
			InviteEvent{InviteID: inv.ID.String(), ExpiresAt: inv.ExpiresAt, OwnerID: owner.ID.String()}, s.Clock.Now())
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
		return Actor{}, Session{}, "", err
	}
	s.Bus.Notify(max(seq, aseq))
	sess, cookie, err := s.openSession(ctx, owner, in.UserAgent)
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	return owner, sess, cookie, nil
}
