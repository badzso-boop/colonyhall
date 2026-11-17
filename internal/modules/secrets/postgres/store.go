// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres is the Postgres secrets store.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/secrets"
	"colonyhall/internal/modules/secrets/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Store implements secrets.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Insert implements secrets.Store.
func (s *Store) Insert(ctx context.Context, sec secrets.Secret) error {
	return s.q(ctx).InsertSecret(ctx, sqlcgen.InsertSecretParams{
		ID: sec.ID, WorkspaceID: sec.WorkspaceID, Kind: sec.Kind, Name: sec.Name, Provider: sec.Provider,
		KeyID: sec.Sealed.KeyID, Nonce: sec.Sealed.Nonce, Ciphertext: sec.Sealed.Ciphertext, Hint: sec.Hint,
		CreatedBy: sec.CreatedBy, CreatedAt: sec.CreatedAt,
	})
}

// Get implements secrets.Store.
func (s *Store) Get(ctx context.Context, workspaceID, id ids.ID) (secrets.Secret, error) {
	r, err := s.q(ctx).GetSecret(ctx, sqlcgen.GetSecretParams{WorkspaceID: workspaceID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return secrets.Secret{}, db.ErrNotFound
	}
	if err != nil {
		return secrets.Secret{}, err
	}
	return secrets.Secret{ID: r.ID, WorkspaceID: r.WorkspaceID, Kind: r.Kind, Name: r.Name, Provider: r.Provider,
		Sealed: secretbox.Sealed{KeyID: r.KeyID, Nonce: r.Nonce, Ciphertext: r.Ciphertext}, Hint: r.Hint,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, RotatedAt: r.RotatedAt, DeletedAt: r.DeletedAt}, nil
}

// Replace implements secrets.Store.
func (s *Store) Replace(ctx context.Context, workspaceID, id ids.ID, sealed secretbox.Sealed, hint string, at time.Time) error {
	n, err := s.q(ctx).ReplaceSecret(ctx, sqlcgen.ReplaceSecretParams{WorkspaceID: workspaceID, ID: id,
		KeyID: sealed.KeyID, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext, Hint: hint, RotatedAt: &at})
	return rows(n, err)
}

// Delete implements secrets.Store.
func (s *Store) Delete(ctx context.Context, workspaceID, id ids.ID, at time.Time) error {
	n, err := s.q(ctx).DeleteSecret(ctx, sqlcgen.DeleteSecretParams{WorkspaceID: workspaceID, ID: id, DeletedAt: &at})
	return rows(n, err)
}

// ListForRotation implements secrets.Store.
func (s *Store) ListForRotation(ctx context.Context, activeKey string) ([]secrets.Secret, error) {
	rs, err := s.q(ctx).ListSecretsForRotation(ctx, activeKey)
	if err != nil {
		return nil, err
	}
	out := make([]secrets.Secret, len(rs))
	for i, r := range rs {
		out[i] = secrets.Secret{ID: r.ID, WorkspaceID: r.WorkspaceID, Kind: r.Kind, Name: r.Name, Provider: r.Provider,
			Sealed: secretbox.Sealed{KeyID: r.KeyID, Nonce: r.Nonce, Ciphertext: r.Ciphertext}, Hint: r.Hint,
			CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, RotatedAt: r.RotatedAt}
	}
	return out, nil
}

// Reseal implements secrets.Store.
func (s *Store) Reseal(ctx context.Context, workspaceID, id ids.ID, sealed secretbox.Sealed) error {
	n, err := s.q(ctx).ResealSecret(ctx, sqlcgen.ResealSecretParams{WorkspaceID: workspaceID, ID: id,
		KeyID: sealed.KeyID, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
	return rows(n, err)
}

func rows(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return db.ErrNotFound
	}
	return nil
}
