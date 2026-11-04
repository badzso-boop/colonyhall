package identity

import (
	"context"
	"errors"

	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/palette"
)

// Agent lifecycle (round 33). These run inside the agents module's
// transaction, which also writes the assignment, grants and memberships;
// the caller notifies the bus after its commit.

// HandleState says whether a handle can be given to a new agent.
type HandleState struct {
	// Taken by an actor that is not archived.
	Taken bool
	// Archived: an archived agent had it (the new one takes it over).
	Archived bool
}

// CheckAgentHandle validates a handle for a new agent.
func (s *Service) CheckAgentHandle(ctx context.Context, by Actor, handle string) (HandleState, error) {
	if by.Kind != KindHuman || !by.IsAdmin() {
		return HandleState{}, ErrForbidden
	}
	if err := ValidateHandle(handle); err != nil {
		return HandleState{}, err
	}
	var st HandleState
	if _, err := s.Store.ActorByHandle(ctx, by.WorkspaceID, handle); err == nil {
		st.Taken = true
	} else if !errors.Is(err, db.ErrNotFound) {
		return HandleState{}, err
	}
	all, err := s.Store.ListAllActors(ctx, by.WorkspaceID)
	if err != nil {
		return HandleState{}, err
	}
	for _, a := range all {
		if a.Handle == handle && a.ArchivedAt != nil {
			st.Archived = true
		}
	}
	return st, nil
}

// AgentActorInput is the profile of a new agent.
type AgentActorInput struct {
	Handle      string
	DisplayName string
	AvatarEmoji string
	Color       string
	Description string
	// WithToken: an API-key agent (no machine) gets a token, shown once.
	WithToken bool
	// Secret, if set, is the token (the development seed only).
	Secret string
	// OwnerID is the person the agent belongs to (round 38); ids.Nil: nobody's.
	OwnerID ids.ID
}

// ValidateAgentActor checks the profile before anything is written.
func ValidateAgentActor(in AgentActorInput) error {
	if err := ValidateHandle(in.Handle); err != nil {
		return err
	}
	if in.Color != "" && !palette.Valid(in.Color) {
		return ErrInvalidColor
	}
	if in.AvatarEmoji != "" && !ValidAvatarEmoji(in.AvatarEmoji) {
		return ErrInvalidAvatarEmoji
	}
	if len([]rune(in.Description)) > MaxDescriptionLen {
		return ErrInvalidDescription
	}
	return nil
}

// CreateAgentActor creates the actor of a new agent (human admins only;
// the agents module calls it inside its transaction). It returns the
// token secret for an API-key agent, else "".
func (s *Service) CreateAgentActor(ctx context.Context, by Actor, in AgentActorInput) (Actor, string, error) {
	if by.Kind != KindHuman || !by.IsAdmin() || by.Halted {
		return Actor{}, "", ErrForbidden
	}
	if err := ValidateAgentActor(in); err != nil {
		return Actor{}, "", err
	}
	a, secret, _, err := s.createActor(ctx, by.ID, NewActorInput{
		WorkspaceID: by.WorkspaceID, Kind: KindAgent, Handle: in.Handle, DisplayName: in.DisplayName, Role: RoleMember,
		NoToken: !in.WithToken, Secret: in.Secret, Description: in.Description, AvatarEmoji: in.AvatarEmoji, Color: in.Color,
		OwnerID: in.OwnerID,
	})
	if err != nil {
		return Actor{}, "", err
	}
	if !in.WithToken {
		secret = ""
	}
	return a, secret, nil
}

// ArchiveAgent stops an agent for good: it is archived, its tokens and
// sessions end. Its messages keep it as their author. Human admins only.
func (s *Service) ArchiveAgent(ctx context.Context, by Actor, agentID ids.ID) (Actor, error) {
	if by.Kind != KindHuman || !by.IsAdmin() {
		return Actor{}, ErrForbidden
	}
	a, err := s.Store.ActorByID(ctx, agentID)
	if err != nil {
		return Actor{}, err
	}
	if a.WorkspaceID != by.WorkspaceID {
		return Actor{}, db.ErrNotFound
	}
	if a.Kind != KindAgent {
		return Actor{}, ErrNotAnAgent
	}
	if a.ArchivedAt != nil {
		return a, nil
	}
	now := s.Clock.Now()
	err = s.record(ctx, by, EventActorArchived, "actor", a.ID, ActorArchived{ActorID: a.ID.String(), Handle: a.Handle},
		func(ctx context.Context) error {
			if err := s.Store.ArchiveActor(ctx, a.ID, by.ID, now); err != nil {
				return err
			}
			if err := s.Store.RevokeActorTokens(ctx, a.ID, now); err != nil {
				return err
			}
			return s.Store.RevokeSessionsForActor(ctx, a.ID, ids.Nil, now)
		})
	if err != nil {
		return Actor{}, err
	}
	a.ArchivedAt, a.ArchivedBy, a.State = &now, by.ID, StateStopped
	return a, nil
}

// ListActorsWithArchived lists the workspace's actors, archived ones too
// (for "archivált" badges and the agent directory's archive filter).
func (s *Service) ListActorsWithArchived(ctx context.Context, by Actor) ([]Actor, error) {
	list, err := s.Store.ListAllActors(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*Actor, len(list))
	for i := range list {
		ptrs[i] = &list[i]
	}
	return list, s.colour(ctx, by.WorkspaceID, ptrs)
}
