package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

func (e *env) getMessages(channelID string) []apispec.Message {
	var list apispec.MessageList
	e.do("GET", "/api/v1/channels/"+channelID+"/messages", server.DevAdminToken, nil, &list)
	return list.Messages
}

func TestReplyRoutingIncidentReproduction(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	// 1. Create #visegradevent channel
	privKind := apispec.Private
	var visegradChan apispec.Channel
	status := e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{
		Name: "visegradevent",
		Kind: &privKind,
	}, &visegradChan)
	if status != http.StatusCreated {
		t.Fatalf("create channel: status %d", status)
	}

	// 2. Create @visegrad-claude and @visegrad-agy agents
	claudeID, claudeToken := e.createActor("agent", "visegrad-claude", "member")
	agyID, agyToken := e.createActor("agent", "visegrad-agy", "member")

	// 3. Add both agents to #visegradevent
	canWrite := true
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", visegradChan.Id), server.DevAdminToken, apispec.AddMemberRequest{
		ActorId:  uuid.MustParse(claudeID),
		CanWrite: &canWrite,
	}, nil)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", visegradChan.Id), server.DevAdminToken, apispec.AddMemberRequest{
		ActorId:  uuid.MustParse(agyID),
		CanWrite: &canWrite,
	}, nil)

	// 4. Admin pairs machine and sets up direct mode guard for @visegrad-claude
	base := t.TempDir()
	root := filepath.Join(base, "project")
	_ = os.MkdirAll(root, 0o700)
	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	tokenFile := filepath.Join(base, "host.token")
	if err := guard.Enroll(ctx, e.srv.URL, pc.Code, "claude-box", tokenFile); err != nil {
		t.Fatal(err)
	}
	var hosts struct{ Hosts []struct{ ID string } }
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hosts)
	hostID := hosts.Hosts[0].ID

	e.do("PUT", "/api/v1/agents/"+claudeID+"/host", server.DevAdminToken, map[string]any{
		"host_id":         hostID,
		"client":          "claude-code",
		"mode":            "direct",
		"root_dir":        root,
		"sandbox":         false, // full access, as @visegrad-claude in production
		"permission_mode": "auto",
	}, nil)

	var mu sync.Mutex
	var directReqs []guard.DirectRequest
	var mcpConfigs [][]byte
	ad := &scriptedAdapter{
		direct: func(ctx context.Context, req guard.DirectRequest) (guard.RunResult, error) {
			mu.Lock()
			directReqs = append(directReqs, req)
			if req.MCPConfigPath != "" {
				cfg, _ := os.ReadFile(req.MCPConfigPath)
				mcpConfigs = append(mcpConfigs, cfg)
			}
			mu.Unlock()
			return guard.RunResult{Text: "Test runs completed: 30/30 green in visegradevent", SessionID: req.SessionID}, nil
		},
	}

	off := false
	sup := &guard.Supervisor{
		Cfg: guard.Config{
			Server:           e.srv.URL,
			HostTokenFile:    tokenFile,
			Roots:            []string{root},
			SandboxDir:       filepath.Join(base, "sandbox"),
			RequireSandbox:   &off,
			AllowUnsandboxed: true,
			ConfigPoll:       20 * time.Millisecond,
			Defaults: guard.AgentConfig{
				Poll:      20 * time.Millisecond,
				Debounce:  time.Millisecond,
				HaltCheck: 10 * time.Millisecond,
				// The approved item is written after the approval event the
				// guard may wake on; poll so the test does not wait for the
				// next event.
				FallbackPoll: 50 * time.Millisecond,
			},
		},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(guard.AgentConfig) guard.Adapter { return ad },
	}
	if err := sup.Cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = sup.RunHost(ctx) }()

	// 5. Admin starts thread T in #visegradevent
	qT, _ := e.send(server.DevAdminToken, visegradChan.Id.String(), map[string]any{
		"body": "Norbi: @visegrad-claude please implement feature and send test runs to @visegrad-agy by DM",
	})
	threadT := qT.ID

	// Wait for Claude's initial run in thread T
	eventually(t, "claude initial run in thread T", func() bool {
		msgs := e.messages(visegradChan.Id.String())
		for _, m := range msgs {
			if m.ThreadRootID == threadT && m.AuthorID == claudeID {
				return true
			}
		}
		return false
	})

	// 6. @visegrad-claude in thread T sends DM to @visegrad-agy with origin headers
	claudeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", claudeToken, headerTripper{
		headers: map[string]string{
			"X-Colonyhall-Channel": visegradChan.Id.String(),
			"X-Colonyhall-Thread":  threadT,
		},
		base: http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = claudeSession.Close() }()

	res, err := claudeSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "send",
		Arguments: map[string]any{
			"channel": "@visegrad-agy",
			"text":    "Please run test suite for feature",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("claude send result: isErr=%v, content=%+v", res.IsError, res.Content)
	if res.IsError {
		t.Fatalf("MCP send failed: %+v", res)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// 7. Verify the DM message has origin attached
	var dmChan struct{ ID string }
	e.do("POST", "/api/v1/dms", claudeToken, map[string]any{"actor_ids": []string{agyID}}, &dmChan)
	dmMsgs := e.getMessages(dmChan.ID)
	if len(dmMsgs) != 1 {
		t.Fatalf("expected 1 DM message, got %d", len(dmMsgs))
	}
	dmReqFull := dmMsgs[0]
	if dmReqFull.Origin == nil {
		t.Fatal("expected origin on DM message, got nil")
	}
	if dmReqFull.Origin.ChannelId != visegradChan.Id || dmReqFull.Origin.ThreadId.String() != threadT {
		t.Fatalf("origin mismatch: got %+v", dmReqFull.Origin)
	}

	// 8. @visegrad-agy replies in the thread of that DM message
	agySession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", agyToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = agySession.Close() }()

	res, err = agySession.CallTool(ctx, &mcp.CallToolParams{
		Name: "send",
		Arguments: map[string]any{
			"channel": "@visegrad-claude",
			"thread":  dmReqFull.Id.String(),
			"text":    "30/30 zöld",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("agy send result: isErr=%v, content=%s", res.IsError, res.Content[0].(*mcp.TextContent).Text)
	if res.IsError {
		t.Fatalf("agy MCP reply failed: %+v", res)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// 8b. @visegrad-claude has full access, so an agent's message reaches it
	// only after an admin approves it: it arrives as a "delegated" item. This
	// is the path the 2026-10-28 reply took.
	var qList apispec.QuestionList
	e.do("GET", "/api/v1/questions?state=open", server.DevAdminToken, nil, &qList)
	if len(qList.Questions) != 1 {
		t.Fatalf("expected 1 approval question for the reply, got %d", len(qList.Questions))
	}
	if s := e.do("POST", fmt.Sprintf("/api/v1/questions/%s/answer", qList.Questions[0].Id), server.DevAdminToken,
		apispec.AnswerQuestionRequest{Answer: "Jóváhagyom"}, nil); s != http.StatusOK {
		t.Fatalf("approve reply: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// 9. Get Agy's reply message from the DM
	dmMsgs = e.getMessages(dmChan.ID)
	if len(dmMsgs) != 2 {
		t.Fatalf("expected 2 messages in DM (Claude request + Agy reply), got %d", len(dmMsgs))
	}
	agyReplyMsg := dmMsgs[1]

	// 10. The guard wakes @visegrad-claude and runs in thread T's session
	expectedSessionID, _ := guard.DirectSessionID(claudeID, threadT)
	eventually(t, "claude guard runs reply in thread T session", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, req := range directReqs {
			if req.SessionID == expectedSessionID && strings.Contains(req.Prompt, `answers_your_request="`+dmReqFull.Id.String()+`"`) {
				return true
			}
		}
		return false
	})

	// Verify MCP config headers carry X-Colonyhall-Thread: threadT
	mu.Lock()
	if len(mcpConfigs) < 2 {
		t.Fatalf("expected at least 2 runs with MCP configs, got %d", len(mcpConfigs))
	}
	lastMCP := mcpConfigs[len(mcpConfigs)-1]
	mu.Unlock()

	var mcpParsed struct {
		MCPServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(lastMCP, &mcpParsed); err != nil {
		t.Fatalf("parse MCP config: %v", err)
	}
	if mcpParsed.MCPServers["colonyhall"].Headers["X-Colonyhall-Thread"] != threadT {
		t.Fatalf("expected X-Colonyhall-Thread header %s, got %+v", threadT, mcpParsed.MCPServers["colonyhall"].Headers)
	}

	// 11. Verify the answer was posted into thread T in #visegradevent, NOT in the DM
	// The link to the DM reply is a checked field, not text in the body.
	eventually(t, "answer posted into thread T", func() bool {
		var thread apispec.Thread
		e.do("GET", "/api/v1/messages/"+threadT+"/thread", server.DevAdminToken, nil, &thread)
		for _, m := range thread.Replies {
			if m.AuthorId.String() == claudeID && strings.Contains(m.Body, "Test runs completed") &&
				!strings.Contains(m.Body, "dm_reply") && m.Answers != nil &&
				m.Answers.MessageId == agyReplyMsg.Id && m.Answers.ChannelId.String() == dmChan.ID &&
				m.Answers.AuthorId.String() == agyID {
				return true
			}
		}
		return false
	})

	// Verify NO text message was posted in DM by Claude in response to the reply
	dmMsgsAfter := e.messages(dmChan.ID)
	// Only 2 messages: Claude's request and Agy's reply
	if len(dmMsgsAfter) != 2 {
		t.Fatalf("expected 2 messages in DM (no text reply in DM), got %d: %+v", len(dmMsgsAfter), dmMsgsAfter)
	}

	// 12. Verify 👍 reaction was added in the DM on Agy's reply message
	eventually(t, "thumbs up reaction added to reply in DM", func() bool {
		msgs := e.getMessages(dmChan.ID)
		for _, m := range msgs {
			if m.Id == agyReplyMsg.Id && m.Reactions != nil {
				for _, r := range *m.Reactions {
					if r.Emoji == "👍" {
						return true
					}
				}
			}
		}
		return false
	})
}

func TestReplyRoutingTopLevelDMReply(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	privKind := apispec.Private
	var visegradChan apispec.Channel
	e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{
		Name: "channel-top",
		Kind: &privKind,
	}, &visegradChan)

	claudeID, claudeToken := e.createActor("agent", "claude-top", "member")
	agyID, agyToken := e.createActor("agent", "agy-top", "member")

	canWrite := true
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", visegradChan.Id), server.DevAdminToken, apispec.AddMemberRequest{ActorId: uuid.MustParse(claudeID), CanWrite: &canWrite}, nil)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", visegradChan.Id), server.DevAdminToken, apispec.AddMemberRequest{ActorId: uuid.MustParse(agyID), CanWrite: &canWrite}, nil)

	qT, _ := e.send(server.DevAdminToken, visegradChan.Id.String(), map[string]any{"body": "Start work in thread"})
	threadT := qT.ID

	// Claude sends DM to Agy with origin headers
	claudeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", claudeToken, headerTripper{
		headers: map[string]string{
			"X-Colonyhall-Channel": visegradChan.Id.String(),
			"X-Colonyhall-Thread":  threadT,
		},
		base: http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = claudeSession.Close() }()

	_, err = claudeSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "send",
		Arguments: map[string]any{"channel": "@agy-top", "text": "Top level request"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	var dmChan struct{ ID string }
	e.do("POST", "/api/v1/dms", claudeToken, map[string]any{"actor_ids": []string{agyID}}, &dmChan)
	reqMsgID := e.messages(dmChan.ID)[0].ID

	// Agy replies TOP-LEVEL in the DM (not in a thread)
	agySession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", agyToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = agySession.Close() }()

	_, err = agySession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "send",
		Arguments: map[string]any{"channel": "@claude-top", "text": "Top level answer"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify Claude's inbox item gets reply_origin
	var claudeInbox apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", claudeToken, nil, &claudeInbox)
	if len(claudeInbox.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(claudeInbox.Items))
	}
	it := claudeInbox.Items[0]
	if it.ReplyOrigin == nil {
		t.Fatalf("expected reply_origin for top-level DM reply, got nil")
	}
	if it.ReplyOrigin.ChannelId != visegradChan.Id || it.ReplyOrigin.ThreadRootId.String() != threadT || it.ReplyOrigin.RequestMessageId.String() != reqMsgID {
		t.Fatalf("unexpected reply_origin: %+v", it.ReplyOrigin)
	}
}

func TestReplyRoutingNegativeCases(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	privKind := apispec.Private
	var testChan apispec.Channel
	e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{
		Name: "test-channel",
		Kind: &privKind,
	}, &testChan)

	claudeID, claudeToken := e.createActor("agent", "claude-neg", "member")
	agyID, agyToken := e.createActor("agent", "agy-neg", "member")

	canWrite := true
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", testChan.Id), server.DevAdminToken, apispec.AddMemberRequest{ActorId: uuid.MustParse(claudeID), CanWrite: &canWrite}, nil)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", testChan.Id), server.DevAdminToken, apispec.AddMemberRequest{ActorId: uuid.MustParse(agyID), CanWrite: &canWrite}, nil)

	qT, _ := e.send(server.DevAdminToken, testChan.Id.String(), map[string]any{"body": "Thread T"})
	threadT := qT.ID

	var dmChan struct{ ID string }
	e.do("POST", "/api/v1/dms", claudeToken, map[string]any{"actor_ids": []string{agyID}}, &dmChan)

	// Negative Case A: A human sent the request -> no reply_origin
	{
		hReq, _ := e.send(server.DevAdminToken, dmChan.ID, map[string]any{"body": "Human request"})
		// Agy replies in the thread of human request
		e.send(agyToken, dmChan.ID, map[string]any{"body": "Agy answer to human", "thread_root_id": hReq.ID})
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		var inbox apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", claudeToken, nil, &inbox)
		for _, item := range inbox.Items {
			if item.ReplyOrigin != nil {
				t.Fatalf("expected nil reply_origin when human sent request, got %+v", item.ReplyOrigin)
			}
		}
		// Clear inbox
		var ackIDs []string
		for _, it := range inbox.Items {
			ackIDs = append(ackIDs, it.Id.String())
		}
		if len(ackIDs) > 0 {
			e.do("POST", "/api/v1/agent/inbox/ack", claudeToken, map[string]any{"ids": ackIDs}, nil)
		}
	}

	// Negative Case B: Origin message is retracted -> no reply_origin
	{
		claudeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", claudeToken, headerTripper{
			headers: map[string]string{
				"X-Colonyhall-Channel": testChan.Id.String(),
				"X-Colonyhall-Thread":  threadT,
			},
			base: http.DefaultTransport,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = claudeSession.CallTool(ctx, &mcp.CallToolParams{
			Name:      "send",
			Arguments: map[string]any{"channel": "@agy-neg", "text": "Will be retracted"},
		})
		_ = claudeSession.Close()
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}

		// Find the message and retract it directly via store
		msgs := e.messages(dmChan.ID)
		reqMsg := msgs[len(msgs)-1]
		if err := e.app.Conversation.Store.RetractMessage(ctx, uuid.MustParse(reqMsg.ID), uuid.MustParse(claudeID), time.Now()); err != nil {
			t.Fatal(err)
		}

		// Agy replies in the thread of the retracted message
		e.send(agyToken, dmChan.ID, map[string]any{"body": "Answer after retract", "thread_root_id": reqMsg.ID})
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}

		var inbox apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", claudeToken, nil, &inbox)
		for _, item := range inbox.Items {
			if item.ReplyOrigin != nil {
				t.Fatalf("expected nil reply_origin when request was retracted, got %+v", item.ReplyOrigin)
			}
		}
		// Clear inbox
		var ackIDs []string
		for _, it := range inbox.Items {
			ackIDs = append(ackIDs, it.Id.String())
		}
		if len(ackIDs) > 0 {
			e.do("POST", "/api/v1/agent/inbox/ack", claudeToken, map[string]any{"ids": ackIDs}, nil)
		}
	}

	// Negative Case C: Agent left origin channel -> no reply_origin
	{
		claudeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", claudeToken, headerTripper{
			headers: map[string]string{
				"X-Colonyhall-Channel": testChan.Id.String(),
				"X-Colonyhall-Thread":  threadT,
			},
			base: http.DefaultTransport,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = claudeSession.CallTool(ctx, &mcp.CallToolParams{
			Name:      "send",
			Arguments: map[string]any{"channel": "@agy-neg", "text": "Before leaving channel"},
		})
		_ = claudeSession.Close()
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}

		msgs := e.messages(dmChan.ID)
		reqMsg := msgs[len(msgs)-1]

		// Claude is removed from testChan
		e.do("DELETE", fmt.Sprintf("/api/v1/channels/%s/members/%s", testChan.Id, claudeID), server.DevAdminToken, nil, nil)

		// Agy replies in thread
		e.send(agyToken, dmChan.ID, map[string]any{"body": "Answer after Claude left", "thread_root_id": reqMsg.ID})
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}

		var inbox apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", claudeToken, nil, &inbox)
		for _, item := range inbox.Items {
			if item.ReplyOrigin != nil {
				t.Fatalf("expected nil reply_origin when agent left channel, got %+v", item.ReplyOrigin)
			}
		}
		// Clear inbox
		var ackIDs []string
		for _, it := range inbox.Items {
			ackIDs = append(ackIDs, it.Id.String())
		}
		if len(ackIDs) > 0 {
			e.do("POST", "/api/v1/agent/inbox/ack", claudeToken, map[string]any{"ids": ackIDs}, nil)
		}

		// Re-add Claude to channel for subsequent tests
		e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", testChan.Id), server.DevAdminToken, apispec.AddMemberRequest{ActorId: uuid.MustParse(claudeID), CanWrite: &canWrite}, nil)
	}

	// Negative Case D: Agent posted newer top-level DM message without origin -> no reply_origin
	{
		// 1. Claude posts top-level with origin
		claudeSessionWithHdr, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", claudeToken, headerTripper{
			headers: map[string]string{
				"X-Colonyhall-Channel": testChan.Id.String(),
				"X-Colonyhall-Thread":  threadT,
			},
			base: http.DefaultTransport,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = claudeSessionWithHdr.CallTool(ctx, &mcp.CallToolParams{
			Name:      "send",
			Arguments: map[string]any{"channel": "@agy-neg", "text": "Top-level with origin"},
		})
		_ = claudeSessionWithHdr.Close()

		// 2. Claude posts newer top-level WITHOUT origin
		claudeSessionPlain, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", claudeToken, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = claudeSessionPlain.CallTool(ctx, &mcp.CallToolParams{
			Name:      "send",
			Arguments: map[string]any{"channel": "@agy-neg", "text": "Newer top-level without origin"},
		})
		_ = claudeSessionPlain.Close()

		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}

		// 3. Agy replies top-level in DM
		e.send(agyToken, dmChan.ID, map[string]any{"body": "Top level reply after newer message"})
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}

		var inbox apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", claudeToken, nil, &inbox)
		for _, item := range inbox.Items {
			if item.ReplyOrigin != nil {
				t.Fatalf("expected nil reply_origin when newer top-level message had no origin, got %+v", item.ReplyOrigin)
			}
		}
	}
}

