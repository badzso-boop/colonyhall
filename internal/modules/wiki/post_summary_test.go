// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package wiki

import (
	"testing"

	"colonyhall/internal/platform/ids"
)

func TestPostSummary(t *testing.T) {
	a, b, c := ids.ID{1}, ids.ID{2}, ids.ID{3}
	count, last, parts := PostSummary([]PostAuthor{
		{AuthorID: a, Posts: 1, LastRev: 1},
		{AuthorID: b, Posts: 3, LastRev: 7},
		{AuthorID: c, Posts: 2, LastRev: 4},
	})
	if count != 6 || last != b || len(parts) != 3 || parts[0] != b || parts[1] != c || parts[2] != a {
		t.Fatalf("got %d %v %v", count, last, parts)
	}
	if n, l, p := PostSummary(nil); n != 0 || l != ids.Nil || p != nil {
		t.Fatalf("empty: %d %v %v", n, l, p)
	}
	var many []PostAuthor
	for i := range 8 {
		many = append(many, PostAuthor{AuthorID: ids.ID{byte(i + 1)}, Posts: 1, LastRev: i + 1})
	}
	if _, _, p := PostSummary(many); len(p) != MaxParticipants || p[0] != (ids.ID{8}) {
		t.Fatalf("capped: %v", p)
	}
}
