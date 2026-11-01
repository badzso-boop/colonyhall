// Package postgres stores the agents' own instructions in PostgreSQL.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/agents"
	"colonyhall/internal/modules/agents/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements agents.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// CreateInstructions stores an agent's own instructions.
func (s *Store) CreateInstructions(ctx context.Context, in agents.Instructions) error {
	return s.q(ctx).CreateInstructions(ctx, sqlcgen.CreateInstructionsParams{
		AgentID: in.AgentID, WorkspaceID: in.WorkspaceID, Version: int32(in.Version), Text: in.Text,
		CreatedBy: in.CreatedBy, CreatedAt: in.CreatedAt,
	})
}

// InstructionsByAgent returns the newest version.
func (s *Store) InstructionsByAgent(ctx context.Context, agentID ids.ID) (agents.Instructions, error) {
	r, err := s.q(ctx).InstructionsByAgent(ctx, agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return agents.Instructions{}, db.ErrNotFound
	}
	if err != nil {
		return agents.Instructions{}, err
	}
	return agents.Instructions{AgentID: r.AgentID, WorkspaceID: r.WorkspaceID, Version: int(r.Version), Text: r.Text,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt}, nil
}
