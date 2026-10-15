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
	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/hosts"
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
	Audit        audit.Store
	Hosts        hosts.Store
}

// Run executes the whole suite. newBackend must return empty stores.
func Run(t *testing.T, newBackend func(t *testing.T) Backend) {
	t.Run("eventlog", func(t *testing.T) { testEventlog(t, newBackend(t)) })
	t.Run("identity", func(t *testing.T) { testIdentity(t, newBackend(t)) })
	t.Run("conversation", func(t *testing.T) { testConversation(t, newBackend(t)) })
	t.Run("delivery", func(t *testing.T) { testDelivery(t, newBackend(t)) })
	t.Run("audit", func(t *testing.T) { testAudit(t, newBackend(t)) })
	t.Run("hosts", func(t *testing.T) { testHosts(t, newBackend(t)) })
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
	if a, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("s3cret"), t0); err != nil || a.ID != f.agent.ID {
		t.Fatalf("token lookup: %v %+v", err, a)
	}
	if _, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("other"), t0); !errors.Is(err, db.ErrNotFound) {
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

	// Tokens, actor state, emergency stop.
	tok := identity.Token{ID: gen.New(), ActorID: f.agent.ID, Hash: identity.HashSecret("t2"), Label: "laptop", CreatedAt: t0.Add(time.Second)}
	must(t, b.Identity.CreateToken(ctx, tok))
	must(t, b.Identity.RevokeToken(ctx, tok.ID, t0))
	if got, err := b.Identity.TokenByID(ctx, tok.ID); err != nil || got.RevokedAt == nil || got.Label != "laptop" {
		t.Fatalf("TokenByID after revoke: %+v %v", got, err)
	}
	if _, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("t2"), t0); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("revoked token still authenticates: %v", err)
	}
	toks, err := b.Identity.ListTokens(ctx, f.agent.ID)
	must(t, err)
	if len(toks) != 2 || toks[1].ID != tok.ID {
		t.Fatalf("ListTokens oldest first: %+v", toks)
	}
	must(t, b.Identity.SetActorState(ctx, f.agent.ID, identity.StatePaused))
	if a, _ := b.Identity.ActorByID(ctx, f.agent.ID); a.State != identity.StatePaused {
		t.Fatalf("state not saved: %q", a.State)
	}
	stopAt := t0.Add(time.Minute)
	must(t, b.Identity.SetEmergencyStop(ctx, f.ws.ID, &stopAt, f.human.ID))
	if w, _ := b.Identity.WorkspaceByID(ctx, f.ws.ID); w.EmergencyStopAt == nil || w.EmergencyStopBy != f.human.ID {
		t.Fatalf("emergency stop not saved: %+v", w)
	}
	must(t, b.Identity.SetEmergencyStop(ctx, f.ws.ID, nil, ids.Nil))
	if w, _ := b.Identity.WorkspaceByID(ctx, f.ws.ID); w.EmergencyStopAt != nil {
		t.Fatalf("emergency stop not cleared: %+v", w)
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
	testThreadsAndReactions(t, b, f, msgs)
}

func testThreadsAndReactions(t *testing.T, b Backend, f fixture, msgs []conversation.Message) {
	ctx := context.Background()
	reply := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: f.agent.ID,
		ThreadRootID: msgs[0].ID, Kind: "text", Body: "reply", CreatedAt: t0}
	must(t, b.Conversation.InsertMessage(ctx, reply))
	thread, err := b.Conversation.ThreadMessages(ctx, msgs[0].ID)
	must(t, err)
	if len(thread) != 2 || thread[0].ID != msgs[0].ID || thread[1].ID != reply.ID {
		t.Fatalf("ThreadMessages: %+v", thread)
	}
	last, err := b.Conversation.LastMessageNotBy(ctx, f.channel.ID, f.agent.ID)
	must(t, err)
	if last.ID != msgs[2].ID {
		t.Fatalf("LastMessageNotBy agent: %s", last.ID)
	}
	if _, err := b.Conversation.LastMessageNotBy(ctx, gen.New(), f.agent.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("LastMessageNotBy empty channel: %v", err)
	}

	r := conversation.Reaction{MessageID: msgs[0].ID, ActorID: f.agent.ID, Emoji: "👍", CreatedAt: t0}
	if ok, err := b.Conversation.AddReaction(ctx, r); err != nil || !ok {
		t.Fatalf("AddReaction: %v %v", ok, err)
	}
	if ok, err := b.Conversation.AddReaction(ctx, r); err != nil || ok {
		t.Fatalf("second AddReaction must be a no-op: %v %v", ok, err)
	}
	r2 := conversation.Reaction{MessageID: msgs[0].ID, ActorID: f.human.ID, Emoji: "👍", CreatedAt: t0.Add(time.Second)}
	if _, err := b.Conversation.AddReaction(ctx, r2); err != nil {
		t.Fatal(err)
	}
	got, err := b.Conversation.ReactionsFor(ctx, []ids.ID{msgs[0].ID, msgs[1].ID})
	must(t, err)
	if len(got) != 2 || got[0].ActorID != f.agent.ID || got[1].ActorID != f.human.ID {
		t.Fatalf("ReactionsFor: %+v", got)
	}
	if ok, err := b.Conversation.RemoveReaction(ctx, msgs[0].ID, f.agent.ID, "👍", t0); err != nil || !ok {
		t.Fatalf("RemoveReaction: %v %v", ok, err)
	}
	if ok, err := b.Conversation.RemoveReaction(ctx, msgs[0].ID, f.agent.ID, "👍", t0); err != nil || ok {
		t.Fatalf("second RemoveReaction must be a no-op: %v %v", ok, err)
	}
	got, _ = b.Conversation.ReactionsFor(ctx, []ids.ID{msgs[0].ID})
	if len(got) != 1 || got[0].ActorID != f.human.ID {
		t.Fatalf("after remove: %+v", got)
	}
	// A removed reaction can be added again.
	if ok, err := b.Conversation.AddReaction(ctx, r); err != nil || !ok {
		t.Fatalf("re-add: %v %v", ok, err)
	}
}

