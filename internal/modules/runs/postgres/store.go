// Package postgres implements runs.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/runs"
	"colonyhall/internal/modules/runs/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements runs.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// InsertError stores an error.
func (s *Store) InsertError(ctx context.Context, e runs.Error) error {
	var thread *ids.ID
	if e.ThreadRootID != ids.Nil {
		thread = &e.ThreadRootID
	}
	return s.q(ctx).InsertError(ctx, sqlcgen.InsertErrorParams{ID: e.ID, WorkspaceID: e.WorkspaceID, AgentID: e.AgentID,
		ChannelID: e.ChannelID, ThreadRootID: thread, Summary: e.Summary, Detail: e.Detail, CreatedAt: e.CreatedAt})
}

// ErrorByID loads an error.
func (s *Store) ErrorByID(ctx context.Context, id ids.ID) (runs.Error, error) {
	r, err := s.q(ctx).ErrorByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return runs.Error{}, db.ErrNotFound
	}
	if err != nil {
		return runs.Error{}, err
	}
	e := runs.Error{ID: r.ID, WorkspaceID: r.WorkspaceID, AgentID: r.AgentID, ChannelID: r.ChannelID, Summary: r.Summary,
		Detail: r.Detail, CreatedAt: r.CreatedAt}
	if r.ThreadRootID != nil {
		e.ThreadRootID = *r.ThreadRootID
	}
	return e, nil
}

// InsertRun stores an agent run.
func (s *Store) InsertRun(ctx context.Context, r runs.Run) error {
	var hostID, channelID, threadRootID, errorID, replyMsgID *ids.ID
	if r.HostID != ids.Nil {
		hostID = &r.HostID
	}
	if r.ChannelID != ids.Nil {
		channelID = &r.ChannelID
	}
	if r.ThreadRootID != ids.Nil {
		threadRootID = &r.ThreadRootID
	}
	if r.ErrorID != ids.Nil {
		errorID = &r.ErrorID
	}
	if r.ReplyMessageID != ids.Nil {
		replyMsgID = &r.ReplyMessageID
	}
	webReads := r.WebReads
	if webReads == nil {
		webReads = []string{}
	}
	stepsJSON := []byte("[]")
	if len(r.Steps) > 0 {
		if b, err := json.Marshal(r.Steps); err == nil {
			stepsJSON = b
		}
	}
	return s.q(ctx).InsertRun(ctx, sqlcgen.InsertRunParams{
		ID:               r.ID,
		WorkspaceID:      r.WorkspaceID,
		AgentID:          r.AgentID,
		HostID:           hostID,
		Phase:            r.Phase,
		Client:           r.Client,
		Model:            r.Model,
		SessionID:        r.SessionID,
		ChannelID:        channelID,
		ThreadRootID:     threadRootID,
		Items:            int32(r.Items),
		InputTokens:      r.InputTokens,
		OutputTokens:     r.OutputTokens,
		CacheReadTokens:  r.CacheReadTokens,
		CacheWriteTokens: r.CacheWriteTokens,
		ContextTokens:    r.ContextTokens,
		DurationMs:       int32(r.DurationMS),
		Status:           r.Status,
		ErrorID:          errorID,
		CreatedAt:        r.CreatedAt,
		WebReads:         webReads,
		Steps:            stepsJSON,
		ReplyMessageID:   replyMsgID,
	})
}

// ListRuns loads runs for a workspace, optionally filtered by agent_id, newest first.
func (s *Store) ListRuns(ctx context.Context, workspaceID ids.ID, agentID ids.ID, limit int) ([]runs.Run, error) {
	var filterAgent *ids.ID
	if agentID != ids.Nil {
		filterAgent = &agentID
	}
	rows, err := s.q(ctx).ListRuns(ctx, sqlcgen.ListRunsParams{
		WorkspaceID: workspaceID,
		AgentID:     filterAgent,
		Limit:       int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]runs.Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, runFromRow(r))
	}
	return out, nil
}

// Usage calculates daily usage per agent in Europe/Budapest since the given timestamp.
func (s *Store) Usage(ctx context.Context, workspaceID ids.ID, since time.Time) ([]runs.DayUsage, error) {
	rows, err := s.q(ctx).Usage(ctx, sqlcgen.UsageParams{
		WorkspaceID: workspaceID,
		CreatedAt:   since,
	})
	if err != nil {
		return nil, err
	}
	out := make([]runs.DayUsage, 0, len(rows))
	for _, r := range rows {
		total := r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens
		out = append(out, runs.DayUsage{
			AgentID:          r.AgentID,
			Day:              r.Day,
			InputTokens:      r.InputTokens,
			OutputTokens:     r.OutputTokens,
			CacheReadTokens:  r.CacheReadTokens,
			CacheWriteTokens: r.CacheWriteTokens,
			TotalTokens:      total,
			RunCount:         int(r.RunCount),
			ErrorCount:       int(r.ErrorCount),
		})
	}
	return out, nil
}

// TokensSince returns the sum of all tokens consumed by the agent since the given timestamp.
func (s *Store) TokensSince(ctx context.Context, workspaceID, agentID ids.ID, since time.Time) (int64, error) {
	return s.q(ctx).AgentTokensSince(ctx, sqlcgen.AgentTokensSinceParams{
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		CreatedAt:   since,
	})
}

