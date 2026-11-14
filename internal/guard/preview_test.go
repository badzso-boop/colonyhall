// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestValidateComposeYAML_Refusals(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "root")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}

	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}

	symlinkToOutside := filepath.Join(rootDir, "escape_link")
	if err := os.Symlink(outsideDir, symlinkToOutside); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		yaml       string
		wantErrSub string
	}{
		{
			name:       "no services",
			yaml:       `version: "3.8"`,
			wantErrSub: "has no services",
		},
		{
			name: "privileged true",
			yaml: `
services:
  web:
    image: nginx
    privileged: true
`,
			wantErrSub: "privileged mode",
		},
		{
			name: "cap_add",
			yaml: `
services:
  web:
    image: nginx
    cap_add:
      - SYS_ADMIN
`,
			wantErrSub: "cap_add",
		},
		{
			name: "devices",
			yaml: `
services:
  web:
    image: nginx
    devices:
      - "/dev/sda:/dev/sda"
`,
			wantErrSub: "devices",
		},
		{
			name: "network_mode host",
			yaml: `
services:
  web:
    image: nginx
    network_mode: host
`,
			wantErrSub: "network_mode: host",
		},
		{
			name: "pid host",
			yaml: `
services:
  web:
    image: nginx
    pid: host
`,
			wantErrSub: "pid: host",
		},
		{
			name: "ipc host",
			yaml: `
services:
  web:
    image: nginx
    ipc: host
`,
			wantErrSub: "ipc: host",
		},
		{
			name: "userns_mode",
			yaml: `
services:
  web:
    image: nginx
    userns_mode: "host"
`,
			wantErrSub: "userns_mode",
		},
		{
			name: "security_opt",
			yaml: `
services:
  web:
    image: nginx
    security_opt:
      - seccomp:unconfined
`,
			wantErrSub: "security_opt",
		},
		{
			name: "docker socket volume string",
			yaml: `
services:
  web:
    image: nginx
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
`,
			wantErrSub: "docker socket",
		},
		{
			name: "docker socket volume object",
			yaml: `
services:
  web:
    image: nginx
    volumes:
      - type: bind
        source: /var/run/docker.sock
        target: /var/run/docker.sock
`,
			wantErrSub: "docker socket",
		},
		{
			name: "bind mount outside root string",
			yaml: fmt.Sprintf(`
services:
  web:
    image: nginx
    volumes:
      - %s:/data
`, outsideDir),
			wantErrSub: "outside root directory",
		},
		{
			name: "bind mount relative escape",
			yaml: `
services:
  web:
    image: nginx
    volumes:
      - ../outside:/data
`,
			wantErrSub: "outside root directory",
		},
		{
			name: "bind mount symlink escape",
			yaml: fmt.Sprintf(`
services:
  web:
    image: nginx
    volumes:
      - %s:/data
`, symlinkToOutside),
			wantErrSub: "outside root directory",
		},
		{
			name: "bind mount object outside root",
			yaml: fmt.Sprintf(`
services:
  web:
    image: nginx
    volumes:
      - type: bind
        source: %s
        target: /data
`, outsideDir),
			wantErrSub: "outside root directory",
		},
		{
			name: "build context string outside root",
			yaml: `
services:
  web:
    build: ../outside
`,
			wantErrSub: "outside root directory",
		},
		{
			name: "build context object outside root",
			yaml: `
services:
  web:
    build:
      context: ../outside
`,
			wantErrSub: "outside root directory",
		},
		{
			name: "build context symlink escape",
			yaml: fmt.Sprintf(`
services:
  web:
    build: %s
`, symlinkToOutside),
			wantErrSub: "outside root directory",
		},
		// Paths compose resolves against the home folder or the environment.
		{name: "home bind", yaml: "services:\n  web:\n    image: nginx\n    volumes: [\"~/.ssh:/keys\"]\n", wantErrSub: "uses ~ or a variable"},
		{name: "env var bind", yaml: "services:\n  web:\n    image: nginx\n    volumes: [\"${HOME}/.ssh:/keys\"]\n", wantErrSub: "uses ~ or a variable"},
		{name: "env var build", yaml: "services:\n  web:\n    build: $HOME/x\n", wantErrSub: "not a folder inside the root"},
		{name: "include", yaml: "include: [../other.yaml]\nservices:\n  web:\n    image: nginx\n", wantErrSub: "include"},
		{name: "extends", yaml: "services:\n  web:\n    image: nginx\n    extends: {file: /etc/x.yaml, service: a}\n", wantErrSub: "extends"},
		{name: "volume driver_opts", yaml: "services:\n  web:\n    image: nginx\n    volumes: [\"data:/d\"]\nvolumes:\n  data:\n    driver_opts: {type: none, o: bind, device: /}\n", wantErrSub: "driver_opts"},
		{name: "secrets file", yaml: "services:\n  web:\n    image: nginx\nsecrets:\n  k:\n    file: /home/u/.ssh/id_ed25519\n", wantErrSub: "secrets"},
		{name: "env_file outside", yaml: "services:\n  web:\n    image: nginx\n    env_file: ../outside/.env\n", wantErrSub: "env_file"},
		{name: "env_file home", yaml: "services:\n  web:\n    image: nginx\n    env_file: [\"~/.aws/credentials\"]\n", wantErrSub: "env_file"},
	}

	composePath := filepath.Join(rootDir, "docker-compose.yaml")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateComposeYAML([]byte(tc.yaml), composePath, rootDir)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErrSub, err.Error())
			}
		})
	}
}

