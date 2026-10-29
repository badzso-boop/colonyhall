package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// dialWS connects to /ws/ui with token and waits until the replay is done.
func (e *env) dialWS(ctx context.Context, token string) *websocket.Conn {
	e.t.Helper()
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", nil)
	if err != nil {
		e.t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	e.t.Cleanup(func() { _ = ws.CloseNow() })
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": token}); err != nil {
		e.t.Fatal(err)
	}
	expectFrame(e.t, ctx, ws, "welcome")
	expectFrame(e.t, ctx, ws, "replay_done")
	return ws
}

// eventsUntilMessage reads event frames until a message.created with body,
// returning the event types seen before it.
func eventsUntilMessage(t *testing.T, ctx context.Context, ws *websocket.Conn, body string) []frame {
	t.Helper()
	var seen []frame
	for {
		f := expectFrame(t, ctx, ws, "event")
		if f.EventType == "message.created" {
			var p struct{ Body string }
			_ = json.Unmarshal(f.Data, &p)
			if p.Body == body {
				return seen
			}
		}
		seen = append(seen, f)
	}
}

func itemCreatedFor(t *testing.T, frames []frame, recipientID string) int {
	t.Helper()
	n := 0
	for _, f := range frames {
		if f.EventType != "delivery.item_created" {
			continue
		}
		var p map[string]any
		_ = json.Unmarshal(f.Data, &p)
		if p["recipient_id"] == recipientID {
			// The signal names no channel: the recipient need not be a member.
			if len(p) != 2 || p["item_kind"] == nil {
				t.Fatalf("delivery.item_created carries more than recipient_id and item_kind: %s", f.Data)
			}
			n++
		}
	}
	return n
}

// TestApprovedDelegationSignalsTheTarget: the guard checks its inbox on any
// WebSocket event; the approved item is written after the approval, so
// without a signal of its own the target waited for its fallback poll
// (2026-10-29: up to a minute). The signal reaches the recipient alone.
func TestApprovedDelegationSignalsTheTarget(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	hostID, _ := e.enroll("machine-wake")
	sourceID, sourceTok := createSandboxedAgent(e, hostID, "wake-source", "Source")
	targetID, targetTok := createFullAccessAgent(e, hostID, "wake-target", "Target")

	targetWS := e.dialWS(ctx, targetTok)
	sourceWS := e.dialWS(ctx, sourceTok)

	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@wake-target build the release",
	}, nil); s != http.StatusCreated {
		t.Fatalf("send: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("want 1 open question, got %d", len(qList.Questions))
	}
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken,
		apispec.AnswerQuestionRequest{Answer: "Jóváhagyom"}, nil); s != http.StatusOK {
		t.Fatalf("approve: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// A sentinel message both agents can see marks the end of the stream.
	const sentinel = "sentinel after approval"
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": sentinel}); s != http.StatusCreated {
		t.Fatalf("sentinel: %d", s)
	}

	targetFrames := eventsUntilMessage(t, ctx, targetWS, sentinel)
	if n := itemCreatedFor(t, targetFrames, targetID); n != 1 {
		t.Fatalf("target got %d delivery.item_created signals, want 1 (events: %v)", n, eventTypes(targetFrames))
	}
	// When the signal arrives the item is already in the inbox.
	var inbox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inbox)
	if inbox.Total != 1 || inbox.Items[0].Kind != "delegated" {
		t.Fatalf("target inbox: %+v", inbox)
	}

	// The source agent sees the channel but not the target's inbox signal.
	sourceFrames := eventsUntilMessage(t, ctx, sourceWS, sentinel)
	for _, f := range sourceFrames {
		if f.EventType == "delivery.item_created" {
			t.Fatalf("source agent %s received another agent's inbox signal: %s", sourceID, f.Data)
		}
	}
}

// TestDeniedDelegationSignalsTheSource: the source agent's answer item after
// a denial gets the same signal, for the source only.
func TestDeniedDelegationSignalsTheSource(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	hostID, _ := e.enroll("machine-wake-deny")
	sourceID, sourceTok := createSandboxedAgent(e, hostID, "deny-source", "Source")
	_, targetTok := createFullAccessAgent(e, hostID, "deny-target", "Target")

	sourceWS := e.dialWS(ctx, sourceTok)
	targetWS := e.dialWS(ctx, targetTok)

	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{Body: "@deny-target wipe the disk"}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("want 1 open question, got %d", len(qList.Questions))
	}
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken,
		apispec.AnswerQuestionRequest{Answer: "Elutasítom"}, nil); s != http.StatusOK {
		t.Fatalf("deny: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	const sentinel = "sentinel after denial"
	e.send(server.DevAdminToken, general, map[string]any{"body": sentinel})

	if n := itemCreatedFor(t, eventsUntilMessage(t, ctx, sourceWS, sentinel), sourceID); n != 1 {
		t.Fatalf("source got %d answer signals, want 1", n)
	}
	for _, f := range eventsUntilMessage(t, ctx, targetWS, sentinel) {
		if f.EventType == "delivery.item_created" {
			t.Fatalf("denied target received an inbox signal: %s", f.Data)
		}
	}
}

func eventTypes(frames []frame) []string {
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		out = append(out, f.EventType)
	}
	return out
}
