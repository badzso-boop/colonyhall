package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/fakeagent"
	"colonyhall/internal/server"
)

type previewOut struct {
	ID          string  `json:"id"`
	AgentID     string  `json:"agent_id"`
	HostID      string  `json:"host_id"`
	ChannelID   string  `json:"channel_id"`
	Name        string  `json:"name"`
	ComposePath string  `json:"compose_path"`
	State       string  `json:"state"`
	URL         *string `json:"url"`
	Error       *string `json:"error"`
	ExpiresAt   string  `json:"expires_at"`
}

type previewListOut struct {
	Previews []previewOut `json:"previews"`
}

func TestPreviewAssignmentFlag(t *testing.T) {
	e := newEnv(t)
	hostID, _ := e.enroll("dev-box")
	create := func(handle string, host map[string]any) (int, bool) {
		var created struct {
			Host struct {
				AllowPreview bool `json:"allow_preview"`
			} `json:"host"`
		}
		s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": handle, "host": host}, &created)
		return s, created.Host.AllowPreview
	}
	base := func(preview bool, extra map[string]any) map[string]any {
		h := map[string]any{"host_id": hostID, "client": "claude-code", "model": "haiku", "allow_work": true, "root_dir": "/app", "allow_preview": preview}
		for k, v := range extra {
			h[k] = v
		}
		return h
	}
	if s, preview := create("no-preview", base(false, nil)); s != http.StatusCreated || preview {
		t.Fatalf("allow_preview false: %d %v", s, preview)
	}
	if s, preview := create("with-preview", base(true, nil)); s != http.StatusCreated || !preview {
		t.Fatalf("allow_preview true: %d %v", s, preview)
	}
	// A preview runs docker compose: a sandboxed agent cannot get it.
	if s, _ := create("sandboxed-preview", base(true, map[string]any{"mode": "direct", "sandbox": true})); s != http.StatusBadRequest {
		t.Fatalf("sandboxed agent with preview: %d", s)
	}
}

func TestPreviewMCPLifecycle(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	general := e.generalID()
	hostID, hostToken := e.enroll("dev-box")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Unassigned / allow_preview = false -> preview_start fails
	e.configure(agent, map[string]any{
		"host_id":       hostID,
		"client":        "claude-code",
		"model":         "haiku",
		"allow_work":    true,
		"root_dir":      "/app",
		"allow_preview": false,
	})

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "preview_start",
		Arguments: map[string]any{
			"name":         "web-app",
			"compose_path": "docker-compose.yaml",
			"channel_id":   general,
		},
	})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("expected error from preview_start when allow_preview is false")
	}

	// 2. Enable allow_preview = true
	e.configure(agent, map[string]any{
		"host_id":       hostID,
		"client":        "claude-code",
		"model":         "haiku",
		"allow_work":    true,
		"root_dir":      "/app",
		"allow_preview": true,
	})

	// Call preview_start -> succeeds
	startRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "preview_start",
		Arguments: map[string]any{
			"name":         "web-app",
			"compose_path": "docker-compose.yaml",
			"channel_id":   general,
		},
	})
	if err != nil {
		t.Fatalf("preview_start: %v", err)
	}
	if len(startRes.Content) == 0 {
		t.Fatalf("empty content in startRes")
	}

	// 3. Guard checks host config -> carries the requested preview
	var cfg struct {
		Version  string `json:"version"`
		Previews []struct {
			ID          string `json:"id"`
			AgentID     string `json:"agent_id"`
			Name        string `json:"name"`
			ComposePath string `json:"compose_path"`
			State       string `json:"state"`
		} `json:"previews"`
	}
	if s := e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg); s != http.StatusOK {
		t.Fatalf("host config: %d", s)
	}
	if len(cfg.Previews) != 1 {
		t.Fatalf("expected 1 preview in host config, got %d", len(cfg.Previews))
	}
	p := cfg.Previews[0]
	if p.Name != "web-app" || p.State != "requested" || p.ComposePath != "docker-compose.yaml" {
		t.Fatalf("unexpected preview in host config: %+v", p)
	}

	// 4. Guard reports preview running with URL
	previewURL := "http://127.0.0.1:41000"
	if s := e.do("POST", "/api/v1/host/previews/"+p.ID, hostToken,
		map[string]any{"state": "running", "url": previewURL}, nil); s != http.StatusNoContent && s != http.StatusOK {
		t.Fatalf("report preview running: %d", s)
	}

	// Drain background event processing so chat announcement is posted
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// Check that server posted the URL into the channel
	var msgs struct {
		Messages []struct {
			Body string `json:"body"`
		} `json:"messages"`
	}
	e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, nil, &msgs)
	foundAnnouncement := false
	for _, m := range msgs.Messages {
		if strings.Contains(m.Body, previewURL) && strings.Contains(m.Body, "web-app") {
			foundAnnouncement = true
			break
		}
	}
	if !foundAnnouncement {
		t.Errorf("expected announcement message containing %q in channel messages", previewURL)
	}

	// 5. Agent calls preview_stop
	stopRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "preview_stop",
		Arguments: map[string]any{
			"name": "web-app",
		},
	})
	if err != nil {
		t.Fatalf("preview_stop: %v", err)
	}
	if len(stopRes.Content) == 0 {
		t.Fatalf("empty content in stopRes")
	}

	// Check preview state via REST
	var getP previewOut
	e.do("GET", "/api/v1/previews/"+p.ID, server.DevAdminToken, nil, &getP)
	if getP.State != "stopped" {
		t.Errorf("expected state stopped, got %s", getP.State)
	}
}

