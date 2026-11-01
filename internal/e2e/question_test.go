package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/server"
)

func (e *env) runDueQuestions() int {
	e.t.Helper()
	n, err := e.app.Question.RunDue(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.app.Drain(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// createAgent creates an API-key agent in #general (POST /agents).
func (e *env) createAgent(handle, name string) (string, string) {
	e.t.Helper()
	return e.newAgent(handle, name, nil, nil)
}

// TestQuestionMCPLifecycle tests the complete flow:
// ask (to human) -> verify thread message -> reject duplicate in same thread ->
// human answers via REST -> answer arrives in thread and wakes asker inbox ->
// new question can now be asked in thread -> withdraw by asker.
func TestQuestionMCPLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	// 1. Agent asks a question to humans in #general
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":  "general",
			"question": "Should we deploy v1 to production?",
			"to":       "human",
			"options":  []string{"yes", "no"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("ask failed: %+v", res)
	}
	qRef := strings.TrimSpace(res.Content[0].(*mcp.TextContent).Text)
	if qRef != "Q-1" {
		t.Fatalf("expected Q-1, got %q", qRef)
	}

	// Verify message posted to #general
	msgs := e.messages(general)
	if len(msgs) == 0 {
		t.Fatal("expected message in #general")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Body, "Q-1 (to humans): Should we deploy v1 to production?") ||
		!strings.Contains(last.Body, "Options: yes, no") {
		t.Fatalf("unexpected question message body: %s", last.Body)
	}
	threadRootID := last.ID

	// 2. Asking a second open question in the same thread must fail
	dupRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":  "general",
			"thread":   threadRootID,
			"question": "Another question in same thread?",
			"to":       "human",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dupRes.IsError {
		t.Fatal("expected duplicate open question in thread to fail")
	}

	// 3. Check question via REST API
	var qList apispec.QuestionList
	if s := e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList); s != http.StatusOK {
		t.Fatalf("list open questions: %d", s)
	}
	if len(qList.Questions) != 1 || qList.Questions[0].FormattedNumber != "Q-1" || !qList.Questions[0].ToHuman {
		t.Fatalf("unexpected questions list: %+v", qList.Questions)
	}

	// 4. Human answers the question via REST
	var answered apispec.Question
	if s := e.do("POST", "/api/v1/questions/Q-1/answer", server.DevAdminToken, map[string]any{
		"answer": "yes",
	}, &answered); s != http.StatusOK {
		t.Fatalf("answer question: %d", s)
	}
	if answered.State != apispec.QuestionStateAnswered || answered.Answer == nil || *answered.Answer != "yes" {
		t.Fatalf("unexpected answered question: %+v", answered)
	}

	// Verify answer was posted to thread
	msgs = e.messages(general)
	last = msgs[len(msgs)-1]
	if !strings.Contains(last.Body, "Q-1 answer: yes") {
		t.Fatalf("expected answer message in thread, got: %s", last.Body)
	}

	// Drain background events and verify inbox of asker (fake-agent)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var in apispec.Inbox
	if s := e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in); s != http.StatusOK {
		t.Fatalf("get inbox: %d", s)
	}
	if len(in.Items) != 1 {
		t.Fatalf("expected 1 inbox item for answer, got %d", len(in.Items))
	}
	item := in.Items[0]
	if item.Kind != "answer" || !strings.Contains(item.Body, "Q-1 answer: yes") || item.AuthorKind != apispec.InboxItemAuthorKindHuman {
		t.Fatalf("unexpected inbox item: %+v", item)
	}

	// 5. Asker can now ask a new question in the same thread
	res2, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":  "general",
			"thread":   threadRootID,
			"question": "What is the rollback plan?",
			"to":       "human",
		},
	})
	if err != nil || res2.IsError {
		t.Fatalf("second ask failed: %v %+v", err, res2)
	}
	q2Ref := strings.TrimSpace(res2.Content[0].(*mcp.TextContent).Text)
	if q2Ref != "Q-2" {
		t.Fatalf("expected Q-2, got %q", q2Ref)
	}

	// 6. Asker withdraws Q-2
	wRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "withdraw",
		Arguments: map[string]any{
			"question": "Q-2",
		},
	})
	if err != nil || wRes.IsError {
		t.Fatalf("withdraw failed: %v %+v", err, wRes)
	}

	msgs = e.messages(general)
	last = msgs[len(msgs)-1]
	if !strings.Contains(last.Body, "Q-2 withdrawn by asker.") {
		t.Fatalf("expected withdraw message in thread, got: %s", last.Body)
	}

	// Verify Q-2 is withdrawn
	var openList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &openList)
	if len(openList.Questions) != 0 {
		t.Fatalf("expected 0 open questions, got %d", len(openList.Questions))
	}
	var withdrawnList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=withdrawn", server.DevAdminToken, nil, &withdrawnList)
	if len(withdrawnList.Questions) != 1 || withdrawnList.Questions[0].FormattedNumber != "Q-2" {
		t.Fatalf("unexpected withdrawn questions: %+v", withdrawnList.Questions)
	}
}