func TestReplyRoutingForgePrevention(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Admin creates a secret channel where agent is NOT a member
	privKind := apispec.Private
	var secretChan apispec.Channel
	e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{
		Name: "secret-chan",
		Kind: &privKind,
	}, &secretChan)

	qSecret, _ := e.send(server.DevAdminToken, secretChan.Id.String(), map[string]any{"body": "Secret thread"})

	// Create agents
	_, claudeToken := e.createActor("agent", "claude-forge", "member")
	agyID, _ := e.createActor("agent", "agy-forge", "member")

	// Claude attempts to send DM with origin headers pointing to secretChan where Claude is NOT a member
	claudeSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", claudeToken, headerTripper{
		headers: map[string]string{
			"X-Colonyhall-Channel": secretChan.Id.String(),
			"X-Colonyhall-Thread":  qSecret.ID,
		},
		base: http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = claudeSession.Close() }()

	_, err = claudeSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "send",
		Arguments: map[string]any{"channel": "@agy-forge", "text": "Forge attempt"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// Message in DM must NOT store forged origin
	var dmChan struct{ ID string }
	e.do("POST", "/api/v1/dms", claudeToken, map[string]any{"actor_ids": []string{agyID}}, &dmChan)
	msgs := e.getMessages(dmChan.ID)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 DM message, got %d", len(msgs))
	}
	if msgs[0].Origin != nil {
		t.Fatalf("expected nil origin for forged channel, got %+v", msgs[0].Origin)
	}
}

