// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestCheckFileFetch_Scope(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "roots", "main")
	grantDir := filepath.Join(root, "project")
	outsideDir := filepath.Join(tmp, "other")

	_ = os.MkdirAll(grantDir, 0o755)
	_ = os.MkdirAll(outsideDir, 0o755)

	validFile := filepath.Join(grantDir, "main.go")
	_ = os.WriteFile(validFile, []byte("package main"), 0o644)

	outsideFile := filepath.Join(outsideDir, "secret.txt")
	_ = os.WriteFile(outsideFile, []byte("secret"), 0o644)

	roots := []string{root}
	grants := []Grant{
		{Path: grantDir, Mode: "read"},
	}

	// 1. Sandboxed agent
	// File inside grant -> OK
	realPath, _, isImg, errCode := CheckFileFetch(validFile, roots, grants, true, tmp, nil, "")
	if errCode != "" || realPath != validFile || isImg {
		t.Fatalf("sandboxed inside grant: got real=%s err=%s, want no err", realPath, errCode)
	}

	// File outside grant -> denied
	_, _, _, errCode = CheckFileFetch(outsideFile, roots, grants, true, tmp, nil, "")
	if errCode != "denied" {
		t.Fatalf("sandboxed outside grant: got err=%s, want denied", errCode)
	}

	// Write grant also allows reading
	writeGrants := []Grant{
		{Path: grantDir, Mode: "write", Write: true},
	}
	realPath, _, _, errCode = CheckFileFetch(validFile, roots, writeGrants, true, tmp, nil, "")
	if errCode != "" || realPath != validFile {
		t.Fatalf("sandboxed write grant: got real=%s err=%s, want no err", realPath, errCode)
	}

	// 2. Full-access agent
	// File inside roots -> OK (even if outside grants)
	fileInRoot := filepath.Join(root, "config.json")
	_ = os.WriteFile(fileInRoot, []byte("{}"), 0o644)
	realPath, _, _, errCode = CheckFileFetch(fileInRoot, roots, nil, false, tmp, nil, "")
	if errCode != "" || realPath != fileInRoot {
		t.Fatalf("full-access inside roots: got real=%s err=%s, want no err", realPath, errCode)
	}

	// File outside roots -> denied
	_, _, _, errCode = CheckFileFetch(outsideFile, roots, nil, false, tmp, nil, "")
	if errCode != "denied" {
		t.Fatalf("full-access outside roots: got err=%s, want denied", errCode)
	}
}

