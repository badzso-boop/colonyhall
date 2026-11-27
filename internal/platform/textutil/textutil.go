// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package textutil holds small text helpers shared by the modules.
package textutil

import "unicode/utf8"

// Clip keeps the first n runes of s and adds an ellipsis when it cut
// anything; a string of at most n runes comes back unchanged.
func Clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
