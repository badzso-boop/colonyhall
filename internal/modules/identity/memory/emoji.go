// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package memory

import (
	"context"
	"sort"
	"time"

	"colonyhall/internal/platform/ids"
)

type emojiUse struct {
	emoji    string
	uses     int
	lastUsed time.Time
}

// byRecent orders newest last use first (ties: by emoji, as in Postgres).
func byRecent(a, b emojiUse) bool {
	if !a.lastUsed.Equal(b.lastUsed) {
		return a.lastUsed.After(b.lastUsed)
	}
	return a.emoji < b.emoji
}

func (s *Store) sortedEmoji(actorID ids.ID, less func(a, b emojiUse) bool) []emojiUse {
	out := make([]emojiUse, 0, len(s.emoji[actorID]))
	for _, u := range s.emoji[actorID] {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

// RecordEmojiUse counts one use of an emoji and keeps the actor's newest
// keep distinct emoji.
func (s *Store) RecordEmojiUse(_ context.Context, _, actorID ids.ID, emoji string, at time.Time, keep int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.emoji[actorID]
	if m == nil {
		m = map[string]emojiUse{}
		s.emoji[actorID] = m
	}
	u := m[emoji]
	u.emoji = emoji
	u.uses++
	if at.After(u.lastUsed) {
		u.lastUsed = at
	}
	m[emoji] = u
	for _, old := range s.sortedEmoji(actorID, byRecent)[min(keep, len(m)):] {
		delete(m, old.emoji)
	}
	return nil
}

// EmojiUsage returns the actor's recent and frequent emoji, at most limit each.
func (s *Store) EmojiUsage(_ context.Context, actorID ids.ID, limit int) (recent, frequent []string, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pick := func(list []emojiUse) []string {
		out := []string{}
		for _, u := range list[:min(limit, len(list))] {
			out = append(out, u.emoji)
		}
		return out
	}
	recent = pick(s.sortedEmoji(actorID, byRecent))
	frequent = pick(s.sortedEmoji(actorID, func(a, b emojiUse) bool {
		if a.uses != b.uses {
			return a.uses > b.uses
		}
		return byRecent(a, b)
	}))
	return recent, frequent, nil
}