func TestCheckFileFetch_DenyRules(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	root := filepath.Join(tmp, "root")
	exchange := filepath.Join(tmp, "exchange")

	_ = os.MkdirAll(home, 0o755)
	_ = os.MkdirAll(root, 0o755)
	_ = os.MkdirAll(exchange, 0o755)

	hostTokenFile := filepath.Join(tmp, "host.token")
	_ = os.WriteFile(hostTokenFile, []byte("host-secret"), 0o600)
	claudeTokenFile := filepath.Join(tmp, "claude.token")
	_ = os.WriteFile(claudeTokenFile, []byte("claude-secret"), 0o600)

	roots := []string{root, home}
	grants := []Grant{{Path: root, Mode: "read"}, {Path: home, Mode: "read"}}

	// Denied home folders
	sshKey := filepath.Join(home, ".ssh", "id_rsa")
	_ = os.MkdirAll(filepath.Dir(sshKey), 0o700)
	_ = os.WriteFile(sshKey, []byte("key"), 0o600)

	_, _, _, errCode := CheckFileFetch(sshKey, roots, grants, false, home, []string{hostTokenFile, claudeTokenFile}, exchange)
	if errCode != "denied" {
		t.Fatalf("denied home .ssh: got %s, want denied", errCode)
	}

	// ~/.config/colonyhall
	colonyCfg := filepath.Join(home, ".config", "colonyhall", "guard.yaml")
	_ = os.MkdirAll(filepath.Dir(colonyCfg), 0o700)
	_ = os.WriteFile(colonyCfg, []byte("server: http://..."), 0o600)
	_, _, _, errCode = CheckFileFetch(colonyCfg, roots, grants, false, home, []string{hostTokenFile, claudeTokenFile}, exchange)
	if errCode != "denied" {
		t.Fatalf("denied ~/.config/colonyhall: got %s, want denied", errCode)
	}

	// Token files
	_, _, _, errCode = CheckFileFetch(hostTokenFile, roots, grants, false, home, []string{hostTokenFile, claudeTokenFile}, exchange)
	if errCode != "denied" {
		t.Fatalf("denied hostTokenFile: got %s, want denied", errCode)
	}
	_, _, _, errCode = CheckFileFetch(claudeTokenFile, roots, grants, false, home, []string{hostTokenFile, claudeTokenFile}, exchange)
	if errCode != "denied" {
		t.Fatalf("denied claudeTokenFile: got %s, want denied", errCode)
	}

	// Exchange dir
	exFile := filepath.Join(exchange, "shared.txt")
	_ = os.WriteFile(exFile, []byte("shared"), 0o644)
	_, _, _, errCode = CheckFileFetch(exFile, roots, grants, false, home, []string{hostTokenFile, claudeTokenFile}, exchange)
	if errCode != "denied" {
		t.Fatalf("denied exchangeDir: got %s, want denied", errCode)
	}

	// Secrets inside granted root
	secretFiles := []string{
		".env",
		".env.local",
		".env.production",
		"server.pem",
		"private.key",
		"id_rsa",
		"id_rsa.pub",
		"id_ed25519",
		"bundle.p12",
		"credentials.json",
		"credentials-prod",
		"auth.token",
		".netrc",
		".git-credentials",
	}
	for _, name := range secretFiles {
		secPath := filepath.Join(root, name)
		_ = os.WriteFile(secPath, []byte("secret"), 0o600)
		_, _, _, errCode = CheckFileFetch(secPath, roots, grants, false, home, []string{hostTokenFile, claudeTokenFile}, exchange)
		if errCode != "denied" {
			t.Errorf("secret %q: got %s, want denied", name, errCode)
		}
	}
}

func TestCheckFileFetch_SymlinksAndTypes(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	outside := filepath.Join(tmp, "outside")
	_ = os.MkdirAll(root, 0o755)
	_ = os.MkdirAll(outside, 0o755)

	target := filepath.Join(outside, "target.txt")
	_ = os.WriteFile(target, []byte("target"), 0o644)

	symlinkToOutside := filepath.Join(root, "outside_link.txt")
	_ = os.Symlink(target, symlinkToOutside)

	roots := []string{root}
	grants := []Grant{{Path: root, Mode: "read"}}

	// Symlink pointing outside scope -> denied
	_, _, _, errCode := CheckFileFetch(symlinkToOutside, roots, grants, true, tmp, nil, "")
	if errCode != "denied" {
		t.Fatalf("symlink to outside: got %s, want denied", errCode)
	}

	// Broken symlink -> not_found
	brokenLink := filepath.Join(root, "broken_link.txt")
	_ = os.Symlink(filepath.Join(root, "non_existent.txt"), brokenLink)
	_, _, _, errCode = CheckFileFetch(brokenLink, roots, grants, true, tmp, nil, "")
	if errCode != "not_found" {
		t.Fatalf("broken symlink: got %s, want not_found", errCode)
	}

	// Directory -> not_a_file
	subDir := filepath.Join(root, "subdir")
	_ = os.MkdirAll(subDir, 0o755)
	_, _, _, errCode = CheckFileFetch(subDir, roots, grants, true, tmp, nil, "")
	if errCode != "not_a_file" {
		t.Fatalf("directory: got %s, want not_a_file", errCode)
	}

	// Missing file -> not_found
	_, _, _, errCode = CheckFileFetch(filepath.Join(root, "does_not_exist.txt"), roots, grants, true, tmp, nil, "")
	if errCode != "not_found" {
		t.Fatalf("missing file: got %s, want not_found", errCode)
	}
}

