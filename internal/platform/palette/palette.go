// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package palette is the fixed set of colours for machines and agents
// (round 33). The web client maps each name to theme tokens; the server
// stores and checks only the name, never a hex value.
package palette

// Hues in the order new machines get them.
var Hues = []string{"blue", "green", "orange", "violet", "teal", "pink", "amber", "cyan", "lime", "red"}

// Valid reports whether c is one of the hues.
func Valid(c string) bool {
	for _, h := range Hues {
		if h == c {
			return true
		}
	}
	return false
}

// Default is the colour of the n-th machine of a workspace (0-based).
func Default(n int) string {
	if n < 0 {
		n = 0
	}
	return Hues[n%len(Hues)]
}
