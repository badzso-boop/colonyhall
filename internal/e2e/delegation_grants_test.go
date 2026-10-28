package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

func (e *env) createUser(handle, name string) (string, string) {
	e.t.Helper()
	var created struct {
		Actor apispec.Actor `json:"actor"`
		Token string        `json:"token"`
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken, map[string]any{
		"kind":         "human",
		"handle":       handle,
		"display_name": name,
	}, &created); s != http.StatusCreated {
		e.t.Fatalf("create user: %d", s)
	}
	return created.Actor.Id.String(), created.Token
}

func TestDelegationGrantFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	hostID, _ := e.enroll("machine-grant-1")
	// Both agents are full-access agents (as in the Norbi 2026-10-27 incident)
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "projects", "Projects Agent")
	targetID, targetTok := createFullAccessAgent(e, hostID, "agy-homelab", "Homelab Agent")

	// Open a DM between the two agents
	var dmChan apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}

	// 1. First message from projects to agy-homelab in DM (top-level)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-homelab please run the migrations",
	}, nil)

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Target inbox is empty (not delivered yet)
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 0 {
		t.Fatalf("expected 0 items before approve, got %d", inBox.Total)
	}

	// Question is created
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 open question, got %d", len(qList.Questions))
	}
	q := qList.Questions[0]

	// 2. Admin approves
	var answered apispec.Question
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", q.Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, &answered); s != http.StatusOK {
		t.Fatalf("answer question: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Target gets 2 items: 1 from Norbi's answer message in the DM (which wakes DM participants),
	// and 1 delegated item for the approved task!
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	var item1 *apispec.InboxItem
	for i := range inBox.Items {
		if inBox.Items[i].Kind == "delegated" {
			item1 = &inBox.Items[i]
			break
		}
	}
	if item1 == nil {
		t.Fatalf("expected delegated item in inbox, got %+v", inBox.Items)
	}
	if item1.DelegatedBy == nil || *item1.DelegatedBy != "norbi" {
		t.Fatalf("expected delegated_by norbi, got %v", item1.DelegatedBy)
	}
	if item1.ViaGrant != nil && *item1.ViaGrant {
		t.Fatalf("expected via_grant false for direct approval")
	}

	// Ack items
	ackIDs := make([]string, len(inBox.Items))
	for i, it := range inBox.Items {
		ackIDs[i] = it.Id.String()
	}
	e.do("POST", "/api/v1/agent/inbox/ack", targetTok, map[string]any{"ids": ackIDs}, nil)

	// Check grant API
	var grantList apispec.DelegationGrantList
	if s := e.do("GET", "/api/v1/delegation-grants?active=true", server.DevAdminToken, nil, &grantList); s != http.StatusOK {
		t.Fatalf("get delegation grants: %d", s)
	}
	if len(grantList.Grants) != 1 {
		t.Fatalf("expected 1 active grant, got %d", len(grantList.Grants))
	}
	g := grantList.Grants[0]
	if g.ApproverHandle != "norbi" {
		t.Fatalf("expected approver norbi, got %s", g.ApproverHandle)
	}
	if (g.AgentAHandle != "projects" && g.AgentAHandle != "agy-homelab") ||
		(g.AgentBHandle != "projects" && g.AgentBHandle != "agy-homelab") {
		t.Fatalf("unexpected grant pair handles: %s <-> %s", g.AgentAHandle, g.AgentBHandle)
	}

	// 3. Follow-up message projects -> agy-homelab in the SAME conversation
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-homelab and then verify logs",
	}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// No new questions!
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 0 {
		t.Fatalf("expected 0 open questions with active grant, got %d", len(qList.Questions))
	}

	// Target agy-homelab receives task immediately with via_grant = true
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 1 {
		t.Fatalf("expected 1 item for second message, got %d", inBox.Total)
	}
	item2 := inBox.Items[0]
	if item2.Kind != "delegated" {
		t.Fatalf("expected delegated item, got %s", item2.Kind)
	}
	if item2.DelegatedBy == nil || *item2.DelegatedBy != "norbi" {
		t.Fatalf("expected delegated_by norbi, got %v", item2.DelegatedBy)
	}
	if item2.ViaGrant == nil || !*item2.ViaGrant {
		t.Fatalf("expected via_grant = true for second message")
	}

	// Ack item2
	e.do("POST", "/api/v1/agent/inbox/ack", targetTok, map[string]any{"ids": []string{item2.Id.String()}}, nil)

	// 4. Reverse direction reply: agy-homelab -> projects in the SAME conversation
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), targetTok, apispec.SendMessageRequest{
		Body: "@projects migrations complete, please check status",
	}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Still no new questions!
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 0 {
		t.Fatalf("expected 0 open questions for reverse reply, got %d", len(qList.Questions))
	}

	// Source projects gets the delegated item with via_grant = true
	var sourceBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", sourceTok, nil, &sourceBox)
	var item3 *apispec.InboxItem
	for i := range sourceBox.Items {
		if sourceBox.Items[i].Kind == "delegated" {
			item3 = &sourceBox.Items[i]
			break
		}
	}
	if item3 == nil {
		t.Fatalf("expected delegated item in source inbox for reverse reply, got %+v", sourceBox.Items)
	}
	if item3.DelegatedBy == nil || *item3.DelegatedBy != "norbi" {
		t.Fatalf("expected delegated_by norbi, got %v", item3.DelegatedBy)
	}
	if item3.ViaGrant == nil || !*item3.ViaGrant {
		t.Fatalf("expected via_grant = true for reverse reply")
	}
}