func TestValidateComposeFile_OutsideRoot(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "root")
	outsideDir := filepath.Join(tmpDir, "outside")
	_ = os.MkdirAll(rootDir, 0755)
	_ = os.MkdirAll(outsideDir, 0755)

	outsideCompose := filepath.Join(outsideDir, "compose.yaml")
	_ = os.WriteFile(outsideCompose, []byte("services:\n  app:\n    image: nginx\n"), 0644)

	// Direct outside path
	_, err := ValidateComposeFile(outsideCompose, rootDir)
	if err == nil || !strings.Contains(err.Error(), "outside root directory") {
		t.Fatalf("expected outside root error, got %v", err)
	}

	// Symlink inside pointing outside
	linkPath := filepath.Join(rootDir, "compose.yaml")
	_ = os.Symlink(outsideCompose, linkPath)
	_, err = ValidateComposeFile(linkPath, rootDir)
	if err == nil || !strings.Contains(err.Error(), "outside root directory") {
		t.Fatalf("expected outside root error for symlinked compose, got %v", err)
	}
}

func TestValidateComposeYAML_ValidAndResourceLimits(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "root")
	_ = os.MkdirAll(rootDir, 0755)

	composePath := filepath.Join(rootDir, "docker-compose.yaml")
	raw := `
services:
  frontend:
    image: node:alpine
    ports:
      - "3000:3000"
    expose:
      - 8080
  backend:
    image: golang:alpine
    deploy:
      resources:
        limits:
          cpus: "2.0"
          memory: "1024M"
    expose:
      - "8081"
`
	info, err := ValidateComposeYAML([]byte(raw), composePath, rootDir)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if len(info.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(info.Services))
	}
	if !info.NeedsDefaultLimits["frontend"] {
		t.Errorf("expected frontend to need default limits")
	}
	if info.NeedsDefaultLimits["backend"] {
		t.Errorf("expected backend to not need default limits")
	}

	// Ports: frontend has 3000 and 8080, backend has 8081
	foundPorts := map[string][]int{}
	for _, p := range info.Ports {
		foundPorts[p.Service] = append(foundPorts[p.Service], p.ContainerPort)
	}
	if len(foundPorts["frontend"]) != 2 {
		t.Errorf("frontend ports: got %v, expected 2", foundPorts["frontend"])
	}
	if len(foundPorts["backend"]) != 1 || foundPorts["backend"][0] != 8081 {
		t.Errorf("backend ports: got %v, expected [8081]", foundPorts["backend"])
	}
}

