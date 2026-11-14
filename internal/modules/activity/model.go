// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package activity is the personal activity feed (round 55, part H): what
// concerns one person, built from the event log by a consumer. The REST
// view, the WebSocket badge and (round 43) push all read it.
package activity

import (
	"errors"
	"time"

	"colonyhall/internal/platform/ids"
)

// Kind is what an item is about.
type Kind string

// Kinds.
const (
	KindMention     Kind = "mention"
	KindThreadReply Kind = "thread_reply"
	KindReaction    Kind = "reaction"
	KindApproval    Kind = "approval"
	KindQuestion    Kind = "question"
	KindImportant   Kind = "important"
	KindReminder    Kind = "reminder"
)

// ValidKind reports whether k is a known kind.
func ValidKind(k Kind) bool {
	switch k {
	case KindMention, KindThreadReply, KindReaction, KindApproval, KindQuestion, KindImportant, KindReminder:
		return true
	}
	return false
}

// Item is one entry of a person's feed. The ids point at what it is about;
// RefID is the approval, question or saved item.
type Item struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	RecipientID  ids.ID
	Kind         Kind
	ChannelID    ids.ID
	MessageID    ids.ID
	ThreadRootID ids.ID
	ActorID      ids.ID
	RefID        ids.ID
	Emoji        string
	SourceSeq    int64
	CreatedAt    time.Time
	ReadAt       *time.Time
}

// Cursor pages the feed: items older than (At, ID).
type Cursor struct {
	At time.Time
	ID ids.ID
}

// Query narrows a page.
type Query struct {
	Kind       Kind // "" = every kind
	UnreadOnly bool
	After      *Cursor
	Limit      int
}

// Limits.
const (
	PageSize      = 30
	MaxPageSize   = 100
	ExcerptLength = 200
)

// Errors.
var (
	ErrForbidden = errors.New("the activity feed is for people only")
	ErrBadKind   = errors.New("unknown activity kind")
)
