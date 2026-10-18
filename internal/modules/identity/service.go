package identity

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Service holds the identity use cases.
type Service struct {
	Store  Store
	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock

	limiterOnce  sync.Once
	loginLimiter *loginLimiter
}

// NewActorInput describes an actor to create.
type NewActorInput struct {
	WorkspaceID ids.ID
	Kind        Kind
	Handle      string
	DisplayName string
	Role        Role
	// Secret, if set, is used as the token instead of a random one.
	// Only the development seed uses this.
	Secret string
}

// Bootstrap creates a workspace and its owner. It returns the owner's token.
func (s *Service) Bootstrap(ctx context.Context, workspaceName, ownerHandle, ownerName, ownerSecret string) (Workspace, Actor, string, error) {
	ws := Workspace{ID: s.IDs.New(), Name: workspaceName, CreatedAt: s.Clock.Now()}
	var owner Actor
	var secret string
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateWorkspace(ctx, ws); err != nil {
			return err
		}
		var err error
		owner, secret, seq, err = s.createActor(ctx, ids.Nil, NewActorInput{
			WorkspaceID: ws.ID, Kind: KindHuman, Handle: ownerHandle,
			DisplayName: ownerName, Role: RoleOwner, Secret: ownerSecret,
		})
		return err
	})
	if err != nil {
		return Workspace{}, Actor{}, "", err
	}
	s.Bus.Notify(seq)
	return ws, owner, secret, nil
}

// CreateActor creates a human or agent. Only human admins may do this, and
// only with a role below their own (an admin cannot mint another admin).
// It returns the new actor and its token secret (shown once).
func (s *Service) CreateActor(ctx context.Context, by Actor, in NewActorInput) (Actor, string, error) {
	if by.Kind != KindHuman || !by.IsAdmin() || by.Halted || by.Role.Rank() <= in.Role.Rank() {
		return Actor{}, "", ErrForbidden
	}
	in.WorkspaceID = by.WorkspaceID
	var a Actor
	var secret string
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		a, secret, seq, err = s.createActor(ctx, by.ID, in)
		return err
	})
	if err != nil {
		return Actor{}, "", err
	}
	s.Bus.Notify(seq)
	return a, secret, nil
}

func (s *Service) createActor(ctx context.Context, byID ids.ID, in NewActorInput) (Actor, string, int64, error) {
	if err := ValidateHandle(in.Handle); err != nil {
		return Actor{}, "", 0, err
	}
	if in.Kind != KindHuman && in.Kind != KindAgent {
		return Actor{}, "", 0, fmt.Errorf("%w: %q", ErrInvalidKind, in.Kind)
	}
	if in.DisplayName == "" {
		in.DisplayName = in.Handle
	}
	a := Actor{
		ID: s.IDs.New(), WorkspaceID: in.WorkspaceID, Kind: in.Kind, Handle: in.Handle,
		DisplayName: in.DisplayName, Role: in.Role, State: "active", CreatedAt: s.Clock.Now(),
	}
	if err := s.Store.CreateActor(ctx, a); err != nil {
		if errors.Is(err, db.ErrConflict) {
			return Actor{}, "", 0, fmt.Errorf("handle @%s is taken: %w", in.Handle, err)
		}
		return Actor{}, "", 0, err
	}
	secret := in.Secret
	if secret == "" {
		var err error
		if secret, err = NewSecret(); err != nil {
			return Actor{}, "", 0, err
		}
	}
	tok := Token{ID: s.IDs.New(), ActorID: a.ID, Hash: HashSecret(secret), Label: "default", CreatedAt: s.Clock.Now()}
	if err := s.Store.CreateToken(ctx, tok); err != nil {
		return Actor{}, "", 0, err
	}
	actorID := byID
	if actorID == ids.Nil {
		actorID = a.ID
	}
	ev, err := eventlog.NewEvent(s.IDs.New(), a.WorkspaceID, EventActorCreated, actorID, ids.Nil,
		"actor", a.ID, ActorCreated{ActorID: a.ID.String(), Kind: a.Kind, Handle: a.Handle, Role: a.Role}, s.Clock.Now())
	if err != nil {
		return Actor{}, "", 0, err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return Actor{}, "", 0, err
	}
	return a, secret, ev.Seq, nil
}

// EnsureSystemActor returns the actor with handle in workspaceID, or creates an active agent actor with no token.
func (s *Service) EnsureSystemActor(ctx context.Context, workspaceID ids.ID, handle, displayName string) (Actor, error) {
	a, err := s.Store.ActorByHandle(ctx, workspaceID, handle)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return Actor{}, err
	}
	if displayName == "" {
		displayName = handle
	}
	a = Actor{
		ID:          s.IDs.New(),
		WorkspaceID: workspaceID,
		Kind:        KindAgent,
		Handle:      handle,
		DisplayName: displayName,
		Role:        RoleMember,
		State:       StateActive,
		CreatedAt:   s.Clock.Now(),
	}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateActor(ctx, a); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return nil
			}
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), a.WorkspaceID, EventActorCreated, a.ID, ids.Nil,
			"actor", a.ID, ActorCreated{ActorID: a.ID.String(), Kind: a.Kind, Handle: a.Handle, Role: a.Role}, s.Clock.Now())
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
		return Actor{}, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return s.Store.ActorByHandle(ctx, workspaceID, handle)
}

// Authenticate resolves a bearer token to an active actor.
func (s *Service) Authenticate(ctx context.Context, secret string) (Actor, error) {
	if secret == "" {
		return Actor{}, ErrUnauthorized
	}
	a, err := s.Store.ActorByTokenHash(ctx, HashSecret(secret), s.Clock.Now())
	if errors.Is(err, db.ErrNotFound) {
		return Actor{}, ErrUnauthorized
	}
	if err != nil {
		return Actor{}, err
	}
	return s.load(ctx, a)
}

// ActorByID implements Directory.
func (s *Service) ActorByID(ctx context.Context, id ids.ID) (Actor, error) {
	return s.Store.ActorByID(ctx, id)
}

// ActorByHandle implements Directory.
func (s *Service) ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (Actor, error) {
	return s.Store.ActorByHandle(ctx, workspaceID, handle)
}

// ActorsByIDs implements Directory.
func (s *Service) ActorsByIDs(ctx context.Context, idList []ids.ID) ([]Actor, error) {
	return s.Store.ActorsByIDs(ctx, idList)
}

// ListActors lists the workspace's actors.
func (s *Service) ListActors(ctx context.Context, by Actor) ([]Actor, error) {
	return s.Store.ListActors(ctx, by.WorkspaceID)
}

// Admins implements Directory.
func (s *Service) Admins(ctx context.Context, workspaceID ids.ID) ([]Actor, error) {
	actors, err := s.Store.ListActors(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var admins []Actor
	for _, a := range actors {
		if a.IsAdmin() && !a.Halted {
			admins = append(admins, a)
		}
	}
	return admins, nil
}

// WorkspaceByID implements Directory.
func (s *Service) WorkspaceByID(ctx context.Context, id ids.ID) (Workspace, error) {
	return s.Store.WorkspaceByID(ctx, id)
}