func TestDelegationGrantExpiry(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	hostID, _ := e.enroll("machine-grant-exp")
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "projects-exp", "Projects Agent")
	targetID, _ := createFullAccessAgent(e, hostID, "agy-exp", "Homelab Agent")

	var dmChan apispec.Channel
	e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan)

	// Message 1 -> approval -> grant
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-exp step 1",
	}, nil)
	_ = e.app.Drain(ctx)

	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, nil)
	_ = e.app.Drain(ctx)

	// Idle expiry test: advance clock by 61 minutes (default idle is 60m)
	e.clock.Advance(61 * time.Minute)

	// Next message should ask again!
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-exp step 2 after silence",
	}, nil)
	_ = e.app.Drain(ctx)

	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 question after 61m idle expiry, got %d", len(qList.Questions))
	}

	// Approve step 2 -> new grant created (or active grant renewed)
	e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, nil)
	_ = e.app.Drain(ctx)

	// Absolute max expiry test:
	// Grant created at current time T. Default max is 12 hours.
	// Message sent every 50 minutes keeps idle timer active.
	// But at 12 hours and 1 minute, the grant MUST expire regardless of recent activity!
	for i := 0; i < 11; i++ {
		e.clock.Advance(50 * time.Minute)
		e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
			Body: fmt.Sprintf("@agy-exp heartbeat %d", i),
		}, nil)
		_ = e.app.Drain(ctx)
		e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
		if len(qList.Questions) != 0 {
			t.Fatalf("expected no question at iteration %d, got %d", i, len(qList.Questions))
		}
	}

	// 11 * 50 = 550 minutes. Advance 50m (600m), message
	e.clock.Advance(50 * time.Minute)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-exp 600m",
	}, nil)
	_ = e.app.Drain(ctx)

	// Advance 50m (650m), message
	e.clock.Advance(50 * time.Minute)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-exp 650m",
	}, nil)
	_ = e.app.Drain(ctx)

	// Advance 50m (700m), message
	e.clock.Advance(50 * time.Minute)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-exp 700m",
	}, nil)
	_ = e.app.Drain(ctx)

	// Advance 25 minutes (total 725m > 720m = 12 hours!) with only 25 minutes of silence (well within 60m idle)!
	e.clock.Advance(25 * time.Minute)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-exp past 12 hours",
	}, nil)
	_ = e.app.Drain(ctx)

	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 question after exceeding 12 hours max lifetime, got %d", len(qList.Questions))
	}
}

