package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func (e *env) createActor(kind, handle, role string) (id, token string) {
	if kind == "agent" {
		// Agents are created with POST /agents (round 33), not in #general here.
		var created struct {
			Actor struct{ ID string }
			Token string
		}
		if s := e.do("POST", "/api/v1/agents", server.DevAdminToken,
			map[string]any{"handle": handle, "api_key": true}, &created); s != http.StatusCreated {
			e.t.Fatalf("create agent %s: status %d", handle, s)
		}
		return created.Actor.ID, created.Token
	}
	var created struct {
		Actor struct{ ID string }
		Token string
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken,
		map[string]any{"kind": kind, "handle": handle, "role": role}, &created); s != http.StatusCreated {
		e.t.Fatalf("create %s: status %d", handle, s)
	}
	return created.Actor.ID, created.Token
}

func (e *env) inbox(token string) []struct {
	ID, Kind, Author, Body string
	ItemID                 string `json:"item_id"`
} {
	var in struct {
		Items []struct {
			ID, Kind, Author, Body string
			ItemID                 string `json:"item_id"`
		}
	}
	if err := e.app.Drain(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	if s := e.do("GET", "/api/v1/agent/inbox", token, nil, &in); s != http.StatusOK {
		e.t.Fatalf("inbox: status %d", s)
	}
	return in.Items
}

func reactionPath(msgID, emoji string) string {
	return "/api/v1/messages/" + msgID + "/reactions/" + url.PathEscape(emoji)
}

func TestDirectMessages(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	var ch1, ch2 struct {
		ID        string
		Kind      string
		MemberIDs []string `json:"member_ids"`
	}
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{agent}}, &ch1); s != http.StatusOK {
		t.Fatalf("open dm: status %d", s)
	}
	if ch1.Kind != "dm" {
		t.Fatalf("kind %q", ch1.Kind)
	}
	// The other side opening it gets the same channel.
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &me)
	e.do("POST", "/api/v1/dms", server.DevAgentToken, map[string]any{"actor_ids": []string{me.ID}}, &ch2)
	if ch1.ID == "" || ch1.ID != ch2.ID {
		t.Fatalf("dm not deduplicated: %s vs %s", ch1.ID, ch2.ID)
	}

	// A DM wakes the agent without a mention.
	if _, s := e.send(server.DevAdminToken, ch1.ID, map[string]any{"body": "are you there"}); s != http.StatusCreated {
		t.Fatalf("send dm: status %d", s)
	}
	items := e.inbox(server.DevAgentToken)
	if len(items) != 1 || items[0].Body != "are you there" || items[0].Author != "norbi" {
		t.Fatalf("agent inbox: %+v", items)
	}

	// Someone outside the DM cannot read it, nor open a DM with themselves only.
	_, outsider := e.createActor("agent", "outsider", "member")
	if s := e.do("GET", "/api/v1/channels/"+ch1.ID+"/messages", outsider, nil, nil); s != http.StatusForbidden {
		t.Fatalf("outsider reads dm: status %d", s)
	}
	var outsiderMe struct{ ID string }
	e.do("GET", "/api/v1/me", outsider, nil, &outsiderMe)
	if s := e.do("POST", "/api/v1/dms", outsider, map[string]any{"actor_ids": []string{outsiderMe.ID}}, nil); s != http.StatusBadRequest {
		t.Fatalf("dm with self only: status %d", s)
	}
}

func TestThreadEndpoint(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "question"})
	e.send(server.DevAgentToken, general, map[string]any{"body": "an answer with substance", "thread_root_id": root.ID})
	e.send(server.DevAdminToken, general, map[string]any{"body": "unrelated"})

	var th struct {
		Root    message
		Replies []message
	}
	if s := e.do("GET", "/api/v1/messages/"+root.ID+"/thread", server.DevAdminToken, nil, &th); s != http.StatusOK {
		t.Fatalf("thread: status %d", s)
	}
	if th.Root.ID != root.ID || len(th.Replies) != 1 || th.Replies[0].Body != "an answer with substance" {
		t.Fatalf("thread: %+v", th)
	}
	// Asking with a reply's id returns the same thread.
	var th2 struct{ Root message }
	e.do("GET", "/api/v1/messages/"+th.Replies[0].ID+"/thread", server.DevAdminToken, nil, &th2)
	if th2.Root.ID != root.ID {
		t.Fatalf("thread via reply: root %s", th2.Root.ID)
	}
}

