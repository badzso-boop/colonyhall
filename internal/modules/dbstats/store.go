package dbstats

import (
	"context"
	"time"
)

// Store reads sizes from the database and keeps the daily samples.
// Implementations: postgres, memory (no database: Size and LargestTables
// return ErrNoDatabase).
type Store interface {
	// Size is the whole database's size in bytes.
	Size(ctx context.Context) (int64, error)
	// LargestTables lists the biggest tables first.
	LargestTables(ctx context.Context, limit int) ([]TableSize, error)
	// RecordSample stores the day's size unless that day already has one.
	RecordSample(ctx context.Context, s Sample) error
	// Samples lists the samples from since on, oldest first.
	Samples(ctx context.Context, since time.Time) ([]Sample, error)
	// PruneSamples deletes the samples before the day and returns how many went.
	PruneSamples(ctx context.Context, before time.Time) (int64, error)
}
