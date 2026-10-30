// Package storetest is the shared contract test suite for every Store
// implementation (Mattermost storetest pattern). The same tests run against
// the in-memory reference stores and against Postgres, which proves the two
// are interchangeable.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/agentrules"
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/git"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/lease"
	"colonyhall/internal/modules/notes"
	"colonyhall/internal/modules/planner"
	"colonyhall/internal/modules/preview"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/modules/runs"
	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/modules/sidebar"
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
	Question     question.Store
	Files        files.Store
	Git          git.Store
	Preview      preview.Store
	AgentRules   agentrules.Store
	Notes        notes.Store
	Planner      planner.Store
	Decisions    decision.Store
	Leases       lease.Store
	Sidebar      sidebar.Store
	Compaction   compaction.Store
	Delegation   delegation.Store
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
	t.Run("question", func(t *testing.T) { testQuestion(t, newBackend(t)) })
	t.Run("files", func(t *testing.T) { testFiles(t, newBackend(t)) })
	t.Run("git", func(t *testing.T) { testGit(t, newBackend(t)) })
	t.Run("preview", func(t *testing.T) { testPreview(t, newBackend(t)) })
	t.Run("agentrules", func(t *testing.T) { testAgentRules(t, newBackend(t)) })
	t.Run("notes", func(t *testing.T) { testNotes(t, newBackend(t)) })
	t.Run("planner", func(t *testing.T) { testPlanner(t, newBackend(t)) })
	t.Run("decisions", func(t *testing.T) { testDecisions(t, newBackend(t)) })
	t.Run("leases", func(t *testing.T) { testLeases(t, newBackend(t)) })
	t.Run("sidebar", func(t *testing.T) { testSidebar(t, newBackend(t)) })
	t.Run("compaction", func(t *testing.T) { testCompaction(t, newBackend(t)) })
	t.Run("delegation", func(t *testing.T) { testDelegation(t, newBackend(t)) })
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

	must(t, b.Identity.UpdateActorProfile(ctx, f.human.ID, "Lead Developer", "🚀", "Europe/Budapest", "Busy coding"))
	updatedHuman, err := b.Identity.ActorByID(ctx, f.human.ID)
	must(t, err)
	if updatedHuman.Description != "Lead Developer" || updatedHuman.AvatarEmoji != "🚀" || updatedHuman.Timezone != "Europe/Budapest" || updatedHuman.StatusText != "Busy coding" {
		t.Fatalf("UpdateActorProfile not saved: %+v", updatedHuman)
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
	memList, err := b.Conversation.ListMembers(ctx, f.channel.ID)
	must(t, err)
	if len(memList) != 2 {
		t.Fatalf("ListMembers: %v", memList)
	}
	// Soft remove agent member
	must(t, b.Conversation.RemoveMember(ctx, f.channel.ID, f.agent.ID, t0.Add(time.Hour)))
	members, err = b.Conversation.MemberIDs(ctx, f.channel.ID)
	must(t, err)
	if len(members) != 1 || members[0] != f.human.ID {
		t.Fatalf("expected only human member left: %v", members)
	}
	if _, err := b.Conversation.Member(ctx, f.channel.ID, f.agent.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for soft-removed member: %v", err)
	}
	agentViews, err := b.Conversation.ChannelsForActor(ctx, f.ws.ID, f.agent.ID, false)
	must(t, err)
	if len(agentViews) != 0 {
		t.Fatalf("removed member should have 0 channels: %+v", agentViews)
	}
	// Re-add agent member
	must(t, b.Conversation.AddMember(ctx, conversation.Member{
		ChannelID: f.channel.ID,
		ActorID:   f.agent.ID,
		CanWrite:  true,
		JoinedAt:  t0.Add(2 * time.Hour),
	}))
	members, err = b.Conversation.MemberIDs(ctx, f.channel.ID)
	must(t, err)
	if len(members) != 2 {
		t.Fatalf("expected 2 members after re-add: %v", members)
	}
	testThreadsAndReactions(t, b, f, msgs)
}

// testReplyParticipants: distinct reply authors per thread, the most recent
// replier first, at most max, retracted replies left out. It uses its own
// channel so the other conversation checks are not affected.
func testReplyParticipants(t *testing.T, b Backend, f fixture) {
	ctx := context.Background()
	ch := conversation.Channel{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: conversation.KindPublic,
		Name: "threads", CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, b.Conversation.CreateChannel(ctx, ch))
	third := identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindAgent, Handle: "third",
		DisplayName: "Third", Role: identity.RoleMember, State: "active", CreatedAt: t0}
	must(t, b.Identity.CreateActor(ctx, third))
	add := func(rootID, author ids.ID) conversation.Message {
		m := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: ch.ID, AuthorID: author,
			ThreadRootID: rootID, Kind: "text", Body: "r", CreatedAt: t0}
		must(t, b.Conversation.InsertMessage(ctx, m))
		return m
	}
	root := add(ids.Nil, f.human.ID)
	other := add(ids.Nil, f.human.ID)
	// Replies in order: agent, human, agent again, third, then a retracted human reply.
	add(root.ID, f.agent.ID)
	add(root.ID, f.human.ID)
	add(root.ID, f.agent.ID)
	add(root.ID, third.ID)
	gone := add(root.ID, f.human.ID)
	must(t, b.Conversation.RetractMessage(ctx, gone.ID, f.human.ID, t0))
	add(other.ID, f.agent.ID)

	got, err := b.Conversation.ReplyParticipants(ctx, []ids.ID{root.ID, other.ID, gen.New()}, 3)
	must(t, err)
	want := []ids.ID{third.ID, f.agent.ID, f.human.ID}
	if fmt.Sprint(got[root.ID]) != fmt.Sprint(want) {
		t.Fatalf("ReplyParticipants(root) = %v, want %v (most recent first, retracted left out)", got[root.ID], want)
	}
	if fmt.Sprint(got[other.ID]) != fmt.Sprint([]ids.ID{f.agent.ID}) || len(got) != 2 {
		t.Fatalf("ReplyParticipants: %v", got)
	}
	capped, err := b.Conversation.ReplyParticipants(ctx, []ids.ID{root.ID}, 2)
	must(t, err)
	if fmt.Sprint(capped[root.ID]) != fmt.Sprint(want[:2]) {
		t.Fatalf("ReplyParticipants max 2 = %v", capped[root.ID])
	}
	if none, err := b.Conversation.ReplyParticipants(ctx, nil, 3); err != nil || len(none) != 0 {
		t.Fatalf("no roots: %v %v", none, err)
	}
}

