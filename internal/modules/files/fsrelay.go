// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files

import (
	"errors"
	"sync"
	"time"

	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Common errors for filesystem relay operations.
var (
	ErrFsTimeout = errors.New("a gép nem válaszol")
	ErrFsGone    = errors.New("request expired")
)

// FsPendingRequest holds the in-memory state of an active fs operation.
type FsPendingRequest struct {
	ID        string
	AgentID   ids.ID
	Op        string
	Root      string
	Path      string
	Query     string
	Name      string
	Lines     int
	KilledBy  string
	CreatedAt time.Time
	ResultCh  chan FsResult
}

// FsResult delivers either the JSON payload or an error/code from the guard.
type FsResult struct {
	Payload   any
	ErrorCode string
	Err       error
}

// FsRelay manages ephemeral filesystem requests in memory.
type FsRelay struct {
	mu      sync.Mutex
	pending map[string]*FsPendingRequest
	history map[string]time.Time
}

// NewFsRelay creates a new in-memory filesystem request relay.
func NewFsRelay() *FsRelay {
	return &FsRelay{
		pending: make(map[string]*FsPendingRequest),
		history: make(map[string]time.Time),
	}
}

// Register adds a pending request to the relay.
func (r *FsRelay) Register(id string, agentID ids.ID, op, root, path, query, name string, lines int, killedBy string, now time.Time) *FsPendingRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	req := &FsPendingRequest{
		ID:        id,
		AgentID:   agentID,
		Op:        op,
		Root:      root,
		Path:      path,
		Query:     query,
		Name:      name,
		Lines:     lines,
		KilledBy:  killedBy,
		CreatedAt: now,
		ResultCh:  make(chan FsResult, 1),
	}
	r.pending[id] = req
	return req
}

// Get returns the pending request if found and matching the agent.
func (r *FsRelay) Get(id string, agentID ids.ID) (*FsPendingRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.pending[id]
	if !ok {
		return nil, db.ErrNotFound
	}
	if req.AgentID != agentID {
		return nil, db.ErrNotFound // A request for another agent → 404
	}
	return req, nil
}

// Deliver unblocks the waiting client with the result, or returns ErrFsGone if timed out.
func (r *FsRelay) Deliver(id string, agentID ids.ID, res FsResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.pending[id]
	if !ok {
		if _, existed := r.history[id]; existed {
			return ErrFsGone // 410
		}
		return db.ErrNotFound // 404
	}
	if req.AgentID != agentID {
		return db.ErrNotFound
	}
	delete(r.pending, id)
	r.history[id] = time.Now()

	// Prune history older than 5 minutes
	now := time.Now()
	for k, t := range r.history {
		if now.Sub(t) > 5*time.Minute {
			delete(r.history, k)
		}
	}

	req.ResultCh <- res
	return nil
}

// Timeout removes the request from pending and records it in history.
func (r *FsRelay) Timeout(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[id]; ok {
		delete(r.pending, id)
		r.history[id] = time.Now()
	}
}
