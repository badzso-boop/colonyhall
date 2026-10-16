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
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/runs"
	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/modules/wiki"
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
	Runs         runs.Store
	Schedule     schedule.Store
	Approval     approval.Store
	Wiki         wiki.Store
}

// Run executes the whole suite. newBackend must return empty stores.
func Run(t *testing.T, newBackend func(t *testing.T) Backend) {
	t.Run("eventlog", func(t *testing.T) { testEventlog(t, newBackend(t)) })
	t.Run("identity", func(t *testing.T) { testIdentity(t, newBackend(t)) })
	t.Run("conversation", func(t *testing.T) { testConversation(t, newBackend(t)) })
	t.Run("delivery", func(t *testing.T) { testDelivery(t, newBackend(t)) })
	t.Run("audit", func(t *testing.T) { testAudit(t, newBackend(t)) })
	t.Run("hosts", func(t *testing.T) { testHosts(t, newBackend(t)) })
	t.Run("runs", func(t *testing.T) { testRuns(t, newBackend(t)) })
	t.Run("schedule", func(t *testing.T) { testSchedule(t, newBackend(t)) })
	t.Run("approval", func(t *testing.T) { testApproval(t, newBackend(t)) })
	t.Run("wiki", func(t *testing.T) { testWiki(t, newBackend(t)) })
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
		Sandbox: "bwrap 0.9.0", AllowUnsandboxed: true, ExchangeDir: "/mnt/z/exchange", ExchangeOK: true, Rejected: []hosts.Rejected{{AgentID: "a", Path: "/etc", Reason: "outside roots"}}}
	must(t, hs.SaveReport(ctx, h.ID, rep, t0.Add(time.Minute)))
	must(t, hs.SetHostState(ctx, h.ID, hosts.StateRevoked))
	list, err := hs.ListHosts(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 1 || list[0].State != hosts.StateRevoked || list[0].LastSeenAt == nil ||
		list[0].Report.Sandbox != "bwrap 0.9.0" || !list[0].Report.AllowUnsandboxed ||
		list[0].Report.ExchangeDir != "/mnt/z/exchange" || !list[0].Report.ExchangeOK ||
		len(list[0].Report.Rejected) != 1 || list[0].Report.Clients[0].Version != "2" {
		t.Fatalf("ListHosts: %+v", list)
	}

	a := hosts.Assignment{
		AgentID: f.agent.ID, HostID: h.ID, WorkspaceID: f.ws.ID, Client: "claude-code", Model: "haiku",
		WorkModel: "sonnet", AllowWork: true, Mode: hosts.AgentModeDirect, RootDir: "/work", Sandbox: false,
		Approvals: hosts.ApprovalsChat, MaxSessionTokens: 120000, DailyTokenLimit: 500000, CreatedBy: f.human.ID, CreatedAt: t0,
	}
	must(t, hs.SetAssignment(ctx, a))
	ga, err := hs.AssignmentByAgent(ctx, f.agent.ID)
	if err != nil || ga.Approvals != hosts.ApprovalsChat || ga.MaxSessionTokens != 120000 || ga.DailyTokenLimit != 500000 {
		t.Fatalf("approvals / limits: %+v %v", ga, err)
	}
	a.Model = "opus"
	a.Mode = hosts.AgentModeCompact
	a.Sandbox = true
	a.Approvals = hosts.ApprovalsNone
	a.MaxSessionTokens = 80000
	a.DailyTokenLimit = 250000
	must(t, hs.SetAssignment(ctx, a)) // replaces
	ga, err = hs.AssignmentByAgent(ctx, f.agent.ID)
	if err != nil || ga.Model != "opus" || !ga.AllowWork || ga.Mode != hosts.AgentModeCompact || ga.RootDir != "/work" || !ga.Sandbox || ga.Approvals != hosts.ApprovalsNone || ga.MaxSessionTokens != 80000 || ga.DailyTokenLimit != 250000 {
		t.Fatalf("AssignmentByAgent: %+v %v", ga, err)
	}
	as, err := hs.AssignmentsForHost(ctx, h.ID)
	if err != nil || len(as) != 1 || as[0].Approvals != hosts.ApprovalsNone || as[0].MaxSessionTokens != 80000 || as[0].DailyTokenLimit != 250000 {
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

func testRuns(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	e := runs.Error{ID: gen.New(), WorkspaceID: f.ws.ID, AgentID: f.agent.ID, ChannelID: f.channel.ID,
		Summary: "provider unavailable", Detail: "503 from agy", CreatedAt: t0}
	must(t, b.Runs.InsertError(ctx, e))
	got, err := b.Runs.ErrorByID(ctx, e.ID)
	if err != nil || got.Detail != "503 from agy" || got.ThreadRootID != ids.Nil || got.AgentID != f.agent.ID {
		t.Fatalf("ErrorByID: %+v %v", got, err)
	}
	withThread := e
	withThread.ID, withThread.ThreadRootID = gen.New(), gen.New()
	must(t, b.Runs.InsertError(ctx, withThread))
	if got, _ := b.Runs.ErrorByID(ctx, withThread.ID); got.ThreadRootID != withThread.ThreadRootID {
		t.Fatalf("thread root: %+v", got)
	}
	if _, err := b.Runs.ErrorByID(ctx, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}

	// Insert runs and test Budapest midnight boundary (CEST = UTC+2)
	// 2026-10-15 21:59:59 UTC = 2026-10-15 23:59:59 CEST (day "2026-10-15")
	tDay1 := time.Date(2026, 10, 15, 21, 59, 59, 0, time.UTC)
	r1 := runs.Run{
		ID:               gen.New(),
		WorkspaceID:      f.ws.ID,
		AgentID:          f.agent.ID,
		Phase:            runs.PhaseWork,
		Client:           "claude-code",
		Model:            "claude-3-7-sonnet",
		SessionID:        "sess-1",
		ChannelID:        f.channel.ID,
		Items:            1,
		InputTokens:      100,
		OutputTokens:     50,
		CacheReadTokens:  10,
		CacheWriteTokens: 5,
		DurationMS:       1200,
		Status:           runs.StatusOk,
		CreatedAt:        tDay1,
	}
	must(t, b.Runs.InsertRun(ctx, r1))

	// 2026-10-15 22:00:01 UTC = 2026-10-16 00:00:01 CEST (day "2026-10-16")
	tDay2 := time.Date(2026, 10, 15, 22, 0, 1, 0, time.UTC)
	r2 := runs.Run{
		ID:               gen.New(),
		WorkspaceID:      f.ws.ID,
		AgentID:          f.agent.ID,
		Phase:            runs.PhaseDecide,
		Client:           "claude-code",
		Model:            "claude-3-7-sonnet",
		SessionID:        "sess-2",
		Items:            2,
		InputTokens:      200,
		OutputTokens:     100,
		CacheReadTokens:  20,
		CacheWriteTokens: 10,
		DurationMS:       800,
		Status:           runs.StatusError,
		ErrorID:          e.ID,
		CreatedAt:        tDay2,
	}
	must(t, b.Runs.InsertRun(ctx, r2))

	otherAgent := identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindAgent, Handle: "other-agent", DisplayName: "Other Agent", Role: identity.RoleMember, State: "active", CreatedAt: t0}
	must(t, b.Identity.CreateActor(ctx, otherAgent))

	r3 := runs.Run{
		ID:           gen.New(),
		WorkspaceID:  f.ws.ID,
		AgentID:      otherAgent.ID,
		Phase:        runs.PhaseDirect,
		Client:       "antigravity",
		Model:        "claude-opus-4-6-thinking",
		SessionID:    "sess-3",
		Items:        1,
		InputTokens:  500,
		OutputTokens: 300,
		DurationMS:   5000,
		Status:       runs.StatusOk,
		CreatedAt:    tDay2.Add(time.Minute),
	}
	must(t, b.Runs.InsertRun(ctx, r3))

	// Test ListRuns
	allRuns, err := b.Runs.ListRuns(ctx, f.ws.ID, ids.Nil, 10)
	if err != nil || len(allRuns) != 3 {
		t.Fatalf("ListRuns all: len=%d err=%v", len(allRuns), err)
	}
	if allRuns[0].ID != r3.ID || allRuns[1].ID != r2.ID || allRuns[2].ID != r1.ID {
		t.Fatalf("ListRuns ordering: got [%s, %s, %s]", allRuns[0].ID, allRuns[1].ID, allRuns[2].ID)
	}
	if allRuns[1].ErrorID != e.ID {
		t.Fatalf("ListRuns error id: got %s want %s", allRuns[1].ErrorID, e.ID)
	}

	agentRuns, err := b.Runs.ListRuns(ctx, f.ws.ID, f.agent.ID, 10)
	if err != nil || len(agentRuns) != 2 {
		t.Fatalf("ListRuns filtered: len=%d err=%v", len(agentRuns), err)
	}
	if agentRuns[0].ID != r2.ID || agentRuns[1].ID != r1.ID {
		t.Fatalf("ListRuns filter order: got [%s, %s]", agentRuns[0].ID, agentRuns[1].ID)
	}

	limRuns, err := b.Runs.ListRuns(ctx, f.ws.ID, ids.Nil, 1)
	if err != nil || len(limRuns) != 1 || limRuns[0].ID != r3.ID {
		t.Fatalf("ListRuns limit: len=%d", len(limRuns))
	}

	// Test Usage with Budapest midnight boundary
	// Query since start of 2026-10-15: should contain both days
	usageAll, err := b.Runs.Usage(ctx, f.ws.ID, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	// Expected 3 records:
	// Day 2026-10-16: f.agent and otherAgent (sorted by agent_id ASC)
	// Day 2026-10-15: f.agent
	if len(usageAll) != 3 {
		t.Fatalf("Usage count: got %d want 3", len(usageAll))
	}
	if usageAll[0].Day != "2026-10-16" || usageAll[1].Day != "2026-10-16" || usageAll[2].Day != "2026-10-15" {
		t.Fatalf("Usage day ordering: %s, %s, %s", usageAll[0].Day, usageAll[1].Day, usageAll[2].Day)
	}

	// Check day 15 usage for f.agent
	u15 := usageAll[2]
	if u15.AgentID != f.agent.ID || u15.InputTokens != 100 || u15.OutputTokens != 50 || u15.TotalTokens != 165 || u15.RunCount != 1 || u15.ErrorCount != 0 {
		t.Fatalf("u15 mismatch: %+v", u15)
	}

	// Check day 16 error count for f.agent
	var uAgent16 runs.DayUsage
	for _, u := range usageAll[:2] {
		if u.AgentID == f.agent.ID {
			uAgent16 = u
			break
		}
	}
	if uAgent16.InputTokens != 200 || uAgent16.OutputTokens != 100 || uAgent16.TotalTokens != 330 || uAgent16.RunCount != 1 || uAgent16.ErrorCount != 1 {
		t.Fatalf("uAgent16 mismatch: %+v", uAgent16)
	}

	// Query since 2026-10-15 22:00:00 UTC (Budapest midnight of day 16): day 15 must be excluded!
	sinceDay16 := time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC)
	usageDay16Only, err := b.Runs.Usage(ctx, f.ws.ID, sinceDay16)
	if err != nil {
		t.Fatalf("Usage sinceDay16: %v", err)
	}
	if len(usageDay16Only) != 2 {
		t.Fatalf("Usage sinceDay16 count: got %d want 2", len(usageDay16Only))
	}
	for _, u := range usageDay16Only {
		if u.Day != "2026-10-16" {
			t.Fatalf("Usage sinceDay16 day: got %s want 2026-10-16", u.Day)
		}
	}

	// Test TokensSince:
	// f.agent on day 16 (sinceDay16): r2 had 200 + 100 + 20 + 10 = 330 tokens
	tokDay16, err := b.Runs.TokensSince(ctx, f.ws.ID, f.agent.ID, sinceDay16)
	if err != nil || tokDay16 != 330 {
		t.Fatalf("TokensSince f.agent day16: got %d want 330 err=%v", tokDay16, err)
	}
	// otherAgent on day 16: r3 had 500 + 300 = 800 tokens
	tokOther, err := b.Runs.TokensSince(ctx, f.ws.ID, otherAgent.ID, sinceDay16)
	if err != nil || tokOther != 800 {
		t.Fatalf("TokensSince otherAgent day16: got %d want 800 err=%v", tokOther, err)
	}
	// f.agent since tDay1: r1 (165) + r2 (330) = 495 tokens
	tokAll, err := b.Runs.TokensSince(ctx, f.ws.ID, f.agent.ID, tDay1)
	if err != nil || tokAll != 495 {
		t.Fatalf("TokensSince f.agent all: got %d want 495 err=%v", tokAll, err)
	}
	// Unused agent returns 0
	tokNone, err := b.Runs.TokensSince(ctx, f.ws.ID, gen.New(), tDay1)
	if err != nil || tokNone != 0 {
		t.Fatalf("TokensSince unused: got %d want 0 err=%v", tokNone, err)
	}
}

func testSchedule(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Schedule
	at := t0.Add(time.Hour)
	sc := schedule.Schedule{ID: gen.New(), WorkspaceID: f.ws.ID, CreatedBy: f.human.ID, ChannelID: f.channel.ID, Body: "hi",
		Cron: "0 1 * * *", Timezone: "Europe/Budapest", CatchUp: schedule.CatchUpOnce, State: schedule.StateActive, NextRunAt: at, CreatedAt: t0}
	must(t, st.Create(ctx, sc))
	other := sc
	other.ID, other.NextRunAt, other.ThreadRootID = gen.New(), t0.Add(3*time.Hour), gen.New()
	must(t, st.Create(ctx, other))
	if due, _ := st.Due(ctx, t0, 10); len(due) != 0 {
		t.Fatalf("nothing due yet: %+v", due)
	}
	due, err := st.Due(ctx, t0.Add(time.Hour), 10)
	if err != nil || len(due) != 1 || due[0].ID != sc.ID {
		t.Fatalf("Due: %+v %v", due, err)
	}
	next := at.Add(24 * time.Hour)
	if ok, err := st.Claim(ctx, sc.ID, at, next, schedule.StateActive, at); err != nil || !ok {
		t.Fatalf("Claim: %v %v", ok, err)
	}
	if ok, _ := st.Claim(ctx, sc.ID, at, next, schedule.StateActive, at); ok {
		t.Fatal("a run can be claimed once")
	}
	msg := gen.New()
	must(t, st.Record(ctx, sc.ID, msg, ""))
	got, err := st.ByID(ctx, sc.ID)
	if err != nil || !got.NextRunAt.Equal(next) || got.RunCount != 1 || got.LastMessageID != msg || got.LastRunAt == nil {
		t.Fatalf("after claim: %+v %v", got, err)
	}
	must(t, st.SetState(ctx, other.ID, schedule.StateCanceled))
	if due, _ := st.Due(ctx, t0.Add(10*time.Hour), 10); len(due) != 0 {
		t.Fatalf("canceled and future schedules are not due: %+v", due)
	}
	list, err := st.List(ctx, f.ws.ID)
	if err != nil || len(list) != 2 || list[0].ThreadRootID != other.ThreadRootID { // by next run
		t.Fatalf("List: %+v %v", list, err)
	}
	if _, err := st.ByID(ctx, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func testApproval(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Approval

	now := t0
	req := approval.ApprovalRequest{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		AgentID:     f.agent.ID,
		ChannelID:   f.channel.ID,
		ToolName:    "Bash",
		Input:       `{"command":"ls"}`,
		State:       approval.StatePending,
		CreatedAt:   now,
		ExpiresAt:   now.Add(10 * time.Minute),
	}
	must(t, st.Create(ctx, req))

	// ByID
	got, err := st.ByID(ctx, req.ID)
	if err != nil || got.ToolName != "Bash" || got.State != approval.StatePending || got.ThreadRootID != ids.Nil {
		t.Fatalf("ByID: %+v %v", got, err)
	}

	// ListPendingByWorkspace
	pending, err := st.ListPendingByWorkspace(ctx, f.ws.ID)
	if err != nil || len(pending) != 1 || pending[0].ID != req.ID {
		t.Fatalf("ListPendingByWorkspace: %+v %v", pending, err)
	}

	// Decide allow
	decAt := now.Add(time.Minute)
	must(t, st.Decide(ctx, req.ID, approval.StateAllowed, f.human.ID, "ok", decAt))

	got, err = st.ByID(ctx, req.ID)
	if err != nil || got.State != approval.StateAllowed || got.DecidedBy != f.human.ID || got.DecisionMessage != "ok" || got.DecidedAt == nil {
		t.Fatalf("after allow: %+v %v", got, err)
	}

	// Deciding again fails with ErrAlreadyDecided
	if err := st.Decide(ctx, req.ID, approval.StateDenied, f.human.ID, "no", decAt); !errors.Is(err, approval.ErrAlreadyDecided) {
		t.Fatalf("expected ErrAlreadyDecided, got %v", err)
	}

	// Now list pending should be empty
	pending, err = st.ListPendingByWorkspace(ctx, f.ws.ID)
	if err != nil || len(pending) != 0 {
		t.Fatalf("expected 0 pending, got %d", len(pending))
	}

	// With thread_root_id and emergency stop expire
	rootID := gen.New()
	req2 := req
	req2.ID = gen.New()
	req2.ThreadRootID = rootID
	req2.State = approval.StatePending
	must(t, st.Create(ctx, req2))

	got2, err := st.ByID(ctx, req2.ID)
	if err != nil || got2.ThreadRootID != rootID {
		t.Fatalf("thread root ID: %+v", got2)
	}

	// ExpirePendingByWorkspace
	n, err := st.ExpirePendingByWorkspace(ctx, f.ws.ID, decAt)
	if err != nil || n != 1 {
		t.Fatalf("ExpirePendingByWorkspace: %d %v", n, err)
	}
	got2, err = st.ByID(ctx, req2.ID)
	if err != nil || got2.State != approval.StateExpired || got2.DecisionMessage != "emergency stop" {
		t.Fatalf("after expire: %+v", got2)
	}
}

func testWiki(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Wiki

	pID := gen.New()
	page := wiki.Page{
		ID:          pID,
		WorkspaceID: f.ws.ID,
		Slug:        "welcome",
		Title:       "Welcome Page",
		Tags:        []string{"intro", "general"},
		CreatedBy:   f.human.ID,
		CreatedAt:   t0,
	}
	rev1 := wiki.Revision{
		PageID:    pID,
		Rev:       1,
		Body:      "Initial content",
		Kind:      wiki.RevisionCreate,
		AuthorID:  f.human.ID,
		CreatedAt: t0,
	}
	must(t, st.CreatePage(ctx, page, rev1))

	// Duplicate slug in workspace returns ErrConflict
	dup := page
	dup.ID = gen.New()
	if err := st.CreatePage(ctx, dup, rev1); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate slug, got: %v", err)
	}

	// PageBySlug
	got, err := st.PageBySlug(ctx, f.ws.ID, "welcome")
	must(t, err)
	if got.ID != pID || got.Rev != 1 || got.Body != "Initial content" || len(got.Tags) != 2 {
		t.Fatalf("PageBySlug: %+v", got)
	}

	// PageByID
	gotByID, err := st.PageByID(ctx, pID)
	must(t, err)
	if gotByID.Slug != "welcome" || gotByID.Rev != 1 || gotByID.Body != "Initial content" {
		t.Fatalf("PageByID: %+v", gotByID)
	}

	// SlugExists
	exists, err := st.SlugExists(ctx, f.ws.ID, "welcome")
	must(t, err)
	if !exists {
		t.Fatal("expected SlugExists to be true for 'welcome'")
	}
	exists, err = st.SlugExists(ctx, f.ws.ID, "nonexistent")
	must(t, err)
	if exists {
		t.Fatal("expected SlugExists to be false for 'nonexistent'")
	}

	// AddRevision
	t1 := t0.Add(time.Hour)
	rev2 := wiki.Revision{
		PageID:    pID,
		Rev:       2,
		Body:      "Initial content\n\n---\n*@norbi, 2026-10-15 10:00:*\n\nSecond note",
		Kind:      wiki.RevisionAppend,
		AuthorID:  f.human.ID,
		CreatedAt: t1,
	}
	must(t, st.AddRevision(ctx, rev2))

	// PageBySlug reflects latest revision
	got, err = st.PageBySlug(ctx, f.ws.ID, "welcome")
	must(t, err)
	if got.Rev != 2 || got.Body != rev2.Body || !got.UpdatedAt.Equal(t1) {
		t.Fatalf("PageBySlug after rev2: rev=%d, updatedAt=%v", got.Rev, got.UpdatedAt)
	}

	// ListRevisions
	revs, err := st.ListRevisions(ctx, pID)
	must(t, err)
	if len(revs) != 2 || revs[0].Rev != 1 || revs[1].Rev != 2 {
		t.Fatalf("ListRevisions: %+v", revs)
	}

	// UpdatePageMeta
	must(t, st.UpdatePageMeta(ctx, pID, "Welcome Home", "welcome-home", []string{"intro", "updated"}))
	got, err = st.PageBySlug(ctx, f.ws.ID, "welcome-home")
	must(t, err)
	if got.Title != "Welcome Home" || got.Slug != "welcome-home" || len(got.Tags) != 2 || got.Tags[1] != "updated" {
		t.Fatalf("Page after UpdatePageMeta: %+v", got)
	}

	// ListPages
	pID2 := gen.New()
	page2 := wiki.Page{
		ID:          pID2,
		WorkspaceID: f.ws.ID,
		Slug:        "alpha",
		Title:       "Alpha Page",
		Tags:        []string{"a"},
		CreatedBy:   f.human.ID,
		CreatedAt:   t0,
	}
	rev2_1 := wiki.Revision{
		PageID:    pID2,
		Rev:       1,
		Body:      "Alpha body",
		Kind:      wiki.RevisionCreate,
		AuthorID:  f.human.ID,
		CreatedAt: t0,
	}
	must(t, st.CreatePage(ctx, page2, rev2_1))

	pages, err := st.ListPages(ctx, f.ws.ID, false)
	must(t, err)
	if len(pages) != 2 || pages[0].Slug != "alpha" || pages[1].Slug != "welcome-home" {
		t.Fatalf("ListPages ordering: %+v", pages)
	}

	// SoftDelete
	must(t, st.SoftDelete(ctx, pID2, t1))
	pages, err = st.ListPages(ctx, f.ws.ID, false)
	must(t, err)
	if len(pages) != 1 || pages[0].Slug != "welcome-home" {
		t.Fatalf("ListPages without deleted: %+v", pages)
	}
	pages, err = st.ListPages(ctx, f.ws.ID, true)
	must(t, err)
	if len(pages) != 2 {
		t.Fatalf("ListPages with deleted: %+v", pages)
	}

	// Restore
	must(t, st.Restore(ctx, pID2))
	pages, err = st.ListPages(ctx, f.ws.ID, false)
	must(t, err)
	if len(pages) != 2 {
		t.Fatalf("ListPages after restore: %+v", pages)
	}

	// Non-existent lookups return db.ErrNotFound
	if _, err := st.PageBySlug(ctx, f.ws.ID, "does-not-exist"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("PageBySlug non-existent: %v", err)
	}
	if _, err := st.PageByID(ctx, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("PageByID non-existent: %v", err)
	}
	if _, err := st.ListRevisions(ctx, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("ListRevisions non-existent: %v", err)
	}
}