func TestDelegationGrantScope(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	hostID, _ := e.enroll("machine-grant-scope")
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "projects-sc", "Projects Agent")
	targetID, _ := createFullAccessAgent(e, hostID, "agy-sc", "Homelab Agent")
	thirdID, _ := createFullAccessAgent(e, hostID, "third-sc", "Third Agent")

	// Message in general channel thread
	var sendRes1 struct {
		Message *apispec.Message `json:"message"`
	}
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@agy-sc thread root task",
	}, &sendRes1)
	_ = e.app.Drain(ctx)
	if sendRes1.Message == nil {
		t.Fatalf("expected message returned")
	}
	msg1ID := sendRes1.Message.Id

	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 question")
	}
	// Approve -> grant for thread msg1ID
	e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, nil)
	_ = e.app.Drain(ctx)

	// Follow-up inside the SAME thread msg1ID: covered!
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		ThreadRootId: &msg1ID,
		Body:         "@agy-sc in-thread follow-up",
	}, nil)
	_ = e.app.Drain(ctx)
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 0 {
		t.Fatalf("expected 0 questions in same thread, got %d", len(qList.Questions))
	}

	// Another top-level message in general: another conversation -> asks again!
	var msg2 apispec.Message
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@agy-sc another top-level task",
	}, &msg2)
	_ = e.app.Drain(ctx)
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected question for different thread, got %d", len(qList.Questions))
	}

	// Message mentioning a third full-access agent: asks again for third agent!
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		ThreadRootId: &msg1ID,
		Body:         "@third-sc task for third agent",
	}, nil)
	_ = e.app.Drain(ctx)
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	// Now 2 questions open (one from msg2, one from third-sc)
	if len(qList.Questions) != 2 {
		t.Fatalf("expected 2 questions (msg2 and third-sc), got %d", len(qList.Questions))
	}
	_ = thirdID
	_ = targetID
	_ = sourceID
}

func TestDelegationGrantPendingBatchApproval(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	hostID, _ := e.enroll("machine-grant-batch")
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "projects-batch", "Projects Agent")
	targetID, targetTok := createFullAccessAgent(e, hostID, "agy-batch", "Homelab Agent")

	var dmChan apispec.Channel
	e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan)

	// Send message 1: creates Q-1
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-batch message 1",
	}, nil)
	_ = e.app.Drain(ctx)

	// While Q-1 is open, send message 2 and message 3 in the same DM conversation
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-batch message 2 (queued)",
	}, nil)
	_ = e.app.Drain(ctx)

	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-batch message 3 (queued)",
	}, nil)
	_ = e.app.Drain(ctx)

	// Also send message in a different channel (general)
	general := e.generalID()
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@agy-batch message in general",
	}, nil)
	_ = e.app.Drain(ctx)

	// There should be 4 open questions
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 4 {
		t.Fatalf("expected 4 open questions, got %d", len(qList.Questions))
	}

	// Find the question for message 1
	var q1ID string
	for _, q := range qList.Questions {
		if q.ChannelId.String() == dmChan.Id.String() {
			if q1ID == "" || q.Number < 100 {
				q1ID = q.Id.String()
			}
		}
	}

	// Count messages in DM before approval
	var msgsBefore struct {
		Messages []any `json:"messages"`
	}
	e.do("GET", fmt.Sprintf("/api/v1/channels/%s/messages?limit=100", dmChan.Id), server.DevAdminToken, nil, &msgsBefore)
	countBefore := len(msgsBefore.Messages)

	// Admin approves Q-1
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", q1ID), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, nil); s != http.StatusOK {
		t.Fatalf("approve Q-1: %d", s)
	}
	_ = e.app.Drain(ctx)

	// CRITICAL CHECK: In round 26 Part 6, delegation questions do NOT post Q-NN or Q-NN answer chat messages!
	// So NO new chat messages were posted in the DM for Q-1, Q-2, or Q-3.
	var msgsAfter struct {
		Messages []any `json:"messages"`
	}
	e.do("GET", fmt.Sprintf("/api/v1/channels/%s/messages?limit=100", dmChan.Id), server.DevAdminToken, nil, &msgsAfter)
	countAfter := len(msgsAfter.Messages)
	if countAfter != countBefore {
		t.Fatalf("expected 0 new messages in DM (delegation questions have no chat messages), but count changed by %d (before: %d, after: %d)",
			countAfter-countBefore, countBefore, countAfter)
	}

	// Target gets 3 inbox items (3 approved tasks: 1 direct + 2 via grant, 0 chat messages)
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 3 {
		t.Fatalf("expected target to receive 3 items (3 delegated tasks, 0 dm chat messages), got %d", inBox.Total)
	}
	delegatedCount := 0
	viaGrantCount := 0
	for _, it := range inBox.Items {
		if it.Kind == "delegated" {
			delegatedCount++
			if it.ViaGrant != nil && *it.ViaGrant {
				viaGrantCount++
			}
		}
	}
	if delegatedCount != 3 {
		t.Fatalf("expected 3 delegated items, got %d", delegatedCount)
	}
	if viaGrantCount != 2 {
		t.Fatalf("expected 2 items via_grant, got %d", viaGrantCount)
	}

	// Question for the different conversation (general) remains OPEN
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 question remaining open (the one in general), got %d", len(qList.Questions))
	}
	if qList.Questions[0].ChannelId.String() != general {
		t.Fatalf("expected remaining open question to be in general, got %s", qList.Questions[0].ChannelId)
	}
}