// TestQuestionRedirectFullAccess tests that asking a direct agent without sandbox
// redirects the question to humans.
func TestQuestionRedirectFullAccess(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	// 1. Enroll host and create full-access agent
	hostID, _ := e.enroll("server-box")
	fullAgentID, _ := e.createAgent("full-agent", "Full Access Agent")

	e.configure(fullAgentID, map[string]any{
		"host_id":         hostID,
		"client":          "claude-code",
		"mode":            "direct",
		"root_dir":        "/tmp",
		"sandbox":         false,
		"permission_mode": "auto",
	})

	// 2. fake-agent asks @full-agent
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":  "general",
			"to":       "@full-agent",
			"question": "Can you delete stale logs?",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("ask failed: %v %+v", err, res)
	}

	// 3. Verify redirected to human in REST and thread message
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(qList.Questions))
	}
	q := qList.Questions[0]
	if !q.ToHuman || q.ToActorId != nil {
		t.Fatalf("expected to_human=true and to_actor_id=nil, got: %+v", q)
	}

	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Body, "(to humans, redirected: full-access agent)") {
		t.Fatalf("expected redirection note in message, got: %s", last.Body)
	}
}

// TestQuestionPermissions tests target authorization and withdraw authorization.
func TestQuestionPermissions(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, workerToken := e.createAgent("worker-agent", "Worker")
	_, bystanderToken := e.createAgent("bystander-agent", "Bystander")

	fakeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fakeSession.Close() }()

	workerSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", workerToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workerSession.Close() }()

	bystanderSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", bystanderToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bystanderSession.Close() }()

	// fake-agent asks worker-agent
	askRes, err := fakeSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":  "general",
			"to":       "@worker-agent",
			"question": "Is task #42 finished?",
		},
	})
	if err != nil || askRes.IsError {
		t.Fatalf("ask failed: %v %+v", err, askRes)
	}
	qRef := strings.TrimSpace(askRes.Content[0].(*mcp.TextContent).Text)

	// Bystander agent attempts to answer: must fail
	ansRes, err := bystanderSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "answer",
		Arguments: map[string]any{
			"question": qRef,
			"answer":   "I think so",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ansRes.IsError {
		t.Fatal("bystander agent answered a question targeted to worker-agent")
	}

	// Bystander agent attempts to withdraw: must fail (only asker may withdraw)
	wRes, err := bystanderSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "withdraw",
		Arguments: map[string]any{
			"question": qRef,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !wRes.IsError {
		t.Fatal("bystander agent withdrew a question asked by fake-agent")
	}

	// Worker agent answers: must succeed
	workAnsRes, err := workerSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "answer",
		Arguments: map[string]any{
			"question": qRef,
			"answer":   "Finished successfully",
		},
	})
	if err != nil || workAnsRes.IsError {
		t.Fatalf("worker-agent answer failed: %v %+v", err, workAnsRes)
	}

	// Verify asker gets the answer in inbox
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var in apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if len(in.Items) != 1 || in.Items[0].Author != "worker-agent" || in.Items[0].Kind != "answer" {
		t.Fatalf("unexpected inbox: %+v", in.Items)
	}
}

// TestQuestionEscalation tests that an unanswered question to an agent escalates
// after 30 minutes, mentions admins, and can be answered by any human.
func TestQuestionEscalation(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	_, targetToken := e.createAgent("slow-agent", "Slow Agent")

	fakeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fakeSession.Close() }()

	targetSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", targetToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = targetSession.Close() }()

	// fake-agent asks slow-agent
	askRes, err := fakeSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":  "general",
			"to":       "@slow-agent",
			"question": "Can you handle this load?",
		},
	})
	if err != nil || askRes.IsError {
		t.Fatalf("ask failed: %v %+v", err, askRes)
	}
	qRef := strings.TrimSpace(askRes.Content[0].(*mcp.TextContent).Text)

	// Check due questions: none yet
	if n := e.runDueQuestions(); n != 0 {
		t.Fatalf("expected 0 due questions, got %d", n)
	}

	// Advance clock by 31 minutes
	e.clock.Advance(31 * time.Minute)

	// Run due questions: 1 escalated
	if n := e.runDueQuestions(); n != 1 {
		t.Fatalf("expected 1 due question processed, got %d", n)
	}

	// Verify state in REST
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=escalated", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 || qList.Questions[0].FormattedNumber != qRef {
		t.Fatalf("expected escalated question %s, got: %+v", qRef, qList.Questions)
	}

	// Verify escalation message in thread mentions admin (@norbi)
	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Body, "@norbi") || !strings.Contains(last.Body, "was unanswered for 30 minutes and has been escalated to humans.") {
		t.Fatalf("expected escalation note mentioning admin, got: %s", last.Body)
	}

	// Target agent can no longer answer since it's escalated to humans
	ansRes, err := targetSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "answer",
		Arguments: map[string]any{
			"question": qRef,
			"answer":   "late answer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ansRes.IsError {
		t.Fatal("target agent answered an escalated question; only humans may answer")
	}

	// Human answers the escalated question
	var ans apispec.Question
	if s := e.do("POST", "/api/v1/questions/"+qRef+"/answer", server.DevAdminToken, map[string]any{
		"answer": "Human taking over: yes",
	}, &ans); s != http.StatusOK {
		t.Fatalf("human answer escalated: %d", s)
	}
	if ans.State != apispec.QuestionStateAnswered {
		t.Fatalf("expected state answered, got %s", ans.State)
	}

	// Asker gets answer in inbox
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var in apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if len(in.Items) != 1 || in.Items[0].Author != "norbi" {
		t.Fatalf("unexpected inbox: %+v", in.Items)
	}
}

