// Package postgres implements identity.Store on Postgres.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements identity.Store.
type Store struct{ DB *db.Postgres }

const actorColumns = `id, workspace_id, kind, handle, display_name, role, state, created_at`

// CreateWorkspace inserts a workspace.
func (s *Store) CreateWorkspace(ctx context.Context, w identity.Workspace) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `INSERT INTO workspace (id, name, created_at) VALUES ($1, $2, $3)`,
		w.ID, w.Name, w.CreatedAt)
	return err
}

// CreateActor inserts an actor.
func (s *Store) CreateActor(ctx context.Context, a identity.Actor) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `INSERT INTO actor (`+actorColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		a.ID, a.WorkspaceID, a.Kind, a.Handle, a.DisplayName, a.Role, a.State, a.CreatedAt)
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// ActorByID loads one actor.
func (s *Store) ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error) {
	return scanActor(s.DB.Q(ctx).QueryRow(ctx, `SELECT `+actorColumns+` FROM actor WHERE id = $1`, id))
}

// ActorByHandle loads one actor by @handle.
func (s *Store) ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (identity.Actor, error) {
	return scanActor(s.DB.Q(ctx).QueryRow(ctx,
		`SELECT `+actorColumns+` FROM actor WHERE workspace_id = $1 AND handle = $2`, workspaceID, handle))
}

// ActorsByIDs loads several actors; unknown IDs are skipped.
func (s *Store) ActorsByIDs(ctx context.Context, idList []ids.ID) ([]identity.Actor, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `SELECT `+actorColumns+` FROM actor WHERE id = ANY($1) ORDER BY handle`, idList)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []identity.Actor
	for rows.Next() {
		a, err := scanActor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateToken stores a token hash.
func (s *Store) CreateToken(ctx context.Context, t identity.Token) error {
	_, err := s.DB.Q(ctx).Exec(ctx,
		`INSERT INTO api_token (id, actor_id, token_hash, label, created_at) VALUES ($1,$2,$3,$4,$5)`,
		t.ID, t.ActorID, t.Hash, t.Label, t.CreatedAt)
	return err
}

// ActorByTokenHash finds the actor owning a non-revoked token.
func (s *Store) ActorByTokenHash(ctx context.Context, hash []byte) (identity.Actor, error) {
	return scanActor(s.DB.Q(ctx).QueryRow(ctx, `
		SELECT a.id, a.workspace_id, a.kind, a.handle, a.display_name, a.role, a.state, a.created_at
		FROM api_token t JOIN actor a ON a.id = t.actor_id
		WHERE t.token_hash = $1 AND t.revoked_at IS NULL`, hash))
}

func scanActor(row pgx.Row) (identity.Actor, error) {
	var a identity.Actor
	err := row.Scan(&a.ID, &a.WorkspaceID, &a.Kind, &a.Handle, &a.DisplayName, &a.Role, &a.State, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Actor{}, db.ErrNotFound
	}
	return a, err
}

// ListActors lists every actor in the workspace, sorted by handle.
func (s *Store) ListActors(ctx context.Context, workspaceID ids.ID) ([]identity.Actor, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `SELECT `+actorColumns+` FROM actor WHERE workspace_id = $1 ORDER BY handle`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []identity.Actor
	for rows.Next() {
		a, err := scanActor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
