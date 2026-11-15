// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/tenant"
)

// Break-glass access (round 51b, docs/24-hosztolt-ajanlat.md 6. pont): the
// platform operator's read-only look into one workspace, with a reason and
// for a limited time. It opens on the customer's own host through a
// one-time handoff token, as a session of the workspace's operator actor
// (a person without a password or tokens, admin rank so it sees what the
// owner sees). The request layer (httpx.RequireAuth) lets such a session
// read only the routes in BreakGlassRoutes, refuses every write, records
// every read in the workspace's audit and makes the database connection
// read-only; the conversation module hides DMs and private channels from
// it. Every other way in (WebSocket, MCP, tokens) refuses the session.

// Break-glass events. They name the access, the operator and the reason;
// a read names the method and the path (never a query string or content).
const (
	EventBreakGlassOpened  = "operator.break_glass_opened"
	EventBreakGlassStarted = "operator.break_glass_started"
	EventBreakGlassRead    = "operator.break_glass_read"
	EventBreakGlassClosed  = "operator.break_glass_closed"
)

// Who closed an access.
const (
	BreakGlassClosedByOperator = "operator"
	BreakGlassClosedByOwner    = "owner"
	BreakGlassClosedByExpiry   = "expired"
)

// Limits.
const (
	// BreakGlassHandoffTTL: the handoff link works this long, once.
	BreakGlassHandoffTTL = 60 * time.Second
	// BreakGlassActorHandle is the operator actor's handle (a number is
	// added when a customer already has it).
	BreakGlassActorHandle = "colonyhall-operator"
)

// BreakGlass is one access.
type BreakGlass struct {
	ID               ids.ID
	WorkspaceID      ids.ID
	OperatorID       ids.ID
	OperatorHandle   string
	ActorID          ids.ID
	Reason           string
	DevMode          bool
	OpenedAt         time.Time
	ExpiresAt        time.Time
	HandoffHash      []byte
	HandoffExpiresAt time.Time
	HandoffUsedAt    *time.Time
	ClosedAt         *time.Time
	ClosedBy         string
}

// Open reports whether the access is open at now.
func (b BreakGlass) Open(now time.Time) bool { return b.ClosedAt == nil && now.Before(b.ExpiresAt) }

// BreakGlassAccess marks an actor loaded from a break-glass session
// (computed, never stored).
type BreakGlassAccess struct {
	ID             ids.ID
	ExpiresAt      time.Time
	OperatorHandle string
}

// BreakGlassEvent is the payload of the break-glass events.
type BreakGlassEvent struct {
	BreakGlassID   string    `json:"break_glass_id"`
	OperatorHandle string    `json:"operator_handle,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitzero"`
	DevMode        bool      `json:"dev_mode,omitempty"`
	ClosedBy       string    `json:"closed_by,omitempty"`
	Method         string    `json:"method,omitempty"`
	Path           string    `json:"path,omitempty"`
}

var (
	// ErrBreakGlassInvalid: the handoff token is unknown, used, expired,
	// or for another host (one answer for all, 410).
	ErrBreakGlassInvalid = errors.New("this break-glass link is no longer valid")
	// ErrReadOnlyAccess: a break-glass session tried to write (403).
	ErrReadOnlyAccess = errors.New("the operator's break-glass access is read-only")
	// ErrBreakGlassScope: a break-glass session asked for something it may
	// not read (DMs, private channels, search, …; 403).
	ErrBreakGlassScope = errors.New("not available to the operator's break-glass access")
)

type breakGlassAllowedKey struct{}

// AllowBreakGlass marks ctx as the request layer that enforces the
// break-glass rules (httpx.RequireAuth); AuthenticateSession refuses a
// break-glass session anywhere else (WebSocket, MCP, …).
func AllowBreakGlass(ctx context.Context) context.Context {
	return context.WithValue(ctx, breakGlassAllowedKey{}, true)
}

func breakGlassAllowed(ctx context.Context) bool {
	v, _ := ctx.Value(breakGlassAllowedKey{}).(bool)
	return v
}

// OpenBreakGlassInput is an operator's request; ctx is scoped to the workspace.
type OpenBreakGlassInput struct {
	WorkspaceID    ids.ID
	OperatorID     ids.ID
	OperatorHandle string
	Reason         string
	ExpiresAt      time.Time
	DevMode        bool
}

