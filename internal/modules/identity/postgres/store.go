// Package postgres implements identity.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/identity/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements identity.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// CreateWorkspace inserts a workspace.
func (s *Store) CreateWorkspace(ctx context.Context, w identity.Workspace) error {
	return s.q(ctx).CreateWorkspace(ctx, sqlcgen.CreateWorkspaceParams{ID: w.ID, Name: w.Name, CreatedAt: w.CreatedAt})
}

// CreateActor inserts an actor.
func (s *Store) CreateActor(ctx context.Context, a identity.Actor) error {
	err := s.q(ctx).CreateActor(ctx, sqlcgen.CreateActorParams{
		ID: a.ID, WorkspaceID: a.WorkspaceID, Kind: string(a.Kind), Handle: a.Handle,
		DisplayName: a.DisplayName, Role: string(a.Role), State: a.State, CreatedAt: a.CreatedAt,
	})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// ActorByID loads one actor.
func (s *Store) ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error) {
	return one(s.q(ctx).ActorByID(ctx, id))
}

// ActorByHandle loads one actor by @handle.
func (s *Store) ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (identity.Actor, error) {
	return one(s.q(ctx).ActorByHandle(ctx, sqlcgen.ActorByHandleParams{WorkspaceID: workspaceID, Handle: handle}))
}

// ActorsByIDs loads several actors, sorted by handle; unknown IDs are skipped.
func (s *Store) ActorsByIDs(ctx context.Context, idList []ids.ID) ([]identity.Actor, error) {
	return many(s.q(ctx).ActorsByIDs(ctx, idList))
}

// ListActors lists every actor in the workspace, sorted by handle.
func (s *Store) ListActors(ctx context.Context, workspaceID ids.ID) ([]identity.Actor, error) {
	return many(s.q(ctx).ListActors(ctx, workspaceID))
}

// CreateToken stores a token hash.
func (s *Store) CreateToken(ctx context.Context, t identity.Token) error {
	return s.q(ctx).CreateToken(ctx, sqlcgen.CreateTokenParams{
		ID: t.ID, ActorID: t.ActorID, TokenHash: t.Hash, Label: t.Label, CreatedAt: t.CreatedAt,
	})
}

// ActorByTokenHash finds the actor owning a non-revoked token.
func (s *Store) ActorByTokenHash(ctx context.Context, hash []byte) (identity.Actor, error) {
	return one(s.q(ctx).ActorByTokenHash(ctx, hash))
}

func toActor(r sqlcgen.Actor) identity.Actor {
	return identity.Actor{ID: r.ID, WorkspaceID: r.WorkspaceID, Kind: identity.Kind(r.Kind), Handle: r.Handle,
		DisplayName: r.DisplayName, Role: identity.Role(r.Role), State: r.State, CreatedAt: r.CreatedAt}
}

func one(r sqlcgen.Actor, err error) (identity.Actor, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Actor{}, db.ErrNotFound
	}
	if err != nil {
		return identity.Actor{}, err
	}
	return toActor(r), nil
}

func many(rows []sqlcgen.Actor, err error) ([]identity.Actor, error) {
	if err != nil {
		return nil, err
	}
	out := make([]identity.Actor, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActor(r))
	}
	return out, nil
}

// ActorsByHandle finds the handle in every workspace.
func (s *Store) ActorsByHandle(ctx context.Context, handle string) ([]identity.Actor, error) {
	return many(s.q(ctx).ActorsByHandle(ctx, handle))
}

// WorkspaceByName loads a workspace.
func (s *Store) WorkspaceByName(ctx context.Context, name string) (identity.Workspace, error) {
	w, err := s.q(ctx).WorkspaceByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Workspace{}, db.ErrNotFound
	}
	return identity.Workspace{ID: w.ID, Name: w.Name, CreatedAt: w.CreatedAt}, err
}

// Credential returns the actor's password hash.
func (s *Store) Credential(ctx context.Context, actorID ids.ID) (string, error) {
	h, err := s.q(ctx).Credential(ctx, actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", db.ErrNotFound
	}
	return h, err
}

// SetCredential stores or replaces the actor's password hash.
func (s *Store) SetCredential(ctx context.Context, actorID ids.ID, hash string, at time.Time) error {
	return s.q(ctx).SetCredential(ctx, sqlcgen.SetCredentialParams{ActorID: actorID, PasswordHash: hash, UpdatedAt: at})
}

// CreateSession stores a session.
func (s *Store) CreateSession(ctx context.Context, x identity.Session) error {
	return s.q(ctx).CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: x.ID, ActorID: x.ActorID, TokenHash: x.TokenHash, CsrfToken: x.CSRFToken,
		UserAgent: x.UserAgent, CreatedAt: x.CreatedAt, ExpiresAt: x.ExpiresAt,
	})
}

// SessionByTokenHash returns a non-revoked session and its actor.
func (s *Store) SessionByTokenHash(ctx context.Context, hash []byte) (identity.Session, identity.Actor, error) {
	r, err := s.q(ctx).SessionByTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Session{}, identity.Actor{}, db.ErrNotFound
	}
	if err != nil {
		return identity.Session{}, identity.Actor{}, err
	}
	x := r.Session
	return identity.Session{ID: x.ID, ActorID: x.ActorID, TokenHash: x.TokenHash, CSRFToken: x.CsrfToken,
		UserAgent: x.UserAgent, CreatedAt: x.CreatedAt, ExpiresAt: x.ExpiresAt}, toActor(r.Actor), nil
}

// RevokeSession ends a session.
func (s *Store) RevokeSession(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).RevokeSession(ctx, sqlcgen.RevokeSessionParams{ID: id, RevokedAt: &at})
}

// RevokeSessionsForActor revokes every session of the actor except keep.
func (s *Store) RevokeSessionsForActor(ctx context.Context, actorID, keep ids.ID, at time.Time) error {
	return s.q(ctx).RevokeSessionsForActor(ctx, sqlcgen.RevokeSessionsForActorParams{At: at, ActorID: actorID, Keep: keep})
}