func TestCheckFileFetch_MIMEAndSize(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	_ = os.MkdirAll(root, 0o755)

	// Image file (PNG header)
	imgFile := filepath.Join(root, "test.png")
	pngHeader := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4")
	_ = os.WriteFile(imgFile, pngHeader, 0o644)

	realPath, mimeType, isImg, errCode := CheckFileFetch(imgFile, []string{root}, nil, false, tmp, nil, "")
	if errCode != "" || realPath != imgFile {
		t.Fatalf("PNG check: err=%s", errCode)
	}
	if !isImg || !strings.Contains(mimeType, "image/") {
		t.Fatalf("expected image mime, got isImg=%v, mime=%s", isImg, mimeType)
	}

	// Text file
	txtFile := filepath.Join(root, "doc.txt")
	_ = os.WriteFile(txtFile, []byte("plain text"), 0o644)
	realPath, mimeType, isImg, errCode = CheckFileFetch(txtFile, []string{root}, nil, false, tmp, nil, "")
	if errCode != "" || realPath != txtFile {
		t.Fatalf("text check: err=%s", errCode)
	}
	if isImg || !strings.HasPrefix(mimeType, "text/") {
		t.Fatalf("expected text mime, got isImg=%v, mime=%s", isImg, mimeType)
	}
}

