// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func createFullAccessAgent(e *env, hostID, handle, name string) (string, string) {
	e.t.Helper()
	return e.newAgent(handle, name, map[string]any{
		"host_id": hostID, "client": "claude-code", "model": "haiku", "allow_work": true,
		"mode": "direct", "root_dir": "/app", "sandbox": false,
	}, nil)
}

func createSandboxedAgent(e *env, hostID, handle, name string) (string, string) {
	e.t.Helper()
	return e.newAgent(handle, name, map[string]any{
		"host_id": hostID, "client": "claude-code", "model": "haiku", "allow_work": true,
		"mode": "compact", "sandbox": true,
	}, nil)
}

func TestDelegationFlowApprove(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	hostID, _ := e.enroll("machine-1")
	sourceID, sourceTok := createSandboxedAgent(e, hostID, "projects", "Projects Agent")
	targetID, targetTok := createFullAccessAgent(e, hostID, "agy-homelab", "Homelab Agent")

	// Source agent sends task message mentioning full-access target agent
	msgText := "@agy-homelab deploy the repository to production"
	var msg apispec.Message
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: msgText,
	}, &msg); s != http.StatusCreated {
		t.Fatalf("send message: %d", s)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// 1. Target's inbox must be empty! (Not woken directly)
	var inBox apispec.Inbox
	if s := e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox); s != http.StatusOK {
		t.Fatalf("get target inbox: %d", s)
	}
	if inBox.Total != 0 {
		t.Fatalf("expected target inbox to be empty before approval, got %d items", inBox.Total)
	}

	// 2. Pending delegation exists
	var delList apispec.DelegationList
	if s := e.do("GET", "/api/v1/delegations?state=pending", server.DevAdminToken, nil, &delList); s != http.StatusOK {
		t.Fatalf("list pending delegations: %d", s)
	}
	if len(delList.Delegations) != 1 {
		t.Fatalf("expected 1 pending delegation, got %d", len(delList.Delegations))
	}
	del := delList.Delegations[0]
	if del.State != apispec.DelegationStatePending {
		t.Fatalf("expected pending state, got %s", del.State)
	}
	if del.FromAgentId.String() != sourceID || del.ToAgentId.String() != targetID {
		t.Fatalf("unexpected delegation actors: from=%s to=%s", del.FromAgentId, del.ToAgentId)
	}

	// 3. Question created to humans
	var qList apispec.QuestionList
	if s := e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList); s != http.StatusOK {
		t.Fatalf("list questions: %d", s)
	}
	if len(qList.Questions) == 0 {
		t.Fatalf("expected open question")
	}
	q := qList.Questions[0]
	if !q.ToHuman {
		t.Fatalf("expected question to human")
	}

	// 4. Admin approves
	var answered apispec.Question
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", q.Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, &answered); s != http.StatusOK {
		t.Fatalf("answer question: %d", s)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain after approve: %v", err)
	}

	// 5. Delegation is now approved
	if s := e.do("GET", "/api/v1/delegations?state=approved", server.DevAdminToken, nil, &delList); s != http.StatusOK {
		t.Fatalf("list approved delegations: %d", s)
	}
	if len(delList.Delegations) != 1 {
		t.Fatalf("expected 1 approved delegation, got %d", len(delList.Delegations))
	}
	if delList.Delegations[0].State != apispec.DelegationStateApproved {
		t.Fatalf("expected approved state, got %s", delList.Delegations[0].State)
	}
	if delList.Delegations[0].DecidedByHandle == nil || *delList.Delegations[0].DecidedByHandle != "norbi" {
		t.Fatalf("expected decided by norbi, got %v", delList.Delegations[0].DecidedByHandle)
	}

	// 6. Target agent gets exactly ONE inbox item of kind "delegated", carrying the approver
	if s := e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox); s != http.StatusOK {
		t.Fatalf("get target inbox: %d", s)
	}
	if inBox.Total != 1 {
		t.Fatalf("expected target inbox to have exactly 1 item, got %d", inBox.Total)
	}
	item := inBox.Items[0]
	if item.Kind != "delegated" {
		t.Fatalf("expected item kind 'delegated', got %s", item.Kind)
	}
	if item.DelegatedBy == nil || *item.DelegatedBy != "norbi" {
		t.Fatalf("expected delegated_by 'norbi', got %v", item.DelegatedBy)
	}
	if item.Author != "projects" || string(item.AuthorKind) != "agent" {
		t.Fatalf("expected author 'projects' (agent), got %s (%s)", item.Author, item.AuthorKind)
	}

	// 7. Source agent gets 0 items
	var sourceBox apispec.Inbox
	if s := e.do("GET", "/api/v1/agent/inbox", sourceTok, nil, &sourceBox); s != http.StatusOK {
		t.Fatalf("get source inbox: %d", s)
	}
	if sourceBox.Total != 0 {
		t.Fatalf("expected source inbox to be empty on approve, got %d", sourceBox.Total)
	}
}

