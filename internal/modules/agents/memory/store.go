// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory keeps the agents' own instructions in memory (tests).
package memory

import (
	"context"
	"sync"

	"colonyhall/internal/modules/agents"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements agents.Store in memory.
type Store struct {
	mu   sync.RWMutex
	byID map[ids.ID][]agents.Instructions
}

// New returns an empty store.
func New() *Store { return &Store{byID: map[ids.ID][]agents.Instructions{}} }

// CreateInstructions stores an agent's own instructions; a version is written once.
func (s *Store) CreateInstructions(_ context.Context, in agents.Instructions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.byID[in.AgentID] {
		if x.Version == in.Version {
			return db.ErrConflict
		}
	}
	s.byID[in.AgentID] = append(s.byID[in.AgentID], in)
	return nil
}

// InstructionsByAgent returns the newest version.
func (s *Store) InstructionsByAgent(_ context.Context, agentID ids.ID) (agents.Instructions, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.byID[agentID]
	if len(list) == 0 {
		return agents.Instructions{}, db.ErrNotFound
	}
	best := list[0]
	for _, x := range list[1:] {
		if x.Version > best.Version {
			best = x
		}
	}
	return best, nil
}
