// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of delivery.Store.
package memory

import (
	"bytes"
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/platform/ids"
)

type itemKey struct {
	recipient ids.ID
	kind      string
	item      ids.ID
}

// Store implements delivery.Store in memory.
type Store struct {
	mu             sync.Mutex
	rows           map[ids.ID]delivery.Delivery
	index          map[itemKey]ids.ID
	terminalEvents map[ids.ID]delivery.TerminalEvent
}

// New returns an empty store.
func New() *Store {
	return &Store{
		rows:           map[ids.ID]delivery.Delivery{},
		index:          map[itemKey]ids.ID{},
		terminalEvents: map[ids.ID]delivery.TerminalEvent{},
	}
}

// Insert stores a delivery once per (recipient, item).
func (s *Store) Insert(_ context.Context, d delivery.Delivery) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := itemKey{d.RecipientID, d.ItemKind, d.ItemID}
	if _, ok := s.index[k]; ok {
		return false, nil
	}
	s.index[k] = d.ID
	s.rows[d.ID] = d
	return true, nil
}

// Pending returns unacknowledged deliveries, oldest first.
func (s *Store) Pending(ctx context.Context, recipientID ids.ID, limit int) ([]delivery.Delivery, int, error) {
	return s.PendingExcept(ctx, recipientID, nil, limit)
}

// PendingExcept is Pending without the items of the given channels.
func (s *Store) PendingExcept(_ context.Context, recipientID ids.ID, except []ids.ID, limit int) ([]delivery.Delivery, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []delivery.Delivery
	for _, d := range s.rows {
		if d.RecipientID == recipientID && d.AckedAt == nil && !slices.Contains(except, d.ChannelID) {
			all = append(all, d)
		}
	}
	sort.Slice(all, func(i, j int) bool { return bytes.Compare(all[i].ID[:], all[j].ID[:]) < 0 })
	total := len(all)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, total, nil
}

// Ack marks deliveries as handled.
func (s *Store) Ack(_ context.Context, recipientID ids.ID, idList []ids.ID, at time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range idList {
		d, ok := s.rows[id]
		if !ok || d.RecipientID != recipientID || d.AckedAt != nil {
			continue
		}
		t := at
		d.AckedAt = &t
		s.rows[id] = d
		n++
	}
	return n, nil
}

// AckByItemID marks all deliveries for itemID as handled.
func (s *Store) AckByItemID(_ context.Context, itemID ids.ID, at time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, d := range s.rows {
		if d.ItemID == itemID && d.AckedAt == nil {
			t := at
			d.AckedAt = &t
			s.rows[id] = d
			n++
		}
	}
	return n, nil
}

// AckByChannel marks the channel's pending deliveries as handled.
func (s *Store) AckByChannel(_ context.Context, channelID, recipientID ids.ID, at time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, d := range s.rows {
		if d.ChannelID == channelID && d.AckedAt == nil && (recipientID == ids.Nil || d.RecipientID == recipientID) {
			t := at
			d.AckedAt = &t
			s.rows[id] = d
			n++
		}
	}
	return n, nil
}

// InsertTerminalEvent records a terminal event in memory.
func (s *Store) InsertTerminalEvent(_ context.Context, te delivery.TerminalEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminalEvents[te.ID] = te
	return nil
}

// TerminalEventsByIDs returns terminal events by IDs.
func (s *Store) TerminalEventsByIDs(_ context.Context, eventIDs []ids.ID) ([]delivery.TerminalEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []delivery.TerminalEvent
	for _, id := range eventIDs {
		if te, ok := s.terminalEvents[id]; ok {
			out = append(out, te)
		}
	}
	return out, nil
}
