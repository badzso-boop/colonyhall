package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/delegation/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements delegation.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func uuidPtr(id ids.ID) *uuid.UUID {
	if id == ids.Nil {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

func fromUUIDPtr(u *uuid.UUID) *ids.ID {
	if u == nil {
		return nil
	}
	id := ids.ID(*u)
	return &id
}

func toModel(r sqlcgen.Delegation) delegation.Delegation {
	d := delegation.Delegation{
		ID:          ids.ID(r.ID),
		WorkspaceID: ids.ID(r.WorkspaceID),
		MessageID:   ids.ID(r.MessageID),
		ChannelID:   ids.ID(r.ChannelID),
		FromAgentID: ids.ID(r.FromAgentID),
		ToAgentID:   ids.ID(r.ToAgentID),
		QuestionID:  ids.ID(r.QuestionID),
		State:       r.State,
		DecidedBy:   fromUUIDPtr(r.DecidedBy),
		DecidedAt:   r.DecidedAt,
		CreatedAt:   r.CreatedAt,
	}
	if r.ThreadRootID != nil {
		d.ThreadRootID = ids.ID(*r.ThreadRootID)
	}
	return d
}

func (s *Store) Insert(ctx context.Context, d delegation.Delegation) error {
	_, err := s.q(ctx).InsertDelegation(ctx, sqlcgen.InsertDelegationParams{
		ID:           uuid.UUID(d.ID),
		WorkspaceID:  uuid.UUID(d.WorkspaceID),
		MessageID:    uuid.UUID(d.MessageID),
		ChannelID:    uuid.UUID(d.ChannelID),
		ThreadRootID: uuidPtr(d.ThreadRootID),
		FromAgentID:  uuid.UUID(d.FromAgentID),
		ToAgentID:    uuid.UUID(d.ToAgentID),
		QuestionID:   uuid.UUID(d.QuestionID),
		State:        d.State,
		CreatedAt:    d.CreatedAt,
	})
	return err
}

func (s *Store) GetByID(ctx context.Context, id ids.ID) (delegation.Delegation, error) {
	r, err := s.q(ctx).GetDelegationByID(ctx, uuid.UUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Delegation{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Delegation{}, err
	}
	return toModel(r), nil
}

func (s *Store) GetByQuestionID(ctx context.Context, questionID ids.ID) (delegation.Delegation, error) {
	r, err := s.q(ctx).GetDelegationByQuestionID(ctx, uuid.UUID(questionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Delegation{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Delegation{}, err
	}
	return toModel(r), nil
}

func (s *Store) GetByMessageAndTarget(ctx context.Context, messageID, toAgentID ids.ID) (delegation.Delegation, error) {
	r, err := s.q(ctx).GetDelegationByMessageAndTarget(ctx, sqlcgen.GetDelegationByMessageAndTargetParams{
		MessageID: uuid.UUID(messageID),
		ToAgentID: uuid.UUID(toAgentID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Delegation{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Delegation{}, err
	}
	return toModel(r), nil
}

func (s *Store) UpdateStateConditional(ctx context.Context, id ids.ID, expectedState, newState string, decidedBy *ids.ID, decidedAt time.Time) (int64, error) {
	var decBy *uuid.UUID
	if decidedBy != nil && *decidedBy != ids.Nil {
		u := uuid.UUID(*decidedBy)
		decBy = &u
	}
	return s.q(ctx).UpdateDelegationStateConditional(ctx, sqlcgen.UpdateDelegationStateConditionalParams{
		ID:        uuid.UUID(id),
		State:     expectedState,
		State_2:   newState,
		DecidedBy: decBy,
		DecidedAt: &decidedAt,
	})
}

func (s *Store) ListByWorkspace(ctx context.Context, workspaceID ids.ID, state *string) ([]delegation.Delegation, error) {
	rows, err := s.q(ctx).ListDelegationsByWorkspace(ctx, sqlcgen.ListDelegationsByWorkspaceParams{
		WorkspaceID: uuid.UUID(workspaceID),
		State:       state,
	})
	if err != nil {
		return nil, err
	}
	out := make([]delegation.Delegation, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}
