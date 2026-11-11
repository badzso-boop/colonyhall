package identity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/tenant"
)

// Service holds the identity use cases.
type Service struct {
	Store  Store
	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock
	// Logger receives the security log (round 45); nil: slog.Default.
	Logger *slog.Logger

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
func (s *Service) Bootstrap(ctx context.Context, workspaceName, slug, ownerHandle, ownerName, ownerSecret string) (Workspace, Actor, string, error) {
	ws := Workspace{ID: s.IDs.New(), Name: workspaceName, CreatedAt: s.Clock.Now(), State: WorkspaceActive}
	switch {
	case slug == "":
		ws.Slug = SlugFromName(workspaceName, ws.ID)
	case !ValidSlug(slug):
		return Workspace{}, Actor{}, "", ErrInvalidSlug
	default:
		ws.Slug = slug
	}
	var owner Actor
	var secret string
	var seq int64
	// Everything here belongs to the new workspace (round 45).
	err := s.Tx.InTx(db.WithWorkspace(ctx, ws.ID), func(ctx context.Context) error {
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
	ctx, err := s.credentialScope(ctx, credToken, s.Store.WorkspaceOfToken, HashSecret(secret))
	if err != nil {
		return Actor{}, err
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

// SetWorkspaceSlug gives the named workspace a new subdomain name (the
// set-slug command; no actor, it runs on the server).
func (s *Service) SetWorkspaceSlug(ctx context.Context, workspaceName, slug string) (Workspace, error) {
	if !ValidSlug(slug) {
		return Workspace{}, ErrInvalidSlug
	}
	ws, err := s.Store.WorkspaceByName(ctx, workspaceName)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.Store.SetWorkspaceSlug(ctx, ws.ID, slug); err != nil {
		return Workspace{}, err
	}
	ws.Slug = slug
	return ws, nil
}

// Credential kinds for credentialScope and EventCrossTenant.
const (
	credToken   = "token"
	credSession = "session"
)

// credentialScope finds the workspace a token or session belongs to and
// narrows ctx to it, before anything else is read (round 45): with
// row-level security, the credential's own rows are only visible there.
// In multi-tenant mode a credential of another workspace than the
// subdomain's is a leaked secret: it is revoked, recorded in its own
// workspace's audit log and logged as an error, and the caller gets the
// same answer as for an unknown credential.
func (s *Service) credentialScope(ctx context.Context, kind string, find func(context.Context, []byte) (ids.ID, error), hash []byte) (context.Context, error) {
	ws, err := find(ctx, hash)
	if errors.Is(err, db.ErrNotFound) {
		return ctx, ErrUnauthorized
	}
	if err != nil {
		return ctx, err
	}
	if t, ok := tenant.From(ctx); ok && t.Workspace != ws {
		s.crossTenant(db.WithWorkspace(ctx, ws), ws, t, kind, hash)
		return ctx, ErrUnauthorized
	}
	return db.WithWorkspace(ctx, ws), nil
}

// crossTenant revokes a credential used on another workspace's subdomain
// and records it; ctx is narrowed to the credential's workspace. Failures
// are logged: the request is refused either way.
func (s *Service) crossTenant(ctx context.Context, ws ids.ID, t tenant.Request, kind string, hash []byte) {
	now := s.Clock.Now()
	use := CrossTenantUse{Credential: kind, UsedOn: t.Slug, RemoteAddr: t.RemoteAddr}
	var actorID ids.ID
	var err error
	switch kind {
	case credToken:
		var tok Token
		if tok, err = s.Store.RevokeTokenByHash(ctx, hash, now); err == nil {
			actorID, use.Prefix = tok.ActorID, tok.Prefix
		}
	case credSession:
		var sess Session
		if sess, _, err = s.Store.SessionByTokenHash(ctx, hash); err == nil {
			actorID = sess.ActorID
			err = s.Store.RevokeSession(ctx, sess.ID, now)
		}
	}
	use.ActorID = actorID.String()
	s.logger().Error("SECURITY: cross-tenant credential use; the credential is revoked",
		"credential", kind, "prefix", use.Prefix, "actor_id", actorID, "credential_workspace", ws,
		"request_workspace", t.Workspace, "subdomain", t.Slug, "remote_addr", t.RemoteAddr, "user_agent", t.UserAgent)
	if err != nil {
		s.logger().Error("cross-tenant: revoking the credential failed", "credential", kind, "err", err)
	}
	by := Actor{ID: actorID, WorkspaceID: ws}
	if err := s.record(ctx, by, EventCrossTenant, "actor", actorID, use, func(context.Context) error { return nil }); err != nil {
		s.logger().Error("cross-tenant: recording the audit event failed", "err", err)
	}
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
