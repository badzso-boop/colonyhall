// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManualClientCheck(t *testing.T) {
	if os.Getenv("COLONY_MANUAL_CHECK") != "1" {
		t.Skip("skipping manual client check (set COLONY_MANUAL_CHECK=1 to run)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	home, _ := os.UserHomeDir()
	tmpDir := t.TempDir()

	sb := DetectSandbox(ctx, home, tmpDir)
	if sb == nil || !sb.HasUnshareNet {
		t.Skip("bwrap with unshare-net not available")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	runClient := func(client string, mode string, allow []string, prompt string, extraArgs []string) string {
		agentDir := filepath.Join(tmpDir, client+"-"+mode)
		workDir := filepath.Join(agentDir, "work")
		_ = os.MkdirAll(workDir, 0o755)
		sockPath := filepath.Join(agentDir, "proxy.sock")

		pr, err := StartSandboxProxy(ProxyConfig{
			Mode:      mode,
			Allow:     allow,
			AgentName: client,
			SockPath:  sockPath,
			Log:       logger,
		})
		if err != nil {
			t.Fatalf("failed to start proxy: %v", err)
		}
		defer func() { _ = pr.Close() }()

		sb.NetworkMode = mode
		cmdName := client
		switch client {
		case "claude-code":
			cmdName = "claude"
		case "antigravity":
			cmdName = "agy"
		}

		args := append([]string{"-p", prompt}, extraArgs...)
		bwrapArgs, env, err := sb.Command(client, agentDir, workDir, nil, cmdName, args)
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}

		cmd := exec.CommandContext(ctx, sb.Bwrap, bwrapArgs...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		outputStr := strings.TrimSpace(string(out))
		pr.mu.Lock()
		seenList := make([]string, 0, len(pr.seenHosts))
		for h := range pr.seenHosts {
			seenList = append(seenList, h)
		}
		refusedList := make([]string, 0, len(pr.refusedHosts))
		for h := range pr.refusedHosts {
			refusedList = append(refusedList, h)
		}
		pr.mu.Unlock()

		t.Logf("[%s %s] err=%v, seen=%v, refused=%v\noutput:\n%s", client, mode, err, seenList, refusedList, outputStr)
		if err != nil {
			t.Errorf("[%s %s] failed: %v\noutput: %s", client, mode, err, outputStr)
		}
		return outputStr
	}

	t.Log("=== 1. Checking claude-code in log mode ===")
	runClient("claude-code", "log", nil, "Answer with PONG", []string{"--permission-mode", "default"})

	t.Log("=== 2. Checking claude-code in filtered mode ===")
	runClient("claude-code", "filtered", DefaultClaudeAllow, "Answer with PONG", []string{"--permission-mode", "default"})

	t.Log("=== 3. Checking antigravity in log mode ===")
	runClient("antigravity", "log", nil, "Answer with PONG", []string{"--output-format", "json"})

	t.Log("=== 4. Checking antigravity in filtered mode ===")
	runClient("antigravity", "filtered", DefaultAntigravityAllow, "Answer with PONG", []string{"--output-format", "json"})
}