// TestThreadParticipants: a thread root carries its most recent repliers
// (for the "N válasz" summary row) in the channel list and the thread.
func TestThreadParticipants(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "plan?"})
	plain, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "no replies"})
	e.send(server.DevAgentToken, general, map[string]any{"body": "agent first", "thread_root_id": root.ID})
	e.send(server.DevAdminToken, general, map[string]any{"body": "then the admin", "thread_root_id": root.ID})

	var admin, agent struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &admin)
	e.do("GET", "/api/v1/me", server.DevAgentToken, nil, &agent)
	want := []string{admin.ID, agent.ID}

	var list apispec.MessageList
	e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, nil, &list)
	found := 0
	for _, m := range list.Messages {
		switch m.Id.String() {
		case root.ID:
			found++
			if m.ReplyParticipantIds == nil || fmt.Sprint(*m.ReplyParticipantIds) != fmt.Sprint(want) {
				t.Fatalf("root participants in the list: %v, want %v", m.ReplyParticipantIds, want)
			}
		case plain.ID:
			found++
			if m.ReplyParticipantIds != nil {
				t.Fatalf("a message without replies has participants: %v", *m.ReplyParticipantIds)
			}
		}
	}
	if found != 2 {
		t.Fatalf("messages not in the list: %d", found)
	}

	var th apispec.Thread
	e.do("GET", "/api/v1/messages/"+root.ID+"/thread", server.DevAdminToken, nil, &th)
	if th.Root.ReplyParticipantIds == nil || fmt.Sprint(*th.Root.ReplyParticipantIds) != fmt.Sprint(want) {
		t.Fatalf("root participants in the thread: %v", th.Root.ReplyParticipantIds)
	}
}

func TestReactions(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "ship it?"})
	for range 2 { // the second PUT is a no-op
		if s := e.do("PUT", reactionPath(m.ID, "👍"), server.DevAgentToken, nil, nil); s != http.StatusNoContent {
			t.Fatalf("react: status %d", s)
		}
	}
	if s := e.do("PUT", reactionPath(m.ID, "<b>"), server.DevAgentToken, nil, nil); s != http.StatusBadRequest {
		t.Fatalf("bad emoji: status %d", s)
	}
	var list struct{ Messages []message }
	e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, nil, &list)
	got := list.Messages[len(list.Messages)-1].Reactions
	if len(got) != 1 || got[0].Emoji != "👍" || got[0].Count != 1 {
		t.Fatalf("reactions: %+v", got)
	}
	if s := e.do("DELETE", reactionPath(m.ID, "👍"), server.DevAgentToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("unreact: status %d", s)
	}
	var after struct{ Messages []message }
	e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, nil, &after)
	if r := after.Messages[len(after.Messages)-1].Reactions; len(r) != 0 {
		t.Fatalf("after unreact: %+v", r)
	}
}

// TestThumbsDownReachesAgent: a 👎 on an agent's message lands in its inbox
// (from the reactor), but a 👍 does not.
func TestThumbsDownReachesAgent(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	m, _ := e.send(server.DevAgentToken, general, map[string]any{"body": "I will delete the old branch"})
	e.do("PUT", reactionPath(m.ID, "👍"), server.DevAdminToken, nil, nil)
	if items := e.inbox(server.DevAgentToken); len(items) != 0 {
		t.Fatalf("👍 must not reach the inbox: %+v", items)
	}
	e.do("PUT", reactionPath(m.ID, "👎"), server.DevAdminToken, nil, nil)
	items := e.inbox(server.DevAgentToken)
	if len(items) != 1 || items[0].Kind != "reaction_down" || items[0].Author != "norbi" || items[0].ItemID != m.ID {
		t.Fatalf("👎 inbox: %+v", items)
	}
}

func TestAgentMessageRules(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent can you check the build?"})

	// An acknowledgement becomes a 👍 on the message it answers.
	var res sendResult
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken,
		map[string]any{"body": "OK, thanks!", "thread_root_id": q.ID}, &res); s != http.StatusOK {
		t.Fatalf("ack: status %d", s)
	}
	if res.Message != nil || res.ConvertedToReaction == nil || res.ConvertedToReaction.MessageID != q.ID {
		t.Fatalf("ack result: %+v", res)
	}

	long := strings.Repeat("word ", 150)
	if _, s := e.send(server.DevAgentToken, general, map[string]any{"body": long}); s != http.StatusBadRequest {
		t.Fatalf("long message: status %d", s)
	}
	if _, s := e.send(server.DevAgentToken, general, map[string]any{"body": long, "elaborate": true}); s != http.StatusCreated {
		t.Fatalf("elaborate: status %d", s)
	}
	if _, s := e.send(server.DevAgentToken, general, map[string]any{"body": "@here the build is red"}); s != http.StatusBadRequest {
		t.Fatalf("@here: status %d", s)
	}
	for _, h := range []string{"a1", "a2", "a3", "a4"} {
		e.createActor("agent", h, "member")
	}
	if _, s := e.send(server.DevAgentToken, general, map[string]any{"body": "@a1 @a2 @a3 @a4 look"}); s != http.StatusBadRequest {
		t.Fatalf("4 mentions: status %d", s)
	}
	// Humans are not limited.
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "OK"}); s != http.StatusCreated {
		t.Fatalf("human ok: status %d", s)
	}
}

