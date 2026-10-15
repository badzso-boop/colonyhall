// Package storetest is the shared contract test suite for every Store
// implementation (Mattermost storetest pattern). The same tests run against
// the in-memory reference stores and against Postgres, which proves the two
// are interchangeable.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Backend bundles one implementation of every store.
type Backend struct {
	Tx           db.TxRunner
	Events       eventlog.Store
	Identity     identity.Store
	Conversation conversation.Store
	Delivery     delivery.Store
}

// Run executes the whole suite. newBackend must return empty stores.
func Run(t *testing.T, newBackend func(t *testing.T) Backend) {
	t.Run("eventlog", func(t *testing.T) { testEventlog(t, newBackend(t)) })
	t.Run("identity", func(t *testing.T) { testIdentity(t, newBackend(t)) })
	t.Run("conversation", func(t *testing.T) { testConversation(t, newBackend(t)) })
	t.Run("delivery", func(t *testing.T) { testDelivery(t, newBackend(t)) })
}

var (
	gen = &ids.Sequential{}
	t0  = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
)

type fixture struct {
	ws      identity.Workspace
	human   identity.Actor
	agent   identity.Actor
	channel conversation.Channel
}

func setup(t *testing.T, b Backend) fixture {
	t.Helper()
	ctx := context.Background()
	f := fixture{ws: identity.Workspace{ID: gen.New(), Name: "test", CreatedAt: t0}}
	f.human = identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindHuman, Handle: "norbi",
		DisplayName: "Norbi", Role: identity.RoleOwner, State: "active", CreatedAt: t0}
	f.agent = identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindAgent, Handle: "bot",
		DisplayName: "Bot", Role: identity.RoleMember, State: "active", CreatedAt: t0}
	f.channel = conversation.Channel{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: conversation.KindPublic,
		Name: "general", CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, b.Identity.CreateWorkspace(ctx, f.ws))
	must(t, b.Identity.CreateActor(ctx, f.human))
	must(t, b.Identity.CreateActor(ctx, f.agent))
	must(t, b.Conversation.CreateChannel(ctx, f.channel))
	must(t, b.Conversation.AddMember(ctx, conversation.Member{ChannelID: f.channel.ID, ActorID: f.human.ID, CanWrite: true, JoinedAt: t0}))
	must(t, b.Conversation.AddMember(ctx, conversation.Member{ChannelID: f.channel.ID, ActorID: f.agent.ID, CanWrite: true, JoinedAt: t0}))
	return f
}

func testEventlog(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	var seqs []int64
	for i := 0; i < 3; i++ {
		e, err := eventlog.NewEvent(gen.New(), f.ws.ID, "test.event", f.human.ID, f.channel.ID, "test", gen.New(), map[string]int{"n": i}, t0)
		must(t, err)
		must(t, b.Tx.InTx(ctx, func(ctx context.Context) error { return b.Events.Append(ctx, &e) }))
		seqs = append(seqs, e.Seq)
	}
	if seqs[0] >= seqs[1] || seqs[1] >= seqs[2] {
		t.Fatalf("sequence numbers must increase: %v", seqs)
	}
	got, err := b.Events.Since(ctx, seqs[0], 10)
	must(t, err)
	if len(got) != 2 || got[0].Seq != seqs[1] || got[0].ChannelID != f.channel.ID {
		t.Fatalf("Since: %+v", got)
	}
	head, err := b.Events.Head(ctx)
	must(t, err)
	if head != seqs[2] {
		t.Fatalf("Head = %d, want %d", head, seqs[2])
	}
	off, err := b.Events.Offset(ctx, "c")
	must(t, err)
	if off != 0 {
		t.Fatalf("new consumer offset = %d", off)
	}
	must(t, b.Events.SetOffset(ctx, "c", seqs[1]))
	must(t, b.Events.SetOffset(ctx, "c", seqs[2]))
	if off, _ = b.Events.Offset(ctx, "c"); off != seqs[2] {
		t.Fatalf("offset = %d, want %d", off, seqs[2])
	}
}

