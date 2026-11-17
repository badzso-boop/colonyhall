// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory secrets store (tests, run-mem).
package memory

import (
	"context"
	"sync"
	"time"

	"colonyhall/internal/modules/secrets"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Store keeps secrets in memory.
type Store struct {
	mu   sync.Mutex
	rows map[ids.ID]secrets.Secret
}

// New returns an empty store.
func New() *Store { return &Store{rows: map[ids.ID]secrets.Secret{}} }

// Insert implements secrets.Store.
func (s *Store) Insert(_ context.Context, sec secrets.Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[sec.ID] = sec
	return nil
}

// Get implements secrets.Store.
func (s *Store) Get(_ context.Context, workspaceID, id ids.ID) (secrets.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.rows[id]
	if !ok || sec.WorkspaceID != workspaceID || sec.DeletedAt != nil {
		return secrets.Secret{}, db.ErrNotFound
	}
	return sec, nil
}

// Replace implements secrets.Store.
func (s *Store) Replace(_ context.Context, workspaceID, id ids.ID, sealed secretbox.Sealed, hint string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.rows[id]
	if !ok || sec.WorkspaceID != workspaceID || sec.DeletedAt != nil {
		return db.ErrNotFound
	}
	sec.Sealed, sec.Hint, sec.RotatedAt = sealed, hint, &at
	s.rows[id] = sec
	return nil
}

// Delete implements secrets.Store.
func (s *Store) Delete(_ context.Context, workspaceID, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.rows[id]
	if !ok || sec.WorkspaceID != workspaceID || sec.DeletedAt != nil {
		return db.ErrNotFound
	}
	sec.DeletedAt = &at
	s.rows[id] = sec
	return nil
}

// ListForRotation implements secrets.Store.
func (s *Store) ListForRotation(_ context.Context, activeKey string) ([]secrets.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []secrets.Secret
	for _, sec := range s.rows {
		if sec.DeletedAt == nil && sec.Sealed.KeyID != activeKey {
			out = append(out, sec)
		}
	}
	return out, nil
}

// Reseal implements secrets.Store.
func (s *Store) Reseal(_ context.Context, workspaceID, id ids.ID, sealed secretbox.Sealed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.rows[id]
	if !ok || sec.WorkspaceID != workspaceID || sec.DeletedAt != nil {
		return db.ErrNotFound
	}
	sec.Sealed = sealed
	s.rows[id] = sec
	return nil
}
