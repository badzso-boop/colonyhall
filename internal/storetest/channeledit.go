package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// testChannelEdit covers round 34: rename, archive, mutes, thread read state
// and acking a channel's deliveries.
func testChannelEdit(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)

	other := conversation.Channel{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: conversation.KindPublic,
		Name: "other", CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, b.Conversation.CreateChannel(ctx, other))

	// Rename and describe; a taken name is a conflict.
	must(t, b.Conversation.UpdateChannel(ctx, f.channel.ID, "renamed", "what it is for"))
	c, err := b.Conversation.ChannelByID(ctx, f.channel.ID)
	must(t, err)
	if c.Name != "renamed" || c.Description != "what it is for" {
		t.Fatalf("update not stored: %+v", c)
	}
	if err := b.Conversation.UpdateChannel(ctx, f.channel.ID, "other", ""); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("rename to a taken name: want ErrConflict, got %v", err)
	}

	// Write permission changes in place; only active members can change.
	must(t, b.Conversation.SetMemberCanWrite(ctx, f.channel.ID, f.agent.ID, false))
	if m, err := b.Conversation.Member(ctx, f.channel.ID, f.agent.ID); err != nil || m.CanWrite {
		t.Fatalf("read-only member: %+v %v", m, err)
	}
	must(t, b.Conversation.SetMemberCanWrite(ctx, f.channel.ID, f.agent.ID, true))
	if m, err := b.Conversation.Member(ctx, f.channel.ID, f.agent.ID); err != nil || !m.CanWrite || !m.JoinedAt.Equal(t0) {
		t.Fatalf("writable again: %+v %v", m, err)
	}
	if err := b.Conversation.SetMemberCanWrite(ctx, other.ID, f.agent.ID, false); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}

	// Lock against agents and unlock.
	lockedAt := t0.Add(time.Minute)
	must(t, b.Conversation.SetAgentLocked(ctx, f.channel.ID, &lockedAt, &f.human.ID))
	lc, err := b.Conversation.ChannelByID(ctx, f.channel.ID)
	must(t, err)
	if !lc.AgentLocked() || lc.AgentLockedBy == nil || *lc.AgentLockedBy != f.human.ID {
		t.Fatalf("lock not stored: %+v", lc)
	}
	locked, err := b.Conversation.AgentLockedChannels(ctx, f.ws.ID)
	must(t, err)
	if len(locked) != 1 || locked[0] != f.channel.ID {
		t.Fatalf("locked channels: %+v", locked)
	}
	lockedViews, err := b.Conversation.ChannelsForActor(ctx, f.ws.ID, f.human.ID, false)
	must(t, err)
	if len(lockedViews) != 1 || !lockedViews[0].AgentLocked() {
		t.Fatalf("channel list must carry agent_locked_at: %+v", lockedViews)
	}
	_, err = b.Delivery.Insert(ctx, delivery.Delivery{ID: gen.New(), WorkspaceID: f.ws.ID, RecipientID: f.agent.ID, ItemKind: delivery.ItemMessage, ItemID: gen.New(), ChannelID: f.channel.ID, Flags: delivery.FlagWake, CreatedAt: t0})
	must(t, err)
	if _, total, err := b.Delivery.PendingExcept(ctx, f.agent.ID, []ids.ID{f.channel.ID}, 10); err != nil || total != 0 {
		t.Fatalf("items of an excepted channel: %d %v", total, err)
	}
	if _, total, err := b.Delivery.PendingExcept(ctx, f.agent.ID, nil, 10); err != nil || total != 1 {
		t.Fatalf("items without exception: %d %v", total, err)
	}
	_, err = b.Delivery.AckByChannel(ctx, f.channel.ID, f.agent.ID, t0)
	must(t, err)
	must(t, b.Conversation.SetAgentLocked(ctx, f.channel.ID, nil, nil))

	// Archive and unarchive.
	at := t0.Add(time.Hour)
	must(t, b.Conversation.SetArchived(ctx, f.channel.ID, &at, &f.human.ID))
	c, err = b.Conversation.ChannelByID(ctx, f.channel.ID)
	must(t, err)
	if c.ArchivedAt == nil || !c.ArchivedAt.Equal(at) || c.ArchivedBy == nil || *c.ArchivedBy != f.human.ID {
		t.Fatalf("archive not stored: %+v", c)
	}
	views, err := b.Conversation.ChannelsForActor(ctx, f.ws.ID, f.human.ID, false)
	must(t, err)
	if len(views) != 1 || !views[0].Archived() {
		t.Fatalf("channel list must carry archived_at: %+v", views)
	}
	must(t, b.Conversation.SetArchived(ctx, f.channel.ID, nil, nil))
	c, err = b.Conversation.ChannelByID(ctx, f.channel.ID)
	must(t, err)
	if c.Archived() || c.ArchivedBy != nil {
		t.Fatalf("unarchive not stored: %+v", c)
	}

	// Mutes are per actor; setting again replaces.
	until := t0.Add(8 * time.Hour)
	must(t, b.Conversation.SetMute(ctx, conversation.Mute{ActorID: f.human.ID, ChannelID: f.channel.ID, Until: &until, CreatedAt: t0}))
	must(t, b.Conversation.SetMute(ctx, conversation.Mute{ActorID: f.human.ID, ChannelID: other.ID, CreatedAt: t0}))
	mutes, err := b.Conversation.Mutes(ctx, f.human.ID)
	must(t, err)
	if len(mutes) != 2 {
		t.Fatalf("want 2 mutes, got %+v", mutes)
	}
	if m, err := b.Conversation.Mutes(ctx, f.agent.ID); err != nil || len(m) != 0 {
		t.Fatalf("one actor's mute must not show for another: %+v %v", m, err)
	}
	must(t, b.Conversation.SetMute(ctx, conversation.Mute{ActorID: f.human.ID, ChannelID: f.channel.ID, CreatedAt: t0}))
	must(t, b.Conversation.DeleteMute(ctx, f.human.ID, other.ID))
	mutes, err = b.Conversation.Mutes(ctx, f.human.ID)
	must(t, err)
	if len(mutes) != 1 || mutes[0].ChannelID != f.channel.ID || mutes[0].Until != nil {
		t.Fatalf("mute replace/delete: %+v", mutes)
	}
	if !mutes[0].Active(t0.Add(1000 * time.Hour)) {
		t.Fatal("a mute without until never expires")
	}

	testMyThreads(t, b, f)

	// AckByChannel: one recipient's, then everyone's.
	for i, rcpt := range []ids.ID{f.agent.ID, f.human.ID} {
		_, err := b.Delivery.Insert(ctx, delivery.Delivery{ID: gen.New(), WorkspaceID: f.ws.ID, RecipientID: rcpt,
			ItemKind: delivery.ItemMessage, ItemID: gen.New(), ChannelID: f.channel.ID, Flags: delivery.FlagWake,
			EventSeq: int64(100 + i), CreatedAt: t0})
		must(t, err)
	}
	_, err = b.Delivery.Insert(ctx, delivery.Delivery{ID: gen.New(), WorkspaceID: f.ws.ID, RecipientID: f.agent.ID,
		ItemKind: delivery.ItemMessage, ItemID: gen.New(), ChannelID: other.ID, Flags: delivery.FlagWake, EventSeq: 102, CreatedAt: t0})
	must(t, err)
	if n, err := b.Delivery.AckByChannel(ctx, f.channel.ID, f.agent.ID, t0); err != nil || n != 1 {
		t.Fatalf("ack one recipient: n=%d err=%v", n, err)
	}
	if _, total, _ := b.Delivery.Pending(ctx, f.agent.ID, 10); total != 1 {
		t.Fatalf("the other channel's item must stay pending, got %d", total)
	}
	if n, err := b.Delivery.AckByChannel(ctx, f.channel.ID, ids.Nil, t0); err != nil || n != 1 {
		t.Fatalf("ack everyone: n=%d err=%v", n, err)
	}
}

