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

// RowBytes is the size of one workspace's rows in the large tables
// (pg_column_size), without indexes (round 54).
type RowBytes struct {
	// Chat: messages, events, the search index, search log, deliveries.
	Chat int64
	// Other: runs, audit, file metadata, wiki revisions, terminal events.
	Other int64
}

// Estimate is a workspace's share of the database.
type Estimate struct {
	Total int64
	// Chat is the chat and search part of Total.
	Chat int64
}

// OverheadFactor turns the rows' pg_column_size sum into a share of the
// database with indexes, page and tuple overhead and free space. Measured on
// the dev stack (round 54, docs/02-muszaki-specifikacio.md "Tárhely"): the
// ten tables' pg_total_relation_size over their rows' pg_column_size is 1.42
// together (1.22 for the search log, which dominates, up to 6.1 for small
// tables); 2.0 also covers the tables the estimate leaves out. On the dev
// database (one workspace) it gives 29.0 MB for a pg_database_size of
// 32.2 MB.
const OverheadFactor = 2.0
