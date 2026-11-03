package identity

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// IssueHostToken mints a short-lived token for an agent that runs on a
// guard's host. The hosts module calls it after checking that the host is
// approved and the agent is assigned to it; the secret is returned once.
func (s *Service) IssueHostToken(ctx context.Context, agentID, hostID ids.ID, ttl time.Duration) (string, time.Time, error) {
	a, err := s.Store.ActorByID(ctx, agentID)
	if err != nil {
		return "", time.Time{}, err
	}
	if a.Kind != KindAgent || a.State == StateStopped {
		return "", time.Time{}, ErrForbidden
	}
	secret, err := NewSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.Clock.Now()
	exp := now.Add(ttl)
	t := Token{ID: s.IDs.New(), ActorID: a.ID, Hash: HashSecret(secret), Prefix: TokenPrefix(secret), Label: "host", CreatedAt: now, HostID: hostID, ExpiresAt: &exp}
	if err := s.Store.CreateToken(ctx, t); err != nil {
		return "", time.Time{}, err
	}
	return secret, exp, nil
}

// RevokeHostTokens revokes the tokens a host was given (for one agent, or
// all when agentID is ids.Nil).
func (s *Service) RevokeHostTokens(ctx context.Context, hostID, agentID ids.ID) error {
	return s.Store.RevokeHostTokens(ctx, hostID, agentID, s.Clock.Now())
}
