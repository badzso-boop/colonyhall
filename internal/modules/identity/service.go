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

	// HostColors maps assigned agents to their machine's colour, for
	// Actor.EffectiveColor (wired to the hosts module; nil: none).
	HostColors func(ctx context.Context, workspaceID ids.ID) (map[ids.ID]string, error)

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
	// Password, if set, is the person's initial password.
	Password string
	// Secret, if set, is used as the token instead of a random one.
	// Only the development seed uses this.
	Secret string
	// NoToken: no API token (an agent run by a guard gets short-lived
	// host-issued tokens instead).
	NoToken bool

	// Profile set at creation (round 33).
	Description string
	AvatarEmoji string
	Color       string
	// OwnerID is an agent's owner (round 38); the caller checked it with
	// ValidOwner.
	OwnerID ids.ID
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
	if in.Kind == KindAgent {
		// Agents are configured once, as a whole (POST /agents).
		return Actor{}, "", ErrAgentConfigImmutable
	}
	// Only the owner creates people.
	if by.Role != RoleOwner {
		return Actor{}, "", ErrForbidden
	}
	if in.Password != "" {
		if err := ValidatePassword(in.Password); err != nil {
			return Actor{}, "", err
		}
	}
	in.WorkspaceID = by.WorkspaceID
	var a Actor
	var secret string
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		a, secret, seq, err = s.createActor(ctx, by.ID, in)
		if err != nil || in.Password == "" {
			return err
		}
		return s.ResetPassword(ctx, a.ID, in.Password)
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
	switch in.Role {
	case RoleOwner, RoleAdmin, RoleMember, RoleObserver:
	default:
		// Without this the database check constraint caught it: a 500
		// instead of a validation error.
		return Actor{}, "", 0, fmt.Errorf("%w: %q", ErrInvalidRole, in.Role)
	}
	if in.DisplayName == "" {
		in.DisplayName = in.Handle
	}
	a := Actor{
		ID: s.IDs.New(), WorkspaceID: in.WorkspaceID, Kind: in.Kind, Handle: in.Handle,
		DisplayName: in.DisplayName, Role: in.Role, State: "active", CreatedAt: s.Clock.Now(),
		Description: in.Description, AvatarEmoji: in.AvatarEmoji, Color: in.Color,
	}
	if in.Kind == KindAgent {
		a.OwnerID = in.OwnerID
	}
	if err := s.Store.CreateActor(ctx, a); err != nil {
		if errors.Is(err, db.ErrConflict) {
			return Actor{}, "", 0, fmt.Errorf("handle @%s is taken: %w", in.Handle, err)
		}
		return Actor{}, "", 0, err
	}
	secret := in.Secret
	if !in.NoToken {
		if secret == "" {
			var err error
			if secret, err = NewSecret(); err != nil {
				return Actor{}, "", 0, err
			}
		}
		tok := Token{ID: s.IDs.New(), ActorID: a.ID, Hash: HashSecret(secret), Prefix: TokenPrefix(secret), Label: "default", CreatedAt: s.Clock.Now()}
		if err := s.Store.CreateToken(ctx, tok); err != nil {
			return Actor{}, "", 0, err
		}
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
	// The last use is shown on the Tokens page; losing one is harmless.
	now := s.Clock.Now()
	_ = s.Store.TouchToken(ctx, HashSecret(secret), now, now.Add(-touchEvery))
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
	list, err := s.Store.ListActors(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*Actor, len(list))
	for i := range list {
		ptrs[i] = &list[i]
	}
	return list, s.colour(ctx, by.WorkspaceID, ptrs)
}

// colour fills EffectiveColor: the actor's own colour, else its machine's.
func (s *Service) colour(ctx context.Context, workspaceID ids.ID, list []*Actor) error {
	var hostColors map[ids.ID]string
	if s.HostColors != nil {
		m, err := s.HostColors(ctx, workspaceID)
		if err != nil {
			return err
		}
		hostColors = m
	}
	for _, a := range list {
		a.EffectiveColor = a.Color
		if a.EffectiveColor == "" && a.Kind == KindAgent {
			a.EffectiveColor = hostColors[a.ID]
		}
	}
	return nil
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