// OpenBreakGlass opens an access and returns it with the handoff token
// (shown once). The workspace's audit records it, and its owners are
// notified (the activity module).
func (s *Service) OpenBreakGlass(ctx context.Context, in OpenBreakGlassInput) (BreakGlass, string, error) {
	ctx = db.WithWorkspace(ctx, in.WorkspaceID)
	ws, err := s.Store.WorkspaceByID(ctx, in.WorkspaceID)
	if err != nil {
		return BreakGlass{}, "", err
	}
	if ws.State != WorkspaceActive {
		return BreakGlass{}, "", ErrWorkspaceNotActive
	}
	token, err := NewSecret()
	if err != nil {
		return BreakGlass{}, "", err
	}
	now := s.Clock.Now()
	b := BreakGlass{ID: s.IDs.New(), WorkspaceID: in.WorkspaceID, OperatorID: in.OperatorID, OperatorHandle: in.OperatorHandle,
		Reason: strings.TrimSpace(in.Reason), DevMode: in.DevMode, OpenedAt: now, ExpiresAt: in.ExpiresAt,
		HandoffHash: HashSecret(token), HandoffExpiresAt: now.Add(BreakGlassHandoffTTL)}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		actor, aseq, err := s.breakGlassActor(ctx, in.WorkspaceID)
		if err != nil {
			return err
		}
		b.ActorID = actor.ID
		if err := s.Store.CreateBreakGlass(ctx, b); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), in.WorkspaceID, EventBreakGlassOpened, actor.ID, ids.Nil, "break_glass", b.ID,
			BreakGlassEvent{BreakGlassID: b.ID.String(), OperatorHandle: b.OperatorHandle, Reason: b.Reason, ExpiresAt: b.ExpiresAt, DevMode: b.DevMode}, now)
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = max(ev.Seq, aseq)
		return nil
	})
	if err != nil {
		return BreakGlass{}, "", err
	}
	s.Bus.Notify(seq)
	return b, token, nil
}

// breakGlassActor returns the workspace's operator actor, creating it on
// the first access (inside the caller's transaction; the second value is
// the creation event's seq): a person (admin rank, so it sees what the
// owner sees, within the break-glass rules) with no password and no token.
// The access rows remember it, so a customer's own "colonyhall-operator"
// is never taken for it.
func (s *Service) breakGlassActor(ctx context.Context, workspaceID ids.ID) (Actor, int64, error) {
	if id, err := s.Store.BreakGlassActor(ctx, workspaceID); err == nil {
		a, err := s.Store.ActorByID(ctx, id)
		return a, 0, err
	} else if !errors.Is(err, db.ErrNotFound) {
		return Actor{}, 0, err
	}
	for i := range 10 {
		handle := BreakGlassActorHandle
		if i > 0 {
			handle = fmt.Sprintf("%s-%d", BreakGlassActorHandle, i+1)
		}
		// Checked first: a unique violation would abort the transaction.
		if _, err := s.Store.ActorByHandle(ctx, workspaceID, handle); err == nil {
			continue
		} else if !errors.Is(err, db.ErrNotFound) {
			return Actor{}, 0, err
		}
		a, _, seq, err := s.createActor(ctx, ids.Nil, NewActorInput{WorkspaceID: workspaceID, Kind: KindHuman, Handle: handle,
			DisplayName: "Üzemeltető (vészhozzáférés)", Role: RoleAdmin, NoToken: true})
		return a, seq, err
	}
	return Actor{}, 0, fmt.Errorf("no free handle for the operator actor: %w", db.ErrConflict)
}

