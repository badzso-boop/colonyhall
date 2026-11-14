// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory implements dbstats.Store without a database: sizes are
// unknown, samples are kept in memory (for tests).
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/dbstats"
)

// Store implements dbstats.Store in memory.
type Store struct {
	mu      sync.Mutex
	samples map[time.Time]int64
}

var _ dbstats.Store = (*Store)(nil)

// New returns an empty store.
func New() *Store { return &Store{samples: map[time.Time]int64{}} }

// Size: there is no database.
func (s *Store) Size(context.Context) (int64, error) { return 0, dbstats.ErrNoDatabase }

// LargestTables: there is no database.
func (s *Store) LargestTables(context.Context, int) ([]dbstats.TableSize, error) {
	return nil, dbstats.ErrNoDatabase
}

// RecordSample keeps the first sample of a day.
func (s *Store) RecordSample(_ context.Context, x dbstats.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := dbstats.Day(x.Day)
	if _, ok := s.samples[day]; !ok {
		s.samples[day] = x.SizeBytes
	}
	return nil
}

// Samples lists the samples from since on, oldest first.
func (s *Store) Samples(_ context.Context, since time.Time) ([]dbstats.Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []dbstats.Sample{}
	for d, n := range s.samples {
		if !d.Before(dbstats.Day(since)) {
			out = append(out, dbstats.Sample{Day: d, SizeBytes: n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out, nil
}

// PruneSamples deletes the samples before the day.
func (s *Store) PruneSamples(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for d := range s.samples {
		if d.Before(dbstats.Day(before)) {
			delete(s.samples, d)
			n++
		}
	}
	return n, nil
}