// A top-level DM message counts as the answer to the recipient's latest
// request only within delivery.ReplyOriginWindow (12 h, the grant cap): a much
// later, unrelated DM must not be routed into an old work thread.
func TestReplyRoutingTopLevelDMReplyWindow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()
	askerID, askerTok := e.createActor("agent", "asker", "member")
	_, helperTok := e.createActor("agent", "helper", "member")
	canWrite := true
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, apispec.AddMemberRequest{
		ActorId: uuid.MustParse(askerID), CanWrite: &canWrite,
	}, nil)
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@asker please coordinate"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	askerMCP, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", askerTok, headerTripper{
		headers: map[string]string{"X-Colonyhall-Channel": general, "X-Colonyhall-Thread": root.ID},
		base:    http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = askerMCP.Close() }()
	helperMCP, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", helperTok, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = helperMCP.Close() }()

	call := func(s *mcp.ClientSession, channel, text string) {
		t.Helper()
		res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": channel, "text": text}})
		if err != nil || res.IsError {
			t.Fatalf("send to %s: %v %+v", channel, err, res)
		}
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	latestOrigin := func() *apispec.ReplyOrigin {
		t.Helper()
		var ib apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", askerTok, nil, &ib)
		var last *apispec.InboxItem
		for i := range ib.Items {
			if ib.Items[i].Author == "helper" {
				last = &ib.Items[i]
			}
		}
		if last == nil {
			t.Fatalf("no item from helper in %+v", ib.Items)
		}
		return last.ReplyOrigin
	}

	call(askerMCP, "@helper", "please run the tests")
	e.clock.Advance(11 * time.Hour)
	call(helperMCP, "@asker", "30/30 green")
	if ro := latestOrigin(); ro == nil || ro.ThreadRootId.String() != root.ID {
		t.Fatalf("reply after 11 h: expected origin thread %s, got %+v", root.ID, ro)
	}

	e.clock.Advance(2 * time.Hour) // 13 h after the request
	call(helperMCP, "@asker", "unrelated: are you free tomorrow?")
	if ro := latestOrigin(); ro != nil {
		t.Fatalf("a DM 13 h after the request was routed to the old thread: %+v", ro)
	}
}