func TestAgent_ServeFileFetch_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	_ = os.MkdirAll(root, 0o755)

	normalFile := filepath.Join(root, "app.log")
	_ = os.WriteFile(normalFile, []byte("2026-10-22 OK"), 0o644)

	tooLargeFile := filepath.Join(root, "huge.bin")
	_ = os.WriteFile(tooLargeFile, make([]byte, 1024), 0o644) // 1024 bytes

	var uploadCalled atomic.Int32
	var failCalled atomic.Int32
	var mu sync.Mutex
	var lastFailCode string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/agent/file-fetch":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]AgentPendingFileFetch{
				{
					ID:            "fetch-1",
					MessageID:     "msg-1",
					ChannelID:     "ch-1",
					Path:          normalFile,
					MaxFileBytes:  500,
					MaxImageBytes: 1000,
				},
				{
					ID:            "fetch-2",
					MessageID:     "msg-2",
					ChannelID:     "ch-2",
					Path:          tooLargeFile,
					MaxFileBytes:  500, // file is 1024 > 500
					MaxImageBytes: 1000,
				},
			})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/v1/agent/file-fetch/fetch-1/content"):
			uploadCalled.Add(1)
			body, _ := io.ReadAll(r.Body)
			if string(body) != "2026-10-22 OK" {
				t.Errorf("unexpected body: %q", string(body))
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/v1/agent/file-fetch/fetch-2/fail"):
			failCalled.Add(1)
			var in struct {
				ErrorCode string `json:"error_code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			lastFailCode = in.ErrorCode
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	agent := &Agent{
		Client:           &Client{BaseURL: srv.URL, Token: "test-token", HTTP: srv.Client()},
		Log:              slog.Default(),
		Roots:            []string{root},
		Sandbox:          false,
		FileFetchLimiter: make(chan struct{}, 2),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	agent.pollAndServeFileFetches(ctx)

	// Wait for background file fetch workers to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if uploadCalled.Load() == 1 && failCalled.Load() == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if uploadCalled.Load() != 1 {
		t.Fatalf("expected 1 upload call, got %d", uploadCalled.Load())
	}
	if failCalled.Load() != 1 {
		t.Fatalf("expected 1 fail call, got %d", failCalled.Load())
	}
	mu.Lock()
	code := lastFailCode
	mu.Unlock()
	if code != "too_large" {
		t.Fatalf("expected fail code too_large, got %s", code)
	}
}

func TestAgent_ServerErrorDoesNotCrash(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	_ = os.MkdirAll(root, 0o755)
	file := filepath.Join(root, "data.txt")
	_ = os.WriteFile(file, []byte("hello"), 0o644)

	// Server returns 500 Internal Server Error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	agent := &Agent{
		Client:           &Client{BaseURL: srv.URL, Token: "test-token", HTTP: srv.Client()},
		Log:              slog.Default(),
		Roots:            []string{root},
		Sandbox:          false,
		FileFetchLimiter: make(chan struct{}, 2),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Should not panic or crash
	agent.pollAndServeFileFetches(ctx)
}

func TestAgent_OtherGuardAgentIgnored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()

		var h struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		if err := wsjson.Read(r.Context(), c, &h); err != nil {
			return
		}

		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "welcome", "protocol": 1, "head": 10})
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "replay_done", "seq": 10})

		time.Sleep(20 * time.Millisecond)

		// Event for another agent
		_ = wsjson.Write(r.Context(), c, map[string]any{
			"type":       "event",
			"seq":        11,
			"event_type": "file.fetch_requested",
			"data": map[string]any{
				"agent_id": "other-agent-id",
			},
		})

		time.Sleep(50 * time.Millisecond)

		// Event for this agent
		_ = wsjson.Write(r.Context(), c, map[string]any{
			"type":       "event",
			"seq":        12,
			"event_type": "file.fetch_requested",
			"data": map[string]any{
				"agent_id": "my-agent-id",
			},
		})

		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wakeCh := make(chan struct{}, 10)
	fileFetchWakeCh := make(chan struct{}, 10)

	agent := &Agent{
		Client: &Client{
			BaseURL: server.URL,
			Token:   "tok-1",
		},
		Log:                slog.New(slog.NewTextHandler(io.Discard, nil)),
		TokenCheckInterval: time.Minute,
		WSBackoff:          func(int) time.Duration { return 10 * time.Millisecond },
	}
	agent.me = Me{ID: "my-agent-id", Handle: "my-agent"}

	go agent.watchWS(ctx, wakeCh, fileFetchWakeCh)

	// Wait for the wake signal
	select {
	case <-fileFetchWakeCh:
		// Received signal for my-agent-id!
	case <-time.After(2 * time.Second):
		t.Fatal("expected file fetch signal for my-agent-id")
	}

	// Ensure no extra signals were sent (the event for other-agent-id was ignored)
	select {
	case <-fileFetchWakeCh:
		t.Fatal("unexpected extra file fetch signal for other-agent-id")
	case <-time.After(50 * time.Millisecond):
		// Expected: no extra signal
	}
}

// A log keeps growing while it is uploaded: the snapshot sends exactly the
// size it had when opened, so the request's Content-Length stays true.
func TestOpenSnapshotFixesSizeOfGrowingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(p, []byte("line 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, size, err := openSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("line 2 written during the upload\n")
	_ = w.Close()

	got, err := io.ReadAll(io.LimitReader(f, size))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "line 1\n" || size != 7 {
		t.Fatalf("snapshot: size %d, content %q", size, got)
	}
}

// The checked path is re-verified after open: a path that is now a symlink
// to somewhere else, or not a regular file, is refused.
func TestOpenSnapshotRefusesSwappedPath(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	checked := filepath.Join(dir, "report.txt")
	if err := os.Symlink(secret, checked); err != nil {
		t.Fatal(err)
	}
	if f, _, err := openSnapshot(checked); err == nil {
		_ = f.Close()
		t.Fatal("a path swapped for a symlink after the check must be refused")
	}
	if f, _, err := openSnapshot(dir); err == nil {
		_ = f.Close()
		t.Fatal("a directory must be refused")
	}
}