func testIdentity(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	dup := f.agent
	dup.ID = gen.New()
	if err := b.Identity.CreateActor(ctx, dup); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("duplicate handle: got %v, want ErrConflict", err)
	}
	got, err := b.Identity.ActorByHandle(ctx, f.ws.ID, "bot")
	must(t, err)
	if got.ID != f.agent.ID || got.Kind != identity.KindAgent {
		t.Fatalf("ActorByHandle: %+v", got)
	}
	must(t, b.Identity.CreateToken(ctx, identity.Token{ID: gen.New(), ActorID: f.agent.ID, Hash: identity.HashSecret("s3cret"), Label: "x", CreatedAt: t0}))
	if a, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("s3cret")); err != nil || a.ID != f.agent.ID {
		t.Fatalf("token lookup: %v %+v", err, a)
	}
	if _, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("other")); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("unknown token: got %v", err)
	}
	// Credentials and sessions.
	if _, err := b.Identity.Credential(ctx, f.human.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("no credential yet: got %v", err)
	}
	must(t, b.Identity.SetCredential(ctx, f.human.ID, "hash-1", t0))
	must(t, b.Identity.SetCredential(ctx, f.human.ID, "hash-2", t0))
	if h, err := b.Identity.Credential(ctx, f.human.ID); err != nil || h != "hash-2" {
		t.Fatalf("credential must be replaced: %q %v", h, err)
	}
	sess := identity.Session{ID: gen.New(), ActorID: f.human.ID, TokenHash: identity.HashSecret("cookie"),
		CSRFToken: "csrf", UserAgent: "test", CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}
	must(t, b.Identity.CreateSession(ctx, sess))
	got2, a2, err := b.Identity.SessionByTokenHash(ctx, identity.HashSecret("cookie"))
	must(t, err)
	if got2.ID != sess.ID || got2.CSRFToken != "csrf" || a2.ID != f.human.ID || !got2.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Fatalf("SessionByTokenHash: %+v %+v", got2, a2)
	}
	must(t, b.Identity.RevokeSession(ctx, sess.ID, t0))
	if _, _, err := b.Identity.SessionByTokenHash(ctx, identity.HashSecret("cookie")); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("revoked session still found: %v", err)
	}
	s1 := identity.Session{ID: gen.New(), ActorID: f.human.ID, TokenHash: identity.HashSecret("c1"), CSRFToken: "x", CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}
	s2 := identity.Session{ID: gen.New(), ActorID: f.human.ID, TokenHash: identity.HashSecret("c2"), CSRFToken: "x", CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}
	must(t, b.Identity.CreateSession(ctx, s1))
	must(t, b.Identity.CreateSession(ctx, s2))
	must(t, b.Identity.RevokeSessionsForActor(ctx, f.human.ID, s1.ID, t0))
	if _, _, err := b.Identity.SessionByTokenHash(ctx, identity.HashSecret("c1")); err != nil {
		t.Fatalf("kept session revoked: %v", err)
	}
	if _, _, err := b.Identity.SessionByTokenHash(ctx, identity.HashSecret("c2")); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("other session not revoked: %v", err)
	}
	byHandle, err := b.Identity.ActorsByHandle(ctx, "norbi")
	must(t, err)
	if len(byHandle) != 1 || byHandle[0].ID != f.human.ID {
		t.Fatalf("ActorsByHandle: %+v", byHandle)
	}
	if w, err := b.Identity.WorkspaceByName(ctx, "test"); err != nil || w.ID != f.ws.ID {
		t.Fatalf("WorkspaceByName: %+v %v", w, err)
	}

	list, err := b.Identity.ListActors(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 2 || list[0].Handle != "bot" {
		t.Fatalf("ListActors must be sorted by handle: %+v", list)
	}
}

