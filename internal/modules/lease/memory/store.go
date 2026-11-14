// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package memory

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/lease"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements an in-memory lease.Store.
type Store struct {
	mu     sync.RWMutex
	leases []lease.Lease
}

var _ lease.Store = (*Store)(nil)

// New creates a new in-memory lease store.
func New() *Store {
	return &Store{}
}

func (s *Store) LockResource(_ context.Context, _ ids.ID, _ string) error {
	return nil
}

func (s *Store) GetActiveByResource(_ context.Context, workspaceID ids.ID, resource string, now time.Time) (lease.Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.leases) - 1; i >= 0; i-- {
		l := s.leases[i]
		if l.WorkspaceID == workspaceID && l.Resource == resource && l.IsActive(now) {
			return l, nil
		}
	}
	return lease.Lease{}, db.ErrNotFound
}

func (s *Store) GetByID(_ context.Context, id ids.ID) (lease.Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.leases {
		if l.ID == id {
			return l, nil
		}
	}
	return lease.Lease{}, db.ErrNotFound
}

func (s *Store) Create(_ context.Context, l lease.Lease) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases = append(s.leases, l)
	return nil
}

func (s *Store) Extend(_ context.Context, id ids.ID, newExpiresAt time.Time, note *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, l := range s.leases {
		if l.ID == id && l.ReleasedAt == nil {
			s.leases[i].ExpiresAt = newExpiresAt
			if note != nil {
				s.leases[i].Note = *note
			}
			return nil
		}
	}
	return db.ErrNotFound
}

func (s *Store) Release(_ context.Context, id ids.ID, releasedBy ids.ID, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, l := range s.leases {
		if l.ID == id && l.ReleasedAt == nil {
			s.leases[i].ReleasedAt = &now
			if releasedBy != ids.Nil {
				s.leases[i].ReleasedBy = &releasedBy
			}
			return nil
		}
	}
	return db.ErrNotFound
}

func (s *Store) ListActive(_ context.Context, workspaceID ids.ID, now time.Time) ([]lease.Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var res []lease.Lease
	for _, l := range s.leases {
		if l.WorkspaceID == workspaceID && l.IsActive(now) {
			res = append(res, l)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Resource < res[j].Resource
	})
	return res, nil
}

func (s *Store) ListSince(_ context.Context, workspaceID ids.ID, since time.Time) ([]lease.Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var res []lease.Lease
	for _, l := range s.leases {
		end := l.ExpiresAt
		if l.ReleasedAt != nil {
			end = *l.ReleasedAt
		}
		if l.WorkspaceID == workspaceID && !end.Before(since) {
			res = append(res, l)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		if !res[i].AcquiredAt.Equal(res[j].AcquiredAt) {
			return res[i].AcquiredAt.After(res[j].AcquiredAt)
		}
		return bytes.Compare(res[i].ID[:], res[j].ID[:]) > 0
	})
	return res, nil
}

func (s *Store) ReleaseActiveByWorkspace(_ context.Context, workspaceID ids.ID, releasedBy ids.ID, now time.Time) ([]lease.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var released []lease.Lease
	for i, l := range s.leases {
		if l.WorkspaceID == workspaceID && l.IsActive(now) {
			s.leases[i].ReleasedAt = &now
			if releasedBy != ids.Nil {
				s.leases[i].ReleasedBy = &releasedBy
			}
			released = append(released, s.leases[i])
		}
	}
	return released, nil
}

func (s *Store) ReleaseActiveByHolder(_ context.Context, workspaceID ids.ID, holderID ids.ID, releasedBy ids.ID, now time.Time) ([]lease.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var released []lease.Lease
	for i, l := range s.leases {
		if l.WorkspaceID == workspaceID && l.HolderID == holderID && l.IsActive(now) {
			s.leases[i].ReleasedAt = &now
			if releasedBy != ids.Nil {
				s.leases[i].ReleasedBy = &releasedBy
			}
			released = append(released, s.leases[i])
		}
	}
	return released, nil
}
