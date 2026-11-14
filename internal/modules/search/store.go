// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package search

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store is the search index's port. Every query applies Access inside the
// query itself, before ranking: a chunk the reader may not see is never a
// candidate.
type Store interface {
	// ReplaceSource makes chunks the source's whole set of chunks: parts
	// beyond len(chunks) are removed, and a part whose content hash is
	// unchanged keeps its embedding.
	ReplaceSource(ctx context.Context, kind SourceKind, sourceID ids.ID, chunks []Chunk) error
	// DeleteSource removes every chunk of a source.
	DeleteSource(ctx context.Context, kind SourceKind, sourceID ids.ID) error
	// DeleteSummaries removes the compaction summaries of a unit (a
	// channel, or a thread when threadRootID is set).
	DeleteSummaries(ctx context.Context, channelID, threadRootID ids.ID) error

	// PendingEmbeddings returns chunks without an embedding whose retry
	// time (if any) has come, oldest first.
	PendingEmbeddings(ctx context.Context, now time.Time, limit int) ([]PendingChunk, error)
	// SetEmbedding stores a chunk's vector; a chunk removed meanwhile is ignored.
	SetEmbedding(ctx context.Context, id ids.ID, vec []float32, model string, at time.Time) error
	// EmbedFailed counts a failed attempt and sets the next retry time.
	EmbedFailed(ctx context.Context, chunkIDs []ids.ID, retryAt time.Time) error
	// EmbedModels lists the models of the stored vectors (all workspaces).
	EmbedModels(ctx context.Context) ([]string, error)
	// ResetEmbeddings clears the embeddings of every model but keepModel,
	// so the worker re-embeds them; returns how many.
	ResetEmbeddings(ctx context.Context, keepModel string) (int, error)

	// Keyword is the full-text leg (Hungarian stemming and exact words).
	Keyword(ctx context.Context, acc Access, query string, f Filters, limit int) ([]Candidate, error)
	// Semantic is the vector leg: cosine similarity among chunks embedded
	// with model.
	Semantic(ctx context.Context, acc Access, vec []float32, model string, f Filters, limit int) ([]Candidate, error)

	InsertLog(ctx context.Context, e LogEntry) error
	// PurgeLogs removes log rows older than before; returns how many.
	PurgeLogs(ctx context.Context, before time.Time) (int, error)
	Stats(ctx context.Context, workspaceID ids.ID) (Stats, error)
}
