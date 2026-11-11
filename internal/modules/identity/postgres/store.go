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
	w = identity.WithDefaults(w)
	err := s.q(ctx).CreateWorkspace(ctx, sqlcgen.CreateWorkspaceParams{ID: w.ID, Name: w.Name, CreatedAt: w.CreatedAt, Slug: w.Slug, State: w.State})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// CreateActor inserts an actor.
func (s *Store) CreateActor(ctx context.Context, a identity.Actor) error {
	tz := a.Timezone
	if tz == "" {
		tz = identity.DefaultTimezone
	}
	err := s.q(ctx).CreateActor(ctx, sqlcgen.CreateActorParams{
		ID:          a.ID,
		WorkspaceID: a.WorkspaceID,
		Kind:        string(a.Kind),
		Handle:      a.Handle,
		DisplayName: a.DisplayName,
		Role:        string(a.Role),
		State:       a.State,
		CreatedAt:   a.CreatedAt,
		Description: a.Description,
		AvatarEmoji: a.AvatarEmoji,
		Timezone:    tz,
		StatusText:  a.StatusText,
		Color:       nullString(a.Color),
		OwnerID:     nullID(a.OwnerID),
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
		HostID: nullID(t.HostID), ExpiresAt: t.ExpiresAt, Prefix: strOrNil(t.Prefix),
	})
}

// TouchToken records a token's use unless it was recorded after notBefore.
func (s *Store) TouchToken(ctx context.Context, hash []byte, at, notBefore time.Time) error {
	return s.q(ctx).TouchToken(ctx, sqlcgen.TouchTokenParams{TokenHash: hash, At: &at, NotBefore: &notBefore})
}

// TouchSession records a session's use unless it was recorded after notBefore.
func (s *Store) TouchSession(ctx context.Context, id ids.ID, at, notBefore time.Time) error {
	return s.q(ctx).TouchSession(ctx, sqlcgen.TouchSessionParams{ID: id, At: &at, NotBefore: &notBefore})
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ActorByTokenHash finds the actor owning a non-revoked token.
func (s *Store) ActorByTokenHash(ctx context.Context, hash []byte, now time.Time) (identity.Actor, error) {
	return one(s.q(ctx).ActorByTokenHash(ctx, sqlcgen.ActorByTokenHashParams{TokenHash: hash, Now: now}))
}

// RevokeHostTokens revokes a host's agent tokens.
func (s *Store) RevokeHostTokens(ctx context.Context, hostID, agentID ids.ID, at time.Time) error {
	return s.q(ctx).RevokeHostTokens(ctx, sqlcgen.RevokeHostTokensParams{At: &at, HostID: &hostID, AgentID: nullID(agentID)})
}

func toActor(r sqlcgen.Actor) identity.Actor {
	return identity.Actor{
		ID:          r.ID,
		WorkspaceID: r.WorkspaceID,
		Kind:        identity.Kind(r.Kind),
		Handle:      r.Handle,
		DisplayName: r.DisplayName,
		Role:        identity.Role(r.Role),
		State:       r.State,
		CreatedAt:   r.CreatedAt,
		Description: r.Description,
		AvatarEmoji: r.AvatarEmoji,
		Timezone:    r.Timezone,
		StatusText:  r.StatusText,
		Color:       deref(r.Color),
		ArchivedAt:  r.ArchivedAt,
		ArchivedBy:  derefID(r.ArchivedBy),
		OwnerID:     derefID(r.OwnerID),
	}
}

func derefID(id *ids.ID) ids.ID {
	if id == nil {
		return ids.Nil
	}
	return *id
}

// ListAllActors lists archived actors too.
func (s *Store) ListAllActors(ctx context.Context, workspaceID ids.ID) ([]identity.Actor, error) {
	rows, err := s.q(ctx).ListAllActors(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]identity.Actor, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActor(r))
	}
	return out, nil
}

