package guard

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
)

// TestManualCodeGraph runs the machine's real CodeGraph through the guard's
// code paths: the indexer in its own sandbox on a throwaway repository,
// then the MCP server inside a sandboxed work run's mounts, answering a
// codegraph_explore call. Nothing outside temp folders is touched.
//
//	COLONY_MANUAL_CODEGRAPH=1 go test ./internal/guard/ -run TestManualCodeGraph -v
func TestManualCodeGraph(t *testing.T) {
	if os.Getenv("COLONY_MANUAL_CODEGRAPH") != "1" {
		t.Skip("set COLONY_MANUAL_CODEGRAPH=1 to run with the installed CodeGraph")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	home, _ := os.UserHomeDir()
	cg := DetectCodeGraph(ctx, CodeGraphConfig{}, home, nil, discardLogger())
	if cg == nil {
		t.Skip("no CodeGraph installed")
	}
	sb := DetectSandbox(ctx, home, t.TempDir())
	if sb == nil {
		t.Skip("no bwrap")
	}
	t.Logf("codegraph %s at %s", cg.Version, cg.Bundle)

	repo := initTestRepo(t)
	src := "package main\n\n// Greet returns a greeting for name.\nfunc Greet(name string) string { return \"hello \" + name }\n\nfunc main() { println(Greet(\"x\")) }\n"
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cg.Start(ctx)
	cg.Ensure(repo, func(r, name string, args []string) (string, []string, []string, error) {
		return sb.IndexCommand(cg, r, name, args)
	})
	deadline := time.Now().Add(4 * time.Minute)
	for !cg.idle() && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	rep := cg.Report()
	if len(rep.Repos) != 1 || rep.Repos[0].Status != apispec.CodegraphRepoStatusIndexed {
		d := ""
		if len(rep.Repos) == 1 && rep.Repos[0].Detail != nil {
			d = *rep.Repos[0].Detail
		}
		t.Fatalf("index in the sandbox: %+v %s", rep.Repos, d)
	}
	if !exists(filepath.Join(repo, ".codegraph", "codegraph.db")) {
		t.Fatal("no database written")
	}
	if out, _ := gitExec(ctx, repo, "status", "--porcelain", "-u"); strings.Contains(out, ".codegraph") {
		t.Fatalf("index visible to git: %q", out)
	}

	// The MCP server as a sandboxed work run starts it.
	agentDir := filepath.Join(t.TempDir(), "agent")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sb.NetworkMode = "open"
	argv, env, err := sb.CommandWithOptions("claude-code", agentDir, repo, []Grant{{Path: repo, Write: true, Mode: "write"}},
		cg.Bin, codeGraphServeArgs, SandboxOptions{CodeGraph: cg})
	if err != nil {
		t.Fatal(err)
	}
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"codegraph_explore","arguments":{"query":"Greet"}}}
`
	cmd := exec.CommandContext(ctx, sb.Bwrap, argv...)
	cmd.Env = append(env, codeGraphEnv...)
	pr, pw, _ := os.Pipe()
	cmd.Stdin = pr
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = pw.WriteString(in)
	for i := 0; i < 100 && !strings.Contains(out.String(), `"id":2`); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	_ = pw.Close()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	got := out.String()
	if !strings.Contains(got, `"id":2`) || !strings.Contains(got, "main.go") {
		t.Fatalf("explore in the work sandbox: %s", got)
	}
	t.Logf("explore answered (%d bytes)", len(got))
}