func TestGenerateOverrideContent(t *testing.T) {
	ports := []ComposeServicePort{
		{Service: "web", ContainerPort: 80, HostPort: 41000},
		{Service: "web", ContainerPort: 443, HostPort: 41001},
	}
	defaultLimits := map[string]bool{"web": true, "worker": true}
	servicesWithPorts := map[string]bool{"web": true, "worker": true}

	bytes, err := GenerateOverrideContent("127.0.0.1", ports, defaultLimits, servicesWithPorts)
	if err != nil {
		t.Fatalf("generate override: %v", err)
	}

	outStr := string(bytes)
	if !strings.Contains(outStr, "!override") {
		t.Errorf("expected output to contain !override tag, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "127.0.0.1:41000:80") {
		t.Errorf("expected output to contain host port mapping, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "cpus: 1.0") && !strings.Contains(outStr, "cpus: \"1.0\"") && !strings.Contains(outStr, "cpus: '1.0'") {
		t.Errorf("expected output to contain cpus limit, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "memory: 512M") {
		t.Errorf("expected output to contain memory limit, got:\n%s", outStr)
	}

	// Verify it parses as valid YAML
	var parsed any
	if err := yaml.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("override YAML failed to parse: %v", err)
	}
}

func TestPortPool_AllocationAndFree(t *testing.T) {
	pool, err := NewPortPool("41000-41099") // 100 ports -> 10 slots of 10 ports
	if err != nil {
		t.Fatal(err)
	}

	agent1 := "agent-uuid-1"
	agent2 := "agent-uuid-2"

	// Allocate 3 ports for agent1
	ports1, err := pool.Allocate(agent1, "prev-1", 3)
	if err != nil {
		t.Fatalf("allocate agent1: %v", err)
	}
	if len(ports1) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports1))
	}
	for _, p := range ports1 {
		if !pool.IsUsed(p) {
			t.Errorf("port %d should be marked used", p)
		}
	}

	// Allocate 4 more ports for agent1 on another preview
	ports2, err := pool.Allocate(agent1, "prev-2", 4)
	if err != nil {
		t.Fatalf("allocate agent1 second preview: %v", err)
	}
	for _, p := range ports2 {
		for _, prev := range ports1 {
			if p == prev {
				t.Fatalf("port %d re-allocated while still in use", p)
			}
		}
	}

	// Allocate for agent2
	portsAgent2, err := pool.Allocate(agent2, "prev-3", 2)
	if err != nil {
		t.Fatalf("allocate agent2: %v", err)
	}
	if len(portsAgent2) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(portsAgent2))
	}

	// Free prev-1
	pool.Free("prev-1")
	for _, p := range ports1 {
		if pool.IsUsed(p) {
			t.Errorf("port %d should be freed", p)
		}
	}
	// prev-2 still used
	for _, p := range ports2 {
		if !pool.IsUsed(p) {
			t.Errorf("port %d should still be used", p)
		}
	}

	// Agent1 requests 5 ports (now possible because 3 were freed: 10 - 4 = 6 available)
	ports3, err := pool.Allocate(agent1, "prev-4", 5)
	if err != nil {
		t.Fatalf("allocate agent1 after free: %v", err)
	}
	if len(ports3) != 5 {
		t.Fatalf("expected 5 ports, got %d", len(ports3))
	}

	// Exceeding agent quota (only 1 port remains, requesting 2 should fail)
	_, err = pool.Allocate(agent1, "prev-overflow", 2)
	if err == nil {
		t.Fatalf("expected quota overflow error, got nil")
	}
}