func TestDelegationGrantRevokeAndPermissions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	hostID, _ := e.enroll("machine-grant-perm")
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "projects-perm", "Projects Agent")
	targetID, targetTok := createFullAccessAgent(e, hostID, "agy-perm", "Homelab Agent")
	_, userTok := e.createUser("alice", "Alice Regular")

	var dmChan apispec.Channel
	e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan)

	// Create grant via message + approval
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-perm hello",
	}, nil)
	_ = e.app.Drain(ctx)

	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, nil)
	_ = e.app.Drain(ctx)

	var grantList apispec.DelegationGrantList
	e.do("GET", "/api/v1/delegation-grants?active=true", server.DevAdminToken, nil, &grantList)
	if len(grantList.Grants) != 1 {
		t.Fatalf("expected 1 active grant")
	}
	gID := grantList.Grants[0].Id

	// 1. Agent cannot list grants -> 403
	if s := e.do("GET", "/api/v1/delegation-grants", targetTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent list grants expected 403, got %d", s)
	}

	// 2. Regular human cannot revoke grant -> 403
	if s := e.do("POST", fmt.Sprintf("/api/v1/delegation-grants/%s/revoke", gID), userTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("regular human revoke expected 403, got %d", s)
	}

	// 3. Agent cannot revoke grant -> 403
	if s := e.do("POST", fmt.Sprintf("/api/v1/delegation-grants/%s/revoke", gID), targetTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent revoke expected 403, got %d", s)
	}

	// 4. Admin revokes grant -> 200
	var revoked apispec.DelegationGrant
	if s := e.do("POST", fmt.Sprintf("/api/v1/delegation-grants/%s/revoke", gID), server.DevAdminToken, nil, &revoked); s != http.StatusOK {
		t.Fatalf("admin revoke expected 200, got %d", s)
	}
	if revoked.RevokedByHandle == nil || *revoked.RevokedByHandle != "norbi" {
		t.Fatalf("expected revoked_by_handle norbi, got %v", revoked.RevokedByHandle)
	}

	// 5. Revoking again -> 409 Conflict
	if s := e.do("POST", fmt.Sprintf("/api/v1/delegation-grants/%s/revoke", gID), server.DevAdminToken, nil, nil); s != http.StatusConflict {
		t.Fatalf("revoke inactive expected 409, got %d", s)
	}

	// 6. After revoke, next message asks again!
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-perm after revoke",
	}, nil)
	_ = e.app.Drain(ctx)

	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected question after grant revoked, got %d", len(qList.Questions))
	}
}

