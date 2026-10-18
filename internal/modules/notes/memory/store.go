package memory

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"colonyhall/internal/modules/notes"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements an in-memory notes.Store.
type Store struct {
	mu    sync.Mutex
	notes []notes.Note
}

// New creates an in-memory Store.
func New() *Store {
	return &Store{}
}

// Create stores a new note version.
func (s *Store) Create(_ context.Context, n notes.Note) (notes.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n.ID == ids.Nil {
		n.ID = uuid.New()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	s.notes = append(s.notes, n)
	return n, nil
}

// GetLatest returns the newest note for the agent and thread.
func (s *Store) GetLatest(_ context.Context, workspaceID, agentID, threadRootID ids.ID) (notes.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.notes) - 1; i >= 0; i-- {
		n := s.notes[i]
		if n.WorkspaceID == workspaceID && n.AgentID == agentID && n.ThreadRootID == threadRootID {
			return n, nil
		}
	}
	return notes.Note{}, db.ErrNotFound
}

// ListByAgent returns notes for the agent, newest first, optionally filtered by thread.
func (s *Store) ListByAgent(_ context.Context, workspaceID, agentID ids.ID, threadRootID *ids.ID) ([]notes.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []notes.Note
	for i := len(s.notes) - 1; i >= 0; i-- {
		n := s.notes[i]
		if n.WorkspaceID == workspaceID && n.AgentID == agentID {
			if threadRootID == nil || n.ThreadRootID == *threadRootID {
				result = append(result, n)
			}
		}
	}
	// Sort newest first by CreatedAt
	slices.SortFunc(result, func(a, b notes.Note) int {
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return 1
		}
		return 0
	})
	return result, nil
}
