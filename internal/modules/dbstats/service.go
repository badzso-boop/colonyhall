package dbstats

import (
	"context"
	"errors"
	"time"

	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

const (
	// TopTables is how many tables the report lists.
	TopTables = 10
	// GrowthDays is how far back the report's samples go.
	GrowthDays = 90
	// KeepDays is how long samples are kept.
	KeepDays = 400
)

// Service samples and reports the database size.
type Service struct {
	Store Store
	Clock clock.Clock
}

// Day is the UTC calendar day of t, at midnight.
func Day(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// SampleToday records today's size (once a day, whichever server is first)
// and prunes samples older than KeepDays. Without a database it does nothing.
func (s *Service) SampleToday(ctx context.Context) error {
	size, err := s.Store.Size(ctx)
	if errors.Is(err, ErrNoDatabase) {
		return nil
	}
	if err != nil {
		return err
	}
	today := Day(s.Clock.Now())
	if err := s.Store.RecordSample(ctx, Sample{Day: today, SizeBytes: size}); err != nil {
		return err
	}
	_, err = s.Store.PruneSamples(ctx, today.AddDate(0, 0, -KeepDays))
	return err
}

// Report returns the size, the largest tables and the last GrowthDays of
// samples; ok is false when the store has no database.
func (s *Service) Report(ctx context.Context) (r Report, ok bool, err error) {
	if r.SizeBytes, err = s.Store.Size(ctx); errors.Is(err, ErrNoDatabase) {
		return Report{}, false, nil
	} else if err != nil {
		return r, false, err
	}
	if r.Tables, err = s.Store.LargestTables(ctx, TopTables); err != nil {
		return r, false, err
	}
	r.Growth, err = s.Store.Samples(ctx, Day(s.Clock.Now()).AddDate(0, 0, -GrowthDays))
	return r, err == nil, err
}

// Workspaces lists every workspace; ok is false without a database. System
// mode.
func (s *Service) Workspaces(ctx context.Context) ([]ids.ID, bool, error) {
	list, err := s.Store.WorkspaceIDs(ctx)
	if errors.Is(err, ErrNoDatabase) {
		return nil, false, nil
	}
	return list, err == nil, err
}

// WorkspaceEstimate is the workspace's share of the database (round 54).
// On a server with one workspace it is the whole database (round 50's
// size); with several, the workspace's rows in the large tables times
// OverheadFactor. ctx must be scoped to the workspace.
func (s *Service) WorkspaceEstimate(ctx context.Context, workspaceID ids.ID, only bool) (Estimate, error) {
	if only {
		size, err := s.Store.Size(ctx)
		if err != nil {
			return Estimate{}, err
		}
		chat, err := s.Store.ChatTablesSize(ctx)
		if err != nil {
			return Estimate{}, err
		}
		return Estimate{Total: size, Chat: min(chat, size)}, nil
	}
	rb, err := s.Store.WorkspaceRowBytes(ctx, workspaceID)
	if err != nil {
		return Estimate{}, err
	}
	chat := int64(float64(rb.Chat) * OverheadFactor)
	return Estimate{Total: chat + int64(float64(rb.Other)*OverheadFactor), Chat: chat}, nil
}