func TestDelegationSettings(t *testing.T) {
	e := newEnv(t)
	_, userTok := e.createUser("bob", "Bob Regular")
	_, agentTok := e.createAgent("bot-settings", "Bot")

	// 1. Defaults
	var st apispec.DelegationSettings
	if s := e.do("GET", "/api/v1/delegation-settings", server.DevAdminToken, nil, &st); s != http.StatusOK {
		t.Fatalf("get settings expected 200, got %d", s)
	}
	if st.GrantIdleMinutes != 60 || st.GrantMaxHours != 12 {
		t.Fatalf("expected defaults 60m / 12h, got %dm / %dh", st.GrantIdleMinutes, st.GrantMaxHours)
	}

	// 2. Regular human can GET
	if s := e.do("GET", "/api/v1/delegation-settings", userTok, nil, &st); s != http.StatusOK {
		t.Fatalf("user get settings expected 200, got %d", s)
	}

	// 3. Agent cannot GET settings -> 403
	if s := e.do("GET", "/api/v1/delegation-settings", agentTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent get settings expected 403, got %d", s)
	}

	// 4. Regular human cannot PUT settings -> 403
	if s := e.do("PUT", "/api/v1/delegation-settings", userTok, apispec.UpdateDelegationSettingsRequest{
		GrantIdleMinutes: 30,
		GrantMaxHours:    6,
	}, nil); s != http.StatusForbidden {
		t.Fatalf("user put settings expected 403, got %d", s)
	}

	// 5. Agent cannot PUT settings -> 403
	if s := e.do("PUT", "/api/v1/delegation-settings", agentTok, apispec.UpdateDelegationSettingsRequest{
		GrantIdleMinutes: 30,
		GrantMaxHours:    6,
	}, nil); s != http.StatusForbidden {
		t.Fatalf("agent put settings expected 403, got %d", s)
	}

	// 6. Validation bounds: idle < 5 -> 400
	if s := e.do("PUT", "/api/v1/delegation-settings", server.DevAdminToken, apispec.UpdateDelegationSettingsRequest{
		GrantIdleMinutes: 4,
		GrantMaxHours:    12,
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("idle < 5 expected 400, got %d", s)
	}

	// Validation bounds: idle > 1440 -> 400
	if s := e.do("PUT", "/api/v1/delegation-settings", server.DevAdminToken, apispec.UpdateDelegationSettingsRequest{
		GrantIdleMinutes: 1441,
		GrantMaxHours:    12,
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("idle > 1440 expected 400, got %d", s)
	}

	// Validation bounds: max < 1 -> 400
	if s := e.do("PUT", "/api/v1/delegation-settings", server.DevAdminToken, apispec.UpdateDelegationSettingsRequest{
		GrantIdleMinutes: 60,
		GrantMaxHours:    0,
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("max < 1 expected 400, got %d", s)
	}

	// Validation bounds: max > 72 -> 400
	if s := e.do("PUT", "/api/v1/delegation-settings", server.DevAdminToken, apispec.UpdateDelegationSettingsRequest{
		GrantIdleMinutes: 60,
		GrantMaxHours:    73,
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("max > 72 expected 400, got %d", s)
	}

	// 7. Admin updates valid settings -> 200
	var updatedSt apispec.DelegationSettings
	if s := e.do("PUT", "/api/v1/delegation-settings", server.DevAdminToken, apispec.UpdateDelegationSettingsRequest{
		GrantIdleMinutes: 45,
		GrantMaxHours:    24,
	}, &updatedSt); s != http.StatusOK {
		t.Fatalf("admin put settings expected 200, got %d", s)
	}
	if updatedSt.GrantIdleMinutes != 45 || updatedSt.GrantMaxHours != 24 {
		t.Fatalf("expected 45m / 24h, got %dm / %dh", updatedSt.GrantIdleMinutes, updatedSt.GrantMaxHours)
	}
}

func TestDelegationGrantDenyAndExpire(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	hostID, _ := e.enroll("machine-grant-deny")
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "projects-deny", "Projects Agent")
	targetID, _ := createFullAccessAgent(e, hostID, "agy-deny", "Homelab Agent")

	var dmChan apispec.Channel
	e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan)

	// Case 1: Deny -> no grant created
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-deny task to deny",
	}, nil)
	_ = e.app.Drain(ctx)

	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 question")
	}

	e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Elutasítom",
	}, nil)
	_ = e.app.Drain(ctx)

	var grantList apispec.DelegationGrantList
	e.do("GET", "/api/v1/delegation-grants", server.DevAdminToken, nil, &grantList)
	if len(grantList.Grants) != 0 {
		t.Fatalf("expected 0 grants after deny, got %d", len(grantList.Grants))
	}

	// Case 2: Expire -> no grant created
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-deny task to expire",
	}, nil)
	_ = e.app.Drain(ctx)

	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 question")
	}

	// Advance clock past 24h deadline so question expires
	e.clock.Advance(25 * time.Hour)
	_ = e.app.Drain(ctx)

	e.do("GET", "/api/v1/delegation-grants", server.DevAdminToken, nil, &grantList)
	if len(grantList.Grants) != 0 {
		t.Fatalf("expected 0 grants after expire, got %d", len(grantList.Grants))
	}
}

