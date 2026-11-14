// Package memory is the in-memory activity store.
package memory

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"colonyhall/internal/modules/activity"
	"colonyhall/internal/platform/ids"
)

type sourceKey struct {
	recipient ids.ID
	seq       int64
	kind      activity.Kind
}

// Store implements activity.Store in memory.
type Store struct {
	mu    sync.Mutex
	items []activity.Item
	seen  map[sourceKey]bool
}

// New returns an empty store.
func New() *Store { return &Store{seen: map[sourceKey]bool{}} }

// Insert implements activity.Store.
func (s *Store) Insert(_ context.Context, it activity.Item) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := sourceKey{it.RecipientID, it.SourceSeq, it.Kind}
	if s.seen[k] {
		return false, nil
	}
	s.seen[k] = true
	s.items = append(s.items, it)
	return true, nil
}

func newestFirst(a, b activity.Item) int {
	if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
		return c
	}
	return bytes.Compare(b.ID[:], a.ID[:])
}

// List implements activity.Store.
func (s *Store) List(_ context.Context, workspaceID, recipientID ids.ID, q activity.Query) ([]activity.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []activity.Item
	for _, it := range s.items {
		if it.WorkspaceID != workspaceID || it.RecipientID != recipientID || (q.Kind != "" && it.Kind != q.Kind) || (q.UnreadOnly && it.ReadAt != nil) {
			continue
		}
		if q.After != nil && newestFirst(activity.Item{CreatedAt: q.After.At, ID: q.After.ID}, it) >= 0 {
			continue
		}
		out = append(out, it)
	}
	slices.SortFunc(out, newestFirst)
	if len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// CountUnread implements activity.Store.
func (s *Store) CountUnread(_ context.Context, workspaceID, recipientID ids.ID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, it := range s.items {
		if it.WorkspaceID == workspaceID && it.RecipientID == recipientID && it.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

// MarkRead implements activity.Store.
func (s *Store) MarkRead(_ context.Context, recipientID ids.ID, itemIDs []ids.ID, channelID ids.ID, at time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for i, it := range s.items {
		if it.RecipientID != recipientID || it.ReadAt != nil {
			continue
		}
		if len(itemIDs) > 0 && !slices.Contains(itemIDs, it.ID) {
			continue
		}
		if channelID != ids.Nil && it.ChannelID != channelID {
			continue
		}
		s.items[i].ReadAt = &at
		n++
	}
	return n, nil
}
