package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

func TestE2E_SandboxPlannerTools(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	adminToken := server.DevAdminToken

	// 1. Admin creates a task in planner
	var createdTask map[string]any
	if status := e.do("POST", "/api/v1/planner/tasks", adminToken, map[string]any{
		"title":       "Kenyér és tej vásárlása",
		"description": "2 liter tej és félbarna kenyér",
	}, &createdTask); status != http.StatusCreated {
		t.Fatalf("create task: %d", status)
	}

	// 2. Register host
	base := t.TempDir()
	root := filepath.Join(base, "projects")
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0o700); err != nil {
		t.Fatal(err)
	}

	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", adminToken, nil, &pc)
	tokenFile := filepath.Join(base, "host.token")
	if err := guard.Enroll(ctx, e.srv.URL, pc.Code, "sandboxed-host", tokenFile); err != nil {
		t.Fatal(err)
	}

	var hosts struct{ Hosts []struct{ ID string } }
	e.do("GET", "/api/v1/hosts", adminToken, nil, &hosts)
	hostID := hosts.Hosts[0].ID

	// 3. Assign agent to host with sandbox: true and allow_work: true
	agentID := e.agentID()

	e.configure(agentID, map[string]any{
		"host_id":    hostID,
		"client":     "claude-code",
		"model":      "haiku",
		"allow_work": true,
		"mode":       "compact",
		"sandbox":    true,
	}, map[string]any{"path": app, "mode": "write"})

	var mu sync.Mutex
	var capturedReq guard.WorkRequest
	var tasksFromMCP string

	ad := &scriptedAdapter{
		decide: func(ctx context.Context, req guard.DecideRequest) (guard.Decision, error) {
			return guard.Decision{
				Actions: []guard.Action{
					{Ref: "1", Kind: guard.ActWork, Task: "check planner tasks"},
				},
			}, nil
		},
		work: func(ctx context.Context, req guard.WorkRequest) (string, error) {
			mu.Lock()
			capturedReq = req
			mu.Unlock()

			if req.Wrap != nil {
				if _, _, _, err := req.Wrap("claude", []string{"-p", "hello"}); err != nil {
					return "wrap error: " + err.Error(), nil
				}
			}

			// Check MCPConfigPath
			if req.MCPConfigPath == "" {
				return "error: empty MCPConfigPath", nil
			}
			raw, err := os.ReadFile(req.MCPConfigPath)
			if err != nil {
				return "error reading MCP config: " + err.Error(), nil
			}
			var cfg struct {
				MCPServers map[string]struct {
					Type string `json:"type"`
					URL  string `json:"url"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return "error parsing MCP config: " + err.Error(), nil
			}
			srv, ok := cfg.MCPServers["colony-planner"]
			if !ok {
				return "error: colony-planner not in mcpServers", nil
			}
			if srv.URL != "http://127.0.0.1:3130/mcp" {
				return "unexpected colony-planner URL: " + srv.URL, nil
			}

			// In sandbox mode, guard starts PlannerServer on agentDir/planner.sock
			// agentDir is parent of agentDir.config
			cfgDir := filepath.Dir(filepath.Dir(req.MCPConfigPath)) // <sbDir>/<agentID>.config/claude -> <sbDir>/<agentID>.config
			agentDir := strings.TrimSuffix(cfgDir, ".config")
			sockPath := filepath.Join(agentDir, "planner.sock")

			httpClient := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						return net.Dial("unix", sockPath)
					},
				},
			}
			mcpCli := mcp.NewClient(&mcp.Implementation{Name: "sandboxed-work-runner", Version: "1.0"}, nil)
			tr := &mcp.StreamableClientTransport{
				Endpoint:   "http://localhost/mcp",
				HTTPClient: httpClient,
			}
			sess, err := mcpCli.Connect(ctx, tr, nil)
			if err != nil {
				return "mcp connect error: " + err.Error(), nil
			}
			defer func() { _ = sess.Close() }()

			res, err := sess.CallTool(ctx, &mcp.CallToolParams{
				Name:      "planner_read",
				Arguments: map[string]any{},
			})
			if err != nil {
				return "call planner_read error: " + err.Error(), nil
			}
			var textContent string
			for _, c := range res.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					textContent += tc.Text
				}
			}
			mu.Lock()
			tasksFromMCP = textContent
			mu.Unlock()

			return "Feladatok lekérve: " + textContent, nil
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
			},
		},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(guard.AgentConfig) guard.Adapter { return ad },
	}
	if err := sup.Cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = sup.RunHost(ctx) }()

	eventually(t, "host report with roots", func() bool {
		var list struct {
			Hosts []struct {
				Report struct {
					Roots []string
				} `json:"report"`
			} `json:"hosts"`
		}
		e.do("GET", "/api/v1/hosts", adminToken, nil, &list)
		return len(list.Hosts) > 0 && len(list.Hosts[0].Report.Roots) == 1
	})
	time.Sleep(50 * time.Millisecond)

	general := e.generalID()
	q, _ := e.send(adminToken, general, map[string]any{"body": "@fake-agent mik a feladataim?"})

	eventually(t, "work result with tasks from planner", func() bool {
		msgs := e.messages(general)
		for _, m := range msgs {
			if m.ThreadRootID == q.ID && strings.Contains(m.Body, "Kenyér és tej vásárlása") {
				return true
			}
		}
		return false
	})

	mu.Lock()
	req := capturedReq
	tasks := tasksFromMCP
	mu.Unlock()

	if !req.AllowPlanner {
		t.Errorf("expected WorkRequest.AllowPlanner to be true")
	}
	if !strings.Contains(req.Prompt, "planner_read") {
		t.Errorf("expected prompt to mention planner_read:\n%s", req.Prompt)
	}
	if !strings.Contains(tasks, "Kenyér és tej vásárlása") {
		t.Errorf("expected tasks output to contain task title, got:\n%s", tasks)
	}
}