func TestDelegationSandboxedUnchanged(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	hostID, _ := e.enroll("machine-grant-sb")
	_, sourceTok := createFullAccessAgent(e, hostID, "projects-sb", "Projects Agent")
	targetID, targetTok := createSandboxedAgent(e, hostID, "sandboxed-bot", "Sandboxed Agent")

	// Message to sandboxed agent
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@sandboxed-bot run simple calculation",
	}, nil)
	_ = e.app.Drain(ctx)

	// No question created!
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 0 {
		t.Fatalf("expected 0 questions for sandboxed target, got %d", len(qList.Questions))
	}

	// No grant created
	var grantList apispec.DelegationGrantList
	e.do("GET", "/api/v1/delegation-grants", server.DevAdminToken, nil, &grantList)
	if len(grantList.Grants) != 0 {
		t.Fatalf("expected 0 grants, got %d", len(grantList.Grants))
	}

	// Sandboxed agent gets normal message item in inbox
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 1 {
		t.Fatalf("expected sandboxed agent to receive message, got %d", inBox.Total)
	}
	if inBox.Items[0].Kind != "message" {
		t.Fatalf("expected kind message for sandboxed, got %s", inBox.Items[0].Kind)
	}
	_ = targetID
}

// TestDelegationApprovedWithoutGrantNotLabelled: delegations approved before
// grants existed (round 24) have no grant; the API must not report them as
// approved by a grant.
func TestDelegationApprovedWithoutGrantNotLabelled(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	hostID, _ := e.enroll("machine-grant-old")
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "projects", "Projects Agent")
	targetID, _ := createFullAccessAgent(e, hostID, "agy-homelab", "Homelab Agent")

	var dmChan apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-homelab please deploy",
	}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	var list apispec.DelegationList
	e.do("GET", "/api/v1/delegations?state=pending", server.DevAdminToken, nil, &list)
	if len(list.Delegations) != 1 {
		t.Fatalf("expected 1 pending delegation, got %d", len(list.Delegations))
	}
	// Approve it the way round 24 did: a state change, no grant.
	delID := ids.ID(list.Delegations[0].Id)
	decider, err := ids.Parse(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := e.app.Delegation.Store.UpdateStateConditional(ctx, delID, "pending", "approved", &decider, e.clock.Now()); err != nil || n != 1 {
		t.Fatalf("approve: n=%d err=%v", n, err)
	}

	e.do("GET", "/api/v1/delegations?state=approved", server.DevAdminToken, nil, &list)
	if len(list.Delegations) != 1 {
		t.Fatalf("expected 1 approved delegation, got %d", len(list.Delegations))
	}
	if g := list.Delegations[0].ApprovedByGrant; g != nil && *g {
		t.Fatalf("a delegation approved without a grant is labelled approved_by_grant")
	}
}