func TestPreviewSupervisor_Reconcile(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "work")
	_ = os.MkdirAll(rootDir, 0755)

	// Create valid compose file in rootDir
	composeContent := `
services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
`
	_ = os.WriteFile(filepath.Join(rootDir, "docker-compose.yaml"), []byte(composeContent), 0644)

	var reportedReports []map[string]any
	var reportsMu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/host/previews/") {
			var body map[string]any
			_ = yaml.NewDecoder(r.Body).Decode(&body)
			reportsMu.Lock()
			body["path"] = r.URL.Path
			reportedReports = append(reportedReports, body)
			reportsMu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	fakeDocker := &FakeDockerRunner{}

	sup, err := NewPreviewSupervisor(client, fakeDocker, []string{tmpDir}, "127.0.0.1", "41000-41099", nil)
	if err != nil {
		t.Fatal(err)
	}

	agentID := "00000000-0000-0000-0000-000000000001"
	agents := []HostAgent{
		{
			AgentID:      agentID,
			Handle:       "alpha",
			RootDir:      rootDir,
			AllowPreview: true,
			Mode:         "direct",
		},
	}

	previewID := "prev-123"
	previews := []HostPreview{
		{
			ID:          previewID,
			AgentID:     agentID,
			Name:        "web-test",
			ComposePath: "docker-compose.yaml",
			State:       "requested",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}

	// 1. Reconcile "requested" -> should run Up and report "running"
	sup.Reconcile(context.Background(), previews, agents, map[string]bool{})

	if len(fakeDocker.UpCalls) != 1 {
		t.Fatalf("expected 1 Up call, got %d: %v", len(fakeDocker.UpCalls), fakeDocker.UpCalls)
	}
	expectedProject := "colony-alpha-web-test"
	if !strings.HasPrefix(fakeDocker.UpCalls[0], expectedProject+"|") {
		t.Errorf("expected Up project %q, got call: %s", expectedProject, fakeDocker.UpCalls[0])
	}

	reportsMu.Lock()
	if len(reportedReports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reportedReports))
	}
	if reportedReports[0]["state"] != "running" {
		t.Errorf("expected report state running, got %v", reportedReports[0]["state"])
	}
	urlVal, ok := reportedReports[0]["url"].(string)
	if !ok || !strings.HasPrefix(urlVal, "http://127.0.0.1:410") {
		t.Errorf("expected valid preview URL, got %v", reportedReports[0]["url"])
	}
	reportsMu.Unlock()

	// 2. Now server reports it as "running"
	previews[0].State = "running"
	previews[0].URL = urlVal

	// Reconcile with expiry in the past -> should Down and report "stopped"
	previews[0].ExpiresAt = time.Now().Add(-time.Minute)
	sup.Reconcile(context.Background(), previews, agents, map[string]bool{})

	if len(fakeDocker.DownCalls) != 1 {
		t.Fatalf("expected 1 Down call, got %d: %v", len(fakeDocker.DownCalls), fakeDocker.DownCalls)
	}

	reportsMu.Lock()
	if len(reportedReports) != 2 || reportedReports[1]["state"] != "stopped" {
		t.Errorf("expected second report state stopped, got %v", reportedReports)
	}
	reportsMu.Unlock()
}