func testThreadsAndReactions(t *testing.T, b Backend, f fixture, msgs []conversation.Message) {
	testReplyParticipants(t, b, f)
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

	// Origin message fields and LatestTopLevelMessageByAuthor
	origChanID := f.channel.ID
	origThreadID := msgs[0].ID
	mWithOrigin := conversation.Message{
		ID:              gen.New(),
		WorkspaceID:     f.ws.ID,
		ChannelID:       f.channel.ID,
		AuthorID:        f.agent.ID,
		Kind:            "text",
		Body:            "origin request",
		Mentions:        []ids.ID{},
		OriginChannelID: origChanID,
		OriginThreadID:  origThreadID,
		CreatedAt:       t0.Add(2 * time.Hour),
	}
	must(t, b.Conversation.InsertMessage(ctx, mWithOrigin))
	gotWithOrigin, err := b.Conversation.MessageByID(ctx, mWithOrigin.ID)
	must(t, err)
	if gotWithOrigin.OriginChannelID != origChanID || gotWithOrigin.OriginThreadID != origThreadID {
		t.Fatalf("message origin not preserved: got channel %v, thread %v", gotWithOrigin.OriginChannelID, gotWithOrigin.OriginThreadID)
	}

	latestTop, err := b.Conversation.LatestTopLevelMessageByAuthor(ctx, f.channel.ID, f.agent.ID)
	must(t, err)
	if latestTop.ID != mWithOrigin.ID {
		t.Fatalf("expected latest top level message %s, got %s", mWithOrigin.ID, latestTop.ID)
	}

	// A reply in thread does not overwrite the latest top-level message
	replyMsg := conversation.Message{
		ID:           gen.New(),
		WorkspaceID:  f.ws.ID,
		ChannelID:    f.channel.ID,
		ThreadRootID: origThreadID,
		AuthorID:     f.agent.ID,
		Kind:         "text",
		Body:         "thread reply",
		Mentions:     []ids.ID{},
		CreatedAt:    t0.Add(3 * time.Hour),
		// The answer link to another message survives a round trip.
		AnswersMessageID: mWithOrigin.ID,
	}
	must(t, b.Conversation.InsertMessage(ctx, replyMsg))
	if got, err := b.Conversation.MessageByID(ctx, replyMsg.ID); err != nil || got.AnswersMessageID != mWithOrigin.ID {
		t.Fatalf("answers_message_id not preserved: %v %v", got.AnswersMessageID, err)
	}
	latestTopAfterReply, err := b.Conversation.LatestTopLevelMessageByAuthor(ctx, f.channel.ID, f.agent.ID)
	must(t, err)
	if latestTopAfterReply.ID != mWithOrigin.ID {
		t.Fatalf("thread reply should not be latest top-level: expected %s, got %s", mWithOrigin.ID, latestTopAfterReply.ID)
	}

	// Unknown author or channel returns ErrNotFound
	if _, err := b.Conversation.LatestTopLevelMessageByAuthor(ctx, f.channel.ID, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown author, got %v", err)
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

	// Terminal events
	exitCode := 0
	te := delivery.TerminalEvent{
		ID:           gen.New(),
		WorkspaceID:  f.ws.ID,
		AgentID:      f.agent.ID,
		ChannelID:    f.channel.ID,
		ThreadRootID: ids.Nil,
		Name:         "ros-build",
		ExitCode:     &exitCode,
		DurationMs:   2592000,
		Tail:         "build complete",
		CreatedAt:    t0,
	}
	must(t, b.Delivery.InsertTerminalEvent(ctx, te))
	teList, err := b.Delivery.TerminalEventsByIDs(ctx, []ids.ID{te.ID})
	must(t, err)
	if len(teList) != 1 || teList[0].Name != "ros-build" || teList[0].ExitCode == nil || *teList[0].ExitCode != 0 {
		t.Fatalf("TerminalEventsByIDs unexpected: %+v", teList)
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
		Sandbox: "bwrap 0.9.0", AllowUnsandboxed: true, ExchangeDir: "/mnt/z/exchange", ExchangeOK: true, Rejected: []hosts.Rejected{{AgentID: "a", Path: "/etc", Reason: "outside roots"}},
		Worktrees: []hosts.Worktree{{AgentID: f.agent.ID.String(), Path: "/work/repo-worktrees/agent", Branch: "colony/agent/work"}}}
	must(t, hs.SaveReport(ctx, h.ID, rep, t0.Add(time.Minute)))
	must(t, hs.SetHostState(ctx, h.ID, hosts.StateRevoked))
	list, err := hs.ListHosts(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 1 || list[0].State != hosts.StateRevoked || list[0].LastSeenAt == nil ||
		list[0].Report.Sandbox != "bwrap 0.9.0" || !list[0].Report.AllowUnsandboxed ||
		list[0].Report.ExchangeDir != "/mnt/z/exchange" || !list[0].Report.ExchangeOK ||
		len(list[0].Report.Rejected) != 1 || list[0].Report.Clients[0].Version != "2" ||
		len(list[0].Report.Worktrees) != 1 || list[0].Report.Worktrees[0].Branch != "colony/agent/work" {
		t.Fatalf("ListHosts: %+v", list)
	}

	a := hosts.Assignment{
		AgentID: f.agent.ID, HostID: h.ID, WorkspaceID: f.ws.ID, Client: "claude-code", Model: "haiku",
		WorkModel: "sonnet", AllowWork: true, Mode: hosts.AgentModeDirect, RootDir: "/work", WorktreeRepo: "/work/repo", Sandbox: false,
		Approvals: hosts.ApprovalsChat, MaxSessionTokens: 120000, DailyTokenLimit: 500000, AllowPreview: true, AllowWebRead: false, CreatedBy: f.human.ID, CreatedAt: t0,
	}
	must(t, hs.SetAssignment(ctx, a))
	ga, err := hs.AssignmentByAgent(ctx, f.agent.ID)
	if err != nil || ga.Approvals != hosts.ApprovalsChat || ga.MaxSessionTokens != 120000 || ga.DailyTokenLimit != 500000 || ga.WorktreeRepo != "/work/repo" || !ga.AllowPreview || ga.AllowWebRead {
		t.Fatalf("approvals / limits / worktree / web_read: %+v %v", ga, err)
	}
	a.Model = "opus"
	a.Mode = hosts.AgentModeCompact
	a.WorktreeRepo = "/work/repo2"
	a.Sandbox = true
	a.Approvals = hosts.ApprovalsNone
	a.AllowPreview = false
	a.AllowWebRead = true
	a.MaxSessionTokens = 80000
	a.DailyTokenLimit = 250000
	must(t, hs.SetAssignment(ctx, a)) // replaces
	ga, err = hs.AssignmentByAgent(ctx, f.agent.ID)
	if err != nil || ga.Model != "opus" || !ga.AllowWork || ga.Mode != hosts.AgentModeCompact || ga.RootDir != "/work" || ga.WorktreeRepo != "/work/repo2" || !ga.Sandbox || ga.Approvals != hosts.ApprovalsNone || ga.MaxSessionTokens != 80000 || ga.DailyTokenLimit != 250000 || ga.AllowPreview || !ga.AllowWebRead {
		t.Fatalf("AssignmentByAgent: %+v %v", ga, err)
	}
	as, err := hs.AssignmentsForHost(ctx, h.ID)
	if err != nil || len(as) != 1 || as[0].Approvals != hosts.ApprovalsNone || as[0].WorktreeRepo != "/work/repo2" || as[0].MaxSessionTokens != 80000 || as[0].DailyTokenLimit != 250000 || as[0].AllowPreview || !as[0].AllowWebRead {
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
		ContextTokens:    9000,
		DurationMS:       1200,
		Status:           runs.StatusOk,
		CreatedAt:        tDay1,
		WebReads:         []string{"https://pkg.go.dev/net/http"},
		Steps:            []runs.Step{{Kind: "read", Label: "main.go", Path: "main.go", State: "done", StartedAt: tDay1, DurationMs: 150}},
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
	if len(allRuns[2].WebReads) != 1 || allRuns[2].WebReads[0] != "https://pkg.go.dev/net/http" {
		t.Fatalf("ListRuns web reads: got %v want [https://pkg.go.dev/net/http]", allRuns[2].WebReads)
	}
	if len(allRuns[2].Steps) != 1 || allRuns[2].Steps[0].Label != "main.go" {
		t.Fatalf("ListRuns steps: got %+v want [{Label:main.go}]", allRuns[2].Steps)
	}
	if allRuns[2].ContextTokens != 9000 || allRuns[1].ContextTokens != 0 {
		t.Fatalf("ListRuns context tokens: got %d and %d, want 9000 and 0 (unknown)", allRuns[2].ContextTokens, allRuns[1].ContextTokens)
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

func testQuestion(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Question

	// We need a root message in conversation so foreign key constraint passes in Postgres
	rootMsg := conversation.Message{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		ChannelID:   f.channel.ID,
		AuthorID:    f.human.ID,
		Kind:        "text",
		Body:        "Thread start",
		CreatedAt:   t0,
	}
	must(t, b.Conversation.InsertMessage(ctx, rootMsg))

	// NextNumber on empty workspace
	n, err := st.NextNumber(ctx, f.ws.ID)
	must(t, err)
	if n != 1 {
		t.Fatalf("expected next number 1, got %d", n)
	}

	// Insert q1
	q1ID := gen.New()
	yes := "yes"
	dl1 := t0.Add(time.Hour)
	q1 := question.Question{
		ID:            q1ID,
		Number:        1,
		WorkspaceID:   f.ws.ID,
		AskedBy:       f.agent.ID,
		ToActorID:     &f.human.ID,
		ToHuman:       true,
		ChannelID:     f.channel.ID,
		ThreadRootID:  rootMsg.ID,
		Text:          "Should we deploy to prod?",
		Options:       []string{"yes", "no"},
		DefaultOption: &yes,
		Deadline:      &dl1,
		State:         question.StateOpen,
		CreatedAt:     t0,
	}
	must(t, st.Insert(ctx, q1))

	// NextNumber after q1
	n, err = st.NextNumber(ctx, f.ws.ID)
	must(t, err)
	if n != 2 {
		t.Fatalf("expected next number 2, got %d", n)
	}

	// Duplicate number in same workspace returns ErrConflict
	dup := q1
	dup.ID = gen.New()
	if err := st.Insert(ctx, dup); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate number, got %v", err)
	}

	// Duplicate ID returns ErrConflict
	dupID := q1
	dupID.Number = 99
	if err := st.Insert(ctx, dupID); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate ID, got %v", err)
	}

	// GetByID
	got, err := st.GetByID(ctx, q1ID)
	must(t, err)
	if got.Number != 1 || got.Text != "Should we deploy to prod?" || len(got.Options) != 2 || got.State != question.StateOpen {
		t.Fatalf("unexpected GetByID: %+v", got)
	}

	// GetByNumber
	gotNum, err := st.GetByNumber(ctx, f.ws.ID, 1)
	must(t, err)
	if gotNum.ID != q1ID || gotNum.Number != 1 {
		t.Fatalf("unexpected GetByNumber: %+v", gotNum)
	}

	// CountOpenByThreadAsker
	cnt, err := st.CountOpenByThreadAsker(ctx, rootMsg.ID, f.agent.ID)
	must(t, err)
	if cnt != 1 {
		t.Fatalf("expected open count 1, got %d", cnt)
	}
	cntOther, err := st.CountOpenByThreadAsker(ctx, rootMsg.ID, f.human.ID)
	must(t, err)
	if cntOther != 0 {
		t.Fatalf("expected open count 0 for other asker, got %d", cntOther)
	}

	// List questions
	list, err := st.List(ctx, f.ws.ID, nil)
	must(t, err)
	if len(list) != 1 || list[0].ID != q1ID {
		t.Fatalf("unexpected List: %+v", list)
	}
	openState := question.StateOpen
	listOpen, err := st.List(ctx, f.ws.ID, &openState)
	must(t, err)
	if len(listOpen) != 1 {
		t.Fatalf("expected 1 open question, got %d", len(listOpen))
	}
	ansState := question.StateAnswered
	listAns, err := st.List(ctx, f.ws.ID, &ansState)
	must(t, err)
	if len(listAns) != 0 {
		t.Fatalf("expected 0 answered questions, got %d", len(listAns))
	}

	// Answer q1
	ansTime := t0.Add(5 * time.Minute)
	ansQ, ok, err := st.Answer(ctx, q1ID, "yes", &f.human.ID, ansTime)
	must(t, err)
	if !ok || ansQ.State != question.StateAnswered || ansQ.Answer == nil || *ansQ.Answer != "yes" || ansQ.AnsweredBy == nil || *ansQ.AnsweredBy != f.human.ID {
		t.Fatalf("unexpected Answer result: ok=%v, ansQ=%+v", ok, ansQ)
	}

	// Answering again returns ok=false
	_, ok, err = st.Answer(ctx, q1ID, "no", &f.human.ID, ansTime)
	must(t, err)
	if ok {
		t.Fatal("expected ok=false when answering already answered question")
	}

	// CountOpenByThreadAsker after answer
	cnt, err = st.CountOpenByThreadAsker(ctx, rootMsg.ID, f.agent.ID)
	must(t, err)
	if cnt != 0 {
		t.Fatalf("expected open count 0 after answering, got %d", cnt)
	}

	// Withdraw test with q2
	q2ID := gen.New()
	q2 := question.Question{
		ID:           q2ID,
		Number:       2,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.agent.ID,
		ToHuman:      true,
		ChannelID:    f.channel.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "Second question?",
		Options:      []string{},
		State:        question.StateOpen,
		CreatedAt:    t0.Add(10 * time.Minute),
	}
	must(t, st.Insert(ctx, q2))

	wTime := t0.Add(15 * time.Minute)
	withdrawnQ, ok, err := st.Withdraw(ctx, q2ID, wTime)
	must(t, err)
	if !ok || withdrawnQ.State != question.StateWithdrawn || withdrawnQ.WithdrawnAt == nil {
		t.Fatalf("unexpected Withdraw result: ok=%v, withdrawnQ=%+v", ok, withdrawnQ)
	}

	// Withdrawing again returns ok=false
	_, ok, err = st.Withdraw(ctx, q2ID, wTime)
	must(t, err)
	if ok {
		t.Fatal("expected ok=false when withdrawing already withdrawn question")
	}

	// Escalate test with q3 (to agent)
	q3ID := gen.New()
	q3 := question.Question{
		ID:           q3ID,
		Number:       3,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.human.ID,
		ToActorID:    &f.agent.ID,
		ToHuman:      false,
		ChannelID:    f.channel.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "Agent question to escalate?",
		Options:      []string{},
		State:        question.StateOpen,
		CreatedAt:    t0.Add(20 * time.Minute),
	}
	must(t, st.Insert(ctx, q3))

	escTime := t0.Add(50 * time.Minute)
	escQ, ok, err := st.Escalate(ctx, q3ID, escTime)
	must(t, err)
	if !ok || escQ.State != question.StateEscalated || escQ.EscalatedAt == nil {
		t.Fatalf("unexpected Escalate result: ok=%v, escQ=%+v", ok, escQ)
	}

	// Escalating again returns ok=false
	_, ok, err = st.Escalate(ctx, q3ID, escTime)
	must(t, err)
	if ok {
		t.Fatal("expected ok=false when escalating already escalated question")
	}

	// An escalated question can be answered
	ansEscQ, ok, err := st.Answer(ctx, q3ID, "escalated answer", &f.human.ID, escTime.Add(time.Minute))
	must(t, err)
	if !ok || ansEscQ.State != question.StateAnswered {
		t.Fatalf("unexpected Answer after Escalate: ok=%v, ansEscQ=%+v", ok, ansEscQ)
	}

	// Expire test with q4
	q4ID := gen.New()
	q4 := question.Question{
		ID:           q4ID,
		Number:       4,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.agent.ID,
		ToActorID:    &f.agent.ID,
		ToHuman:      false,
		ChannelID:    f.channel.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "Will expire?",
		Options:      []string{},
		State:        question.StateOpen,
		CreatedAt:    t0.Add(60 * time.Minute),
	}
	must(t, st.Insert(ctx, q4))

	expTime := t0.Add(70 * time.Minute)
	expQ, ok, err := st.Expire(ctx, q4ID, expTime)
	must(t, err)
	if !ok || expQ.State != question.StateExpired || expQ.ExpiredAt == nil {
		t.Fatalf("unexpected Expire result: ok=%v, expQ=%+v", ok, expQ)
	}

	// Due test:
	// q5: to agent, created at t0 (old, no deadline) -> due for escalation
	q5ID := gen.New()
	q5 := question.Question{
		ID:           q5ID,
		Number:       5,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.human.ID,
		ToActorID:    &f.agent.ID,
		ToHuman:      false,
		ChannelID:    f.channel.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "Due for escalation?",
		Options:      []string{},
		State:        question.StateOpen,
		CreatedAt:    t0,
	}
	must(t, st.Insert(ctx, q5))

	// q6: to agent, created recently, but deadline is past -> due for deadline
	dl6 := t0.Add(10 * time.Minute)
	q6ID := gen.New()
	q6 := question.Question{
		ID:           q6ID,
		Number:       6,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.human.ID,
		ToActorID:    &f.agent.ID,
		ToHuman:      false,
		ChannelID:    f.channel.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "Due for deadline?",
		Options:      []string{"a", "b"},
		Deadline:     &dl6,
		State:        question.StateOpen,
		CreatedAt:    t0.Add(5 * time.Minute),
	}
	must(t, st.Insert(ctx, q6))

	// q7: to human with deadline but allow_expire=false (should never be returned by Due)
	dl7 := t0.Add(10 * time.Minute)
	q7ID := gen.New()
	q7 := question.Question{
		ID:           q7ID,
		Number:       7,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.agent.ID,
		ToHuman:      true,
		ChannelID:    f.channel.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "To human, should not be due",
		Options:      []string{},
		Deadline:     &dl7,
		State:        question.StateOpen,
		CreatedAt:    t0,
	}
	must(t, st.Insert(ctx, q7))

	dueList, err := st.Due(ctx, t0.Add(30*time.Minute), t0.Add(15*time.Minute), 10)
	must(t, err)
	if len(dueList) != 2 {
		t.Fatalf("expected 2 due questions (q5 and q6), got %d: %+v", len(dueList), dueList)
	}
	if dueList[0].ID != q5ID || dueList[1].ID != q6ID {
		t.Fatalf("expected [q5, q6], got %+v", dueList)
	}

	// q8: to human with deadline and allow_expire=true (delegation question) is due when deadline passes
	dl8 := t0.Add(10 * time.Minute)
	q8ID := gen.New()
	q8 := question.Question{
		ID:           q8ID,
		Number:       8,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.agent.ID,
		ToHuman:      true,
		AllowExpire:  true,
		ChannelID:    f.channel.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "To human with deadline and allow_expire, due on deadline",
		Options:      []string{"a", "b"},
		Deadline:     &dl8,
		State:        question.StateOpen,
		CreatedAt:    t0,
	}
	must(t, st.Insert(ctx, q8))

	dueWithHumanDL, err := st.Due(ctx, t0, t0.Add(15*time.Minute), 10)
	must(t, err)
	foundQ8 := false
	for _, q := range dueWithHumanDL {
		if q.ID == q8ID {
			foundQ8 = true
			break
		}
	}
	if !foundQ8 {
		t.Fatalf("expected q8 to be due when deadline passed, got %+v", dueWithHumanDL)
	}

	// Non-existent lookups return db.ErrNotFound
	if _, err := st.GetByID(ctx, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetByID non-existent: %v", err)
	}
	if _, err := st.GetByNumber(ctx, f.ws.ID, 9999); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetByNumber non-existent: %v", err)
	}
}

func testFiles(t *testing.T, b Backend) {
	ctx := context.Background()
	ws := gen.New()
	u1 := gen.New()
	ch1 := gen.New()
	ch2 := gen.New()

	// Seed workspace, actor, channels
	must(t, b.Identity.CreateWorkspace(ctx, identity.Workspace{ID: ws, Name: "acme", CreatedAt: t0}))
	must(t, b.Identity.CreateActor(ctx, identity.Actor{ID: u1, WorkspaceID: ws, Kind: identity.KindHuman, Handle: "alice", DisplayName: "Alice", State: "active", Role: identity.RoleOwner, CreatedAt: t0}))
	must(t, b.Conversation.CreateChannel(ctx, conversation.Channel{ID: ch1, WorkspaceID: ws, Kind: conversation.KindPublic, Name: "general", CreatedBy: u1, CreatedAt: t0}))
	must(t, b.Conversation.CreateChannel(ctx, conversation.Channel{ID: ch2, WorkspaceID: ws, Kind: conversation.KindPublic, Name: "random", CreatedBy: u1, CreatedAt: t0}))

	// 1. Insert & ByID
	f1ID := gen.New()
	f1 := files.File{
		ID:          f1ID,
		WorkspaceID: ws,
		UploadedBy:  u1,
		ChannelID:   ch1,
		Name:        "report.pdf",
		Mime:        "application/pdf",
		Size:        1024,
		SHA256:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		CreatedAt:   t0,
	}
	must(t, b.Files.Insert(ctx, f1))

	got, err := b.Files.ByID(ctx, f1ID)
	must(t, err)
	if got.ID != f1ID || got.Name != "report.pdf" || got.Mime != "application/pdf" || got.Size != 1024 {
		t.Fatalf("unexpected file: %+v", got)
	}

	// Not found
	_, err = b.Files.ByID(ctx, gen.New())
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected db.ErrNotFound, got %v", err)
	}

	// 2. ByIDs
	f2ID := gen.New()
	f2 := files.File{
		ID:          f2ID,
		WorkspaceID: ws,
		UploadedBy:  u1,
		ChannelID:   ch1,
		Name:        "notes.txt",
		Mime:        "text/plain",
		Size:        50,
		SHA256:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		CreatedAt:   t0.Add(time.Minute),
	}
	must(t, b.Files.Insert(ctx, f2))

	byIDs, err := b.Files.ByIDs(ctx, []ids.ID{f1ID, f2ID, gen.New()})
	must(t, err)
	if len(byIDs) != 2 {
		t.Fatalf("expected 2 files, got %d", len(byIDs))
	}

	// 3. ListByChannel
	chList, err := b.Files.ListByChannel(ctx, ch1)
	must(t, err)
	if len(chList) != 2 || chList[0].ID != f2ID || chList[1].ID != f1ID {
		t.Fatalf("expected 2 files newest first, got %+v", chList)
	}

	emptyList, err := b.Files.ListByChannel(ctx, ch2)
	must(t, err)
	if len(emptyList) != 0 {
		t.Fatalf("expected 0 files, got %d", len(emptyList))
	}

	// 4. AttachToMessage & AttachmentsForMessage & AttachmentsForMessages
	msg1ID := gen.New()
	msg1 := conversation.Message{
		ID:          msg1ID,
		WorkspaceID: ws,
		ChannelID:   ch1,
		AuthorID:    u1,
		Kind:        "text",
		Body:        "here are the files",
		CreatedAt:   t0,
	}
	must(t, b.Conversation.InsertMessage(ctx, msg1))

	must(t, b.Files.AttachToMessage(ctx, msg1ID, f1ID, 0))
	must(t, b.Files.AttachToMessage(ctx, msg1ID, f2ID, 1))

	atts, err := b.Files.AttachmentsForMessage(ctx, msg1ID)
	must(t, err)
	if len(atts) != 2 || atts[0].ID != f1ID || atts[1].ID != f2ID {
		t.Fatalf("unexpected attachments for message: %+v", atts)
	}

	msg2ID := gen.New()
	msg2 := conversation.Message{
		ID:          msg2ID,
		WorkspaceID: ws,
		ChannelID:   ch1,
		AuthorID:    u1,
		Kind:        "text",
		Body:        "another message with one file",
		CreatedAt:   t0.Add(time.Second),
	}
	must(t, b.Conversation.InsertMessage(ctx, msg2))
	must(t, b.Files.AttachToMessage(ctx, msg2ID, f2ID, 0))

	batchAtts, err := b.Files.AttachmentsForMessages(ctx, []ids.ID{msg1ID, msg2ID, gen.New()})
	must(t, err)
	if len(batchAtts[msg1ID]) != 2 || len(batchAtts[msg2ID]) != 1 || len(batchAtts[gen.New()]) != 0 {
		t.Fatalf("unexpected batch attachments: %+v", batchAtts)
	}

	// 5. HumanOnly file
	f3ID := gen.New()
	f3 := files.File{
		ID:          f3ID,
		WorkspaceID: ws,
		UploadedBy:  u1,
		ChannelID:   ch1,
		Name:        "secret.log",
		Mime:        "text/plain",
		Size:        100,
		SHA256:      "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
		CreatedAt:   t0.Add(2 * time.Minute),
		HumanOnly:   true,
	}
	must(t, b.Files.Insert(ctx, f3))
	gotF3, err := b.Files.ByID(ctx, f3ID)
	must(t, err)
	if !gotF3.HumanOnly {
		t.Fatalf("expected f3.HumanOnly=true")
	}

	// 6. FileFetchRequest lifecycle
	ag1 := gen.New()
	must(t, b.Identity.CreateActor(ctx, identity.Actor{ID: ag1, WorkspaceID: ws, Kind: identity.KindAgent, Handle: "bot", DisplayName: "Bot", State: "active", Role: identity.RoleMember, CreatedAt: t0}))

	req1ID := gen.New()
	req1 := files.FileFetchRequest{
		ID:          req1ID,
		WorkspaceID: ws,
		MessageID:   msg1ID,
		ChannelID:   ch1,
		AgentID:     ag1,
		RequestedBy: u1,
		Path:        "/home/bot/app.log",
		State:       files.FileFetchPending,
		CreatedAt:   t0,
	}
	must(t, b.Files.InsertFileFetchRequest(ctx, req1))

	gotReq1, err := b.Files.FileFetchRequestByID(ctx, req1ID)
	must(t, err)
	if gotReq1.ID != req1ID || gotReq1.State != files.FileFetchPending || gotReq1.Path != "/home/bot/app.log" {
		t.Fatalf("unexpected req1: %+v", gotReq1)
	}

	byMsgPath, err := b.Files.FileFetchRequestByMessageAndPath(ctx, ws, msg1ID, "/home/bot/app.log")
	must(t, err)
	if byMsgPath.ID != req1ID {
		t.Fatalf("unexpected byMsgPath: %+v", byMsgPath)
	}

	byAgentPath, err := b.Files.FileFetchRequestByAgentAndPath(ctx, ws, ag1, "/home/bot/app.log")
	must(t, err)
	if byAgentPath.ID != req1ID {
		t.Fatalf("unexpected byAgentPath: %+v", byAgentPath)
	}

	agentPending, err := b.Files.ListFileFetchRequestsByAgent(ctx, ws, ag1, files.FileFetchPending)
	must(t, err)
	if len(agentPending) != 1 || agentPending[0].ID != req1ID {
		t.Fatalf("unexpected agentPending: %+v", agentPending)
	}

	// Complete request
	completedAt := t0.Add(10 * time.Second)
	must(t, b.Files.UpdateFileFetchRequestCompleted(ctx, req1ID, f3ID, completedAt))
	doneReq, err := b.Files.FileFetchRequestByID(ctx, req1ID)
	must(t, err)
	if doneReq.State != files.FileFetchDone || doneReq.FileID != f3ID || doneReq.CompletedAt == nil || !doneReq.CompletedAt.Equal(completedAt) {
		t.Fatalf("unexpected doneReq: %+v", doneReq)
	}

	// Request without message_id (from agent file browser)
	reqNoMsgID := gen.New()
	reqNoMsg := files.FileFetchRequest{
		ID:          reqNoMsgID,
		WorkspaceID: ws,
		MessageID:   ids.Nil,
		ChannelID:   ch1,
		AgentID:     ag1,
		RequestedBy: u1,
		Path:        "/home/bot/src/main.go",
		State:       files.FileFetchPending,
		CreatedAt:   t0.Add(2 * time.Second),
	}
	must(t, b.Files.InsertFileFetchRequest(ctx, reqNoMsg))
	gotNoMsg, err := b.Files.FileFetchRequestByID(ctx, reqNoMsgID)
	must(t, err)
	if gotNoMsg.MessageID != ids.Nil {
		t.Fatalf("expected MessageID to be ids.Nil, got %v", gotNoMsg.MessageID)
	}
	byAgentPath2, err := b.Files.FileFetchRequestByAgentAndPath(ctx, ws, ag1, "/home/bot/src/main.go")
	must(t, err)
	if byAgentPath2.ID != reqNoMsgID {
		t.Fatalf("unexpected byAgentPath2: %+v", byAgentPath2)
	}
	must(t, b.Files.UpdateFileFetchRequestCompleted(ctx, reqNoMsgID, f3ID, completedAt))

	// Failed request
	req2ID := gen.New()
	req2 := files.FileFetchRequest{
		ID:          req2ID,
		WorkspaceID: ws,
		MessageID:   msg1ID,
		ChannelID:   ch1,
		AgentID:     ag1,
		RequestedBy: u1,
		Path:        "/home/bot/secret.key",
		State:       files.FileFetchPending,
		CreatedAt:   t0.Add(time.Second),
	}
	must(t, b.Files.InsertFileFetchRequest(ctx, req2))
	failedAt := t0.Add(15 * time.Second)
	must(t, b.Files.UpdateFileFetchRequestFailed(ctx, req2ID, files.FileFetchErrorDenied, failedAt))
	failReq, err := b.Files.FileFetchRequestByID(ctx, req2ID)
	must(t, err)
	if failReq.State != files.FileFetchFailed || failReq.ErrorCode != files.FileFetchErrorDenied {
		t.Fatalf("unexpected failReq: %+v", failReq)
	}
	// Only a pending request changes state: a late upload or a second sweep
	// must not overwrite a finished one.
	if err := b.Files.UpdateFileFetchRequestCompleted(ctx, req2ID, f3ID, failedAt); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("completing a failed request: want ErrConflict, got %v", err)
	}
	if err := b.Files.UpdateFileFetchRequestFailed(ctx, req1ID, files.FileFetchErrorHostOffline, failedAt); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("failing a done request: want ErrConflict, got %v", err)
	}

	// Sweep list
	req3ID := gen.New()
	req3 := files.FileFetchRequest{
		ID:          req3ID,
		WorkspaceID: ws,
		MessageID:   msg1ID,
		ChannelID:   ch1,
		AgentID:     ag1,
		RequestedBy: u1,
		Path:        "/home/bot/timeout.log",
		State:       files.FileFetchPending,
		CreatedAt:   t0.Add(-5 * time.Minute),
	}
	must(t, b.Files.InsertFileFetchRequest(ctx, req3))
	expired, err := b.Files.ListPendingFileFetchRequestsBefore(ctx, t0.Add(-2*time.Minute))
	must(t, err)
	if len(expired) != 1 || expired[0].ID != req3ID {
		t.Fatalf("unexpected expired requests: %+v", expired)
	}

	// 7. Settings
	stDefault, err := b.Files.GetFileFetchSettings(ctx, ws)
	must(t, err)
	if stDefault.MaxImageMB != files.DefaultMaxImageMB || stDefault.MaxFileMB != files.DefaultMaxFileMB {
		t.Fatalf("unexpected default settings: %+v", stDefault)
	}

	newSt := files.FileFetchSettings{
		WorkspaceID: ws,
		MaxImageMB:  500,
		MaxFileMB:   300,
		UpdatedAt:   t0.Add(time.Hour),
	}
	must(t, b.Files.UpsertFileFetchSettings(ctx, newSt))
	gotSt, err := b.Files.GetFileFetchSettings(ctx, ws)
	must(t, err)
	if gotSt.MaxImageMB != 500 || gotSt.MaxFileMB != 300 {
		t.Fatalf("unexpected updated settings: %+v", gotSt)
	}
}

