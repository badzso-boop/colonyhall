// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package dbstats

import (
	"context"
	"errors"
	"time"

	"colonyhall/internal/platform/clock"
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
