// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"context"
	"errors"
	"unicode"
	"unicode/utf8"
)

// The emoji picker's recent and frequent (round 55, part C).
const (
	// MaxEmojiLen bounds an emoji, as for reactions (conversation.MaxEmojiLen).
	MaxEmojiLen = 32
	// EmojiUsageLimit is how many recent and frequent emoji GET /me/emoji returns.
	EmojiUsageLimit = 24
	// EmojiUsageKeep is how many distinct emoji are kept per person.
	EmojiUsageKeep = 100
)

// Errors.
var (
	ErrInvalidEmoji   = errors.New("emoji must be a short emoji sequence")
	ErrEmojiForbidden = errors.New("emoji usage is for people only")
)

// ValidEmoji is the reaction rule (conversation's validEmoji, which this
// module cannot import): at least one non-ASCII rune, no letters, spaces or
// controls, and no ASCII besides the keycap characters (0-9 # *).
func ValidEmoji(e string) bool {
	if e == "" || utf8.RuneCountInString(e) > MaxEmojiLen {
		return false
	}
	wide := false
	for _, r := range e {
		switch {
		case r >= utf8.RuneSelf:
			if unicode.IsControl(r) || unicode.IsSpace(r) || unicode.IsLetter(r) {
				return false
			}
			wide = true
		case r >= '0' && r <= '9', r == '#', r == '*':
		default:
			return false
		}
	}
	return wide
}

// RecordEmojiUse notes that the caller picked an emoji (people only).
func (s *Service) RecordEmojiUse(ctx context.Context, by Actor, emoji string) error {
	if by.Kind != KindHuman {
		return ErrEmojiForbidden
	}
	if !ValidEmoji(emoji) {
		return ErrInvalidEmoji
	}
	at := s.Clock.Now()
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		return s.Store.RecordEmojiUse(ctx, by.WorkspaceID, by.ID, emoji, at, EmojiUsageKeep)
	})
}

// EmojiUsage returns the caller's recent and frequent emoji (people only).
func (s *Service) EmojiUsage(ctx context.Context, by Actor) (recent, frequent []string, err error) {
	if by.Kind != KindHuman {
		return nil, nil, ErrEmojiForbidden
	}
	recent, frequent, err = s.Store.EmojiUsage(ctx, by.ID, EmojiUsageLimit)
	if err != nil {
		return nil, nil, err
	}
	if recent == nil {
		recent = []string{}
	}
	if frequent == nil {
		frequent = []string{}
	}
	return recent, frequent, nil
}