// The answer link (answers) and the request origin (origin) are kept only for
// a real reply to the agent's own request from that thread, and are shown
// only to viewers who can read the channel they point to.
func TestReplyRoutingLinksRespectAccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	privKind := apispec.Private
	var secret apispec.Channel
	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{Name: "secret", Kind: &privKind}, &secret); s != http.StatusCreated {
		t.Fatalf("create channel: %d", s)
	}
	askerID, askerTok := e.createActor("agent", "asker", "member")
	helperID, helperTok := e.createActor("agent", "helper", "member")
	otherID, otherTok := e.createActor("agent", "other", "member")
	viewerID, viewerTok := e.createUser("viewer", "Viewer")
	canWrite := true
	for _, id := range []string{askerID, viewerID} {
		e.do("POST", "/api/v1/channels/"+secret.Id.String()+"/members", server.DevAdminToken, apispec.AddMemberRequest{ActorId: uuid.MustParse(id), CanWrite: &canWrite}, nil)
	}
	root, _ := e.send(server.DevAdminToken, secret.Id.String(), map[string]any{"body": "@asker coordinate the tests"})
	root2, _ := e.send(server.DevAdminToken, secret.Id.String(), map[string]any{"body": "another topic"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	mcpSend := func(tok string, headers map[string]string, args map[string]any) {
		t.Helper()
		var rt http.RoundTripper
		if headers != nil {
			rt = headerTripper{headers: headers, base: http.DefaultTransport}
		}
		s, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", tok, rt)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("send %v: %v %+v", args, err, res)
		}
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// asker asks helper from the root thread; helper answers in the DM thread.
	mcpSend(askerTok, map[string]string{"X-Colonyhall-Channel": secret.Id.String(), "X-Colonyhall-Thread": root.ID},
		map[string]any{"channel": "@helper", "text": "please run the tests"})
	var dm struct{ ID string }
	e.do("POST", "/api/v1/dms", askerTok, map[string]any{"actor_ids": []string{helperID}}, &dm)
	request := e.getMessages(dm.ID)[0]
	mcpSend(helperTok, nil, map[string]any{"channel": "@asker", "thread": request.Id.String(), "text": "30/30 green"})
	var reply apispec.Message
	for _, m := range e.getMessages(dm.ID) {
		if m.Body == "30/30 green" {
			reply = m
		}
	}
	// An unrelated DM message from another agent.
	mcpSend(otherTok, nil, map[string]any{"channel": "@asker", "text": "unrelated"})
	var dmOther struct{ ID string }
	e.do("POST", "/api/v1/dms", askerTok, map[string]any{"actor_ids": []string{otherID}}, &dmOther)
	unrelated := e.getMessages(dmOther.ID)[0]

	post := func(thread string, answers uuid.UUID) apispec.Message {
		t.Helper()
		var res apispec.SendResult
		if s := e.do("POST", "/api/v1/channels/"+secret.Id.String()+"/messages", askerTok, map[string]any{
			"body": "result", "thread_root_id": thread, "answers_message_id": answers.String(),
		}, &res); s != http.StatusCreated || res.Message == nil {
			t.Fatalf("post answer: %d %+v", s, res)
		}
		return *res.Message
	}
	if m := post(root.ID, reply.Id); m.Answers == nil || m.Answers.MessageId != reply.Id {
		t.Fatalf("a real reply to the asker's request from this thread lost its link: %+v", m.Answers)
	}
	if m := post(root.ID, unrelated.Id); m.Answers != nil {
		t.Fatalf("a link to an unrelated DM message was stored: %+v", m.Answers)
	}
	if m := post(root2.ID, reply.Id); m.Answers != nil {
		t.Fatalf("a link from a thread the request did not come from was stored: %+v", m.Answers)
	}

	// Readers: the viewer is in #secret but not in the DM; helper is in the
	// DM but not in #secret. The admin can read both.
	answersIn := func(tok string) bool {
		var th apispec.Thread
		e.do("GET", "/api/v1/messages/"+root.ID+"/thread", tok, nil, &th)
		for _, m := range th.Replies {
			if m.Answers != nil {
				return true
			}
		}
		return false
	}
	if !answersIn(server.DevAdminToken) {
		t.Fatal("the admin should see the answer link")
	}
	if answersIn(viewerTok) {
		t.Fatal("a viewer who cannot read the DM sees the answer link")
	}
	originFor := func(tok string) *apispec.MessageOrigin {
		var list apispec.MessageList
		e.do("GET", "/api/v1/channels/"+dm.ID+"/messages", tok, nil, &list)
		for _, m := range list.Messages {
			if m.Id == request.Id {
				return m.Origin
			}
		}
		t.Fatalf("request not visible")
		return nil
	}
	if originFor(server.DevAdminToken) == nil {
		t.Fatal("the admin should see the request's origin")
	}
	if o := originFor(helperTok); o != nil {
		t.Fatalf("helper, not in #secret, sees the origin channel: %+v", o)
	}
}
