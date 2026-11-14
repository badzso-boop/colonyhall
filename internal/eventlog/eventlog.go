// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package eventlog is the append-only, sequence-numbered record of every
// state change (transactional outbox). Services append events inside the
// same transaction as their data change; consumers and the WebSocket
// gateway read them back by sequence number.
//
// Sequence numbers are handed out by a single writer at a time (Synapse
// "stream writers" pattern), so they become visible in commit order and a
// reader that advances its position can never skip an event.
package eventlog

import (
	"context"
	"encoding/json"
	"time"

	"colonyhall/internal/platform/ids"
)

// Event is one recorded state change.
type Event struct {
	Seq           int64
	ID            ids.ID
	WorkspaceID   ids.ID
	Type          string
	ActorID       ids.ID
	ChannelID     ids.ID // ids.Nil when the event is not tied to a channel
	AggregateType string
	AggregateID   ids.ID
	Payload       json.RawMessage
	SchemaVersion int
	At            time.Time
}

// Store persists events and consumer positions.
type Store interface {
	// Append assigns e.Seq and stores e. It must run inside db.TxRunner.InTx.
	Append(ctx context.Context, e *Event) error
	// Since returns up to limit events with Seq > after, in Seq order.
	Since(ctx context.Context, after int64, limit int) ([]Event, error)
	// Head returns the highest committed Seq, or 0.
	Head(ctx context.Context) (int64, error)
	// Offset returns how far the named consumer has processed, or 0.
	Offset(ctx context.Context, consumer string) (int64, error)
	// SetOffset records the named consumer's position.
	SetOffset(ctx context.Context, consumer string, seq int64) error
}

// NewEvent builds an event with a JSON payload. Seq is assigned on Append.
func NewEvent(id ids.ID, workspaceID ids.ID, typ string, actorID, channelID ids.ID,
	aggregateType string, aggregateID ids.ID, payload any, at time.Time) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID: id, WorkspaceID: workspaceID, Type: typ, ActorID: actorID, ChannelID: channelID,
		AggregateType: aggregateType, AggregateID: aggregateID, Payload: raw,
		SchemaVersion: 1, At: at,
	}, nil
}