// testMyThreads: participation by root, reply and mention; unread counting;
// paging; a channel the actor left drops out.
func testMyThreads(t *testing.T, b Backend, f fixture) {
	ctx := context.Background()
	third := identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindHuman, Handle: "anna",
		DisplayName: "Anna", Role: identity.RoleMember, State: "active", CreatedAt: t0}
	must(t, b.Identity.CreateActor(ctx, third))
	must(t, b.Conversation.AddMember(ctx, conversation.Member{ChannelID: f.channel.ID, ActorID: third.ID, CanWrite: true, JoinedAt: t0}))

	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	post := func(author ids.ID, root ids.ID, min int, mentions ...ids.ID) conversation.Message {
		m := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, ThreadRootID: root,
			AuthorID: author, Kind: "text", Body: "x", Mentions: mentions, CreatedAt: at(min)}
		must(t, b.Conversation.InsertMessage(ctx, m))
		if root != ids.Nil {
			must(t, b.Conversation.BumpThread(ctx, root, at(min)))
		}
		return m
	}
	// A: the human wrote the root; the agent replied twice.
	rootA := post(f.human.ID, ids.Nil, 1)
	post(f.agent.ID, rootA.ID, 2)
	post(f.agent.ID, rootA.ID, 3)
	// B: the agent's root; the human replied, then the agent once more.
	rootB := post(f.agent.ID, ids.Nil, 4)
	post(f.human.ID, rootB.ID, 5)
	post(f.agent.ID, rootB.ID, 6)
	// C: the human is only mentioned in a reply.
	rootC := post(third.ID, ids.Nil, 7)
	post(third.ID, rootC.ID, 8, f.human.ID)
	// D: not the human's thread; E: the human's root without replies.
	rootD := post(third.ID, ids.Nil, 9)
	post(f.agent.ID, rootD.ID, 10)
	post(f.human.ID, ids.Nil, 11)

	list, err := b.Conversation.ThreadsForActor(ctx, f.ws.ID, f.human.ID, nil, 10, false)
	must(t, err)
	got := map[ids.ID]conversation.ThreadSummary{}
	var order []ids.ID
	for _, ts := range list {
		got[ts.Root.ID] = ts
		order = append(order, ts.Root.ID)
	}
	if len(list) != 3 || order[0] != rootC.ID || order[1] != rootB.ID || order[2] != rootA.ID {
		t.Fatalf("want threads C, B, A newest first, got %v", order)
	}
	if got[rootA.ID].UnreadReplies != 2 || got[rootB.ID].UnreadReplies != 1 || got[rootC.ID].UnreadReplies != 1 {
		t.Fatalf("unread counts: A=%d B=%d C=%d", got[rootA.ID].UnreadReplies, got[rootB.ID].UnreadReplies, got[rootC.ID].UnreadReplies)
	}
	if !got[rootC.ID].Mentioned || got[rootA.ID].Mentioned {
		t.Fatal("mentioned flag wrong")
	}

	// Reading A to its end; reading never moves back.
	must(t, b.Conversation.SetThreadRead(ctx, f.human.ID, rootA.ID, at(3)))
	must(t, b.Conversation.SetThreadRead(ctx, f.human.ID, rootA.ID, at(2)))
	unread, err := b.Conversation.ThreadsForActor(ctx, f.ws.ID, f.human.ID, nil, 10, true)
	must(t, err)
	if len(unread) != 2 {
		t.Fatalf("after reading A want 2 unread threads, got %d", len(unread))
	}
	list, err = b.Conversation.ThreadsForActor(ctx, f.ws.ID, f.human.ID, nil, 10, false)
	must(t, err)
	for _, ts := range list {
		if ts.Root.ID == rootA.ID && (ts.UnreadReplies != 0 || ts.LastReadAt == nil || !ts.LastReadAt.Equal(at(3))) {
			t.Fatalf("thread A after read: %+v", ts)
		}
	}

	// Paging by cursor.
	page, err := b.Conversation.ThreadsForActor(ctx, f.ws.ID, f.human.ID, nil, 2, false)
	must(t, err)
	last := page[1].Root
	rest, err := b.Conversation.ThreadsForActor(ctx, f.ws.ID, f.human.ID,
		&conversation.ThreadCursor{LastReplyAt: *last.LastReplyAt, RootID: last.ID}, 2, false)
	must(t, err)
	if len(page) != 2 || len(rest) != 1 || rest[0].Root.ID != rootA.ID {
		t.Fatalf("paging: %d then %d", len(page), len(rest))
	}

	// Leaving the channel drops its threads.
	must(t, b.Conversation.RemoveMember(ctx, f.channel.ID, f.human.ID, at(20)))
	if err := b.Conversation.SetMemberCanWrite(ctx, f.channel.ID, f.human.ID, true); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("a member who left: want ErrNotFound, got %v", err)
	}
	list, err = b.Conversation.ThreadsForActor(ctx, f.ws.ID, f.human.ID, nil, 10, false)
	must(t, err)
	if len(list) != 0 {
		t.Fatalf("threads of a left channel must disappear, got %d", len(list))
	}
}