// ArchiveActor stops and archives the actor.
func (s *Store) ArchiveActor(ctx context.Context, id, by ids.ID, at time.Time) error {
	return s.q(ctx).ArchiveActor(ctx, sqlcgen.ArchiveActorParams{ID: id, By: &by, At: &at})
}

// RevokeActorTokens revokes every API token of the actor.
func (s *Store) RevokeActorTokens(ctx context.Context, actorID ids.ID, at time.Time) error {
	return s.q(ctx).RevokeActorTokens(ctx, sqlcgen.RevokeActorTokensParams{ActorID: actorID, At: &at})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
	return toWorkspace(w), err
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
		UserAgent: x.UserAgent, CreatedAt: x.CreatedAt, ExpiresAt: x.ExpiresAt, LastSeenAt: x.LastSeenAt}, toActor(r.Actor), nil
}

// RevokeSession ends a session.
func (s *Store) RevokeSession(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).RevokeSession(ctx, sqlcgen.RevokeSessionParams{ID: id, RevokedAt: &at})
}

// RevokeSessionsForActor revokes every session of the actor except keep.
func (s *Store) RevokeSessionsForActor(ctx context.Context, actorID, keep ids.ID, at time.Time) error {
	return s.q(ctx).RevokeSessionsForActor(ctx, sqlcgen.RevokeSessionsForActorParams{At: at, ActorID: actorID, Keep: keep})
}

func toWorkspace(w sqlcgen.Workspace) identity.Workspace {
	out := identity.Workspace{ID: w.ID, Name: w.Name, CreatedAt: w.CreatedAt, Slug: w.Slug, State: w.State, EmergencyStopAt: w.EmergencyStopAt}
	if w.EmergencyStopBy != nil {
		out.EmergencyStopBy = *w.EmergencyStopBy
	}
	return out
}

func toToken(t sqlcgen.ApiToken) identity.Token {
	out := identity.Token{ID: t.ID, ActorID: t.ActorID, Hash: t.TokenHash, Label: t.Label,
		CreatedAt: t.CreatedAt, RevokedAt: t.RevokedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt}
	if t.Prefix != nil {
		out.Prefix = *t.Prefix
	}
	if t.HostID != nil {
		out.HostID = *t.HostID
	}
	return out
}

func nullID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}

// ListTokens lists an actor's tokens, oldest first.
func (s *Store) ListTokens(ctx context.Context, actorID ids.ID) ([]identity.Token, error) {
	rows, err := s.q(ctx).ListTokens(ctx, actorID)
	if err != nil {
		return nil, err
	}
	out := make([]identity.Token, 0, len(rows))
	for _, r := range rows {
		out = append(out, toToken(r))
	}
	return out, nil
}

// TokenByID loads a token.
func (s *Store) TokenByID(ctx context.Context, id ids.ID) (identity.Token, error) {
	t, err := s.q(ctx).TokenByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Token{}, db.ErrNotFound
	}
	return toToken(t), err
}

// RevokeToken revokes a token.
func (s *Store) RevokeToken(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).RevokeToken(ctx, sqlcgen.RevokeTokenParams{ID: id, RevokedAt: &at})
}

// SetActorState changes an actor's state.
func (s *Store) SetActorState(ctx context.Context, actorID ids.ID, state string) error {
	return s.q(ctx).SetActorState(ctx, sqlcgen.SetActorStateParams{ID: actorID, State: state})
}

// UpdateActorProfile updates an actor's profile fields.
func (s *Store) UpdateActorProfile(ctx context.Context, actorID ids.ID, displayName, desc, avatarEmoji, tz, statusText, color string) error {
	return s.q(ctx).UpdateActorProfile(ctx, sqlcgen.UpdateActorProfileParams{
		ID:          actorID,
		Description: desc,
		AvatarEmoji: avatarEmoji,
		Timezone:    tz,
		StatusText:  statusText,
		DisplayName: displayName,
		Color:       nullString(color),
	})
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// WorkspaceByID loads a workspace.
func (s *Store) WorkspaceByID(ctx context.Context, id ids.ID) (identity.Workspace, error) {
	w, err := s.q(ctx).WorkspaceByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Workspace{}, db.ErrNotFound
	}
	return toWorkspace(w), err
}