func testGit(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Git

	// 1. Link caching
	card1 := git.LinkCard{
		URL:       "https://gitea.local/owner/repo/pulls/1",
		Kind:      git.KindPull,
		Title:     "#1 Initial PR",
		State:     "open",
		Author:    "alice",
		FetchedAt: t0,
	}
	card2 := git.LinkCard{
		URL:       "https://gitea.local/owner/repo/issues/2",
		Kind:      git.KindIssue,
		Title:     "#2 Bug report",
		State:     "closed",
		Author:    "bob",
		FetchedAt: t0.Add(time.Minute),
	}

	must(t, st.SaveLink(ctx, card1))
	must(t, st.SaveLink(ctx, card2))

	got1, err := st.GetLink(ctx, card1.URL)
	must(t, err)
	if got1.Title != card1.Title || got1.State != card1.State || got1.Author != card1.Author || got1.Kind != card1.Kind {
		t.Fatalf("GetLink(card1) = %+v, want %+v", got1, card1)
	}

	// Non-existent link
	if _, err := st.GetLink(ctx, "https://gitea.local/not/found"); !errors.Is(err, git.ErrLinkNotFound) {
		t.Fatalf("expected ErrLinkNotFound, got %v", err)
	}

	// Batch GetLinks
	batch, err := st.GetLinks(ctx, []string{card1.URL, "https://gitea.local/not/found", card2.URL})
	must(t, err)
	if len(batch) != 2 {
		t.Fatalf("GetLinks len = %d, want 2", len(batch))
	}

	// Update existing link
	card1.Title = "#1 Updated Title"
	card1.State = "merged"
	must(t, st.SaveLink(ctx, card1))
	gotUpdated, err := st.GetLink(ctx, card1.URL)
	must(t, err)
	if gotUpdated.Title != "#1 Updated Title" || gotUpdated.State != "merged" {
		t.Fatalf("GetLink updated = %+v", gotUpdated)
	}

	// 2. Repo-Channel mappings
	chID := f.channel.ID
	m1 := git.RepoChannel{
		WorkspaceID: f.ws.ID,
		Repo:        "owner/repo1",
		ChannelID:   chID,
		CreatedAt:   t0,
	}
	m2 := git.RepoChannel{
		WorkspaceID: f.ws.ID,
		Repo:        "owner/repo2",
		ChannelID:   chID,
		CreatedAt:   t0.Add(time.Minute),
	}

	must(t, st.SetRepoChannel(ctx, m1))
	must(t, st.SetRepoChannel(ctx, m2))

	gotM1, err := st.GetRepoChannel(ctx, f.ws.ID, "owner/repo1")
	must(t, err)
	if gotM1.ChannelID != chID || gotM1.Repo != "owner/repo1" {
		t.Fatalf("GetRepoChannel(m1) = %+v", gotM1)
	}

	// List mappings
	list, err := st.ListRepoChannels(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 2 || list[0].Repo != "owner/repo1" || list[1].Repo != "owner/repo2" {
		t.Fatalf("ListRepoChannels = %+v", list)
	}

	// FindRepoChannels by repo
	found, err := st.FindRepoChannels(ctx, "owner/repo1")
	must(t, err)
	if len(found) != 1 || found[0].ChannelID != chID {
		t.Fatalf("FindRepoChannels = %+v", found)
	}

	// Non-existent mapping
	if _, err := st.GetRepoChannel(ctx, f.ws.ID, "owner/nonexistent"); !errors.Is(err, git.ErrMappingNotFound) {
		t.Fatalf("expected ErrMappingNotFound, got %v", err)
	}

	// Delete mapping
	must(t, st.DeleteRepoChannel(ctx, f.ws.ID, "owner/repo1"))
	list, err = st.ListRepoChannels(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 1 || list[0].Repo != "owner/repo2" {
		t.Fatalf("ListRepoChannels after delete = %+v", list)
	}

	// 3. Webhook delivery idempotency
	isNew, err := st.RecordDelivery(ctx, "delivery-123", t0)
	must(t, err)
	if !isNew {
		t.Fatalf("expected delivery-123 to be new")
	}

	isNewAgain, err := st.RecordDelivery(ctx, "delivery-123", t0.Add(time.Second))
	must(t, err)
	if isNewAgain {
		t.Fatalf("expected delivery-123 to NOT be new (duplicate)")
	}

	isNew2, err := st.RecordDelivery(ctx, "delivery-456", t0)
	must(t, err)
	if !isNew2 {
		t.Fatalf("expected delivery-456 to be new")
	}
}

func testPreview(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Preview

	// Host fixture
	hostID := gen.New()
	must(t, b.Hosts.CreateHost(ctx, hosts.Host{
		ID:          hostID,
		WorkspaceID: f.ws.ID,
		Name:        "preview-host",
		TokenHash:   hosts.Hash("tok1"),
		State:       hosts.StateApproved,
		CreatedBy:   f.human.ID,
		CreatedAt:   t0,
	}))

	p1 := preview.Preview{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		AgentID:     f.agent.ID,
		HostID:      hostID,
		ChannelID:   f.channel.ID,
		Name:        "web",
		ComposePath: "docker-compose.yaml",
		State:       preview.StateRequested,
		ExpiresAt:   t0.Add(4 * time.Hour),
		CreatedAt:   t0,
		UpdatedAt:   t0,
	}
	must(t, st.Create(ctx, p1))

	got, err := st.ByID(ctx, p1.ID)
	must(t, err)
	if got.Name != "web" || got.State != preview.StateRequested || got.HostID != hostID {
		t.Fatalf("ByID: %+v", got)
	}

	active, err := st.ActiveByAgentAndName(ctx, f.agent.ID, "web")
	must(t, err)
	if active.ID != p1.ID {
		t.Fatalf("ActiveByAgentAndName: %+v", active)
	}

	byHost, err := st.ActiveByHost(ctx, hostID)
	must(t, err)
	if len(byHost) != 1 || byHost[0].ID != p1.ID {
		t.Fatalf("ActiveByHost: %+v", byHost)
	}

	// UpdateStatus
	must(t, st.UpdateStatus(ctx, p1.ID, preview.StateRunning, "http://127.0.0.1:41000", "", t0.Add(time.Minute)))
	got, err = st.ByID(ctx, p1.ID)
	must(t, err)
	if got.State != preview.StateRunning || got.URL != "http://127.0.0.1:41000" {
		t.Fatalf("UpdateStatus: %+v", got)
	}

	// List
	listWs, err := st.ListByWorkspace(ctx, f.ws.ID)
	must(t, err)
	if len(listWs) != 1 {
		t.Fatalf("ListByWorkspace: %+v", listWs)
	}

	listAg, err := st.ListByAgent(ctx, f.ws.ID, f.agent.ID)
	must(t, err)
	if len(listAg) != 1 {
		t.Fatalf("ListByAgent: %+v", listAg)
	}

	// StopActive
	must(t, st.StopActive(ctx, p1.ID, t0.Add(2*time.Minute)))
	got, err = st.ByID(ctx, p1.ID)
	must(t, err)
	if got.State != preview.StateStopped {
		t.Fatalf("StopActive: %+v", got)
	}

	// Now ActiveByAgentAndName should return db.ErrNotFound
	if _, err := st.ActiveByAgentAndName(ctx, f.agent.ID, "web"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after stop, got: %v", err)
	}

	// Create second preview and test StopActiveByAgent
	p2 := preview.Preview{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		AgentID:     f.agent.ID,
		HostID:      hostID,
		ChannelID:   f.channel.ID,
		Name:        "api",
		ComposePath: "compose.yml",
		State:       preview.StateRunning,
		ExpiresAt:   t0.Add(4 * time.Hour),
		CreatedAt:   t0.Add(3 * time.Minute),
		UpdatedAt:   t0.Add(3 * time.Minute),
	}
	must(t, st.Create(ctx, p2))
	must(t, st.StopActiveByAgent(ctx, f.agent.ID, t0.Add(4*time.Minute)))
	got2, err := st.ByID(ctx, p2.ID)
	must(t, err)
	if got2.State != preview.StateStopped {
		t.Fatalf("StopActiveByAgent: %+v", got2)
	}

	// Create third preview and test StopActiveByWorkspace
	p3 := preview.Preview{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		AgentID:     f.agent.ID,
		HostID:      hostID,
		ChannelID:   f.channel.ID,
		Name:        "worker",
		ComposePath: "compose.yml",
		State:       preview.StateRequested,
		ExpiresAt:   t0.Add(4 * time.Hour),
		CreatedAt:   t0.Add(5 * time.Minute),
		UpdatedAt:   t0.Add(5 * time.Minute),
	}
	must(t, st.Create(ctx, p3))
	must(t, st.StopActiveByWorkspace(ctx, f.ws.ID, t0.Add(6*time.Minute)))
	got3, err := st.ByID(ctx, p3.ID)
	must(t, err)
	if got3.State != preview.StateStopped {
		t.Fatalf("StopActiveByWorkspace: %+v", got3)
	}
}

func testAgentRules(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.AgentRules

	// Initially empty
	if _, err := st.GetLatest(ctx, f.ws.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty latest rules, got: %v", err)
	}
	if _, err := st.GetVersion(ctx, f.ws.ID, 1); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty version 1, got: %v", err)
	}
	history, err := st.ListHistory(ctx, f.ws.ID)
	must(t, err)
	if len(history) != 0 {
		t.Fatalf("expected empty history, got: %v", history)
	}

	// Insert version 1
	r1 := agentrules.Rules{
		WorkspaceID: f.ws.ID,
		Version:     1,
		Text:        "Rule 1",
		CreatedBy:   f.human.ID,
		CreatedAt:   t0,
	}
	must(t, st.Insert(ctx, r1))

	gotLatest, err := st.GetLatest(ctx, f.ws.ID)
	must(t, err)
	if gotLatest.Version != 1 || gotLatest.Text != "Rule 1" {
		t.Fatalf("unexpected latest rules: %+v", gotLatest)
	}

	gotV1, err := st.GetVersion(ctx, f.ws.ID, 1)
	must(t, err)
	if gotV1.Version != 1 || gotV1.Text != "Rule 1" {
		t.Fatalf("unexpected v1 rules: %+v", gotV1)
	}

	if _, err := st.GetVersion(ctx, f.ws.ID, 2); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing v2, got: %v", err)
	}

	// Insert version 2
	r2 := agentrules.Rules{
		WorkspaceID: f.ws.ID,
		Version:     2,
		Text:        "Rule 2",
		CreatedBy:   f.human.ID,
		CreatedAt:   t0.Add(time.Hour),
	}
	must(t, st.Insert(ctx, r2))

	gotLatest2, err := st.GetLatest(ctx, f.ws.ID)
	must(t, err)
	if gotLatest2.Version != 2 || gotLatest2.Text != "Rule 2" {
		t.Fatalf("unexpected latest rules after v2: %+v", gotLatest2)
	}

	history2, err := st.ListHistory(ctx, f.ws.ID)
	must(t, err)
	if len(history2) != 2 || history2[0].Version != 2 || history2[1].Version != 1 {
		t.Fatalf("unexpected history order: %+v", history2)
	}
}

