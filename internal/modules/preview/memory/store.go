// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of preview.Store.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/preview"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements preview.Store in memory.
type Store struct {
	mu       sync.RWMutex
	previews map[ids.ID]preview.Preview
}

// New returns an empty in-memory store.
func New() *Store {
	return &Store{previews: make(map[ids.ID]preview.Preview)}
}

// Create stores a preview.
func (s *Store) Create(_ context.Context, p preview.Preview) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.previews[p.ID] = p
	return nil
}

// ByID loads a preview by its ID.
func (s *Store) ByID(_ context.Context, id ids.ID) (preview.Preview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.previews[id]
	if !ok {
		return preview.Preview{}, db.ErrNotFound
	}
	return p, nil
}

// ListByWorkspace returns all previews for a workspace sorted by CreatedAt DESC.
func (s *Store) ListByWorkspace(_ context.Context, workspaceID ids.ID) ([]preview.Preview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []preview.Preview
	for _, p := range s.previews {
		if p.WorkspaceID == workspaceID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ListByAgent returns all previews for an agent sorted by CreatedAt DESC.
func (s *Store) ListByAgent(_ context.Context, workspaceID, agentID ids.ID) ([]preview.Preview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []preview.Preview
	for _, p := range s.previews {
		if p.WorkspaceID == workspaceID && p.AgentID == agentID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ActiveByAgentAndName returns the active (requested or running) preview for an agent by name.
func (s *Store) ActiveByAgentAndName(_ context.Context, agentID ids.ID, name string) (preview.Preview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.previews {
		if p.AgentID == agentID && p.Name == name && (p.State == preview.StateRequested || p.State == preview.StateRunning) {
			return p, nil
		}
	}
	return preview.Preview{}, db.ErrNotFound
}

// ActiveByHost returns all active (requested or running) previews for a host sorted by CreatedAt ASC.
func (s *Store) ActiveByHost(_ context.Context, hostID ids.ID) ([]preview.Preview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []preview.Preview
	for _, p := range s.previews {
		if p.HostID == hostID && (p.State == preview.StateRequested || p.State == preview.StateRunning) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// UpdateStatus updates the status, URL, error and UpdatedAt of a preview.
func (s *Store) UpdateStatus(_ context.Context, id ids.ID, state, url, errStr string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.previews[id]
	if !ok {
		return db.ErrNotFound
	}
	p.State = state
	p.URL = url
	p.Error = errStr
	p.UpdatedAt = at
	s.previews[id] = p
	return nil
}

// StopActive marks an active preview as stopped.
func (s *Store) StopActive(_ context.Context, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.previews[id]
	if !ok || (p.State != preview.StateRequested && p.State != preview.StateRunning) {
		return nil
	}
	p.State = preview.StateStopped
	p.UpdatedAt = at
	s.previews[id] = p
	return nil
}

// StopActiveByAgent stops all active previews for an agent.
func (s *Store) StopActiveByAgent(_ context.Context, agentID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.previews {
		if p.AgentID == agentID && (p.State == preview.StateRequested || p.State == preview.StateRunning) {
			p.State = preview.StateStopped
			p.UpdatedAt = at
			s.previews[id] = p
		}
	}
	return nil
}

// StopActiveByWorkspace stops all active previews in a workspace.
func (s *Store) StopActiveByWorkspace(_ context.Context, workspaceID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.previews {
		if p.WorkspaceID == workspaceID && (p.State == preview.StateRequested || p.State == preview.StateRunning) {
			p.State = preview.StateStopped
			p.UpdatedAt = at
			s.previews[id] = p
		}
	}
	return nil
}
