// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// testPins: one active pin per message, unpinning keeps the row (round 55, part D).
func testPins(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Conversation
	msg := func(body string, important bool) conversation.Message {
		m := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: f.agent.ID,
			Kind: "text", Body: body, CreatedAt: t0, Important: important}
		must(t, st.InsertMessage(ctx, m))
		return m
	}
	a, c := msg("a", true), msg("b", false)
	got, err := st.MessageByID(ctx, a.ID)
	must(t, err)
	if !got.Important {
		t.Fatal("important is not stored")
	}
	pin := func(m conversation.Message, at time.Time) error {
		return st.InsertPin(ctx, conversation.Pin{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, MessageID: m.ID, PinnedBy: f.agent.ID, PinnedAt: at})
	}
	must(t, pin(a, t0))
	if err := pin(a, t0.Add(time.Second)); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("second active pin of one message: %v", err)
	}
	must(t, pin(c, t0.Add(time.Minute)))
	pins, err := st.ActivePins(ctx, f.channel.ID)
	must(t, err)
	if len(pins) != 2 || pins[0].MessageID != c.ID {
		t.Fatalf("active pins, newest first: %+v", pins)
	}
	among, err := st.PinnedAmong(ctx, []ids.ID{a.ID, c.ID, gen.New()})
	must(t, err)
	if !among[a.ID] || !among[c.ID] || len(among) != 2 {
		t.Fatalf("pinned among: %v", among)
	}

	changed, err := st.Unpin(ctx, a.ID, f.human.ID, t0.Add(2*time.Minute))
	must(t, err)
	if !changed {
		t.Fatal("unpin reported nothing")
	}
	if changed, _ := st.Unpin(ctx, a.ID, f.human.ID, t0.Add(3*time.Minute)); changed {
		t.Fatal("unpinned twice")
	}
	n, err := st.CountActivePins(ctx, f.channel.ID)
	must(t, err)
	if n != 1 {
		t.Fatalf("active after unpin: %d", n)
	}
	// The unpinned row still counts for the agent's daily limit.
	by, err := st.CountPinsBy(ctx, f.channel.ID, f.agent.ID, t0.Add(-time.Hour))
	must(t, err)
	if by != 2 {
		t.Fatalf("pins by the agent since an hour before: %d", by)
	}
	by, err = st.CountPinsBy(ctx, f.channel.ID, f.agent.ID, t0.Add(30*time.Second))
	must(t, err)
	if by != 1 {
		t.Fatalf("pins by the agent in the window: %d", by)
	}
	// Pinning again after an unpin works.
	must(t, pin(a, t0.Add(4*time.Minute)))
}

// testNotifyLevel: the mute row keeps its level (round 55, part I).
func testNotifyLevel(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Conversation
	must(t, st.SetMute(ctx, conversation.Mute{ActorID: f.human.ID, ChannelID: f.channel.ID, CreatedAt: t0, Level: conversation.NotifyAll}))
	mutes, err := st.Mutes(ctx, f.human.ID)
	must(t, err)
	if len(mutes) != 1 || mutes[0].Level != conversation.NotifyAll || mutes[0].Active(t0) {
		t.Fatalf("level all is no mute: %+v", mutes)
	}
	until := t0.Add(time.Hour)
	must(t, st.SetMute(ctx, conversation.Mute{ActorID: f.human.ID, ChannelID: f.channel.ID, Until: &until, CreatedAt: t0, Level: conversation.NotifyNone}))
	mutes, err = st.Mutes(ctx, f.human.ID)
	must(t, err)
	if len(mutes) != 1 || mutes[0].LevelAt(t0) != conversation.NotifyNone || mutes[0].LevelAt(t0.Add(2*time.Hour)) != "" {
		t.Fatalf("a timed mute: %+v", mutes)
	}
}