// SetEmergencyStop turns the emergency stop on or off.
func (s *Store) SetEmergencyStop(ctx context.Context, workspaceID ids.ID, at *time.Time, by ids.ID) error {
	var byPtr *ids.ID
	if at != nil {
		byPtr = &by
	}
	return s.q(ctx).SetEmergencyStop(ctx, sqlcgen.SetEmergencyStopParams{At: at, By: byPtr, ID: workspaceID})
}

// SessionStats counts each person's sessions alive at now, and their last login.
func (s *Store) SessionStats(ctx context.Context, workspaceID ids.ID, now time.Time) (map[ids.ID]identity.SessionStat, error) {
	rows, err := s.q(ctx).SessionStats(ctx, sqlcgen.SessionStatsParams{WorkspaceID: workspaceID, NowAt: now})
	if err != nil {
		return nil, err
	}
	out := make(map[ids.ID]identity.SessionStat, len(rows))
	for _, r := range rows {
		last, seen := r.LastLogin, r.LastSeen
		out[r.ActorID] = identity.SessionStat{Active: int(r.Active), LastLogin: &last, LastSeen: &seen}
	}
	return out, nil
}

// SetActorRole changes an actor's role.
func (s *Store) SetActorRole(ctx context.Context, id ids.ID, role identity.Role) error {
	return s.q(ctx).SetActorRole(ctx, sqlcgen.SetActorRoleParams{ID: id, Role: string(role)})
}

// SetActorOwner sets or clears an agent's owner.
func (s *Store) SetActorOwner(ctx context.Context, id, ownerID ids.ID) error {
	return s.q(ctx).SetActorOwner(ctx, sqlcgen.SetActorOwnerParams{ID: id, OwnerID: nullID(ownerID)})
}

// deleteBatch is how many rows one cleanup statement removes, so a first
// sweep over thousands of rows never holds its locks for long.
const deleteBatch = 1000

// DeleteExpiredHostTokens deletes ended guard tokens, one batch per statement.
func (s *Store) DeleteExpiredHostTokens(ctx context.Context, before time.Time) (int64, error) {
	return deleteInBatches(func() (int64, error) {
		return s.q(ctx).DeleteExpiredHostTokens(ctx, sqlcgen.DeleteExpiredHostTokensParams{Before: before, Batch: deleteBatch})
	})
}

// DeleteEndedSessions deletes ended sessions, one batch per statement.
func (s *Store) DeleteEndedSessions(ctx context.Context, before time.Time) (int64, error) {
	return deleteInBatches(func() (int64, error) {
		return s.q(ctx).DeleteEndedSessions(ctx, sqlcgen.DeleteEndedSessionsParams{Before: before, Batch: deleteBatch})
	})
}

func deleteInBatches(step func() (int64, error)) (int64, error) {
	var total int64
	for {
		n, err := step()
		total += n
		if err != nil || n < deleteBatch {
			return total, err
		}
	}
}

// WorkspaceBySlug loads a workspace by its subdomain name.
func (s *Store) WorkspaceBySlug(ctx context.Context, slug string) (identity.Workspace, error) {
	w, err := s.q(ctx).WorkspaceBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Workspace{}, db.ErrNotFound
	}
	return toWorkspace(w), err
}

// SetWorkspaceSlug renames a workspace's subdomain.
func (s *Store) SetWorkspaceSlug(ctx context.Context, id ids.ID, slug string) error {
	n, err := s.q(ctx).SetWorkspaceSlug(ctx, sqlcgen.SetWorkspaceSlugParams{ID: id, Slug: slug})
	switch {
	case db.IsUniqueViolation(err):
		return db.ErrConflict
	case err != nil:
		return err
	case n == 0:
		return db.ErrNotFound
	}
	return nil
}