func TestDelegationFlowDeny(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	hostID, _ := e.enroll("machine-deny")
	_, sourceTok := createSandboxedAgent(e, hostID, "projects-deny", "Projects Agent")
	_, targetTok := createFullAccessAgent(e, hostID, "agy-homelab-deny", "Homelab Agent")

	// Source agent mentions full-access target agent
	msgText := "@agy-homelab-deny dangerous command"
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: msgText,
	}, nil)

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) == 0 {
		t.Fatalf("expected open question")
	}
	q := qList.Questions[0]

	// Admin denies
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", q.Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Elutasítom",
	}, nil); s != http.StatusOK {
		t.Fatalf("deny question: %d", s)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Target gets nothing
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 0 {
		t.Fatalf("expected target inbox 0, got %d", inBox.Total)
	}

	// Source agent gets an answer item
	var sourceBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", sourceTok, nil, &sourceBox)
	if sourceBox.Total != 1 {
		t.Fatalf("expected source inbox 1 answer item, got %d", sourceBox.Total)
	}
	if sourceBox.Items[0].Kind != "answer" {
		t.Fatalf("expected kind 'answer', got %s", sourceBox.Items[0].Kind)
	}
}

func TestDelegationFlowExpire(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	hostID, _ := e.enroll("machine-exp")
	_, sourceTok := createSandboxedAgent(e, hostID, "projects-exp", "Projects Agent")
	_, targetTok := createFullAccessAgent(e, hostID, "agy-homelab-exp", "Homelab Agent")

	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@agy-homelab-exp do work",
	}, nil)

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Advance clock forward 25 hours (past the 24 hour deadline)
	e.clock.Advance(25 * time.Hour)

	// Run due processing
	if _, err := e.app.Question.RunDue(ctx); err != nil {
		t.Fatalf("rundue: %v", err)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Target is not woken
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 0 {
		t.Fatalf("expected target inbox 0 on expire, got %d", inBox.Total)
	}

	// Source gets answer item
	var sourceBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", sourceTok, nil, &sourceBox)
	if sourceBox.Total != 1 {
		t.Fatalf("expected source inbox 1 item on expire, got %d", sourceBox.Total)
	}
	if sourceBox.Items[0].Kind != "answer" {
		t.Fatalf("expected kind answer, got %s", sourceBox.Items[0].Kind)
	}

	// Delegation state is expired
	var delList apispec.DelegationList
	e.do("GET", "/api/v1/delegations?state=expired", server.DevAdminToken, nil, &delList)
	if len(delList.Delegations) != 1 {
		t.Fatalf("expected 1 expired delegation, got %d", len(delList.Delegations))
	}
}

