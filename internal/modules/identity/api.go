// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Directory is the read-only view other modules may use to resolve actors.
type Directory interface {
	ActorByID(ctx context.Context, id ids.ID) (Actor, error)
	ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (Actor, error)
	ActorsByIDs(ctx context.Context, idList []ids.ID) ([]Actor, error)
	Admins(ctx context.Context, workspaceID ids.ID) ([]Actor, error)
	WorkspaceByID(ctx context.Context, id ids.ID) (Workspace, error)
}

// Authenticator turns a bearer token into an actor.
type Authenticator interface {
	Authenticate(ctx context.Context, secret string) (Actor, error)
}

// RequestAuthenticator accepts both bearer tokens (agents, scripts) and
// browser session cookies (humans).
type RequestAuthenticator interface {
	Authenticator
	AuthenticateSession(ctx context.Context, secret string) (Actor, Session, error)
}

// HostTokens lets the hosts module mint and revoke the short-lived agent
// tokens guards request for their assigned agents.
type HostTokens interface {
	IssueHostToken(ctx context.Context, agentID, hostID ids.ID, ttl time.Duration) (string, time.Time, error)
	RevokeHostTokens(ctx context.Context, hostID, agentID ids.ID) error
}

var (
	_ HostTokens           = (*Service)(nil)
	_ Directory            = (*Service)(nil)
	_ RequestAuthenticator = (*Service)(nil)
)