func TestPreviewHumanControls(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	general := e.generalID()
	hostID, _ := e.enroll("box-2")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	e.configure(agent, map[string]any{
		"host_id":       hostID,
		"client":        "claude-code",
		"model":         "haiku",
		"allow_work":    true,
		"root_dir":      "/app",
		"allow_preview": true,
	})

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "preview_start",
		Arguments: map[string]any{
			"name":         "human-test",
			"compose_path": "docker-compose.yaml",
			"channel_id":   general,
		},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Human lists previews
	var list previewListOut
	if s := e.do("GET", "/api/v1/previews", server.DevAdminToken, nil, &list); s != http.StatusOK {
		t.Fatalf("list previews: %d", s)
	}
	if len(list.Previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(list.Previews))
	}
	pID := list.Previews[0].ID

	// Human stops preview
	var stopped previewOut
	if s := e.do("POST", "/api/v1/previews/"+pID+"/stop", server.DevAdminToken, nil, &stopped); s != http.StatusOK {
		t.Fatalf("stop preview: %d", s)
	}
	if stopped.State != "stopped" {
		t.Errorf("expected state stopped, got %s", stopped.State)
	}
}

func TestPreviewHaltAndEmergency(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	general := e.generalID()
	hostID, hostToken := e.enroll("box-3")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	e.configure(agent, map[string]any{
		"host_id":       hostID,
		"client":        "claude-code",
		"model":         "haiku",
		"allow_work":    true,
		"root_dir":      "/app",
		"allow_preview": true,
	})

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "preview_start",
		Arguments: map[string]any{
			"name":         "emergency-app",
			"compose_path": "docker-compose.yaml",
			"channel_id":   general,
		},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var cfg struct {
		Previews []struct {
			ID string `json:"id"`
		} `json:"previews"`
	}
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg)
	if len(cfg.Previews) != 1 {
		t.Fatalf("expected 1 preview in host config")
	}
	pID := cfg.Previews[0].ID

	// Activate emergency stop
	if s := e.do("POST", "/api/v1/admin/emergency-stop", server.DevAdminToken, map[string]any{"reason": "test"}, nil); s != http.StatusOK {
		t.Fatalf("emergency stop: %d", s)
	}

	// Drain events
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify preview state transitioned to stopped
	var getP previewOut
	e.do("GET", "/api/v1/previews/"+pID, server.DevAdminToken, nil, &getP)
	if getP.State != "stopped" {
		t.Errorf("expected preview to be stopped after emergency stop, got %s", getP.State)
	}
}

func TestPreviewRefusals(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	general := e.generalID()
	hostID, _ := e.enroll("box-4")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	e.configure(agent, map[string]any{
		"host_id":       hostID,
		"client":        "claude-code",
		"model":         "haiku",
		"allow_work":    true,
		"root_dir":      "/app",
		"allow_preview": true,
	})

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	cases := []struct {
		name        string
		prevName    string
		composePath string
		ttl         *int
	}{
		{name: "invalid name with space", prevName: "bad name", composePath: "compose.yaml"},
		{name: "invalid name special chars", prevName: "app@v1!", composePath: "compose.yaml"},
		{name: "absolute compose path", prevName: "ok-app", composePath: "/etc/compose.yaml"},
		{name: "relative traversal compose path", prevName: "ok-app", composePath: "../compose.yaml"},
		{name: "non-yaml compose path", prevName: "ok-app", composePath: "compose.sh"},
		{name: "ttl too high", prevName: "ok-app", composePath: "compose.yaml", ttl: ptr(1441)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{
				"name":         tc.prevName,
				"compose_path": tc.composePath,
				"channel_id":   general,
			}
			if tc.ttl != nil {
				args["ttl_minutes"] = *tc.ttl
			}
			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "preview_start",
				Arguments: args,
			})
			if err == nil && (res == nil || !res.IsError) {
				t.Fatalf("expected error for case %q, got success", tc.name)
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}

// A failed preview's reason (compose output, paths) is for humans only: the
// chat gets a short note, never the detail.
func TestPreviewFailureDetailStaysOutOfChat(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	general := e.generalID()
	hostID, hostToken := e.enroll("dev-box")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	e.configure(agent, map[string]any{"host_id": hostID, "client": "claude-code", "root_dir": "/app", "allow_preview": true})
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "preview_start",
		Arguments: map[string]any{"name": "broken", "compose_path": "docker-compose.yaml", "channel_id": general}}); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Previews []struct {
			ID string `json:"id"`
		} `json:"previews"`
	}
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg)
	if len(cfg.Previews) != 1 {
		t.Fatalf("previews: %+v", cfg.Previews)
	}
	detail := "docker compose up: pull access denied for registry.internal/secret-image"
	if s := e.do("POST", "/api/v1/host/previews/"+cfg.Previews[0].ID, hostToken,
		map[string]any{"state": "failed", "error": detail}, nil); s != http.StatusNoContent && s != http.StatusOK {
		t.Fatalf("report failed: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var msgs struct {
		Messages []struct {
			Body string `json:"body"`
		} `json:"messages"`
	}
	e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, nil, &msgs)
	noted := false
	for _, m := range msgs.Messages {
		if strings.Contains(m.Body, "secret-image") {
			t.Fatalf("failure detail in chat: %q", m.Body)
		}
		noted = noted || strings.Contains(m.Body, "broken")
	}
	if !noted {
		t.Errorf("no failure note in chat: %+v", msgs.Messages)
	}
}
