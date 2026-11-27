// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package textutil

import "testing"

func TestClip(t *testing.T) {
	for _, tc := range []struct {
		s    string
		n    int
		want string
	}{
		{"", 3, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc…"},
		{"árvíztűrő", 4, "árví…"}, // runes, not bytes
		{"ab", 0, "…"},
	} {
		if got := Clip(tc.s, tc.n); got != tc.want {
			t.Errorf("Clip(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
		}
	}
}
