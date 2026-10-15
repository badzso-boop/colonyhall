package e2e

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"colonyhall/internal/server"
)

func (e *env) createActor(kind, handle, role string) (id, token string) {
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