// RedeemBreakGlass uses a handoff token on the workspace's own host and
// opens the read-only session (until the access expires). It returns the
// actor, the session and the cookie secret.
func (s *Service) RedeemBreakGlass(ctx context.Context, token, userAgent string) (Actor, Session, string, error) {
	t, ok := tenant.From(ctx)
	if !ok || token == "" {
		return Actor{}, Session{}, "", ErrBreakGlassInvalid
	}
	now := s.Clock.Now()
	b, err := s.Store.BreakGlassByHandoffHash(ctx, HashSecret(token))
	if errors.Is(err, db.ErrNotFound) || (err == nil && (b.WorkspaceID != t.Workspace || !b.Open(now) ||
		b.HandoffUsedAt != nil || !now.Before(b.HandoffExpiresAt))) {
		return Actor{}, Session{}, "", ErrBreakGlassInvalid
	}
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	a, err := s.Store.ActorByID(ctx, b.ActorID)
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	secret, err := NewSecret()
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	csrf, err := NewSecret()
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	sess := Session{ID: s.IDs.New(), ActorID: a.ID, TokenHash: HashSecret(secret), CSRFToken: csrf,
		UserAgent: truncate(userAgent, 256), CreatedAt: now, ExpiresAt: b.ExpiresAt, BreakGlassID: b.ID}
	err = s.record(ctx, a, EventBreakGlassStarted, "break_glass", b.ID, BreakGlassEvent{BreakGlassID: b.ID.String(), OperatorHandle: b.OperatorHandle},
		func(ctx context.Context) error {
			// The conditional update is the lock: the link works once.
			used, err := s.Store.UseBreakGlassHandoff(ctx, b.ID, now)
			if err != nil {
				return err
			}
			if !used {
				return ErrBreakGlassInvalid
			}
			return s.Store.CreateSession(ctx, sess)
		})
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	a.BreakGlass = &BreakGlassAccess{ID: b.ID, ExpiresAt: b.ExpiresAt, OperatorHandle: b.OperatorHandle}
	return a, sess, secret, nil
}