func TestPreviewSupervisor_HaltAndEmergency(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "work")
	_ = os.MkdirAll(rootDir, 0755)

	_ = os.WriteFile(filepath.Join(rootDir, "docker-compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0644)

	var reportedReports []map[string]any
	var reportsMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = yaml.NewDecoder(r.Body).Decode(&body)
		reportsMu.Lock()
		reportedReports = append(reportedReports, body)
		reportsMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	fakeDocker := &FakeDockerRunner{}
	sup, _ := NewPreviewSupervisor(client, fakeDocker, []string{tmpDir}, "127.0.0.1", "41000-41099", nil)

	agentID := "00000000-0000-0000-0000-000000000002"
	agents := []HostAgent{
		{
			AgentID:      agentID,
			Handle:       "beta",
			RootDir:      rootDir,
			AllowPreview: true,
			Mode:         "direct",
		},
	}

	// 1. Requested while agent halted -> reports failed immediately
	sup.Reconcile(context.Background(), []HostPreview{
		{
			ID:          "p-halted-req",
			AgentID:     agentID,
			Name:        "app",
			ComposePath: "docker-compose.yaml",
			State:       "requested",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}, agents, map[string]bool{agentID: true})

	if len(fakeDocker.UpCalls) != 0 {
		t.Errorf("Up should not have been called when halted")
	}
	reportsMu.Lock()
	if len(reportedReports) != 1 || reportedReports[0]["state"] != "failed" {
		t.Errorf("expected failed report, got %v", reportedReports)
	}
	reportsMu.Unlock()

	// 2. Running preview stopped when agent halts
	sup.Reconcile(context.Background(), []HostPreview{
		{
			ID:          "p-run",
			AgentID:     agentID,
			Name:        "app-run",
			ComposePath: "docker-compose.yaml",
			State:       "requested",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}, agents, map[string]bool{})

	if len(fakeDocker.UpCalls) != 1 {
		t.Fatalf("expected 1 Up call, got %d", len(fakeDocker.UpCalls))
	}

	// Next reconcile tick: agent is halted
	sup.Reconcile(context.Background(), []HostPreview{
		{
			ID:          "p-run",
			AgentID:     agentID,
			Name:        "app-run",
			ComposePath: "docker-compose.yaml",
			State:       "running",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}, agents, map[string]bool{agentID: true})

	if len(fakeDocker.DownCalls) != 1 {
		t.Fatalf("expected 1 Down call when agent halted, got %d", len(fakeDocker.DownCalls))
	}
}

func TestPreviewSupervisor_RestartRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "work")
	_ = os.MkdirAll(rootDir, 0755)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	fakeDocker := &FakeDockerRunner{}
	sup, _ := NewPreviewSupervisor(client, fakeDocker, []string{tmpDir}, "127.0.0.1", "41000-41099", nil)

	agentID := "00000000-0000-0000-0000-000000000003"
	agents := []HostAgent{
		{
			AgentID:      agentID,
			Handle:       "gamma",
			RootDir:      rootDir,
			AllowPreview: true,
			Mode:         "direct",
		},
	}

	previewID := "p-recovered"
	// Simulating that the server reports this preview as already running (with URL)
	// when the supervisor starts up
	runningPreviews := []HostPreview{
		{
			ID:          previewID,
			AgentID:     agentID,
			Name:        "app",
			ComposePath: "docker-compose.yaml",
			State:       "running",
			URL:         "http://127.0.0.1:41020",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}

	sup.Reconcile(context.Background(), runningPreviews, agents, map[string]bool{})

	// Should not have called Up or Down yet
	if len(fakeDocker.UpCalls) != 0 || len(fakeDocker.DownCalls) != 0 {
		t.Errorf("expected no docker calls on recovery, got Up=%d Down=%d", len(fakeDocker.UpCalls), len(fakeDocker.DownCalls))
	}
	// Port 41020 should be marked as used in PortPool
	if !sup.Ports.IsUsed(41020) {
		t.Errorf("port 41020 should be tracked as used after recovery")
	}

	// Server reports it as stopped
	stoppedPreviews := []HostPreview{
		{
			ID:          previewID,
			AgentID:     agentID,
			Name:        "app",
			ComposePath: "docker-compose.yaml",
			State:       "stopped",
		},
	}
	sup.Reconcile(context.Background(), stoppedPreviews, agents, map[string]bool{})

	if len(fakeDocker.DownCalls) != 1 {
		t.Fatalf("expected 1 Down call when server reports stopped, got %d", len(fakeDocker.DownCalls))
	}
	if sup.Ports.IsUsed(41020) {
		t.Errorf("port 41020 should be freed after stop")
	}
}

// A preview runs docker compose, so only a full-access agent whose root is
// inside this machine's roots may start one, whatever the server says.
func TestPreviewSupervisor_RefusesSandboxedAndOutsideRoots(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())
	inside := filepath.Join(tmpDir, "roots", "app")
	outside := filepath.Join(tmpDir, "elsewhere")
	for _, d := range []string{inside, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(d, "docker-compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0o644)
	}
	var mu sync.Mutex
	var reasons []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = yaml.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		reasons = append(reasons, fmt.Sprint(body["error"]))
		mu.Unlock()
	}))
	defer srv.Close()
	fake := &FakeDockerRunner{}
	sup, err := NewPreviewSupervisor(&Client{BaseURL: srv.URL, HTTP: srv.Client()}, fake, []string{filepath.Join(tmpDir, "roots")}, "127.0.0.1", "41000-41099", nil)
	if err != nil {
		t.Fatal(err)
	}
	agents := []HostAgent{
		{AgentID: "a1", Handle: "boxed", RootDir: inside, AllowPreview: true, Mode: "compact", Sandbox: true},
		{AgentID: "a2", Handle: "far", RootDir: outside, AllowPreview: true, Mode: "direct"},
	}
	previews := []HostPreview{
		{ID: "p1", AgentID: "a1", Name: "x", ComposePath: "docker-compose.yaml", State: "requested", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "p2", AgentID: "a2", Name: "y", ComposePath: "docker-compose.yaml", State: "requested", ExpiresAt: time.Now().Add(time.Hour)},
	}
	sup.Reconcile(context.Background(), previews, agents, map[string]bool{})
	if len(fake.UpCalls) != 0 {
		t.Fatalf("docker compose up ran: %v", fake.UpCalls)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reasons) != 2 || !strings.Contains(reasons[0], "full-access") || !strings.Contains(reasons[1], "outside the root folders") {
		t.Fatalf("reasons: %v", reasons)
	}
}