func TestDelegationNonAdminCannotApprove(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	// Create non-admin human (member role)
	var createdUser struct {
		Actor apispec.Actor `json:"actor"`
		Token string        `json:"token"`
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken, map[string]any{
		"kind":         "human",
		"handle":       "regular-user",
		"display_name": "Regular User",
		"role":         "member",
	}, &createdUser); s != http.StatusCreated {
		t.Fatalf("create regular user: %d", s)
	}
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{
		"actor_id": createdUser.Actor.Id.String(),
	}, nil)

	hostID, _ := e.enroll("machine-nonadmin")
	_, sourceTok := createSandboxedAgent(e, hostID, "projects-na", "Projects Agent")
	_, targetTok := createFullAccessAgent(e, hostID, "agy-homelab-na", "Homelab Agent")

	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@agy-homelab-na please run",
	}, nil)

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) == 0 {
		t.Fatalf("expected open question")
	}
	q := qList.Questions[0]

	// Non-admin tries to answer
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", q.Id), createdUser.Token, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, nil); s != http.StatusOK {
		t.Fatalf("non-admin answer: %d", s)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Delegation must still be pending!
	var delList apispec.DelegationList
	e.do("GET", "/api/v1/delegations?state=pending", server.DevAdminToken, nil, &delList)
	if len(delList.Delegations) != 1 {
		t.Fatalf("expected delegation to remain pending after non-admin answer")
	}

	// Target inbox is still empty!
	var inBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 0 {
		t.Fatalf("target should not be woken by non-admin approval")
	}

	// Question is reopened!
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("question should be reopened for admin approval")
	}

	// Now admin approves!
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", q.Id), server.DevAdminToken, apispec.AnswerQuestionRequest{
		Answer: "Jóváhagyom",
	}, nil); s != http.StatusOK {
		t.Fatalf("admin answer: %d", s)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Now target gets the item!
	e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inBox)
	if inBox.Total != 1 {
		t.Fatalf("expected target 1 item after admin approval, got %d", inBox.Total)
	}
}

func TestDelegationSandboxedAndHumanMention(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	hostID, _ := e.enroll("machine-sandbox")
	_, sourceTok := createSandboxedAgent(e, hostID, "projects-sb", "Projects Agent")
	_, sandboxedTok := createSandboxedAgent(e, hostID, "worker-sb", "Worker Sandbox")
	_, fullAccessTok := createFullAccessAgent(e, hostID, "full-access", "Full Access")

	// 1. Agent mentions sandboxed agent -> no delegation, wakes directly
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@worker-sb do work",
	}, nil)

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	var sbBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", sandboxedTok, nil, &sbBox)
	if sbBox.Total != 1 || sbBox.Items[0].Kind != "message" {
		t.Fatalf("sandboxed agent should be woken directly with kind 'message', got %+v", sbBox)
	}

	// No delegation created
	var delList apispec.DelegationList
	e.do("GET", "/api/v1/delegations", server.DevAdminToken, nil, &delList)
	if len(delList.Delegations) != 0 {
		t.Fatalf("no delegation should be created for sandboxed target")
	}

	// 2. Human mentions full-access agent -> no delegation, wakes directly
	e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, apispec.SendMessageRequest{
		Body: "@full-access please run task",
	}, nil)

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	var faBox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", fullAccessTok, nil, &faBox)
	if faBox.Total != 1 || faBox.Items[0].Kind != "message" {
		t.Fatalf("full-access agent should be woken directly by human with kind 'message', got %+v", faBox)
	}

	e.do("GET", "/api/v1/delegations", server.DevAdminToken, nil, &delList)
	if len(delList.Delegations) != 0 {
		t.Fatalf("no delegation should be created for human author")
	}
}

func TestDelegationMultipleFullAccessTargets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()

	hostID, _ := e.enroll("machine-multi")
	_, sourceTok := createSandboxedAgent(e, hostID, "projects-multi", "Projects Agent")
	_, _ = createFullAccessAgent(e, hostID, "fa-one", "Full Access 1")
	_, _ = createFullAccessAgent(e, hostID, "fa-two", "Full Access 2")

	// Message mentions two full-access agents
	e.do("POST", "/api/v1/channels/"+general+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@fa-one and @fa-two please coordinate",
	}, nil)

	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	var delList apispec.DelegationList
	e.do("GET", "/api/v1/delegations?state=pending", server.DevAdminToken, nil, &delList)
	if len(delList.Delegations) != 2 {
		t.Fatalf("expected 2 pending delegations, got %d", len(delList.Delegations))
	}

	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 2 {
		t.Fatalf("expected 2 questions created, got %d", len(qList.Questions))
	}
}

func TestDelegationPermissions(t *testing.T) {
	e := newEnv(t)
	hostID, _ := e.enroll("machine-perm")
	_, sourceTok := createSandboxedAgent(e, hostID, "agent-perm", "Perm Agent")

	// Agent cannot list delegations (humans only)
	if s := e.do("GET", "/api/v1/delegations", sourceTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent list delegations: expected 403, got %d", s)
	}
}