func testNotes(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Notes

	rootMsg := conversation.Message{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		ChannelID:   f.channel.ID,
		AuthorID:    f.human.ID,
		Kind:        "text",
		Body:        "thread root",
		CreatedAt:   t0,
	}
	must(t, b.Conversation.InsertMessage(ctx, rootMsg))

	otherRootMsg := conversation.Message{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		ChannelID:   f.channel.ID,
		AuthorID:    f.human.ID,
		Kind:        "text",
		Body:        "other thread root",
		CreatedAt:   t0.Add(time.Second),
	}
	must(t, b.Conversation.InsertMessage(ctx, otherRootMsg))

	// Empty at first
	_, err := st.GetLatest(ctx, f.ws.ID, f.agent.ID, rootMsg.ID)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existing note, got %v", err)
	}

	// Create note 1
	n1Time := t0
	n1, err := st.Create(ctx, notes.Note{
		WorkspaceID:  f.ws.ID,
		AgentID:      f.agent.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "note version 1",
		CreatedAt:    n1Time,
	})
	must(t, err)

	got, err := st.GetLatest(ctx, f.ws.ID, f.agent.ID, rootMsg.ID)
	must(t, err)
	if got.Text != "note version 1" || got.ID != n1.ID {
		t.Fatalf("unexpected note: %+v", got)
	}

	// Create note 2 (newer version for same thread)
	n2Time := n1Time.Add(time.Minute)
	n2, err := st.Create(ctx, notes.Note{
		WorkspaceID:  f.ws.ID,
		AgentID:      f.agent.ID,
		ThreadRootID: rootMsg.ID,
		Text:         "note version 2",
		CreatedAt:    n2Time,
	})
	must(t, err)

	got2, err := st.GetLatest(ctx, f.ws.ID, f.agent.ID, rootMsg.ID)
	must(t, err)
	if got2.Text != "note version 2" || got2.ID != n2.ID {
		t.Fatalf("unexpected latest note: %+v", got2)
	}

	// Create note for other thread
	n3Time := n2Time.Add(time.Minute)
	n3, err := st.Create(ctx, notes.Note{
		WorkspaceID:  f.ws.ID,
		AgentID:      f.agent.ID,
		ThreadRootID: otherRootMsg.ID,
		Text:         "note for other thread",
		CreatedAt:    n3Time,
	})
	must(t, err)

	// List all notes for agent (newest first: n3, n2, n1)
	all, err := st.ListByAgent(ctx, f.ws.ID, f.agent.ID, nil)
	must(t, err)
	if len(all) != 3 || all[0].ID != n3.ID || all[1].ID != n2.ID || all[2].ID != n1.ID {
		t.Fatalf("unexpected list order: %+v", all)
	}

	// List notes for specific thread rootMsg.ID (newest first: n2, n1)
	threadNotes, err := st.ListByAgent(ctx, f.ws.ID, f.agent.ID, &rootMsg.ID)
	must(t, err)
	if len(threadNotes) != 2 || threadNotes[0].ID != n2.ID || threadNotes[1].ID != n1.ID {
		t.Fatalf("unexpected thread notes: %+v", threadNotes)
	}
}