// TestQuestionDeadlineFallback tests fallback to default choice when deadline passes.
func TestQuestionDeadlineFallback(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	_, _ = e.createAgent("busy-agent", "Busy Agent")

	fakeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fakeSession.Close() }()

	// fake-agent asks busy-agent with default and deadline
	askRes, err := fakeSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":          "general",
			"to":               "@busy-agent",
			"question":         "Approve release?",
			"options":          []string{"approve", "reject"},
			"default":          "reject",
			"deadline_minutes": 10,
		},
	})
	if err != nil || askRes.IsError {
		t.Fatalf("ask failed: %v %+v", err, askRes)
	}
	qRef := strings.TrimSpace(askRes.Content[0].(*mcp.TextContent).Text)

	// Advance 5 minutes: not due yet
	e.clock.Advance(5 * time.Minute)
	if n := e.runDueQuestions(); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	// Advance 6 more minutes (total 11 minutes > 10 min deadline)
	e.clock.Advance(6 * time.Minute)
	if n := e.runDueQuestions(); n != 1 {
		t.Fatalf("expected 1 processed due question, got %d", n)
	}

	// Verify question answered with default option
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=answered", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 || qList.Questions[0].FormattedNumber != qRef {
		t.Fatalf("expected answered question %s, got: %+v", qRef, qList.Questions)
	}
	if qList.Questions[0].Answer == nil || *qList.Questions[0].Answer != "reject" {
		t.Fatalf("expected answer 'reject', got: %+v", qList.Questions[0].Answer)
	}

	// Thread message confirms fallback
	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Body, "deadline reached; falling back to default: reject") {
		t.Fatalf("expected fallback message, got: %s", last.Body)
	}

	// Asker gets answer in inbox
	var in apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if len(in.Items) != 1 || in.Items[0].Kind != "answer" {
		t.Fatalf("unexpected inbox item: %+v", in.Items)
	}
}

// TestQuestionToHumanNeverResolves verifies that questions to humans never
// auto-escalate or auto-expire.
func TestQuestionToHumanNeverResolves(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fakeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fakeSession.Close() }()

	askRes, err := fakeSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":          "general",
			"to":               "human",
			"question":         "Irreversible production change?",
			"options":          []string{"go", "abort"},
			"default":          "abort",
			"deadline_minutes": 5,
		},
	})
	if err != nil || askRes.IsError {
		t.Fatalf("ask failed: %v %+v", err, askRes)
	}
	qRef := strings.TrimSpace(askRes.Content[0].(*mcp.TextContent).Text)

	// Advance clock by 2 hours
	e.clock.Advance(2 * time.Hour)

	// Loop must NOT process questions to humans
	if n := e.runDueQuestions(); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	// Verify question remains open
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 || qList.Questions[0].FormattedNumber != qRef {
		t.Fatalf("question to human should remain open, got: %+v", qList.Questions)
	}
}

// TestQuestionRESTAPIErrors tests contract validation for error cases.
func TestQuestionRESTAPIErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// 1. Not found question
	if s := e.do("POST", "/api/v1/questions/Q-9999/answer", server.DevAdminToken, map[string]any{
		"answer": "answer",
	}, nil); s != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown question, got %d", s)
	}

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "ask",
		Arguments: map[string]any{
			"channel":  "general",
			"question": "A question for error testing?",
			"to":       "human",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("ask failed: %v %+v", err, res)
	}

	// 2. Empty answer on existing question -> 400
	if s := e.do("POST", "/api/v1/questions/Q-1/answer", server.DevAdminToken, map[string]any{
		"answer": "",
	}, nil); s != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty answer, got %d", s)
	}

	// 3. Valid answer -> 200
	if s := e.do("POST", "/api/v1/questions/Q-1/answer", server.DevAdminToken, map[string]any{
		"answer": "now answered",
	}, nil); s != http.StatusOK {
		t.Fatalf("expected 200 for answer, got %d", s)
	}

	// 4. Answering an already answered question -> 409 Conflict
	if s := e.do("POST", "/api/v1/questions/Q-1/answer", server.DevAdminToken, map[string]any{
		"answer": "answer again",
	}, nil); s != http.StatusConflict {
		t.Fatalf("expected 409 for already answered question, got %d", s)
	}
}