func testConversation(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	var msgs []conversation.Message
	for i := 0; i < 3; i++ {
		m := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: f.human.ID,
			Kind: "text", Body: "m", Mentions: []ids.ID{f.agent.ID}, CreatedAt: t0}
		if i == 0 {
			m.ClientMsgID = "client-1"
		}
		if i == 2 {
			m.Mentions = nil // regression: a message without mentions must be storable
		}
		must(t, b.Conversation.InsertMessage(ctx, m))
		msgs = append(msgs, m)
	}
	dup := msgs[0]
	dup.ID = gen.New()
	if err := b.Conversation.InsertMessage(ctx, dup); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("duplicate client id: got %v", err)
	}
	byClient, err := b.Conversation.MessageByClientID(ctx, f.human.ID, "client-1")
	must(t, err)
	if byClient.ID != msgs[0].ID || len(byClient.Mentions) != 1 || byClient.Mentions[0] != f.agent.ID {
		t.Fatalf("MessageByClientID: %+v", byClient)
	}
	page, err := b.Conversation.ListMessages(ctx, f.channel.ID, ids.Nil, 2)
	must(t, err)
	if len(page) != 2 || page[0].ID != msgs[1].ID || page[1].ID != msgs[2].ID {
		t.Fatalf("newest page must be the last 2, oldest first: %+v", page)
	}
	older, err := b.Conversation.ListMessages(ctx, f.channel.ID, msgs[1].ID, 10)
	must(t, err)
	if len(older) != 1 || older[0].ID != msgs[0].ID {
		t.Fatalf("page before: %+v", older)
	}
	must(t, b.Conversation.BumpThread(ctx, msgs[0].ID, t0.Add(time.Minute)))
	root, err := b.Conversation.MessageByID(ctx, msgs[0].ID)
	must(t, err)
	if root.ReplyCount != 1 || root.LastReplyAt == nil {
		t.Fatalf("BumpThread: %+v", root)
	}
	for i := 0; i < 3; i++ {
		_, err := b.Conversation.IncrementMsgCount(ctx, f.channel.ID)
		must(t, err)
	}
	must(t, b.Conversation.IncrementMentions(ctx, f.channel.ID, []ids.ID{f.agent.ID}))
	must(t, b.Conversation.SetSeen(ctx, f.channel.ID, f.human.ID, 3))
	views, err := b.Conversation.ChannelsForActor(ctx, f.ws.ID, f.agent.ID, false)
	must(t, err)
	if len(views) != 1 || views[0].Unread != 3 || views[0].Mentions != 1 {
		t.Fatalf("agent view: %+v", views)
	}
	views, err = b.Conversation.ChannelsForActor(ctx, f.ws.ID, f.human.ID, false)
	must(t, err)
	if views[0].Unread != 0 {
		t.Fatalf("human has seen everything: %+v", views[0])
	}
	members, err := b.Conversation.MemberIDs(ctx, f.channel.ID)
	must(t, err)
	if len(members) != 2 {
		t.Fatalf("MemberIDs: %v", members)
	}
}

func testDelivery(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	item := gen.New()
	d := delivery.Delivery{ID: gen.New(), WorkspaceID: f.ws.ID, RecipientID: f.agent.ID, ItemKind: delivery.ItemMessage,
		ItemID: item, ChannelID: f.channel.ID, Flags: delivery.FlagUnread | delivery.FlagMentioned | delivery.FlagWake, EventSeq: 1, CreatedAt: t0}
	ok, err := b.Delivery.Insert(ctx, d)
	must(t, err)
	if !ok {
		t.Fatal("first insert must insert")
	}
	again := d
	again.ID = gen.New()
	if ok, err = b.Delivery.Insert(ctx, again); err != nil || ok {
		t.Fatalf("second insert for the same item must be a no-op: %v %v", ok, err)
	}
	pending, total, err := b.Delivery.Pending(ctx, f.agent.ID, 10)
	must(t, err)
	if total != 1 || len(pending) != 1 || !pending[0].Flags.Has(delivery.FlagWake) {
		t.Fatalf("Pending: %d %+v", total, pending)
	}
	if n, err := b.Delivery.Ack(ctx, f.human.ID, []ids.ID{d.ID}, t0); err != nil || n != 0 {
		t.Fatalf("someone else cannot ack: %d %v", n, err)
	}
	if n, err := b.Delivery.Ack(ctx, f.agent.ID, []ids.ID{d.ID}, t0); err != nil || n != 1 {
		t.Fatalf("ack: %d %v", n, err)
	}
	if _, total, _ = b.Delivery.Pending(ctx, f.agent.ID, 10); total != 0 {
		t.Fatalf("acked item still pending")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