func testDecisions(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Decisions

	// 1. NextNumber on empty workspace returns 1
	n1, err := st.NextNumber(ctx, f.ws.ID)
	must(t, err)
	if n1 != 1 {
		t.Fatalf("expected next number 1, got %d", n1)
	}

	// 2. Insert decision D-1
	d1 := decision.Decision{
		ID:          gen.New(),
		Number:      1,
		WorkspaceID: f.ws.ID,
		AuthorID:    f.human.ID,
		ChannelID:   f.channel.ID,
		Text:        "Use PostgreSQL as primary database",
		Rationale:   "ACID compliance and relational integrity needed",
		CreatedAt:   t0,
	}
	must(t, st.Insert(ctx, d1))

	// 3. NextNumber is now 2
	n2, err := st.NextNumber(ctx, f.ws.ID)
	must(t, err)
	if n2 != 2 {
		t.Fatalf("expected next number 2, got %d", n2)
	}

	// 4. Duplicate (workspace_id, number) returns conflict
	err = st.Insert(ctx, decision.Decision{
		ID:          gen.New(),
		Number:      1,
		WorkspaceID: f.ws.ID,
		AuthorID:    f.human.ID,
		ChannelID:   f.channel.ID,
		Text:        "duplicate",
		Rationale:   "duplicate",
		CreatedAt:   t0,
	})
	if !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected conflict on duplicate number, got %v", err)
	}

	// 5. Insert decision D-2 that supersedes D-1
	sup := 1
	d2 := decision.Decision{
		ID:          gen.New(),
		Number:      2,
		WorkspaceID: f.ws.ID,
		AuthorID:    f.agent.ID,
		ChannelID:   f.channel.ID,
		Text:        "Use CockroachDB for multi-region scale",
		Rationale:   "PostgreSQL does not support multi-region active-active out of the box",
		Supersedes:  &sup,
		CreatedAt:   t0.Add(time.Hour),
	}
	must(t, st.Insert(ctx, d2))

	// 6. GetByNumber for D-1 should show SupersededBy = 2
	got1, err := st.GetByNumber(ctx, f.ws.ID, 1)
	must(t, err)
	if got1.SupersededBy == nil || *got1.SupersededBy != 2 {
		t.Fatalf("expected D-1 to be superseded by 2, got %+v", got1.SupersededBy)
	}

	// GetByNumber for D-2 should show SupersededBy == nil
	got2, err := st.GetByNumber(ctx, f.ws.ID, 2)
	must(t, err)
	if got2.SupersededBy != nil {
		t.Fatalf("expected D-2 to not be superseded, got %+v", got2.SupersededBy)
	}
	if got2.Supersedes == nil || *got2.Supersedes != 1 {
		t.Fatalf("expected D-2 to supersede 1, got %+v", got2.Supersedes)
	}

	// 7. List validOnly: true -> returns only D-2
	validList, err := st.List(ctx, f.ws.ID, 10, "", true)
	must(t, err)
	if len(validList) != 1 || validList[0].Number != 2 {
		t.Fatalf("expected only D-2 in valid list, got %+v", validList)
	}

	// List validOnly: false -> returns D-2 and D-1 (newest first)
	allList, err := st.List(ctx, f.ws.ID, 10, "", false)
	must(t, err)
	if len(allList) != 2 || allList[0].Number != 2 || allList[1].Number != 1 {
		t.Fatalf("expected D-2 and D-1 in all list, got %+v", allList)
	}

	// Query filter: "PostgreSQL" matches both (D-1 text and D-2 rationale)
	qList, err := st.List(ctx, f.ws.ID, 10, "PostgreSQL", false)
	must(t, err)
	if len(qList) != 2 {
		t.Fatalf("expected 2 matches for 'PostgreSQL', got %d", len(qList))
	}

	// Query filter: "CockroachDB" matches only D-2
	qList2, err := st.List(ctx, f.ws.ID, 10, "CockroachDB", false)
	must(t, err)
	if len(qList2) != 1 || qList2[0].Number != 2 {
		t.Fatalf("expected 1 match for 'CockroachDB', got %+v", qList2)
	}
}