// breakGlassOf checks a break-glass session's access: open, and the
// request layer that enforces the rules. ErrUnauthorized otherwise.
func (s *Service) breakGlassOf(ctx context.Context, sess Session) (*BreakGlassAccess, error) {
	if !breakGlassAllowed(ctx) {
		return nil, ErrUnauthorized
	}
	b, err := s.Store.BreakGlassByID(ctx, sess.BreakGlassID)
	if errors.Is(err, db.ErrNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if !b.Open(s.Clock.Now()) {
		return nil, ErrUnauthorized
	}
	return &BreakGlassAccess{ID: b.ID, ExpiresAt: b.ExpiresAt, OperatorHandle: b.OperatorHandle}, nil
}

// RecordBreakGlassRead writes a break-glass read into the workspace's
// audit (the method and the path; no query string, no content). The
// request is refused when it cannot be recorded.
func (s *Service) RecordBreakGlassRead(ctx context.Context, a Actor, method, path string) error {
	if a.BreakGlass == nil {
		return nil
	}
	return s.record(ctx, a, EventBreakGlassRead, "break_glass", a.BreakGlass.ID,
		BreakGlassEvent{BreakGlassID: a.BreakGlass.ID.String(), Method: method, Path: truncate(path, 512)},
		func(context.Context) error { return nil })
}

// ListBreakGlass lists the workspace's accesses, newest first (owner and
// admins; never the operator actor itself).
func (s *Service) ListBreakGlass(ctx context.Context, by Actor) ([]BreakGlass, error) {
	if by.Kind != KindHuman || !by.IsAdmin() || by.BreakGlass != nil {
		return nil, ErrForbidden
	}
	return s.Store.ListBreakGlass(ctx, by.WorkspaceID, 50)
}

// ListBreakGlassFor lists a workspace's accesses for the operator.
func (s *Service) ListBreakGlassFor(ctx context.Context, workspaceID ids.ID) ([]BreakGlass, error) {
	return s.Store.ListBreakGlass(db.WithWorkspace(ctx, workspaceID), workspaceID, 50)
}

// CloseBreakGlass closes an access at once: the owner (by != nil), or the
// operator (by == nil; ctx is then narrowed to the workspace).
func (s *Service) CloseBreakGlass(ctx context.Context, by *Actor, workspaceID, id ids.ID) error {
	closedBy := BreakGlassClosedByOperator
	var who Actor
	if by != nil {
		if by.Kind != KindHuman || by.Role != RoleOwner || by.BreakGlass != nil {
			return ErrForbidden
		}
		closedBy, who, workspaceID = BreakGlassClosedByOwner, *by, by.WorkspaceID
	}
	ctx = db.WithWorkspace(ctx, workspaceID)
	b, err := s.Store.BreakGlassByID(ctx, id)
	if err != nil {
		return err
	}
	if b.WorkspaceID != workspaceID {
		return db.ErrNotFound
	}
	if by == nil {
		who = Actor{ID: b.ActorID, WorkspaceID: workspaceID}
	}
	return s.closeBreakGlass(ctx, who, b, closedBy)
}

func (s *Service) closeBreakGlass(ctx context.Context, who Actor, b BreakGlass, closedBy string) error {
	now := s.Clock.Now()
	var closed bool
	err := s.record(ctx, who, EventBreakGlassClosed, "break_glass", b.ID, BreakGlassEvent{BreakGlassID: b.ID.String(), ClosedBy: closedBy},
		func(ctx context.Context) error {
			var err error
			closed, err = s.Store.CloseBreakGlass(ctx, b.ID, closedBy, now)
			if err == nil && !closed {
				return errAlreadyClosed
			}
			return err
		})
	if errors.Is(err, errAlreadyClosed) {
		return nil // closed already: nothing to record
	}
	return err
}

var errAlreadyClosed = errors.New("break-glass access already closed")

// ExpireBreakGlass closes the accesses whose time is up (the maintenance
// loop; ctx in system mode, each closed in its own workspace's scope).
func (s *Service) ExpireBreakGlass(ctx context.Context) (int, error) {
	list, err := s.Store.OverdueBreakGlass(ctx, s.Clock.Now())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range list {
		wctx := db.WithWorkspace(ctx, b.WorkspaceID)
		if err := s.closeBreakGlass(wctx, Actor{ID: b.ActorID, WorkspaceID: b.WorkspaceID}, b, BreakGlassClosedByExpiry); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// IsBreakGlassActor reports whether the actor is a workspace's operator
// actor (it never logs in with a password and gets no password).
func (s *Service) IsBreakGlassActor(ctx context.Context, actorID ids.ID) (bool, error) {
	return s.Store.IsBreakGlassActor(ctx, actorID)
}

// ErrWorkspaceNotActive: a suspended workspace cannot be opened.
var ErrWorkspaceNotActive = errors.New("the workspace is not active")

// BreakGlassRoutes are the only routes a break-glass session may call
// (round 51b; the pattern as registered, so a new route is closed until it
// is added here). They show what the decision allows: the public channels
// and their threads, files and pins (the conversation module hides DMs and
// private channels), the people and agents, machines, runs (without their
// steps), usage, settings and the storage; and what the app needs to start
// (the session, the caller's own empty lists). Search, the audit, errors,
// the wiki, the planner, schedules, questions, approvals, sockets and MCP
// are not here. The one write is the logout.
var BreakGlassRoutes = map[string]bool{
	"GET /api/v1/auth/session":                  true,
	"POST /api/v1/auth/logout":                  true,
	"GET /api/v1/version":                       true,
	"GET /api/v1/me":                            true,
	"GET /api/v1/me/activity":                   true,
	"GET /api/v1/me/threads":                    true,
	"GET /api/v1/me/drafts":                     true,
	"GET /api/v1/me/saved":                      true,
	"GET /api/v1/me/emoji":                      true,
	"GET /api/v1/me/files":                      true,
	"GET /api/v1/sidebar/sections":              true,
	"GET /api/v1/actors":                        true,
	"GET /api/v1/admin/status":                  true,
	"GET /api/v1/admin/users":                   true,
	"GET /api/v1/channels":                      true,
	"GET /api/v1/channels/{id}":                 true,
	"GET /api/v1/channels/{id}/members":         true,
	"GET /api/v1/channels/{id}/messages":        true,
	"GET /api/v1/channels/{id}/pins":            true,
	"GET /api/v1/messages/{id}/thread":          true,
	"GET /api/v1/files/{fileId}":                true,
	"GET /api/v1/files/{fileId}/content":        true,
	"GET /api/v1/hosts":                         true,
	"GET /api/v1/runs":                          true,
	"GET /api/v1/usage":                         true,
	"GET /api/v1/agents/{agentId}/instructions": true,
	"GET /api/v1/agent-rules":                   true,
	"GET /api/v1/agent-rules/{version}":         true,
	"GET /api/v1/settings/prices":               true,
	"GET /api/v1/settings/currency":             true,
	"GET /api/v1/settings/compaction":           true,
	"GET /api/v1/settings/file-fetch":           true,
	"GET /api/v1/workspace/storage":             true,
}

// refuseBreakGlassActor: the operator actor gets no credential and no other
// rank (round 51b): only its break-glass sessions, under their rules.
func (s *Service) refuseBreakGlassActor(ctx context.Context, actorID ids.ID) error {
	bg, err := s.Store.IsBreakGlassActor(ctx, actorID)
	if err != nil {
		return err
	}
	if bg {
		return ErrForbidden
	}
	return nil
}
