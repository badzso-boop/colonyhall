package identity

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Directory is the read-only view other modules may use to resolve actors.
type Directory interface {
	ActorByID(ctx context.Context, id ids.ID) (Actor, error)
	ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (Actor, error)
	ActorsByIDs(ctx context.Context, idList []ids.ID) ([]Actor, error)
}

// Authenticator turns a bearer token into an actor.
type Authenticator interface {
	Authenticate(ctx context.Context, secret string) (Actor, error)
}

var (
	_ Directory     = (*Service)(nil)
	_ Authenticator = (*Service)(nil)
)
