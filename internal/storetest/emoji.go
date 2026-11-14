// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
)

// testEmojiUsage: the emoji picker's recent and frequent, per person
// (round 55, part C).
func testEmojiUsage(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Identity
	other := identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindHuman, Handle: "mari",
		DisplayName: "Mari", Role: identity.RoleMember, State: "active", CreatedAt: t0}
	must(t, st.CreateActor(ctx, other))
	use := func(who identity.Actor, emoji string, minute int) {
		t.Helper()
		must(t, b.Tx.InTx(ctx, func(ctx context.Context) error {
			return st.RecordEmojiUse(ctx, f.ws.ID, who.ID, emoji, t0.Add(time.Duration(minute)*time.Minute), 100)
		}))
	}

	if recent, frequent, err := st.EmojiUsage(ctx, f.human.ID, 24); err != nil || len(recent) != 0 || len(frequent) != 0 {
		t.Fatalf("empty: %v %v %v", recent, frequent, err)
	}
	use(f.human, "👍", 1)
	use(f.human, "🎉", 2)
	use(f.human, "👍", 3)
	use(f.human, "❤️", 4)
	use(f.human, "🎉", 5)
	use(f.human, "👍", 6)
	use(f.human, "🚀", 7)
	// An older use than the stored one counts but does not move it back.
	use(f.human, "🎉", 0)
	use(other, "😀", 8)

	recent, frequent, err := st.EmojiUsage(ctx, f.human.ID, 24)
	must(t, err)
	// 👍 3 uses (last 6), 🎉 3 uses (last 5), ❤️ 1 (4), 🚀 1 (7).
	if want := []string{"🚀", "👍", "🎉", "❤️"}; !slices.Equal(recent, want) {
		t.Fatalf("recent: %v, want %v", recent, want)
	}
	if want := []string{"👍", "🎉", "🚀", "❤️"}; !slices.Equal(frequent, want) {
		t.Fatalf("frequent: %v, want %v", frequent, want)
	}
	if recent, frequent, err := st.EmojiUsage(ctx, other.ID, 24); err != nil || !slices.Equal(recent, []string{"😀"}) || !slices.Equal(frequent, []string{"😀"}) {
		t.Fatalf("another person's: %v %v %v", recent, frequent, err)
	}

	// The limit, and pruning beyond keep: 130 distinct emoji, newest kept.
	for i := 0; i < 130; i++ {
		must(t, b.Tx.InTx(ctx, func(ctx context.Context) error {
			return st.RecordEmojiUse(ctx, f.ws.ID, f.human.ID, fmt.Sprintf("%d⃣", i), t0.Add(time.Hour+time.Duration(i)*time.Minute), 100)
		}))
	}
	recent, frequent, err = st.EmojiUsage(ctx, f.human.ID, 24)
	must(t, err)
	if len(recent) != 24 || recent[0] != "129⃣" || recent[23] != "106⃣" {
		t.Fatalf("recent after many: %v", recent)
	}
	if len(frequent) != 24 {
		t.Fatalf("frequent after many: %v", frequent)
	}
	all, _, err := st.EmojiUsage(ctx, f.human.ID, 1000)
	must(t, err)
	if len(all) != 100 || all[99] != "30⃣" || slices.Contains(all, "👍") {
		t.Fatalf("kept %d, oldest %q: the newest 100 must stay", len(all), all[len(all)-1])
	}
	// The other person's emoji are untouched by the pruning.
	if recent, _, err := st.EmojiUsage(ctx, other.ID, 24); err != nil || len(recent) != 1 {
		t.Fatalf("another person's after pruning: %v %v", recent, err)
	}
}
