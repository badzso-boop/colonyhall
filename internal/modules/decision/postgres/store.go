package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/decision/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements decision.Store on Postgres.
type Store struct{ DB *db.Postgres }

var _ decision.Store = (*Store)(nil)

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func toModel(id ids.ID, number int32, wsID ids.ID, authorID ids.ID, chID ids.ID, threadRootID *ids.ID, text, rationale string, supersedes *int32, supersededBy int32, createdAt time.Time) decision.Decision {
	var sup *int
	if supersedes != nil {
		v := int(*supersedes)
		sup = &v
	}
	var supBy *int
	if supersededBy > 0 {
		v := int(supersededBy)
		supBy = &v
	}
	return decision.Decision{
		ID:           id,
		Number:       int(number),
		WorkspaceID:  wsID,
		AuthorID:     authorID,
		ChannelID:    chID,
		ThreadRootID: threadRootID,
		Text:         text,
		Rationale:    rationale,
		Supersedes:   sup,
		SupersededBy: supBy,
		CreatedAt:    createdAt,
	}
}

func (s *Store) Insert(ctx context.Context, d decision.Decision) error {
	var sup *int32
	if d.Supersedes != nil {
		v := int32(*d.Supersedes)
		sup = &v
	}
	var trID *ids.ID
	if d.ThreadRootID != nil && *d.ThreadRootID != ids.Nil {
		trID = d.ThreadRootID
	}
	_, err := s.q(ctx).InsertDecision(ctx, sqlcgen.InsertDecisionParams{
		ID:           d.ID,
		Number:       int32(d.Number),
		WorkspaceID:  d.WorkspaceID,
		AuthorID:     d.AuthorID,
		ChannelID:    d.ChannelID,
		ThreadRootID: trID,
		Text:         d.Text,
		Rationale:    d.Rationale,
		Supersedes:   sup,
		CreatedAt:    d.CreatedAt,
	})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return db.ErrConflict
		}
		return err
	}
	return nil
}

func (s *Store) NextNumber(ctx context.Context, workspaceID ids.ID) (int, error) {
	n, err := s.q(ctx).NextDecisionNumber(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (s *Store) GetByNumber(ctx context.Context, workspaceID ids.ID, number int) (decision.Decision, error) {
	r, err := s.q(ctx).GetDecisionByNumber(ctx, sqlcgen.GetDecisionByNumberParams{
		WorkspaceID: workspaceID,
		Number:      int32(number),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return decision.Decision{}, db.ErrNotFound
		}
		return decision.Decision{}, err
	}
	return toModel(r.ID, r.Number, r.WorkspaceID, r.AuthorID, r.ChannelID, r.ThreadRootID, r.Text, r.Rationale, r.Supersedes, r.SupersededBy, r.CreatedAt), nil
}

func (s *Store) List(ctx context.Context, workspaceID ids.ID, limit int, query string, validOnly bool) ([]decision.Decision, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.q(ctx).ListDecisions(ctx, sqlcgen.ListDecisionsParams{
		WorkspaceID: workspaceID,
		Query:       query,
		ValidOnly:   validOnly,
		Limit:       int32(limit),
	})
	if err != nil {
		return nil, err
	}
	res := make([]decision.Decision, len(rows))
	for i, r := range rows {
		res[i] = toModel(r.ID, r.Number, r.WorkspaceID, r.AuthorID, r.ChannelID, r.ThreadRootID, r.Text, r.Rationale, r.Supersedes, r.SupersededBy, r.CreatedAt)
	}
	return res, nil
}
