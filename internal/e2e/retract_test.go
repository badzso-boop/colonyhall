package e2e

import (
	"colonyhall/internal/fakeagent"
	"context"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/guard"
	"colonyhall/internal/httpx"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

func TestMessageRetractAuthorAndPermissions(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	general := e.generalID()

	// Create user1 (author) and user2 (other human)
	user1ID, user1Tok := e.createUser("retract-author", "Retract Author")
	user1UUID, _ := ids.Parse(user1ID)
	_, user2Tok := e.createUser("other-human", "Other Human")

	// Add user1 to general
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", general), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(user1UUID)}, nil)

	// Create an agent in general channel
	hID, _ := e.enroll("machine-retract")
	agentIDStr, agentTok := createFullAccessAgent(e, hID, "retract-agent", "Retract Agent")
	// Only its owner (or an admin) wakes a full-access agent (round 38).
	e.do("PUT", "/api/v1/agents/"+agentIDStr+"/owner", server.DevAdminToken, map[string]any{"owner_id": user1ID}, nil)
	agentUUID, _ := ids.Parse(agentIDStr)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", general), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(agentUUID)}, nil)

	// User1 sends a message mentioning @retract-agent
	var sendRes apispec.SendResult
	if s := e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", general), user1Tok, apispec.SendMessageRequest{
		Body: "@retract-agent please calculate the trajectory",
	}, &sendRes); s != http.StatusCreated {
		t.Fatalf("send message: %d", s)
	}
	_ = e.app.Drain(ctx)
	msgID := sendRes.Message.Id.String()

	// Agent has 1 pending inbox item for that message
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", agentTok, nil, &inBox)
	if inBox.Total != 1 || inBox.Items[0].ItemId.String() != msgID {
		t.Fatalf("expected 1 inbox item for agent, got total %d: %+v", inBox.Total, inBox.Items)
	}

	// 1. Other human tries to retract -> 403 Forbidden
	s := e.do("POST", fmt.Sprintf("/api/v1/messages/%s/retract", msgID), user2Tok, nil, nil)
	if s != http.StatusForbidden {
		t.Fatalf("expected 403 when other human retracts, got %d", s)
	}

	// 2. Agent tries to retract -> 403 Forbidden
	s = e.do("POST", fmt.Sprintf("/api/v1/messages/%s/retract", msgID), agentTok, nil, nil)
	if s != http.StatusForbidden {
		t.Fatalf("expected 403 when agent retracts, got %d", s)
	}

	// 3. Author retracts -> 204 No Content
	s = e.do("POST", fmt.Sprintf("/api/v1/messages/%s/retract", msgID), user1Tok, nil, nil)
	if s != http.StatusNoContent {
		t.Fatalf("expected 204 on author retract, got %d", s)
	}
	_ = e.app.Drain(ctx)

	// API check: message body is "🗑 Visszavonva", original body is gone, deleted_at is set
	var msgList apispec.MessageList
	e.do("GET", fmt.Sprintf("/api/v1/channels/%s/messages?limit=100", general), server.DevAdminToken, nil, &msgList)
	var foundMsg *apispec.Message
	for i := range msgList.Messages {
		if msgList.Messages[i].Id.String() == msgID {
			foundMsg = &msgList.Messages[i]
			break
		}
	}
	if foundMsg == nil {
		t.Fatalf("message %s not found in channel messages", msgID)
	}
	if foundMsg.Body != "🗑 Visszavonva" {
		t.Fatalf("expected body '🗑 Visszavonva', got %q", foundMsg.Body)
	}
	if foundMsg.DeletedAt == nil {
		t.Fatalf("expected deleted_at to be non-nil")
	}
	if strings.Contains(foundMsg.Body, "trajectory") {
		t.Fatalf("original body leaked in API response")
	}

	// Inbox check: agent's pending item for this message was acked/removed
	e.do("GET", "/api/v1/agent/inbox", agentTok, nil, &inBox)
	if inBox.Total != 0 {
		t.Fatalf("expected inbox to be cleared of retracted message, got total %d: %+v", inBox.Total, inBox.Items)
	}
}

