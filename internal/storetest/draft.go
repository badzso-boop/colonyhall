// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/draft"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// testDrafts: one draft per actor, channel and thread (round 55, part A).
func testDrafts(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Drafts
	root := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: f.human.ID,
		Kind: "text", Body: "root", CreatedAt: t0}
	must(t, b.Conversation.InsertMessage(ctx, root))

	main, err := st.Upsert(ctx, draft.Draft{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, ChannelID: f.channel.ID,
		Body: "one", UpdatedAt: t0})
	must(t, err)
	// Saving the same place again replaces it and keeps the row.
	again, err := st.Upsert(ctx, draft.Draft{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, ChannelID: f.channel.ID,
		Body: "two", FileIDs: []ids.ID{gen.New()}, UpdatedAt: t0.Add(time.Minute)})
	must(t, err)
	if again.ID != main.ID || again.Body != "two" || len(again.FileIDs) != 1 {
		t.Fatalf("upsert: %+v (first %v)", again, main.ID)
	}
	_, err = st.Upsert(ctx, draft.Draft{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, ChannelID: f.channel.ID,
		ThreadRootID: root.ID, Body: "reply", UpdatedAt: t0.Add(2 * time.Minute)})
	must(t, err)
	_, err = st.Upsert(ctx, draft.Draft{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.agent.ID, ChannelID: f.channel.ID,
		Body: "not yours", UpdatedAt: t0})
	must(t, err)

	list, err := st.List(ctx, f.ws.ID, f.human.ID, 10)
	must(t, err)
	if len(list) != 2 || list[0].ThreadRootID != root.ID || list[1].ThreadRootID != ids.Nil {
		t.Fatalf("list, newest first: %+v", list)
	}
	got, err := st.Get(ctx, f.human.ID, f.channel.ID, root.ID)
	must(t, err)
	if got.Body != "reply" {
		t.Fatalf("get thread draft: %+v", got)
	}

	// A delete bounded by time leaves a newer draft.
	before := t0
	gone, err := st.Delete(ctx, f.human.ID, f.channel.ID, ids.Nil, &before)
	must(t, err)
	if gone {
		t.Fatal("a draft newer than the bound was deleted")
	}
	gone, err = st.Delete(ctx, f.human.ID, f.channel.ID, ids.Nil, nil)
	must(t, err)
	if !gone {
		t.Fatal("delete reported nothing")
	}
	if _, err := st.Get(ctx, f.human.ID, f.channel.ID, ids.Nil); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if _, err := st.Get(ctx, f.agent.ID, f.channel.ID, ids.Nil); err != nil {
		t.Fatalf("another actor's draft went too: %v", err)
	}
}