// TestDelegationGrantDMThreadReply reproduces the 2026-10-27 production incident (Q-23 -> Q-25):
// top-level request in a DM, approval -> active grant for the DM.
// The target agent replies in the request message's thread -> no new question, delivered via grant.
func TestDelegationGrantDMThreadReply(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	hostID, _ := e.enroll("machine-grant-dm-thread")
	sourceID, sourceTok := createFullAccessAgent(e, hostID, "visegrad-agy", "Visegrad Agent")
	targetID, targetTok := createFullAccessAgent(e, hostID, "agy-homelab", "Homelab Agent")

	var dmChan apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}

	// 1. Top-level request message in DM
	var sendRes apispec.SendResult
	if s := e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@agy-homelab mi maradt meg a teendokbol?",
	}, &sendRes); s != http.StatusCreated {
		t.Fatalf("send request message: %d", s)
	}
	if sendRes.Message == nil {
		t.Fatal("expected created message in response")
	}
	topReqMsgID := sendRes.Message.Id

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Question Q-23 is created
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 open question, got %d", len(qList.Questions))
	}
	q := qList.Questions[0]

	// 2. Admin approves -> grant created for (DM, nil)
	var answered apispec.Question
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", q.Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, &answered); s != http.StatusOK {
		t.Fatalf("answer question: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Target receives the task
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	var taskItem *apispec.InboxItem
	for i := range inBox.Items {
		if inBox.Items[i].Kind == "delegated" {
			taskItem = &inBox.Items[i]
			break
		}
	}
	if taskItem == nil {
		t.Fatalf("expected delegated item for target, got %+v", inBox.Items)
	}

	// Ack target's inbox
	ackIDs := make([]string, len(inBox.Items))
	for i, it := range inBox.Items {
		ackIDs[i] = it.Id.String()
	}
	e.do("POST", "/api/v1/agent/inbox/ack", targetTok, map[string]any{"ids": ackIDs}, nil)

	// Verify grant is active
	var grantList apispec.DelegationGrantList
	e.do("GET", "/api/v1/delegation-grants?active=true", server.DevAdminToken, nil, &grantList)
	if len(grantList.Grants) != 1 {
		t.Fatalf("expected 1 active grant, got %d", len(grantList.Grants))
	}

	// 3. Target replies in the request's thread (ThreadRootId = topReqMsgID)
	topReqUUID := topReqMsgID
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), targetTok, apispec.SendMessageRequest{
		Body:         "@visegrad-agy itt a jelentes a teendokrol",
		ThreadRootId: &topReqUUID,
	}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Assert: NO new question is created (would have been Q-25 in the incident)
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 0 {
		t.Fatalf("expected 0 open questions with active DM grant, got %d", len(qList.Questions))
	}

	// Source agent gets a delegated item with via_grant = true
	var sourceBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", sourceTok, nil, &sourceBox)
	var threadReplyItem *apispec.InboxItem
	for i := range sourceBox.Items {
		if sourceBox.Items[i].Kind == "delegated" {
			threadReplyItem = &sourceBox.Items[i]
			break
		}
	}
	if threadReplyItem == nil {
		t.Fatalf("expected delegated item in source inbox for thread reply, got %+v", sourceBox.Items)
	}
	if threadReplyItem.ViaGrant == nil || !*threadReplyItem.ViaGrant {
		t.Fatalf("expected via_grant = true for thread reply in DM")
	}
}

