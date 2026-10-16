package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/question"
	"colonyhall/internal/modules/question/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements question.Store on Postgres.
type Store struct{ DB *db.Postgres }

var _ question.Store = (*Store)(nil)

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func toModel(r sqlcgen.Question) question.Question {
	var opts []string
	if len(r.Options) > 0 {
		opts = make([]string, len(r.Options))
		copy(opts, r.Options)
	} else {
		opts = []string{}
	}
	return question.Question{
		ID:            r.ID,
		Number:        int(r.Number),
		WorkspaceID:   r.WorkspaceID,
		AskedBy:       r.AskedBy,
		ToActorID:     r.ToActorID,
		ToHuman:       r.ToHuman,
		ChannelID:     r.ChannelID,
		ThreadRootID:  r.ThreadRootID,
		Text:          r.Text,
		Options:       opts,
		DefaultOption: r.DefaultOption,
		Deadline:      r.Deadline,
		State:         r.State,
		Answer:        r.Answer,
		AnsweredBy:    r.AnsweredBy,
		CreatedAt:     r.CreatedAt,
		AnsweredAt:    r.AnsweredAt,
		EscalatedAt:   r.EscalatedAt,
		WithdrawnAt:   r.WithdrawnAt,
		ExpiredAt:     r.ExpiredAt,
	}
}

func (s *Store) Insert(ctx context.Context, q question.Question) error {
	opts := q.Options
	if opts == nil {
		opts = []string{}
	}
	_, err := s.q(ctx).InsertQuestion(ctx, sqlcgen.InsertQuestionParams{
		ID:            q.ID,
		Number:        int32(q.Number),
		WorkspaceID:   q.WorkspaceID,
		AskedBy:       q.AskedBy,
		ToActorID:     q.ToActorID,
		ToHuman:       q.ToHuman,
		ChannelID:     q.ChannelID,
		ThreadRootID:  q.ThreadRootID,
		Text:          q.Text,
		Options:       opts,
		DefaultOption: q.DefaultOption,
		Deadline:      q.Deadline,
		State:         q.State,
		CreatedAt:     q.CreatedAt,
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
	n, err := s.q(ctx).NextQuestionNumber(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (s *Store) CountOpenByThreadAsker(ctx context.Context, threadRootID, askedBy ids.ID) (int, error) {
	c, err := s.q(ctx).CountOpenByThreadAsker(ctx, sqlcgen.CountOpenByThreadAskerParams{
		ThreadRootID: threadRootID,
		AskedBy:      askedBy,
	})
	if err != nil {
		return 0, err
	}
	return int(c), nil
}

func (s *Store) GetByID(ctx context.Context, id ids.ID) (question.Question, error) {
	r, err := s.q(ctx).GetQuestionByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return question.Question{}, db.ErrNotFound
	}
	if err != nil {
		return question.Question{}, err
	}
	return toModel(r), nil
}

func (s *Store) GetByNumber(ctx context.Context, workspaceID ids.ID, number int) (question.Question, error) {
	r, err := s.q(ctx).GetQuestionByNumber(ctx, sqlcgen.GetQuestionByNumberParams{
		WorkspaceID: workspaceID,
		Number:      int32(number),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return question.Question{}, db.ErrNotFound
	}
	if err != nil {
		return question.Question{}, err
	}
	return toModel(r), nil
}

func (s *Store) List(ctx context.Context, workspaceID ids.ID, state *string) ([]question.Question, error) {
	rows, err := s.q(ctx).ListQuestions(ctx, sqlcgen.ListQuestionsParams{
		WorkspaceID: workspaceID,
		State:       state,
	})
	if err != nil {
		return nil, err
	}
	out := make([]question.Question, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

func (s *Store) Answer(ctx context.Context, id ids.ID, answer string, answeredBy *ids.ID, answeredAt time.Time) (question.Question, bool, error) {
	r, err := s.q(ctx).AnswerQuestion(ctx, sqlcgen.AnswerQuestionParams{
		ID:         id,
		Answer:     &answer,
		AnsweredBy: answeredBy,
		AnsweredAt: &answeredAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return question.Question{}, false, nil
	}
	if err != nil {
		return question.Question{}, false, err
	}
	return toModel(r), true, nil
}

func (s *Store) Withdraw(ctx context.Context, id ids.ID, withdrawnAt time.Time) (question.Question, bool, error) {
	r, err := s.q(ctx).WithdrawQuestion(ctx, sqlcgen.WithdrawQuestionParams{
		ID:          id,
		WithdrawnAt: &withdrawnAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return question.Question{}, false, nil
	}
	if err != nil {
		return question.Question{}, false, err
	}
	return toModel(r), true, nil
}

func (s *Store) Escalate(ctx context.Context, id ids.ID, escalatedAt time.Time) (question.Question, bool, error) {
	r, err := s.q(ctx).EscalateQuestion(ctx, sqlcgen.EscalateQuestionParams{
		ID:          id,
		EscalatedAt: &escalatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return question.Question{}, false, nil
	}
	if err != nil {
		return question.Question{}, false, err
	}
	return toModel(r), true, nil
}

func (s *Store) Expire(ctx context.Context, id ids.ID, expiredAt time.Time) (question.Question, bool, error) {
	r, err := s.q(ctx).ExpireQuestion(ctx, sqlcgen.ExpireQuestionParams{
		ID:        id,
		ExpiredAt: &expiredAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return question.Question{}, false, nil
	}
	if err != nil {
		return question.Question{}, false, err
	}
	return toModel(r), true, nil
}

func (s *Store) Due(ctx context.Context, escalateBefore, deadlineBefore time.Time, limit int) ([]question.Question, error) {
	rows, err := s.q(ctx).ListDueQuestions(ctx, sqlcgen.ListDueQuestionsParams{
		CreatedAt: escalateBefore,
		Deadline:  &deadlineBefore,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]question.Question, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}