func testLeases(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Leases

	// 1. GetActiveByResource on empty -> db.ErrNotFound
	_, err := st.GetActiveByResource(ctx, f.ws.ID, "deploy:prod", t0)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent lease, got %v", err)
	}

	// 2. Create lease L1
	l1 := lease.Lease{
		ID:           gen.New(),
		WorkspaceID:  f.ws.ID,
		Resource:     "deploy:prod",
		HolderID:     f.agent.ID,
		HolderHandle: f.agent.Handle,
		Note:         "deploying release 1.0",
		AcquiredAt:   t0,
		ExpiresAt:    t0.Add(30 * time.Minute),
	}
	must(t, st.Create(ctx, l1))

	// 3. GetActiveByResource returns L1
	active, err := st.GetActiveByResource(ctx, f.ws.ID, "deploy:prod", t0)
	must(t, err)
	if active.ID != l1.ID || active.HolderID != f.agent.ID || active.Resource != "deploy:prod" {
		t.Fatalf("unexpected active lease: %+v", active)
	}

	// 4. Concurrently lock and test race safety on resource
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Tx.InTx(ctx, func(txCtx context.Context) error {
				if err := st.LockResource(txCtx, f.ws.ID, "deploy:prod"); err != nil {
					return err
				}
				_, _ = st.GetActiveByResource(txCtx, f.ws.ID, "deploy:prod", t0)
				return nil
			})
		}()
	}
	wg.Wait()

	// 5. Extend L1
	newExpires := t0.Add(60 * time.Minute)
	newNote := "extended deployment"
	must(t, st.Extend(ctx, l1.ID, newExpires, &newNote))

	ext, err := st.GetByID(ctx, l1.ID)
	must(t, err)
	if !ext.ExpiresAt.Equal(newExpires) || ext.Note != newNote {
		t.Fatalf("unexpected extended lease: %+v", ext)
	}

	// 6. Expired lease is not active
	_, err = st.GetActiveByResource(ctx, f.ws.ID, "deploy:prod", t0.Add(2*time.Hour))
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired lease, got %v", err)
	}

	// 7. Release L1
	nowRel := t0.Add(40 * time.Minute)
	must(t, st.Release(ctx, l1.ID, f.agent.ID, nowRel))

	_, err = st.GetActiveByResource(ctx, f.ws.ID, "deploy:prod", nowRel)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after release, got %v", err)
	}

	// 8. Create two active leases on different resources
	l2 := lease.Lease{
		ID:           gen.New(),
		WorkspaceID:  f.ws.ID,
		Resource:     "port:5433",
		HolderID:     f.agent.ID,
		HolderHandle: f.agent.Handle,
		Note:         "testing migration",
		AcquiredAt:   t0,
		ExpiresAt:    t0.Add(30 * time.Minute),
	}
	l3 := lease.Lease{
		ID:           gen.New(),
		WorkspaceID:  f.ws.ID,
		Resource:     "db:shared",
		HolderID:     f.human.ID,
		HolderHandle: f.human.Handle,
		Note:         "schema inspection",
		AcquiredAt:   t0,
		ExpiresAt:    t0.Add(30 * time.Minute),
	}
	must(t, st.Create(ctx, l2))
	must(t, st.Create(ctx, l3))

	activeList, err := st.ListActive(ctx, f.ws.ID, t0)
	must(t, err)
	if len(activeList) != 2 {
		t.Fatalf("expected 2 active leases, got %d", len(activeList))
	}

	// 9. ReleaseActiveByHolder releases agent's lease
	relHolder, err := st.ReleaseActiveByHolder(ctx, f.ws.ID, f.agent.ID, f.human.ID, t0)
	must(t, err)
	if len(relHolder) != 1 || relHolder[0].Resource != "port:5433" {
		t.Fatalf("expected 1 released lease for holder, got %+v", relHolder)
	}

	// Only l3 remains active
	activeList, err = st.ListActive(ctx, f.ws.ID, t0)
	must(t, err)
	if len(activeList) != 1 || activeList[0].Resource != "db:shared" {
		t.Fatalf("expected 1 active lease remaining, got %+v", activeList)
	}

	// 10. ReleaseActiveByWorkspace releases all remaining
	relWs, err := st.ReleaseActiveByWorkspace(ctx, f.ws.ID, f.human.ID, t0)
	must(t, err)
	if len(relWs) != 1 || relWs[0].Resource != "db:shared" {
		t.Fatalf("expected 1 released lease for workspace, got %+v", relWs)
	}

	activeList, err = st.ListActive(ctx, f.ws.ID, t0)
	must(t, err)
	if len(activeList) != 0 {
		t.Fatalf("expected 0 active leases, got %d", len(activeList))
	}
}