// TestDMNameCannotBeSquatted: nobody can pre-create a normal channel under the
// name a DM will get, so opening the DM never returns someone else's channel.
func TestDMNameCannotBeSquatted(t *testing.T) {
	e := newEnv(t)
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAgentToken, nil, &me)
	var admin struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &admin)
	a, b := uuid.MustParse(me.ID), uuid.MustParse(admin.ID)
	if bytes.Compare(a[:], b[:]) > 0 {
		a, b = b, a
	}
	sum := sha256.Sum256(append(a[:], b[:]...))
	name := "dm-" + hex.EncodeToString(sum[:])[:16]

	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": name, "kind": "public"}, nil); s != http.StatusBadRequest {
		t.Fatalf("squat %s: status %d", name, s)
	}
	var dm struct{ Name, Kind string }
	e.do("POST", "/api/v1/dms", server.DevAgentToken, map[string]any{"actor_ids": []string{admin.ID}}, &dm)
	if dm.Kind != "dm" || dm.Name != name {
		t.Fatalf("dm: %+v (expected name %s)", dm, name)
	}
}

// TestThreadReplyReachesAgentsInTheThread: a human's reply in a channel
// thread, without a mention, reaches the agents that wrote in that thread;
// a reply with a mention reaches only the mentioned; agents' replies and
// threads the agent never wrote in reach nobody this way.
func TestThreadReplyReachesAgentsInTheThread(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	ack := func(token string) {
		var ids []string
		for _, it := range e.inbox(token) {
			ids = append(ids, it.ID)
		}
		if len(ids) > 0 {
			e.do("POST", "/api/v1/agent/inbox/ack", token, map[string]any{"ids": ids}, nil)
		}
	}
	bodies := func(token string) []string {
		var out []string
		for _, it := range e.inbox(token) {
			out = append(out, it.Body)
		}
		return out
	}

	// A second agent in #general.
	var other struct {
		Actor struct{ ID string }
		Token string
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "helper", "api_key": true}, &other); s != http.StatusCreated {
		t.Fatalf("create helper: %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": other.Actor.ID, "can_write": true}, nil); s != http.StatusNoContent {
		t.Fatalf("add helper: %d", s)
	}

	// The human asks @fake-agent; it answers in the thread.
	root, s := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent nézd meg a logokat"})
	if s != http.StatusCreated {
		t.Fatalf("root: %d", s)
	}
	ack(server.DevAgentToken)
	if _, s := e.send(server.DevAgentToken, general, map[string]any{"body": "Megnéztem, minden rendben.", "thread_root_id": root.ID}); s != http.StatusCreated {
		t.Fatalf("agent reply: %d", s)
	}

	// 1. A plain human reply in that thread reaches @fake-agent, not @helper.
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "És a tegnapi futás?", "thread_root_id": root.ID}); s != http.StatusCreated {
		t.Fatalf("reply: %d", s)
	}
	if got := bodies(server.DevAgentToken); len(got) != 1 || got[0] != "És a tegnapi futás?" {
		t.Fatalf("the agent in the thread should get the plain reply, got %v", got)
	}
	if got := bodies(other.Token); len(got) != 0 {
		t.Fatalf("an agent that never wrote in the thread got %v", got)
	}
	ack(server.DevAgentToken)

	// 2. A reply that mentions @helper reaches only @helper.
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "@helper nézd meg te is", "thread_root_id": root.ID}); s != http.StatusCreated {
		t.Fatalf("mention reply: %d", s)
	}
	if got := bodies(server.DevAgentToken); len(got) != 0 {
		t.Fatalf("a reply mentioning someone else reached the thread's agent: %v", got)
	}
	if got := bodies(other.Token); len(got) != 1 {
		t.Fatalf("the mentioned agent should get it, got %v", got)
	}
	ack(other.Token)

	// 3. An agent's reply in the thread wakes nobody (no loops between agents).
	if _, s := e.send(other.Token, general, map[string]any{"body": "Én is megnéztem.", "thread_root_id": root.ID}); s != http.StatusCreated {
		t.Fatalf("helper reply: %d", s)
	}
	if got := bodies(server.DevAgentToken); len(got) != 0 {
		t.Fatalf("an agent's reply woke the other agent: %v", got)
	}

	// 4. A plain reply in a thread @fake-agent never wrote in reaches nobody.
	root2, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "Más téma."})
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "Folytatás.", "thread_root_id": root2.ID}); s != http.StatusCreated {
		t.Fatalf("reply2: %d", s)
	}
	if got := bodies(server.DevAgentToken); len(got) != 0 {
		t.Fatalf("a thread the agent never wrote in reached it: %v", got)
	}
}
