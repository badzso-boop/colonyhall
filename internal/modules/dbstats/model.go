// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package dbstats reports how big the database is and how it grows, for the
// operator's System status page (round 50). It is server-wide: no table here
// belongs to a workspace.
package dbstats

import (
	"errors"
	"time"
)

// ErrNoDatabase is returned by stores without a database (the memory store).
var ErrNoDatabase = errors.New("dbstats: no database")

// TableSize is one table's size with its indexes and TOAST data.
type TableSize struct {
	Name       string
	TotalBytes int64
	// RowsEstimate comes from the planner's statistics, not count(*).
	RowsEstimate int64
}

// Sample is the database size on one day (Day is midnight UTC).
type Sample struct {
	Day       time.Time
	SizeBytes int64
}

// Report is what the status page shows.
type Report struct {
	SizeBytes int64
	Tables    []TableSize
	Growth    []Sample
}