func testSidebar(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Sidebar

	ch2 := conversation.Channel{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		Kind:        conversation.KindPublic,
		Name:        "dev",
		CreatedBy:   f.human.ID,
		CreatedAt:   t0,
	}
	must(t, b.Conversation.CreateChannel(ctx, ch2))

	sec1 := sidebar.Section{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		OwnerID:     f.human.ID,
		Name:        "Section 1",
		Position:    0,
		Collapsed:   false,
		CreatedAt:   t0,
		UpdatedAt:   t0,
	}
	must(t, st.CreateSection(ctx, sec1))

	sec2 := sidebar.Section{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		OwnerID:     f.human.ID,
		Name:        "Section 2",
		Position:    1,
		Collapsed:   false,
		CreatedAt:   t0.Add(time.Minute),
		UpdatedAt:   t0.Add(time.Minute),
	}
	must(t, st.CreateSection(ctx, sec2))

	list, err := st.ListSections(ctx, f.ws.ID, f.human.ID)
	must(t, err)
	if len(list) != 2 || list[0].Name != "Section 1" || list[1].Name != "Section 2" {
		t.Fatalf("unexpected sections list: %+v", list)
	}

	newName := "Renamed 1"
	collapsed := true
	upd, err := st.UpdateSection(ctx, sec1.ID, &newName, nil, &collapsed, t0.Add(2*time.Minute))
	must(t, err)
	if upd.Name != "Renamed 1" || !upd.Collapsed {
		t.Fatalf("update failed: %+v", upd)
	}

	must(t, st.SetItem(ctx, f.human.ID, sec1.ID, f.channel.ID, 0))
	must(t, st.SetItem(ctx, f.human.ID, sec1.ID, ch2.ID, 1))

	gotSec, err := st.GetSection(ctx, sec1.ID)
	must(t, err)
	if len(gotSec.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(gotSec.Items))
	}

	must(t, st.SetItem(ctx, f.human.ID, sec2.ID, ch2.ID, 0))

	gotSec1, err := st.GetSection(ctx, sec1.ID)
	must(t, err)
	if len(gotSec1.Items) != 1 || gotSec1.Items[0].ChannelID != f.channel.ID {
		t.Fatalf("channel was not removed from previous section: %+v", gotSec1.Items)
	}
	gotSec2, err := st.GetSection(ctx, sec2.ID)
	must(t, err)
	if len(gotSec2.Items) != 1 || gotSec2.Items[0].ChannelID != ch2.ID {
		t.Fatalf("channel not added to new section: %+v", gotSec2.Items)
	}

	must(t, st.ReorderSections(ctx, f.human.ID, []ids.ID{sec2.ID, sec1.ID}))
	list, err = st.ListSections(ctx, f.ws.ID, f.human.ID)
	must(t, err)
	if list[0].ID != sec2.ID || list[1].ID != sec1.ID {
		t.Fatalf("reordering failed: %+v", list)
	}

	must(t, st.DeleteSection(ctx, sec2.ID, t0.Add(3*time.Minute)))
	list, err = st.ListSections(ctx, f.ws.ID, f.human.ID)
	must(t, err)
	if len(list) != 1 || list[0].ID != sec1.ID {
		t.Fatalf("delete section failed: %+v", list)
	}
}

func testCompaction(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Compaction

	// 1. Settings
	gotInit, err := st.GetSettings(ctx, f.ws.ID)
	must(t, err)
	if gotInit.CompactThresholdTokens != compaction.DefaultThresholdTokens {
		t.Fatalf("expected default threshold %d, got %d", compaction.DefaultThresholdTokens, gotInit.CompactThresholdTokens)
	}

	defThresh := 60000
	settings := compaction.Settings{
		WorkspaceID:            f.ws.ID,
		CompactThresholdTokens: defThresh,
		CompactorAgentID:       f.agent.ID,
		UpdatedAt:              t0,
	}
	must(t, st.UpsertSettings(ctx, settings))

	gotSt, err := st.GetSettings(ctx, f.ws.ID)
	must(t, err)
	if gotSt.CompactThresholdTokens != defThresh || gotSt.CompactorAgentID != f.agent.ID {
		t.Fatalf("unexpected settings: %+v", gotSt)
	}

	// 2. Channel threshold
	gotThresh, err := st.GetChannelThreshold(ctx, f.channel.ID)
	must(t, err)
	if gotThresh != nil {
		t.Fatalf("expected nil channel threshold, got %v", gotThresh)
	}

	chThresh := 30000
	must(t, st.SetChannelThreshold(ctx, f.channel.ID, &chThresh))
	gotThresh, err = st.GetChannelThreshold(ctx, f.channel.ID)
	must(t, err)
	if gotThresh == nil || *gotThresh != chThresh {
		t.Fatalf("expected %d, got %v", chThresh, gotThresh)
	}

	must(t, st.SetChannelThreshold(ctx, f.channel.ID, nil))
	gotThresh, err = st.GetChannelThreshold(ctx, f.channel.ID)
	must(t, err)
	if gotThresh != nil {
		t.Fatalf("expected nil channel threshold after clear, got %v", gotThresh)
	}

	// 3. Compaction unit
	// Initial unit: not found
	_, err = st.GetUnit(ctx, f.channel.ID, ids.Nil)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected db.ErrNotFound, got %v", err)
	}

	// Add tokens to main stream
	u1, err := st.AddTokensToUnit(ctx, f.ws.ID, f.channel.ID, ids.Nil, 1000, 1, t0.Add(time.Minute))
	must(t, err)
	if u1.MessagesSince != 1 || u1.ApproxTokensSince != 1000 {
		t.Fatalf("unexpected unit after add 1: %+v", u1)
	}
	u2, err := st.AddTokensToUnit(ctx, f.ws.ID, f.channel.ID, ids.Nil, 500, 1, t0.Add(2*time.Minute))
	must(t, err)
	if u2.MessagesSince != 2 || u2.ApproxTokensSince != 1500 {
		t.Fatalf("unexpected unit after add 2: %+v", u2)
	}

	// Thread unit
	threadRootMsg := conversation.Message{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		ChannelID:   f.channel.ID,
		AuthorID:    f.human.ID,
		Kind:        "text",
		Body:        "root message",
		CreatedAt:   t0,
	}
	must(t, b.Conversation.InsertMessage(ctx, threadRootMsg))
	threadRootID := threadRootMsg.ID
	ut1, err := st.AddTokensToUnit(ctx, f.ws.ID, f.channel.ID, threadRootID, 200, 1, t0.Add(3*time.Minute))
	must(t, err)
	if ut1.MessagesSince != 1 || ut1.ApproxTokensSince != 200 || ut1.ThreadRootID != threadRootID {
		t.Fatalf("unexpected thread unit: %+v", ut1)
	}

	// Check main unit not affected by thread
	uCheck, err := st.GetUnit(ctx, f.channel.ID, ids.Nil)
	must(t, err)
	if uCheck.MessagesSince != 2 || uCheck.ApproxTokensSince != 1500 {
		t.Fatalf("main unit affected by thread unit: %+v", uCheck)
	}

	// 4. Compactions
	_, err = st.PendingCompaction(ctx, f.channel.ID, ids.Nil)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected db.ErrNotFound for pending compaction, got %v", err)
	}

	compId := gen.New()
	comp := compaction.Compaction{
		ID:               compId,
		WorkspaceID:      f.ws.ID,
		ChannelID:        f.channel.ID,
		Trigger:          compaction.TriggerAuto,
		CompactorAgentID: f.agent.ID,
		State:            compaction.StatePending,
		MessagesCount:    2,
		ApproxTokensIn:   1500,
		CreatedAt:        t0.Add(5 * time.Minute),
	}
	must(t, st.InsertCompaction(ctx, comp))

	pending, err := st.PendingCompaction(ctx, f.channel.ID, ids.Nil)
	must(t, err)
	if pending.ID != compId || pending.State != compaction.StatePending {
		t.Fatalf("expected pending compaction %s, got %+v", compId, pending)
	}

	gotComp, err := st.CompactionByID(ctx, compId)
	must(t, err)
	if gotComp.State != compaction.StatePending || gotComp.CompactorAgentID != f.agent.ID {
		t.Fatalf("unexpected compaction: %+v", gotComp)
	}

	// Complete compaction
	summaryFile := files.File{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		UploadedBy:  f.agent.ID,
		ChannelID:   f.channel.ID,
		Name:        "summary.md",
		Mime:        "text/markdown",
		Size:        500,
		SHA256:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		CreatedAt:   t0,
	}
	must(t, b.Files.Insert(ctx, summaryFile))
	summaryFileID := summaryFile.ID

	sysMsg := conversation.Message{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		ChannelID:   f.channel.ID,
		AuthorID:    f.agent.ID,
		Kind:        "system",
		Body:        "compacted",
		CreatedAt:   t0.Add(6 * time.Minute),
	}
	must(t, b.Conversation.InsertMessage(ctx, sysMsg))
	sysMsgID := sysMsg.ID

	doneAt := t0.Add(6 * time.Minute)
	must(t, st.CompleteCompaction(ctx, compId, summaryFileID, sysMsgID, 2, 1500, 300, doneAt))

	gotComp, err = st.CompactionByID(ctx, compId)
	must(t, err)
	if gotComp.State != compaction.StateDone || gotComp.SummaryFileID != summaryFileID || gotComp.SystemMessageID != sysMsgID || gotComp.SummaryTokens != 300 {
		t.Fatalf("unexpected done compaction: %+v", gotComp)
	}

	// Pending is now not found
	_, err = st.PendingCompaction(ctx, f.channel.ID, ids.Nil)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected pending compaction to be not found, got %v", err)
	}

	// Reset unit
	must(t, st.ResetUnit(ctx, f.channel.ID, ids.Nil, compId, 0, 0, doneAt))
	uReset, err := st.GetUnit(ctx, f.channel.ID, ids.Nil)
	must(t, err)
	if uReset.MessagesSince != 0 || uReset.ApproxTokensSince != 0 || uReset.LastCompactionID != compId {
		t.Fatalf("unexpected unit after reset: %+v", uReset)
	}

	// Latest done compaction for unit
	lastComp, err := st.LatestDoneCompaction(ctx, f.channel.ID, ids.Nil)
	must(t, err)
	if lastComp.ID != compId {
		t.Fatalf("expected latest done compaction %s, got %+v", compId, lastComp)
	}

	// CompactionIDsBySystemMessageIDs
	mapping, err := st.CompactionIDsBySystemMessageIDs(ctx, []ids.ID{sysMsgID})
	must(t, err)
	if mapping[sysMsgID] != compId {
		t.Fatalf("expected mapping[%s] == %s, got %s", sysMsgID, compId, mapping[sysMsgID])
	}

	// Fail pending compaction test
	failCompId := gen.New()
	failComp := compaction.Compaction{
		ID:               failCompId,
		WorkspaceID:      f.ws.ID,
		ChannelID:        f.channel.ID,
		Trigger:          compaction.TriggerManual,
		CompactorAgentID: f.agent.ID,
		State:            compaction.StatePending,
		CreatedAt:        t0.Add(10 * time.Minute),
	}
	must(t, st.InsertCompaction(ctx, failComp))
	must(t, st.FailPendingCompaction(ctx, failCompId, "agent_error", t0.Add(12*time.Minute)))

	gotFail, err := st.CompactionByID(ctx, failCompId)
	must(t, err)
	if gotFail.State != compaction.StateFailed || gotFail.ErrorCode != "agent_error" {
		t.Fatalf("expected failed compaction, got %+v", gotFail)
	}
}