func testDelivery(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	item := gen.New()
	d := delivery.Delivery{ID: gen.New(), WorkspaceID: f.ws.ID, RecipientID: f.agent.ID, ItemKind: delivery.ItemMessage,
		ItemID: item, ChannelID: f.channel.ID, FromActorID: f.human.ID,
		Flags: delivery.FlagUnread | delivery.FlagMentioned | delivery.FlagWake, EventSeq: 1, CreatedAt: t0}
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
	if total != 1 || len(pending) != 1 || !pending[0].Flags.Has(delivery.FlagWake) || pending[0].FromActorID != f.human.ID {
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

func testAudit(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	svc := &audit.Service{Store: b.Audit}
	for i := 1; i <= 3; i++ {
		e, err := eventlog.NewEvent(gen.New(), f.ws.ID, "test.event", f.human.ID, ids.Nil, "test", gen.New(), map[string]int{"n": i}, t0.Add(time.Duration(i)*time.Nanosecond*1500))
		must(t, err)
		e.Seq = int64(i * 10)
		must(t, svc.HandleEvent(ctx, e))
		must(t, svc.HandleEvent(ctx, e)) // replay is a no-op
	}
	last, err := b.Audit.Last(ctx, f.ws.ID)
	must(t, err)
	if last.Seq != 30 {
		t.Fatalf("Last = %d", last.Seq)
	}
	page, err := b.Audit.Page(ctx, f.ws.ID, 30, 10)
	must(t, err)
	if len(page) != 2 || page[0].Seq != 20 {
		t.Fatalf("Page before 30, newest first: %+v", page)
	}
	fwd, err := b.Audit.Forward(ctx, f.ws.ID, 10, 10)
	must(t, err)
	if len(fwd) != 2 || fwd[0].Seq != 20 {
		t.Fatalf("Forward after 10: %+v", fwd)
	}
	// The stored chain verifies: hashes survive the round trip through the
	// store (timestamp precision, JSON normalisation).
	for i, e := range append([]audit.Entry{page[1]}, fwd...) {
		if string(e.ComputeHash()) != string(e.Hash) {
			t.Fatalf("entry %d (seq %d) does not verify after storage", i, e.Seq)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func testHosts(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	hs := b.Hosts

	code := hosts.PairingCode{ID: gen.New(), WorkspaceID: f.ws.ID, CodeHash: hosts.Hash("CODE1"), CreatedBy: f.human.ID,
		CreatedAt: t0, ExpiresAt: t0.Add(10 * time.Minute)}
	must(t, hs.CreatePairingCode(ctx, code))
	if _, err := hs.UsePairingCode(ctx, hosts.Hash("CODE1"), t0.Add(11*time.Minute)); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expired code: %v", err)
	}
	pc, err := hs.UsePairingCode(ctx, hosts.Hash("CODE1"), t0.Add(time.Minute))
	if err != nil || pc.CreatedBy != f.human.ID {
		t.Fatalf("use code: %+v %v", pc, err)
	}
	if _, err := hs.UsePairingCode(ctx, hosts.Hash("CODE1"), t0.Add(time.Minute)); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("a code works once: %v", err)
	}

	h := hosts.Host{ID: gen.New(), WorkspaceID: f.ws.ID, Name: "laptop", TokenHash: hosts.Hash("colh_x"), State: hosts.StateApproved,
		CreatedBy: f.human.ID, CreatedAt: t0, Report: hosts.Report{Roots: []string{}, Clients: []hosts.Client{}, Rejected: []hosts.Rejected{}}}
	must(t, hs.CreateHost(ctx, h))
	got, err := hs.HostByTokenHash(ctx, hosts.Hash("colh_x"))
	if err != nil || got.ID != h.ID || got.Name != "laptop" {
		t.Fatalf("HostByTokenHash: %+v %v", got, err)
	}
	rep := hosts.Report{GuardVersion: "1", Roots: []string{"/work"}, Clients: []hosts.Client{{Name: "claude-code", Version: "2"}},
		Sandbox: "bwrap 0.9.0", Rejected: []hosts.Rejected{{AgentID: "a", Path: "/etc", Reason: "outside roots"}}}
	must(t, hs.SaveReport(ctx, h.ID, rep, t0.Add(time.Minute)))
	must(t, hs.SetHostState(ctx, h.ID, hosts.StateRevoked))
	list, err := hs.ListHosts(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 1 || list[0].State != hosts.StateRevoked || list[0].LastSeenAt == nil ||
		list[0].Report.Sandbox != "bwrap 0.9.0" || len(list[0].Report.Rejected) != 1 || list[0].Report.Clients[0].Version != "2" {
		t.Fatalf("ListHosts: %+v", list)
	}

	a := hosts.Assignment{AgentID: f.agent.ID, HostID: h.ID, WorkspaceID: f.ws.ID, Client: "claude-code", Model: "haiku",
		WorkModel: "sonnet", AllowWork: true, CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, hs.SetAssignment(ctx, a))
	a.Model = "opus"
	must(t, hs.SetAssignment(ctx, a)) // replaces
	ga, err := hs.AssignmentByAgent(ctx, f.agent.ID)
	if err != nil || ga.Model != "opus" || !ga.AllowWork {
		t.Fatalf("AssignmentByAgent: %+v %v", ga, err)
	}
	as, err := hs.AssignmentsForHost(ctx, h.ID)
	if err != nil || len(as) != 1 {
		t.Fatalf("AssignmentsForHost: %+v %v", as, err)
	}

	g1 := hosts.Grant{ID: gen.New(), WorkspaceID: f.ws.ID, AgentID: f.agent.ID, HostID: h.ID, Path: "/work/b", Mode: hosts.ModeWrite, CreatedBy: f.human.ID, CreatedAt: t0}
	g2 := hosts.Grant{ID: gen.New(), WorkspaceID: f.ws.ID, AgentID: f.agent.ID, HostID: h.ID, Path: "/work/a", Mode: hosts.ModeRead, CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, hs.AddGrant(ctx, g1))
	must(t, hs.AddGrant(ctx, g2))
	gs, err := hs.ActiveGrants(ctx, h.ID, f.agent.ID)
	if err != nil || len(gs) != 2 || gs[0].Path != "/work/a" {
		t.Fatalf("ActiveGrants (by path): %+v %v", gs, err)
	}
	must(t, hs.RevokeGrant(ctx, g2.ID, t0))
	if gs, _ = hs.ActiveGrants(ctx, h.ID, f.agent.ID); len(gs) != 1 || gs[0].ID != g1.ID {
		t.Fatalf("after revoke: %+v", gs)
	}
	if g, err := hs.GrantByID(ctx, g2.ID); err != nil || g.RevokedAt == nil {
		t.Fatalf("revoked grants are kept: %+v %v", g, err)
	}
	must(t, hs.RevokeGrantsForAgent(ctx, f.agent.ID, t0))
	if gs, _ = hs.ActiveGrants(ctx, h.ID, f.agent.ID); len(gs) != 0 {
		t.Fatalf("after RevokeGrantsForAgent: %+v", gs)
	}
	must(t, hs.DeleteAssignment(ctx, f.agent.ID))
	if _, err := hs.AssignmentByAgent(ctx, f.agent.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("deleted assignment: %v", err)
	}

	// Host-bound, expiring agent tokens (identity store).
	exp := t0.Add(time.Hour)
	tok := identity.Token{ID: gen.New(), ActorID: f.agent.ID, Hash: identity.HashSecret("host-tok"), Label: "host", CreatedAt: t0, HostID: h.ID, ExpiresAt: &exp}
	must(t, b.Identity.CreateToken(ctx, tok))
	if _, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("host-tok"), t0.Add(time.Minute)); err != nil {
		t.Fatalf("valid host token: %v", err)
	}
	if _, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("host-tok"), t0.Add(2*time.Hour)); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expired host token: %v", err)
	}
	must(t, b.Identity.RevokeHostTokens(ctx, h.ID, ids.Nil, t0))
	if _, err := b.Identity.ActorByTokenHash(ctx, identity.HashSecret("host-tok"), t0.Add(time.Minute)); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("revoked host token: %v", err)
	}
}
