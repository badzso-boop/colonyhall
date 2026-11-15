// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists identity data. Implementations: postgres, memory.
// Lookups return db.ErrNotFound when nothing matches; CreateActor returns
// db.ErrConflict when an actor that is not archived has the handle. Lookups
// by handle see only actors that are not archived; by id, every actor.
type Store interface {
	CreateWorkspace(ctx context.Context, w Workspace) error
	CreateActor(ctx context.Context, a Actor) error
	ActorByID(ctx context.Context, id ids.ID) (Actor, error)
	ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (Actor, error)
	ActorsByIDs(ctx context.Context, idList []ids.ID) ([]Actor, error)
	// ListActors lists the workspace's actors that are not archived.
	ListActors(ctx context.Context, workspaceID ids.ID) ([]Actor, error)
	// ListAllActors lists archived actors too.
	ListAllActors(ctx context.Context, workspaceID ids.ID) ([]Actor, error)
	// ArchiveActor stops the actor and marks it archived (once).
	ArchiveActor(ctx context.Context, id, by ids.ID, at time.Time) error
	// RevokeActorTokens revokes every API token of the actor.
	RevokeActorTokens(ctx context.Context, actorID ids.ID, at time.Time) error
	CreateToken(ctx context.Context, t Token) error
	ListTokens(ctx context.Context, actorID ids.ID) ([]Token, error)
	TokenByID(ctx context.Context, id ids.ID) (Token, error)
	RevokeToken(ctx context.Context, id ids.ID, at time.Time) error
	// RevokeTokenByHash revokes the token with the hash (if not yet) and
	// returns it; db.ErrNotFound when there is none.
	RevokeTokenByHash(ctx context.Context, hash []byte, at time.Time) (Token, error)
	SetActorState(ctx context.Context, actorID ids.ID, state string) error
	UpdateActorProfile(ctx context.Context, actorID ids.ID, displayName, desc, avatarEmoji, tz, statusText, color string) error
	WorkspaceByID(ctx context.Context, id ids.ID) (Workspace, error)
	// SetEmergencyStop turns the stop on (at != nil) or off (at == nil).
	SetEmergencyStop(ctx context.Context, workspaceID ids.ID, at *time.Time, by ids.ID) error
	// ActorByTokenHash finds the actor of a token that is neither revoked
	// nor expired at now.
	ActorByTokenHash(ctx context.Context, hash []byte, now time.Time) (Actor, error)
	// TouchToken records a token's use unless it was recorded after notBefore.
	TouchToken(ctx context.Context, hash []byte, at, notBefore time.Time) error
	// TouchSession records a session's use unless it was recorded after notBefore.
	TouchSession(ctx context.Context, id ids.ID, at, notBefore time.Time) error
	// RevokeHostTokens revokes the host's agent tokens (one agent, or all
	// when agentID is ids.Nil).
	RevokeHostTokens(ctx context.Context, hostID, agentID ids.ID, at time.Time) error

	// ActorsByHandle finds the handle in every workspace (for login).
	ActorsByHandle(ctx context.Context, handle string) ([]Actor, error)
	WorkspaceByName(ctx context.Context, name string) (Workspace, error)
	// Lookups before the workspace is known (round 45). They see every
	// workspace but answer only "which workspace"; the caller then narrows
	// with db.WithWorkspace and runs the real query. db.ErrNotFound when
	// nothing matches.
	WorkspaceOfToken(ctx context.Context, hash []byte) (ids.ID, error)
	WorkspaceOfSession(ctx context.Context, hash []byte) (ids.ID, error)
	// HandleWorkspaces lists the workspaces (id and name) where the handle
	// belongs to an actor that is not archived.
	HandleWorkspaces(ctx context.Context, handle string) ([]Workspace, error)
	// WorkspaceBySlug finds a workspace by its subdomain name, in any scope.
	WorkspaceBySlug(ctx context.Context, slug string) (Workspace, error)
	// SetWorkspaceSlug renames a workspace's subdomain; db.ErrConflict when
	// another workspace has it, db.ErrNotFound when the workspace is gone.
	SetWorkspaceSlug(ctx context.Context, id ids.ID, slug string) error
	// SetWorkspaceState suspends (with the reason and time) or reactivates
	// a workspace.
	SetWorkspaceState(ctx context.Context, id ids.ID, state, reason string, at *time.Time) error

	// Round 51: the owner's one-time invites.
	CreateInvite(ctx context.Context, i Invite) error
	// RevokeOpenInvites revokes the workspace's unused, unrevoked invites.
	RevokeOpenInvites(ctx context.Context, workspaceID ids.ID, at time.Time) error
	InviteByTokenHash(ctx context.Context, hash []byte) (Invite, error)
	// UseInvite marks an open invite used; false when it was used,
	// revoked or expired at at.
	UseInvite(ctx context.Context, id ids.ID, at time.Time) (bool, error)
	SetInviteUsedBy(ctx context.Context, id, actorID ids.ID) error
	// LatestInvite is the workspace's newest invite (db.ErrNotFound: none).
	LatestInvite(ctx context.Context, workspaceID ids.ID) (Invite, error)

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
	// SessionStats counts each person's sessions alive at now, and their last login.
	SessionStats(ctx context.Context, workspaceID ids.ID, now time.Time) (map[ids.ID]SessionStat, error)
	// SetActorRole changes an actor's role.
	SetActorRole(ctx context.Context, id ids.ID, role Role) error
	// SetActorOwner sets (or, with ids.Nil, clears) an agent's owner.
	SetActorOwner(ctx context.Context, id, ownerID ids.ID) error

	// DeleteExpiredHostTokens deletes short-lived guard tokens (host_id set)
	// that expired or were revoked before the cut-off, in batches, and
	// returns how many went. Tokens without a host are never deleted.
	DeleteExpiredHostTokens(ctx context.Context, before time.Time) (int64, error)
	// DeleteEndedSessions deletes sessions that expired or were revoked
	// before the cut-off and returns how many went.
	DeleteEndedSessions(ctx context.Context, before time.Time) (int64, error)

	// Login and release limits (round 40). RecordAuthFailure,
	// AuthFailures and ClearAuthFailures run before anyone is
	// authenticated: they work in any scope (narrow functions in Postgres).
	RecordAuthFailure(ctx context.Context, f AuthFailure) error
	// AuthFailures returns the counted attempts after since, oldest first:
	// the handle's (not cleared by a success) and the address's (an empty
	// handle or address matches nothing).
	AuthFailures(ctx context.Context, scope, handle, ip string, since time.Time) (AuthFailureTimes, error)
	// ClearAuthFailures marks the handle's attempts in scope as cleared.
	ClearAuthFailures(ctx context.Context, scope, handle string, at time.Time) error
	// WorkspaceLockouts lists the workspace's lockouts after since, newest
	// first, at most 100.
	WorkspaceLockouts(ctx context.Context, workspaceID ids.ID, since time.Time) ([]Lockout, error)
	// UnattributedLockouts lists the lockouts of no workspace (unknown
	// handles), in any scope; single-tenant mode only.
	UnattributedLockouts(ctx context.Context, since time.Time) ([]Lockout, error)
	// DeleteAuthFailures deletes attempts before a time (system mode).
	DeleteAuthFailures(ctx context.Context, before time.Time) (int64, error)

	// RecordEmojiUse counts one use of an emoji by the actor at a time
	// (the last use is the later of the stored and the new one) and keeps
	// only the actor's keep most recently used distinct emoji.
	RecordEmojiUse(ctx context.Context, workspaceID, actorID ids.ID, emoji string, at time.Time, keep int) error
	// EmojiUsage returns the actor's emoji, at most limit each: recent by
	// last use, newest first; frequent by use count, ties newest first.
	EmojiUsage(ctx context.Context, actorID ids.ID, limit int) (recent, frequent []string, err error)
}