func testPlanner(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	st := b.Planner
	owner := f.human.ID

	if _, ok, err := st.MinPosition(ctx, owner); err != nil || ok {
		t.Fatalf("empty min position: %v %v", ok, err)
	}
	done := t0.Add(-20 * 24 * time.Hour)
	recentDone := t0.Add(-time.Hour)
	mk := func(title, status string, pos float64, doneAt *time.Time) planner.Task {
		tk := planner.Task{ID: gen.New(), WorkspaceID: f.ws.ID, OwnerID: owner, Title: title, Status: status,
			Position: pos, DoneAt: doneAt, StatusChangedAt: t0, CreatedAt: t0, UpdatedAt: t0}
		must(t, st.InsertTask(ctx, tk))
		return tk
	}
	a := mk("a", planner.StatusScheduled, 2, nil)
	bTask := mk("b", planner.StatusStarted, 1, nil)
	mk("old done", planner.StatusDone, 0, &done)
	c := mk("recent done", planner.StatusDone, 3, &recentDone)
	if err := st.InsertTask(ctx, a); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("duplicate task: %v", err)
	}
	got, err := st.Tasks(ctx, owner, t0.Add(-14*24*time.Hour))
	must(t, err)
	if len(got) != 3 || got[0].ID != bTask.ID || got[1].ID != a.ID || got[2].ID != c.ID {
		t.Fatalf("tasks order/filter: %+v", got)
	}
	if minPos, ok, err := st.MinPosition(ctx, owner); err != nil || !ok || minPos != 0 {
		t.Fatalf("min position: %v %v %v", minPos, ok, err)
	}
	started := t0.Add(time.Minute)
	a.Status, a.StartedAt, a.Title, a.Position = planner.StatusStarted, &started, "a2", 0.5
	must(t, st.UpdateTask(ctx, a))
	ga, err := st.TaskByID(ctx, a.ID)
	must(t, err)
	if ga.Title != "a2" || ga.Status != planner.StatusStarted || ga.StartedAt == nil || !ga.StartedAt.Equal(started) || ga.Position != 0.5 {
		t.Fatalf("updated task: %+v", ga)
	}
	del := t0.Add(2 * time.Minute)
	a.DeletedAt = &del
	must(t, st.UpdateTask(ctx, a))
	got, _ = st.Tasks(ctx, owner, t0.Add(-14*24*time.Hour))
	if len(got) != 2 {
		t.Fatalf("deleted task still listed: %+v", got)
	}
	if err := st.UpdateTask(ctx, planner.Task{ID: gen.New()}); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if _, err := st.TaskByID(ctx, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("missing task: %v", err)
	}

	if _, err := st.Access(ctx, owner); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("missing access: %v", err)
	}
	acc := planner.Access{OwnerID: owner, WorkspaceID: f.ws.ID, TaskAll: false, TaskAgentIDs: []ids.ID{f.agent.ID}, UpdatedAt: t0}
	must(t, st.PutAccess(ctx, acc))
	acc.CalAgentIDs = []ids.ID{f.agent.ID}
	must(t, st.PutAccess(ctx, acc))
	ga2, err := st.Access(ctx, owner)
	must(t, err)
	if ga2.TaskAll || len(ga2.TaskAgentIDs) != 1 || len(ga2.CalAgentIDs) != 1 || ga2.CalAgentIDs[0] != f.agent.ID {
		t.Fatalf("access: %+v", ga2)
	}
	list, err := st.AccessList(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 1 || list[0].OwnerID != owner {
		t.Fatalf("access list: %+v", list)
	}

	cal := planner.Calendar{ID: gen.New(), WorkspaceID: f.ws.ID, OwnerID: owner, Name: "work", URL: "https://example.com/x.ics", Colour: "#112233", CreatedAt: t0}
	must(t, st.InsertCalendar(ctx, cal))
	due, err := st.CalendarsDue(ctx, t0)
	must(t, err)
	if len(due) != 1 {
		t.Fatalf("never fetched calendar is due: %+v", due)
	}
	must(t, st.SetCalendarFetch(ctx, cal.ID, t0, "HTTP 404"))
	gc, err := st.CalendarByID(ctx, cal.ID)
	must(t, err)
	if gc.LastFetchedAt == nil || gc.LastError != "HTTP 404" || gc.URL != cal.URL {
		t.Fatalf("calendar fetch state: %+v", gc)
	}
	if due, _ = st.CalendarsDue(ctx, t0); len(due) != 0 {
		t.Fatalf("just fetched calendar is due: %+v", due)
	}
	day := t0.Truncate(24 * time.Hour)
	evs := []planner.Event{
		{CalendarID: cal.ID, OwnerID: owner, Title: "standup", Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour)},
		{CalendarID: cal.ID, OwnerID: owner, Title: "holiday", Start: day, End: day.Add(24 * time.Hour), AllDay: true},
		{CalendarID: cal.ID, OwnerID: owner, Title: "next week", Start: day.Add(7 * 24 * time.Hour), End: day.Add(7*24*time.Hour + time.Hour)},
	}
	must(t, st.ReplaceEvents(ctx, cal.ID, evs))
	must(t, st.ReplaceEvents(ctx, cal.ID, evs)) // replacing must not duplicate
	got2, err := st.Events(ctx, owner, day, day.Add(24*time.Hour))
	must(t, err)
	if len(got2) != 2 || got2[0].Title != "holiday" || got2[1].Title != "standup" || !got2[0].AllDay {
		t.Fatalf("events: %+v", got2)
	}
	cals, err := st.Calendars(ctx, owner)
	must(t, err)
	if len(cals) != 1 {
		t.Fatalf("calendars: %+v", cals)
	}
	must(t, st.DeleteCalendar(ctx, cal.ID))
	if got2, _ = st.Events(ctx, owner, day, day.Add(24*time.Hour)); len(got2) != 0 {
		t.Fatalf("events kept after calendar delete: %+v", got2)
	}
}
