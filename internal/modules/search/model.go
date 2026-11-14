// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package search is hybrid keyword and semantic search over chat messages,
// wiki pages, decisions and compaction summaries (round 36a,
// docs/11-rag-terv.md). The index is a projection of those sources, kept
// current by an event-log consumer; it is never primary data.
package search

import (
	"errors"
	"time"

	"colonyhall/internal/platform/ids"
)

// ConsumerName is the indexer's event-log consumer.
const ConsumerName = "search"

// SourceKind is what a chunk was cut from.
type SourceKind string

// Source kinds.
const (
	SourceMessage  SourceKind = "message"
	SourceWiki     SourceKind = "wiki"
	SourceDecision SourceKind = "decision"
	SourceSummary  SourceKind = "summary"
)

// Visibility says which access rule applies to a chunk.
type Visibility string

// Visibilities.
const (
	// VisibilityChannel: readers of the chunk's channel (membership; admins
	// every channel except other people's DMs).
	VisibilityChannel Visibility = "channel"
	// VisibilityWorkspace: everyone in the workspace (wiki, decisions).
	VisibilityWorkspace Visibility = "workspace"
)

// Mode selects the retrieval legs.
type Mode string

// Modes.
const (
	ModeKeyword  Mode = "keyword"
	ModeSemantic Mode = "semantic"
	ModeHybrid   Mode = "hybrid"
)

// Limits of a search.
const (
	DefaultLimit = 8
	MaxLimit     = 20
	// CandidateLimit is how many hits each leg contributes before fusion.
	CandidateLimit = 50
	// RRFK is the Reciprocal Rank Fusion constant.
	RRFK = 60
	// MaxQueryLen bounds the query text (characters).
	MaxQueryLen = 500
)

// KeywordWeight is the keyword leg's weight in the fusion (the semantic
// leg's is 1). The keyword words are OR-ed, so the leg matches almost any
// chunk sharing a common word; at equal weight that noise pushed good
// semantic hits down, and hybrid was worse than semantic alone. On the
// real eval set (32 questions, 2026-11-10) 0.2 gave the best result:
// hybrid MRR@10 0.44 → 0.54, recall@10 0.73 → 0.83; with the reranker (10
// candidates) MRR@10 0.70 → 0.79. A keyword hit still counts, so exact
// names and numbers are found (docs/11, "Mérés").
const KeywordWeight = 0.2

// Degraded flags say what a search had to do without.
const (
	DegradedNoEmbedder = "no_embedder"
	DegradedNoReranker = "no_reranker"
)

// Errors.
var (
	ErrEmptyQuery   = errors.New("search: the query is empty")
	ErrQueryTooLong = errors.New("search: the query is too long")
	ErrBadMode      = errors.New("search: unknown mode")
)

// Chunk is one indexed piece of a source.
type Chunk struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	SourceKind   SourceKind
	SourceID     ids.ID
	SourceRef    string // wiki slug, D-12; empty otherwise
	Part         int
	Visibility   Visibility
	ChannelID    ids.ID // ids.Nil when not channel-bound
	ThreadRootID ids.ID
	AuthorID     ids.ID
	SourceAt     time.Time
	Header       string
	Body         string
	ContentHash  string
	EmbedModel   string // "" until embedded
	IndexedAt    time.Time
}

// Access is the reader's view, computed by the service and applied by the
// store inside the query, before any ranking.
type Access struct {
	WorkspaceID ids.ID
	// ChannelIDs the reader may search (membership, plus for admins every
	// channel except other people's DMs). VisibilityWorkspace chunks are
	// always allowed.
	ChannelIDs []ids.ID
}

// Filters narrow a search.
type Filters struct {
	Kinds     []SourceKind
	ChannelID ids.ID
	AuthorID  ids.ID
	Since     *time.Time
	Until     *time.Time
}

// Candidate is a store hit with the leg's own score and rank (1-based).
type Candidate struct {
	Chunk   Chunk
	Score   float64
	Rank    int
	Snippet string // keyword leg: matched words highlighted with « »
}

// PendingChunk is a chunk waiting for its embedding.
type PendingChunk struct {
	ID     ids.ID
	Header string
	Body   string
}

// Query is a search request.
type Query struct {
	Text    string
	Mode    Mode
	Filters Filters
	Limit   int
	// NoRerank and NoLog are for the evaluation tool only.
	NoRerank bool
	NoLog    bool
}

// Hit is one search result.
type Hit struct {
	Chunk   Chunk
	Snippet string
	// Scores per stage; nil when the stage did not see the hit.
	Keyword  *float64
	Semantic *float64
	Fused    float64
	Rerank   *float64
}

// Result is what a search returns.
type Result struct {
	Hits     []Hit
	Mode     Mode
	Degraded []string
	Notice   string // short, human-readable when degraded
	Timings  map[string]int64
}

// LogEntry is one row of the search log (30 days, humans only).
type LogEntry struct {
	ID          ids.ID
	WorkspaceID ids.ID
	ActorID     ids.ID
	At          time.Time
	Query       string
	Mode        Mode
	ResultCount int
	ResultRefs  []string
	TimingsMS   map[string]int64
	Degraded    []string
}

// Stats is the index state for the status page.
type Stats struct {
	Chunks      int
	Pending     int
	LastEmbedAt *time.Time
	Models      []string
}
