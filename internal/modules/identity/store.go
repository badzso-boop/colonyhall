package identity

import (
	"context"

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
}
