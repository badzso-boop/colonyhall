// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"testing"
	"time"

	"colonyhall/internal/modules/activity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// testActivity: a replayed event adds nothing, the feed pages newest first,
// and marking read is scoped to the recipient (round 55, part H).
func testActivity(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Activity
	add := func(seq int64, kind activity.Kind, at time.Time, channel ids.ID) bool {
		ok, err := st.Insert(ctx, activity.Item{ID: gen.New(), WorkspaceID: f.ws.ID, RecipientID: f.human.ID, Kind: kind,
			ChannelID: channel, MessageID: gen.New(), ActorID: f.agent.ID, SourceSeq: seq, CreatedAt: at})
		must(t, err)
		return ok
	}
	if !add(1, activity.KindMention, t0, f.channel.ID) || !add(1, activity.KindThreadReply, t0, f.channel.ID) || !add(2, activity.KindReaction, t0.Add(time.Second), ids.Nil) {
		t.Fatal("new items not added")
	}
	if add(1, activity.KindMention, t0, f.channel.ID) {
		t.Fatal("a replayed event added an item")
	}
	all, err := st.List(ctx, f.ws.ID, f.human.ID, activity.Query{Limit: 10})
	must(t, err)
	if len(all) != 3 || all[0].Kind != activity.KindReaction {
		t.Fatalf("newest first: %+v", all)
	}
	page, err := st.List(ctx, f.ws.ID, f.human.ID, activity.Query{Limit: 2})
	must(t, err)
	rest, err := st.List(ctx, f.ws.ID, f.human.ID, activity.Query{Limit: 2, After: &activity.Cursor{At: page[1].CreatedAt, ID: page[1].ID}})
	must(t, err)
	if len(page) != 2 || len(rest) != 1 || rest[0].ID == page[0].ID || rest[0].ID == page[1].ID {
		t.Fatalf("paging: %+v then %+v", page, rest)
	}
	if mentions, _ := st.List(ctx, f.ws.ID, f.human.ID, activity.Query{Kind: activity.KindMention, Limit: 10}); len(mentions) != 1 {
		t.Fatalf("kind filter: %+v", mentions)
	}
	// Another person's mark changes nothing here.
	if n, err := st.MarkRead(ctx, f.agent.ID, nil, ids.Nil, t0); err != nil || n != 0 {
		t.Fatalf("someone else's mark: %d %v", n, err)
	}
	n, err := st.MarkRead(ctx, f.human.ID, nil, f.channel.ID, t0)
	must(t, err)
	if n != 2 {
		t.Fatalf("mark a channel: %d", n)
	}
	unread, err := st.CountUnread(ctx, f.ws.ID, f.human.ID)
	must(t, err)
	if unread != 1 {
		t.Fatalf("unread: %d", unread)
	}
	if u, _ := st.List(ctx, f.ws.ID, f.human.ID, activity.Query{UnreadOnly: true, Limit: 10}); len(u) != 1 || u[0].Kind != activity.KindReaction {
		t.Fatalf("unread only: %+v", u)
	}
	n, err = st.MarkRead(ctx, f.human.ID, []ids.ID{all[0].ID}, ids.Nil, t0)
	must(t, err)
	if n != 1 {
		t.Fatalf("mark one: %d", n)
	}
}
