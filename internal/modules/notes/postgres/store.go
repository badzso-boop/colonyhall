package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/notes"
	"colonyhall/internal/modules/notes/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements notes.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Create stores a new note version.
func (s *Store) Create(ctx context.Context, n notes.Note) (notes.Note, error) {
	id := n.ID
	if id == ids.Nil {
		id = uuid.New()
	}
	row, err := s.q(ctx).InsertNote(ctx, sqlcgen.InsertNoteParams{
		ID:           id,
		WorkspaceID:  n.WorkspaceID,
		AgentID:      n.AgentID,
		ThreadRootID: n.ThreadRootID,
		Text:         n.Text,
		CreatedAt:    n.CreatedAt,
	})
	if err != nil {
		return notes.Note{}, err
	}
	return notes.Note{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		AgentID:      row.AgentID,
		ThreadRootID: row.ThreadRootID,
		Text:         row.Text,
		CreatedAt:    row.CreatedAt,
	}, nil
}

// GetLatest returns the newest note for the agent and thread.
func (s *Store) GetLatest(ctx context.Context, workspaceID, agentID, threadRootID ids.ID) (notes.Note, error) {
	row, err := s.q(ctx).GetLatestNote(ctx, sqlcgen.GetLatestNoteParams{
		WorkspaceID:  workspaceID,
		AgentID:      agentID,
		ThreadRootID: threadRootID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return notes.Note{}, db.ErrNotFound
	}
	if err != nil {
		return notes.Note{}, err
	}
	return notes.Note{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		AgentID:      row.AgentID,
		ThreadRootID: row.ThreadRootID,
		Text:         row.Text,
		CreatedAt:    row.CreatedAt,
	}, nil
}

// ListByAgent returns notes for the agent, newest first, optionally filtered by thread.
func (s *Store) ListByAgent(ctx context.Context, workspaceID, agentID ids.ID, threadRootID *ids.ID) ([]notes.Note, error) {
	var threadUUID *uuid.UUID
	if threadRootID != nil {
		u := uuid.UUID(*threadRootID)
		threadUUID = &u
	}
	rows, err := s.q(ctx).ListNotesByAgent(ctx, sqlcgen.ListNotesByAgentParams{
		WorkspaceID:  workspaceID,
		AgentID:      agentID,
		ThreadRootID: threadUUID,
	})
	if err != nil {
		return nil, err
	}
	res := make([]notes.Note, len(rows))
	for i, r := range rows {
		res[i] = notes.Note{
			ID:           r.ID,
			WorkspaceID:  r.WorkspaceID,
			AgentID:      r.AgentID,
			ThreadRootID: r.ThreadRootID,
			Text:         r.Text,
			CreatedAt:    r.CreatedAt,
		}
	}
	return res, nil
}