// Round 40: a preview runs only for an agent this machine accepted. The
// server's list also holds agents the guard refused (here: full-access
// with allow_unsandboxed off); their previews must not start.
func TestPreviewsOnlyForAcceptedAgents(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(tmpDir, "roots", "app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "docker-compose.yaml"), []byte("services:\n  app:\n    image: nginx\n"), 0o644)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	fake := &FakeDockerRunner{}
	ps, err := NewPreviewSupervisor(&Client{BaseURL: srv.URL, HTTP: srv.Client()}, fake, []string{filepath.Join(tmpDir, "roots")}, "127.0.0.1", "41100-41199", nil)
	if err != nil {
		t.Fatal(err)
	}
	sup := &Supervisor{
		Cfg:      Config{Server: srv.URL, Roots: []string{filepath.Join(tmpDir, "roots")}, SandboxDir: filepath.Join(tmpDir, "sb"), AllowUnsandboxed: false},
		host:     &Client{BaseURL: srv.URL, Token: "host-tok", HTTP: srv.Client()},
		roots:    []string{filepath.Join(tmpDir, "roots")},
		home:     tmpDir,
		agents:   map[string]*managed{},
		previews: ps,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(AgentConfig) Adapter {
			return &ClaudeCode{}
		},
	}
	sup.reconcile(context.Background(), HostConfig{
		Version: "v1",
		Agents:  []HostAgent{{AgentID: "a1", Handle: "full", Client: "claude-code", Mode: "direct", RootDir: root, AllowPreview: true, PermissionMode: "auto"}},
		Previews: []HostPreview{{ID: "p1", AgentID: "a1", Name: "x", ComposePath: "docker-compose.yaml", State: "requested",
			ExpiresAt: time.Now().Add(time.Hour)}},
	})
	if len(fake.UpCalls) != 0 {
		t.Fatalf("a refused agent's preview ran: %v", fake.UpCalls)
	}
}

// Round 40: a recovered preview's override never comes from the shared
// /tmp/colony-previews, which any local user can write.
func TestRecoveredOverrideIgnoresSharedTmp(t *testing.T) {
	ps := &PreviewSupervisor{Dir: t.TempDir()}
	shared := filepath.Join(os.TempDir(), "colony-previews", "p-legacy-test")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Skipf("cannot create the shared folder: %v", err)
	}
	defer func() { _ = os.RemoveAll(shared) }()
	if err := os.WriteFile(filepath.Join(shared, "docker-compose.override.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ps.recoveredOverride("p-legacy-test"); !strings.HasPrefix(got, ps.Dir) {
		t.Fatalf("the override came from %s", got)
	}
}
