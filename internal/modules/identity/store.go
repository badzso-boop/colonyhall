package identity

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists identity data. Implementations: postgres, memory.
// Lookups return db.ErrNotFound when nothing matches; CreateActor returns
// db.ErrConflict when the handle is taken.
type Store interface {
	CreateWorkspace(ctx context.Context, w Workspace) error
	CreateActor(ctx context.Context, a Actor) error
	ActorByID(ctx context.Context, id ids.ID) (Actor, error)
	ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (Actor, error)
	ActorsByIDs(ctx context.Context, idList []ids.ID) ([]Actor, error)
	ListActors(ctx context.Context, workspaceID ids.ID) ([]Actor, error)
	CreateToken(ctx context.Context, t Token) error
	ActorByTokenHash(ctx context.Context, hash []byte) (Actor, error)

	// ActorsByHandle finds the handle in every workspace (for login).
	ActorsByHandle(ctx context.Context, handle string) ([]Actor, error)
	WorkspaceByName(ctx context.Context, name string) (Workspace, error)

	// Credential returns the actor's password hash.
	Credential(ctx context.Context, actorID ids.ID) (string, error)
	SetCredential(ctx context.Context, actorID ids.ID, hash string, at time.Time) error

	CreateSession(ctx context.Context, s Session) error
	// SessionByTokenHash returns a non-revoked session and its actor;
	// the caller checks expiry.
	SessionByTokenHash(ctx context.Context, hash []byte) (Session, Actor, error)
	RevokeSession(ctx context.Context, id ids.ID, at time.Time) error
	// RevokeSessionsForActor revokes every session of the actor except keep
	// (ids.Nil keeps none).
	RevokeSessionsForActor(ctx context.Context, actorID, keep ids.ID, at time.Time) error
}