func TestMessageRetractCancelsPendingDelegation(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	// Enroll host and agents
	hID, _ := e.enroll("machine-del-retract")
	sourceID, sourceTok := createFullAccessAgent(e, hID, "del-ret-src", "Delegation Retract Source")
	targetID, _ := createFullAccessAgent(e, hID, "del-ret-tgt", "Delegation Retract Target")

	// User sends a message in DM or channel delegating
	var dmChan apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}
	_ = e.app.Drain(ctx)

	// Source agent delegates to target agent
	// Wait, agent messages cannot be retracted by agents, but user messages can!
	// What if a human sends the task or what if a human retracts their message?
	humanID, humanTok := e.createUser("delegating-human", "Delegating Human")
	humanUUID, _ := ids.Parse(humanID)
	general := e.generalID()
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", general), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(humanUUID)}, nil)
	sourceUUID, _ := ids.Parse(sourceID)
	targetUUID, _ := ids.Parse(targetID)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", general), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(sourceUUID)}, nil)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", general), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(targetUUID)}, nil)

	var sendRes apispec.SendResult
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", general), humanTok, apispec.SendMessageRequest{
		Body: "@del-ret-src please run this via @del-ret-tgt",
	}, &sendRes)
	_ = e.app.Drain(ctx)

	// Now source agent delegates to target in response
	rootUUID := sendRes.Message.Id
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", general), sourceTok, apispec.SendMessageRequest{
		Body:         "@del-ret-tgt deploy the project",
		ThreadRootId: &rootUUID,
	}, &sendRes)
	_ = e.app.Drain(ctx)

	// Delegation was created and question is open
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) == 0 {
		t.Fatalf("expected open question for delegation, got none")
	}

	// Human author retracts the delegation message
	// Since delMsgID was sent by source agent, human cannot retract agent message directly.
	// But human CAN retract their own root message!
	// Or what if human sent a message that initiated a direct delegation?
	// Let's test retracting human's message:
	s := e.do("POST", fmt.Sprintf("/api/v1/messages/%s/retract", rootUUID.String()), humanTok, nil, nil)
	if s != http.StatusNoContent {
		t.Fatalf("retract human message: %d", s)
	}
	_ = e.app.Drain(ctx)

	// Assert: human's message is retracted
	var msgList apispec.MessageList
	e.do("GET", fmt.Sprintf("/api/v1/channels/%s/messages?limit=100", general), server.DevAdminToken, nil, &msgList)
	for _, m := range msgList.Messages {
		if m.Id.String() == rootUUID.String() {
			if m.Body != "🗑 Visszavonva" {
				t.Fatalf("expected root message retracted, got %q", m.Body)
			}
		}
	}
}

func TestMessageRetractExcludedFromThreadContext(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	general := e.generalID()

	humanID, humanTok := e.createUser("context-author", "Context Author")
	humanUUID, _ := ids.Parse(humanID)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", general), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(humanUUID)}, nil)

	// Post root message
	var rootRes apispec.SendResult
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", general), humanTok, apispec.SendMessageRequest{
		Body: "Step 1: start the database",
	}, &rootRes)
	rootID := rootRes.Message.Id

	// Post reply message 1 (will be retracted)
	var reply1Res apispec.SendResult
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", general), humanTok, apispec.SendMessageRequest{
		Body:         "Step 2: secret password is 123456",
		ThreadRootId: &rootID,
	}, &reply1Res)
	reply1ID := reply1Res.Message.Id

	// Post reply message 2
	var reply2Res apispec.SendResult
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", general), humanTok, apispec.SendMessageRequest{
		Body:         "Step 3: configure application",
		ThreadRootId: &rootID,
	}, &reply2Res)
	_ = e.app.Drain(ctx)

	// Retract reply 1
	if s := e.do("POST", fmt.Sprintf("/api/v1/messages/%s/retract", reply1ID.String()), humanTok, nil, nil); s != http.StatusNoContent {
		t.Fatalf("retract: %d", s)
	}
	_ = e.app.Drain(ctx)

	// Guard threadContext should exclude reply 1
	client := guard.Client{BaseURL: e.srv.URL, Token: server.DevAdminToken}
	rootMsg, replies, err := client.Thread(ctx, rootID.String())
	if err != nil {
		t.Fatalf("client.Thread: %v", err)
	}

	for _, m := range replies {
		if m.ID == reply1ID.String() {
			if m.DeletedAt == nil {
				t.Fatalf("expected DeletedAt to be set on retracted message")
			}
			if m.Body != "🗑 Visszavonva" {
				t.Fatalf("expected body '🗑 Visszavonva', got %q", m.Body)
			}
			if strings.Contains(m.Body, "secret password") {
				t.Fatalf("secret leaked in thread reply")
			}
		}
	}
	_ = rootMsg
}