// TestDelegationGrantDMDenyAndSilentOption verifies Part 6:
// 1. Agent cannot set silent: true via public API (403 Forbidden).
// 2. DM delegation creates question without posting any chat messages.
// 3. Denying the question wakes source with 1 answer item, and wakes target with 0 items.
// 4. Terminal start posts silently and wakes nobody.
func TestDelegationGrantDMDenyAndSilentOption(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	// Enroll host and agents
	hID, _ := e.enroll("machine-deny")
	sourceID, sourceTok := createFullAccessAgent(e, hID, "deny-source", "Deny Source")
	targetID, targetTok := createFullAccessAgent(e, hID, "deny-target", "Deny Target")

	// Open DM
	var dmChan apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{
		"actor_ids": []string{sourceID, targetID},
	}, &dmChan); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}
	_ = e.app.Drain(ctx)

	silentTrue := true
	status := e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, map[string]any{
		"body":   "dodging wakes",
		"silent": silentTrue,
	}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when agent attempts silent send, got %d", status)
	}

	// 2. Agent delegates to full-access agent in DM
	var msgsBefore struct {
		Messages []any `json:"messages"`
	}
	e.do("GET", fmt.Sprintf("/api/v1/channels/%s/messages?limit=100", dmChan.Id), server.DevAdminToken, nil, &msgsBefore)
	countBefore := len(msgsBefore.Messages)

	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dmChan.Id), sourceTok, apispec.SendMessageRequest{
		Body: "@deny-target do this dangerous task",
	}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// In DM: exactly 1 message was added (the agent's request message itself)
	// NO "Q-NN (to humans)" message was posted!
	var msgsMid struct {
		Messages []any `json:"messages"`
	}
	e.do("GET", fmt.Sprintf("/api/v1/channels/%s/messages?limit=100", dmChan.Id), server.DevAdminToken, nil, &msgsMid)
	if len(msgsMid.Messages) != countBefore+1 {
		t.Fatalf("expected exactly 1 message added (the original request), got %d (before: %d, now: %d)",
			len(msgsMid.Messages)-countBefore, countBefore, len(msgsMid.Messages))
	}

	// Question is open in the "Válaszra vár" list
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	var qID string
	for _, q := range qList.Questions {
		if q.ChannelId.String() == dmChan.Id.String() {
			qID = q.Id.String()
			break
		}
	}
	if qID == "" {
		t.Fatalf("expected open question for delegation in DM, found none in %+v", qList.Questions)
	}

	// Target inbox has 0 items so far (waiting for approval)
	var targetBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &targetBox)
	if targetBox.Total != 0 {
		t.Fatalf("expected 0 items in target inbox before decision, got %d", targetBox.Total)
	}

	// 3. Admin denies the question
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qID), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Elutasítom",
	}, nil); s != http.StatusOK {
		t.Fatalf("deny question: %d", s)
	}
	_ = e.app.Drain(ctx)

	// In DM: NO "Q-NN answer: ..." message was posted. The short deny note ("@norbi elutasította")
	// is posted with Silent: true, waking nobody directly.
	var msgsAfter struct {
		Messages []apispec.Message `json:"messages"`
	}
	e.do("GET", fmt.Sprintf("/api/v1/channels/%s/messages?limit=100", dmChan.Id), server.DevAdminToken, nil, &msgsAfter)
	if len(msgsAfter.Messages) != len(msgsMid.Messages)+1 {
		t.Fatalf("expected 1 deny note posted in DM, got %d messages (was %d)", len(msgsAfter.Messages), len(msgsMid.Messages))
	}
	lastMsg := msgsAfter.Messages[len(msgsAfter.Messages)-1]
	if !strings.Contains(lastMsg.Body, "elutasította") {
		t.Fatalf("expected deny note '@... elutasította', got %q", lastMsg.Body)
	}

	// Target still has 0 items (never woken)
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &targetBox)
	if targetBox.Total != 0 {
		t.Fatalf("expected target to have 0 items on deny, got %d", targetBox.Total)
	}

	// Source receives exactly 1 answer item
	var sourceBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", sourceTok, nil, &sourceBox)
	if sourceBox.Total != 1 {
		t.Fatalf("expected source to have 1 item on deny, got %d", sourceBox.Total)
	}
	if sourceBox.Items[0].Kind != "answer" {
		t.Fatalf("expected answer item for source, got %s", sourceBox.Items[0].Kind)
	}

	// 4. Terminal start endpoint posts silently and wakes nobody
	e.do("POST", "/api/v1/agent/inbox/ack", sourceTok, map[string]any{"ids": []string{sourceBox.Items[0].Id.String()}}, nil)
	e.do("POST", "/api/v1/agent/terminal-start", sourceTok, map[string]any{
		"channel_id": dmChan.Id.String(),
		"name":       "sim",
		"command":    "ros2 launch turtlebot3_gazebo simulation.launch.py",
	}, nil)
	_ = e.app.Drain(ctx)

	// Neither source nor target gets woken
	e.do("GET", "/api/v1/agent/inbox", sourceTok, nil, &sourceBox)
	if sourceBox.Total != 0 {
		t.Fatalf("expected source to have 0 items after terminal start, got %d", sourceBox.Total)
	}
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &targetBox)
	if targetBox.Total != 0 {
		t.Fatalf("expected target to have 0 items after terminal start, got %d", targetBox.Total)
	}
}