func runFromRow(r sqlcgen.AgentRun) runs.Run {
	item := runs.Run{
		ID:               r.ID,
		WorkspaceID:      r.WorkspaceID,
		AgentID:          r.AgentID,
		Phase:            r.Phase,
		Client:           r.Client,
		Model:            r.Model,
		SessionID:        r.SessionID,
		Items:            int(r.Items),
		InputTokens:      r.InputTokens,
		OutputTokens:     r.OutputTokens,
		CacheReadTokens:  r.CacheReadTokens,
		CacheWriteTokens: r.CacheWriteTokens,
		ContextTokens:    r.ContextTokens,
		DurationMS:       int(r.DurationMs),
		Status:           r.Status,
		CreatedAt:        r.CreatedAt,
		WebReads:         r.WebReads,
	}
	if r.HostID != nil {
		item.HostID = *r.HostID
	}
	if r.ChannelID != nil {
		item.ChannelID = *r.ChannelID
	}
	if r.ThreadRootID != nil {
		item.ThreadRootID = *r.ThreadRootID
	}
	if r.ErrorID != nil {
		item.ErrorID = *r.ErrorID
	}
	if r.ReplyMessageID != nil {
		item.ReplyMessageID = *r.ReplyMessageID
	}
	var steps []runs.Step
	if len(r.Steps) > 0 {
		_ = json.Unmarshal(r.Steps, &steps)
	}
	if steps == nil {
		steps = []runs.Step{}
	}
	item.Steps = steps
	return item
}

// SearchRuns lists runs by filter, newest first.
func (s *Store) SearchRuns(ctx context.Context, workspaceID ids.ID, f runs.RunFilter) ([]runs.Run, error) {
	p := sqlcgen.SearchRunsParams{WorkspaceID: workspaceID, Limit: int32(f.Limit), FromAt: f.From, ToAt: f.To}
	if f.AgentID != ids.Nil {
		p.AgentID = &f.AgentID
	}
	if f.HostID != ids.Nil {
		p.HostID = &f.HostID
	}
	if f.ChannelID != ids.Nil {
		p.ChannelID = &f.ChannelID
	}
	if f.Status != "" {
		p.Status = &f.Status
	}
	if f.Before != nil {
		p.BeforeAt, p.BeforeID = &f.Before.CreatedAt, &f.Before.ID
	}
	rows, err := s.q(ctx).SearchRuns(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]runs.Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, runFromRow(r))
	}
	return out, nil
}

// UsageRuns lists what usage needs of every run since a time.
func (s *Store) UsageRuns(ctx context.Context, workspaceID ids.ID, since time.Time) ([]runs.UsageRun, error) {
	rows, err := s.q(ctx).UsageRuns(ctx, sqlcgen.UsageRunsParams{WorkspaceID: workspaceID, CreatedAt: since})
	if err != nil {
		return nil, err
	}
	out := make([]runs.UsageRun, 0, len(rows))
	for _, r := range rows {
		u := runs.UsageRun{AgentID: r.AgentID, Model: r.Model, Client: r.Client, Status: r.Status, DurationMS: int(r.DurationMs),
			InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CacheReadTokens: r.CacheReadTokens,
			CacheWriteTokens: r.CacheWriteTokens, CreatedAt: r.CreatedAt}
		if r.HostID != nil {
			u.HostID = *r.HostID
		}
		out = append(out, u)
	}
	return out, nil
}

// InsertPrice adds a row to the append-only price table.
func (s *Store) InsertPrice(ctx context.Context, p runs.Price) error {
	return s.q(ctx).InsertPrice(ctx, sqlcgen.InsertPriceParams{ID: p.ID, WorkspaceID: p.WorkspaceID, Model: p.Model,
		InputPerMtok: p.InputPerMTok, OutputPerMtok: p.OutputPerMTok, CacheReadPerMtok: p.CacheReadPerMTok,
		CacheWritePerMtok: p.CacheWritePerMTok, ValidFrom: p.ValidFrom, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt})
}

// Prices lists the workspace's price rows.
func (s *Store) Prices(ctx context.Context, workspaceID ids.ID) ([]runs.Price, error) {
	rows, err := s.q(ctx).Prices(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]runs.Price, 0, len(rows))
	for _, r := range rows {
		out = append(out, runs.Price{ID: r.ID, WorkspaceID: r.WorkspaceID, Model: r.Model, InputPerMTok: r.InputPerMtok,
			OutputPerMTok: r.OutputPerMtok, CacheReadPerMTok: r.CacheReadPerMtok, CacheWritePerMTok: r.CacheWritePerMtok,
			ValidFrom: r.ValidFrom, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Currency returns the workspace's exchange rate.
func (s *Store) Currency(ctx context.Context, workspaceID ids.ID) (runs.Currency, error) {
	r, err := s.q(ctx).Currency(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return runs.Currency{}, db.ErrNotFound
	}
	if err != nil {
		return runs.Currency{}, err
	}
	return runs.Currency{WorkspaceID: r.WorkspaceID, USDHUF: r.UsdHuf, UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt}, nil
}

// PutCurrency stores the workspace's exchange rate.
func (s *Store) PutCurrency(ctx context.Context, c runs.Currency) error {
	return s.q(ctx).PutCurrency(ctx, sqlcgen.PutCurrencyParams{WorkspaceID: c.WorkspaceID, UsdHuf: c.USDHUF, UpdatedBy: c.UpdatedBy, UpdatedAt: c.UpdatedAt})
}