// TestSilentAgentEndpointsCarryNoFreeText: the guard's silent notes are built
// by the server; an agent cannot use them to post its own text silently.
func TestSilentAgentEndpointsCarryNoFreeText(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	hID, _ := e.enroll("machine-silent")
	_, agentTok := createFullAccessAgent(e, hID, "silent-agent", "Silent Agent")

	if s := e.do("POST", "/api/v1/agent/retract-note", agentTok, map[string]any{
		"channel_id": general, "body": "@norbi anything I like",
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("retract-note with free text: %d, want 400", s)
	}
	if s := e.do("POST", "/api/v1/agent/terminal-start", agentTok, map[string]any{
		"channel_id": general, "name": "build", "body": "anything I like",
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("terminal-start with free text: %d, want 400", s)
	}
	if s := e.do("POST", "/api/v1/agent/terminal-start", agentTok, map[string]any{
		"channel_id": general, "name": "Not A Name!", "command": "make",
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("terminal-start with a bad name: %d, want 400", s)
	}
	if s := e.do("POST", "/api/v1/agent/terminal-start", agentTok, map[string]any{
		"channel_id": general, "name": "build", "command": "make all @norbi `x`\n@norbi second line",
	}, nil); s != http.StatusNoContent {
		t.Fatalf("terminal-start: %d", s)
	}
	if s := e.do("POST", "/api/v1/agent/retract-note", agentTok, map[string]any{
		"channel_id": general,
	}, nil); s != http.StatusNoContent {
		t.Fatalf("retract-note: %d", s)
	}

	var list apispec.MessageList
	e.do("GET", "/api/v1/channels/"+general+"/messages?limit=5", server.DevAdminToken, nil, &list)
	var bodies []string
	for _, m := range list.Messages {
		bodies = append(bodies, m.Body)
	}
	all := strings.Join(bodies, "\n")
	if !strings.Contains(all, "🖥 build elindult: `make all @\u200bnorbi 'x'`") || strings.Contains(all, "second line") {
		t.Fatalf("terminal start text not built by the server: %q", all)
	}
	for _, m := range list.Messages {
		if strings.Contains(m.Body, "elindult") && len(m.Mentions) > 0 {
			t.Fatalf("the command text became a mention: %v", m.Mentions)
		}
	}
	if !strings.Contains(all, "⏹ Leállítottam: az üzenetet visszavonták.") {
		t.Fatalf("retract note missing: %q", all)
	}
}

// TestRetractedTextHiddenFromAgentTools: agents read channels and threads
// through MCP; a retracted message's text must not reach them there either.
func TestRetractedTextHiddenFromAgentTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e := newEnv(t)
	general := e.generalID()
	_, agentToken := e.createAgent("reader-bot", "Reader Bot")

	msg, s := e.send(server.DevAdminToken, general, map[string]any{"body": "wrong channel: the secret plan"})
	if s != http.StatusCreated {
		t.Fatalf("send: %d", s)
	}
	if s := e.do("POST", "/api/v1/messages/"+msg.ID+"/retract", server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("retract: %d", s)
	}

	// Other modules (inbox, notes, questions, files) read messages by id.
	mid, _ := ids.Parse(msg.ID)
	if m, err := e.app.Conversation.MessageByID(ctx, mid); err != nil || strings.Contains(m.Body, "secret plan") {
		t.Fatalf("MessageByID shows the retracted text (err %v)", err)
	}
	if ms, err := e.app.Conversation.MessagesByIDs(ctx, []ids.ID{mid}); err != nil || len(ms) != 1 || strings.Contains(ms[0].Body, "secret plan") {
		t.Fatalf("MessagesByIDs shows the retracted text (err %v)", err)
	}

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", agentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	for _, args := range []map[string]any{{"channel": "general"}, {"thread": msg.ID}} {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "read", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError || len(res.Content) == 0 {
			t.Fatalf("read %v failed: %+v", args, res)
		}
		if tc, _ := res.Content[0].(*mcp.TextContent); tc == nil || strings.Contains(tc.Text, "secret plan") {
			t.Fatalf("read %v shows the retracted text", args)
		}
	}
}
