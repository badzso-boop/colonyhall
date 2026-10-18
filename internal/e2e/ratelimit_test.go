package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func TestAgentRateLimitAndParallelMentionsE2E(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	general := e.generalID()

	// 1. Create two agents: bot-one and bot-two
	_, token1 := e.createAgent("bot-one", "Bot One")
	agent2ID, token2 := e.createAgent("bot-two", "Bot Two")

	// 2. Test Parallel Mentions:
	// A human posts a message in #general mentioning both @bot-one and @bot-two
	mentionMsg, status := e.send(server.DevAdminToken, general, map[string]any{
		"body": "Hey @bot-one and @bot-two, please work on this task in parallel",
	})
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for human message, got %d", status)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain after mention msg: %v", err)
	}

	// Verify both bot-one and bot-two received an inbox item
	var inbox1 apispec.Inbox
	status = e.do(http.MethodGet, "/api/v1/agent/inbox", token1, nil, &inbox1)
	if status != http.StatusOK {
		t.Fatalf("expected 200 GET /api/v1/inbox for bot-one, got %d", status)
	}
	found1 := false
	for _, item := range inbox1.Items {
		if item.ChannelId.String() == general {
			found1 = true
			if !item.Mentioned {
				t.Fatalf("expected mentioned=true for bot-one")
			}
			break
		}
	}
	if !found1 {
		t.Fatalf("bot-one did not receive inbox item for mention")
	}

	var inbox2 apispec.Inbox
	status = e.do(http.MethodGet, "/api/v1/agent/inbox", token2, nil, &inbox2)
	if status != http.StatusOK {
		t.Fatalf("expected 200 GET /api/v1/inbox for bot-two, got %d", status)
	}
	found2 := false
	for _, item := range inbox2.Items {
		if item.ChannelId.String() == general {
			found2 = true
			if !item.Mentioned {
				t.Fatalf("expected mentioned=true for bot-two")
			}
			break
		}
	}
	if !found2 {
		t.Fatalf("bot-two did not receive inbox item for mention")
	}

	// 3. Test Agent-Agent Rate Limit in Thread:
	// Post 20 agent messages into the thread of mentionMsg
	threadRootID := mentionMsg.ID

	for i := 1; i <= 20; i++ {
		tok := token1
		if i%2 == 0 {
			tok = token2
		}
		_, status = e.send(tok, general, map[string]any{
			"thread_root_id": threadRootID,
			"body":           fmt.Sprintf("Agent message reply #%d in progress", i),
		})
		if status != http.StatusCreated {
			t.Fatalf("expected 201 for agent message #%d, got %d", i, status)
		}
	}

	// 21st agent message should be refused with 429 Too Many Requests and pause error
	var errResp apispec.Problem
	status = e.do(http.MethodPost, fmt.Sprintf("/api/v1/channels/%s/messages", general), token1, map[string]any{
		"thread_root_id": threadRootID,
		"body":           "Agent message #21 attempting to send",
	}, &errResp)
	if status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 21st agent message, got %d", status)
	}
	if errResp.Detail == nil || !strings.Contains(*errResp.Detail, "this conversation is paused: too many agent messages; a human has to reply") {
		t.Fatalf("unexpected error detail: %+v", errResp.Detail)
	}

	// Verify that a system note was posted in the thread mentioning admins (@norbi)
	var threadMsgs apispec.Thread
	status = e.do(http.MethodGet, fmt.Sprintf("/api/v1/messages/%s/thread", threadRootID), server.DevAdminToken, nil, &threadMsgs)
	if status != http.StatusOK {
		t.Fatalf("expected 200 GET /api/v1/messages/{id}/thread, got %d", status)
	}

	systemNoteCount := 0
	var systemNote apispec.Message
	for _, r := range threadMsgs.Replies {
		if r.Kind == "system" {
			systemNoteCount++
			systemNote = r
		}
	}
	if systemNoteCount != 1 {
		t.Fatalf("expected exactly 1 system note in thread, got %d", systemNoteCount)
	}
	if !strings.Contains(systemNote.Body, "@norbi") {
		t.Fatalf("expected system note to mention admin @norbi, got: %s", systemNote.Body)
	}
	if !strings.Contains(systemNote.Body, "paused") {
		t.Fatalf("expected system note to mention paused, got: %s", systemNote.Body)
	}

	// Subsequent agent attempt should ALSO fail with 429, but NOT post another system note (once per pause)
	status = e.do(http.MethodPost, fmt.Sprintf("/api/v1/channels/%s/messages", general), token2, map[string]any{
		"thread_root_id": threadRootID,
		"body":           "Another agent message attempting during pause",
	}, &errResp)
	if status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on subsequent agent message during pause, got %d", status)
	}

	status = e.do(http.MethodGet, fmt.Sprintf("/api/v1/messages/%s/thread", threadRootID), server.DevAdminToken, nil, &threadMsgs)
	if status != http.StatusOK {
		t.Fatalf("expected 200 GET thread, got %d", status)
	}
	systemNoteCount = 0
	for _, r := range threadMsgs.Replies {
		if r.Kind == "system" {
			systemNoteCount++
		}
	}
	if systemNoteCount != 1 {
		t.Fatalf("expected still exactly 1 system note (once per pause), got %d", systemNoteCount)
	}

	// 4. Any human message in that thread lifts the pause
	_, status = e.send(server.DevAdminToken, general, map[string]any{
		"thread_root_id": threadRootID,
		"body":           "Human checking in: everything looks good, please continue!",
	})
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for human reply lifting pause, got %d", status)
	}

	// Agent can now post again!
	msgAfterLift, status := e.send(token1, general, map[string]any{
		"thread_root_id": threadRootID,
		"body":           "Thank you human, continuing work now",
	})
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for agent message after pause lifted, got %d", status)
	}
	if msgAfterLift.ID == "" {
		t.Fatalf("expected valid message ID after pause lifted")
	}

	// 5. Test Agent-Agent Rate Limit in DM:
	// Open a DM between bot-one and bot-two
	var dmChan apispec.Channel
	status = e.do(http.MethodPost, "/api/v1/dms", token1, map[string]any{
		"actor_ids": []string{agent2ID},
	}, &dmChan)
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("expected 200/201 on open DM, got %d", status)
	}

	for i := 1; i <= 20; i++ {
		tok := token1
		if i%2 == 0 {
			tok = token2
		}
		_, status = e.send(tok, dmChan.Id.String(), map[string]any{
			"body": fmt.Sprintf("DM message #%d", i),
		})
		if status != http.StatusCreated {
			t.Fatalf("expected 201 for DM message #%d, got %d", i, status)
		}
	}

	// 21st DM message should be refused with 429
	status = e.do(http.MethodPost, fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), token1, map[string]any{
		"body": "DM message #21 attempting to send",
	}, &errResp)
	if status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 21st DM message, got %d", status)
	}

	// Check system note was posted in DM
	var dmMsgs apispec.MessageList
	status = e.do(http.MethodGet, fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), server.DevAdminToken, nil, &dmMsgs)
	if status != http.StatusOK {
		t.Fatalf("expected 200 GET DM messages, got %d", status)
	}
	dmSysNoteCount := 0
	for _, m := range dmMsgs.Messages {
		if m.Kind == "system" {
			dmSysNoteCount++
		}
	}
	if dmSysNoteCount != 1 {
		t.Fatalf("expected exactly 1 system note in DM, got %d", dmSysNoteCount)
	}
}
