// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of approval.Store.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/approval"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements approval.Store in memory.
type Store struct {
	mu       sync.RWMutex
	requests map[ids.ID]approval.ApprovalRequest
}

// New returns an empty in-memory store.
func New() *Store {
	return &Store{requests: make(map[ids.ID]approval.ApprovalRequest)}
}

// Create stores an approval request.
func (s *Store) Create(_ context.Context, r approval.ApprovalRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[r.ID] = r
	return nil
}

// ByID loads an approval request by its ID.
func (s *Store) ByID(_ context.Context, id ids.ID) (approval.ApprovalRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.requests[id]
	if !ok {
		return approval.ApprovalRequest{}, db.ErrNotFound
	}
	return r, nil
}

// Decide updates the state of a pending approval request.
func (s *Store) Decide(_ context.Context, id ids.ID, state string, decidedBy ids.ID, message string, decidedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.requests[id]
	if !ok || r.State != approval.StatePending {
		return approval.ErrAlreadyDecided
	}
	r.State = state
	r.DecidedBy = decidedBy
	r.DecisionMessage = message
	r.DecidedAt = &decidedAt
	s.requests[id] = r
	return nil
}

// ExpirePendingByWorkspace marks all pending requests in workspace as expired.
func (s *Store) ExpirePendingByWorkspace(_ context.Context, workspaceID ids.ID, decidedAt time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for id, r := range s.requests {
		if r.WorkspaceID == workspaceID && r.State == approval.StatePending {
			r.State = approval.StateExpired
			r.DecisionMessage = "emergency stop"
			r.DecidedAt = &decidedAt
			s.requests[id] = r
			count++
		}
	}
	return count, nil
}

// ListPendingByWorkspace lists all pending requests for a workspace.
func (s *Store) ListPendingByWorkspace(_ context.Context, workspaceID ids.ID) ([]approval.ApprovalRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []approval.ApprovalRequest
	for _, r := range s.requests {
		if r.WorkspaceID == workspaceID && r.State == approval.StatePending {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}
