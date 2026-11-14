// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package search

import (
	"context"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/platform/ids"
)

// Embedder turns texts into vectors (the Raspberry Pi's text model in
// production, a deterministic fake in tests). Model names the model and its
// version; vectors of different models are never compared.
type Embedder interface {
	Embed(ctx context.Context, texts []string) (vecs [][]float32, model string, err error)
}

// Reranker scores documents against a query with a cross-encoder (the
// homelab's reranker in production). Higher is more relevant.
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []string) (scores []float64, model string, err error)
}

// Conversation is what the search needs from the conversation module.
type Conversation interface {
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
	ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error)
	MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error)
	ChannelIDsOf(ctx context.Context, by identity.Actor, actorID ids.ID) ([]ids.ID, error)
	ListChannels(ctx context.Context, by identity.Actor, archived bool) ([]conversation.ChannelView, error)
}

// Directory resolves authors for headers and results.
type Directory interface {
	ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
}

// Wiki reads a page's current state for indexing (deleted pages too).
type Wiki interface {
	PageForIndex(ctx context.Context, workspaceID ids.ID, slug string) (wiki.Page, error)
}

// Decisions lists every decision of a workspace (the backfill: decisions
// recorded before decision.recorded existed have no event).
type Decisions interface {
	AllForIndex(ctx context.Context, workspaceID ids.ID) ([]decision.Decision, error)
}

// SummaryReader reads a compaction summary file's markdown.
type SummaryReader func(ctx context.Context, fileID ids.ID) (string, error)

// Clock is the time port.
type Clock interface{ Now() time.Time }
