package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
)

// fakeCodeGraph is a codegraph stand-in: it records every call (arguments,
// the environment the guard must set, start and end) in $CG_FAKE_LOG, and
// `init` creates the index database like the real one.
const fakeCodeGraph = `#!/bin/sh
if [ "$1" = "--version" ]; then echo "1.6.2"; exit 0; fi
log="${CG_FAKE_LOG:-/dev/null}"
echo "start $*" >> "$log"
echo "env TELEMETRY=$CODEGRAPH_TELEMETRY DNT=$DO_NOT_TRACK NODAEMON=$CODEGRAPH_NO_DAEMON DIR=$CODEGRAPH_DIR NOUPDATE=$CODEGRAPH_NO_UPDATE_CHECK" >> "$log"
if [ -n "$CG_FAKE_SLEEP" ]; then sleep "$CG_FAKE_SLEEP"; fi
if [ -n "$CG_FAKE_FAIL" ]; then echo "end fail" >> "$log"; exit 1; fi
if [ "$1" = "init" ]; then mkdir -p "$3/.codegraph" && : > "$3/.codegraph/codegraph.db"; fi
echo "end $1" >> "$log"
`

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// installFakeCodeGraph puts the fake at home/.codegraph/versions/v9/bin
// with the ~/.local/bin/codegraph link, like the real install.sh.
func installFakeCodeGraph(t *testing.T, home string) string {
	t.Helper()
	bin := filepath.Join(home, ".codegraph", "versions", "v9", "bin", "codegraph")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte(fakeCodeGraph), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".local", "bin", "codegraph")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	return bin
}

func newTestCodeGraph(t *testing.T) (*CodeGraph, string) {
	t.Helper()
	home := t.TempDir()
	installFakeCodeGraph(t, home)
	logFile := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("CG_FAKE_LOG", logFile)
	cg := DetectCodeGraph(context.Background(), CodeGraphConfig{}, home, nil, discardLogger())
	if cg == nil {
		t.Fatal("the installed fake codegraph was not detected")
	}
	return cg, logFile
}

func waitCodeGraphIdle(t *testing.T, cg *CodeGraph) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cg.idle() {
		if time.Now().After(deadline) {
			t.Fatal("codegraph indexer did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func readCalls(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestDetectCodeGraph(t *testing.T) {
	home := t.TempDir()
	if cg := DetectCodeGraph(context.Background(), CodeGraphConfig{}, home, nil, discardLogger()); cg != nil {
		t.Fatal("no binary: CodeGraph must stay off, without an error")
	}
	bin := installFakeCodeGraph(t, home)
	cg := DetectCodeGraph(context.Background(), CodeGraphConfig{}, home, nil, discardLogger())
	if cg == nil {
		t.Fatal("not detected")
	}
	if cg.Version != "1.6.2" || cg.Bin != bin || cg.Bundle != filepath.Join(home, ".codegraph", "versions", "v9") {
		t.Fatalf("detected %+v", cg)
	}
	off := false
	if DetectCodeGraph(context.Background(), CodeGraphConfig{Enabled: &off}, home, nil, discardLogger()) != nil {
		t.Fatal("enabled: false must switch it off")
	}
	broken := filepath.Join(home, "broken")
	if err := os.WriteFile(broken, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if DetectCodeGraph(context.Background(), CodeGraphConfig{Bin: broken}, home, nil, discardLogger()) != nil {
		t.Fatal("a binary that does not run must leave CodeGraph off")
	}
}

// TestCodeGraphIndexesRepo: init for a new repository, then sync; the env
// switches telemetry, the update check and the daemon off; the index
// folder is excluded from git (CodeGraph's own .gitignore leaves itself
// visible, so `git add -A` would commit it).
func TestCodeGraphIndexesRepo(t *testing.T) {
	cg, logFile := newTestCodeGraph(t)
	clock := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	cg.Now = func() time.Time { return clock }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cg.Start(ctx)

	repo := initTestRepo(t)
	cg.Ensure(repo, nil)
	waitCodeGraphIdle(t, cg)
	calls := readCalls(t, logFile)
	if len(calls) != 3 || calls[0] != "start init --yes "+repo {
		t.Fatalf("calls %q", calls)
	}
	if calls[1] != "env TELEMETRY=0 DNT=1 NODAEMON=1 DIR=.codegraph NOUPDATE=1" {
		t.Fatalf("env %q", calls[1])
	}
	excl, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(excl), "\n.codegraph*/\n") {
		t.Fatalf("exclude file %q %v", excl, err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".codegraph", ".gitignore"), []byte("*\n!.gitignore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _ := gitExec(ctx, repo, "status", "--porcelain", "-u"); strings.Contains(out, ".codegraph") {
		t.Fatalf("the index folder shows in git status: %q", out)
	}

	// Within MinSync: nothing; after it: an incremental sync.
	cg.Ensure(repo, nil)
	waitCodeGraphIdle(t, cg)
	if n := len(readCalls(t, logFile)); n != 3 {
		t.Fatalf("synced again within MinSync: %d calls", n)
	}
	clock = clock.Add(2 * time.Minute)
	cg.Ensure(repo, nil)
	waitCodeGraphIdle(t, cg)
	calls = readCalls(t, logFile)
	if len(calls) != 6 || calls[3] != "start sync --quiet "+repo {
		t.Fatalf("calls %q", calls)
	}
	excl2, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if strings.Count(string(excl2), ".codegraph*/") != 1 {
		t.Fatalf("exclude rule added twice: %q", excl2)
	}

	rep := cg.Report()
	if rep.Version != "1.6.2" || len(rep.Repos) != 1 || rep.Repos[0].Path != repo ||
		rep.Repos[0].Status != apispec.CodegraphRepoStatusIndexed || rep.Repos[0].IndexedAt == nil || rep.Repos[0].Files == nil {
		t.Fatalf("report %+v", rep)
	}
}

// TestCodeGraphOnlyRepoRoots: a folder of projects is never indexed, nor a
// subfolder of a repository; only a repository's root.
func TestCodeGraphOnlyRepoRoots(t *testing.T) {
	cg, logFile := newTestCodeGraph(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cg.Start(ctx)

	projects := t.TempDir()
	repo := filepath.Join(projects, "app")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	sub := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cg.Ensure(projects, nil) // no .git: not even queued
	cg.Ensure(sub, nil)
	cg.Ensure("relative/path", nil)
	waitCodeGraphIdle(t, cg)
	if calls := readCalls(t, logFile); len(calls) != 0 {
		t.Fatalf("indexed something that is not a repository root: %q", calls)
	}
	if len(cg.Report().Repos) != 0 {
		t.Fatalf("report %+v", cg.Report())
	}

	// A .git that is only a marker, not the repository's root.
	if err := os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: nowhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cg.Ensure(sub, nil)
	waitCodeGraphIdle(t, cg)
	if calls := readCalls(t, logFile); len(calls) != 0 {
		t.Fatalf("indexed a subfolder: %q", calls)
	}
	if r := cg.Report().Repos; len(r) != 1 || r[0].Status != apispec.CodegraphRepoStatusSkipped {
		t.Fatalf("report %+v", r)
	}
}

func TestCodeGraphMaxFilesSkip(t *testing.T) {
	cg, logFile := newTestCodeGraph(t)
	cg.MaxFiles = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cg.Start(ctx)
	repo := initTestRepo(t)
	for _, f := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte("package a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cg.Ensure(repo, nil)
	waitCodeGraphIdle(t, cg)
	if calls := readCalls(t, logFile); len(calls) != 0 {
		t.Fatalf("a repository over max_files was indexed: %q", calls)
	}
	r := cg.Report().Repos
	if len(r) != 1 || r[0].Status != apispec.CodegraphRepoStatusSkipped || r[0].Detail == nil || !strings.Contains(*r[0].Detail, "max_files") {
		t.Fatalf("report %+v", r)
	}
}

// TestCodeGraphOneJobAtATime: two repositories queued together are indexed
// one after the other, never in parallel.
func TestCodeGraphOneJobAtATime(t *testing.T) {
	cg, logFile := newTestCodeGraph(t)
	t.Setenv("CG_FAKE_SLEEP", "0.3")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := initTestRepo(t), initTestRepo(t)
	cg.Ensure(a, nil)
	cg.Ensure(b, nil)
	cg.Ensure(a, nil) // already queued: once
	cg.Start(ctx)
	waitCodeGraphIdle(t, cg)
	var seq []string
	for _, l := range readCalls(t, logFile) {
		if strings.HasPrefix(l, "start") || strings.HasPrefix(l, "end") {
			seq = append(seq, strings.Fields(l)[0])
		}
	}
	if !slices.Equal(seq, []string{"start", "end", "start", "end"}) {
		t.Fatalf("jobs overlapped or repeated: %q", seq)
	}
}

// TestCodeGraphFailedRetriedLater: a failed index is not retried in a loop,
// only after RetryAfter.
func TestCodeGraphFailedRetriedLater(t *testing.T) {
	cg, logFile := newTestCodeGraph(t)
	t.Setenv("CG_FAKE_FAIL", "1")
	clock := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	cg.Now = func() time.Time { return clock }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cg.Start(ctx)
	repo := initTestRepo(t)
	starts := func() int {
		n := 0
		for _, l := range readCalls(t, logFile) {
			if strings.HasPrefix(l, "start") {
				n++
			}
		}
		return n
	}
	cg.Ensure(repo, nil)
	waitCodeGraphIdle(t, cg)
	for range 3 {
		cg.Ensure(repo, nil)
		waitCodeGraphIdle(t, cg)
	}
	if n := starts(); n != 1 {
		t.Fatalf("failed index retried at once: %d runs", n)
	}
	if r := cg.Report().Repos; len(r) != 1 || r[0].Status != apispec.CodegraphRepoStatusFailed {
		t.Fatalf("report %+v", r)
	}
	clock = clock.Add(61 * time.Minute)
	cg.Ensure(repo, nil)
	waitCodeGraphIdle(t, cg)
	if n := starts(); n != 2 {
		t.Fatalf("not retried after RetryAfter: %d runs", n)
	}
}

func TestReposFromSteps(t *testing.T) {
	root := t.TempDir()
	for _, r := range []string{"app", "lib/core"} {
		if err := os.MkdirAll(filepath.Join(root, r, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "app", "internal", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	steps := []Step{
		{Path: "app/internal/x/a.go"},
		{Path: "app/main.go"},
		{Path: "lib/core/b.go"},
		{Path: "notes.md"},            // the root's own file
		{Path: "../outside/c.go"},     // outside the root
		{Path: "/etc/passwd"},         // absolute
		{Path: "plain/dir/nogit.txt"}, // no repository
	}
	got := reposFromSteps(root, steps, 5)
	want := []string{filepath.Join(root, "app"), filepath.Join(root, "lib", "core")}
	if !slices.Equal(got, want) {
		t.Fatalf("repos %q, want %q", got, want)
	}
	if got := reposFromSteps(root, steps, 1); len(got) != 1 {
		t.Fatalf("limit ignored: %q", got)
	}
}

func TestWithCodeGraphHint(t *testing.T) {
	if WithCodeGraphHint("rules", false) != "rules" {
		t.Fatal("hint added without CodeGraph")
	}
	got := WithCodeGraphHint("# Rules\n", true)
	if !strings.HasPrefix(got, "# Rules\n\n## Finding code: CodeGraph") || !strings.Contains(got, "codegraph_explore once") {
		t.Fatalf("hint %q", got)
	}
	if WithCodeGraphHint("", true) != CodeGraphHint {
		t.Fatal("hint without rules")
	}
}

// TestCodeGraphMCPConfigs: the server entry of each client, with the env
// and Claude Code's alwaysLoad.
func TestCodeGraphMCPConfigs(t *testing.T) {
	cg := &CodeGraph{Bin: "/home/u/.codegraph/versions/v9/bin/codegraph", Bundle: "/home/u/.codegraph/versions/v9"}
	raw, err := json.Marshal(cg.ClaudeServer())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"stdio"`, `"command":"/home/u/.codegraph/versions/v9/bin/codegraph"`,
		`"args":["serve","--mcp","--no-watch"]`, `"alwaysLoad":true`, `"CODEGRAPH_TELEMETRY":"0"`, `"DO_NOT_TRACK":"1"`,
		`"CODEGRAPH_NO_DAEMON":"1"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("claude entry lacks %s: %s", want, raw)
		}
	}
	agy, _ := json.Marshal(cg.AgyServer())
	if !strings.Contains(string(agy), `"command":"/home/u/.codegraph/versions/v9/bin/codegraph"`) || !strings.Contains(string(agy), `"DO_NOT_TRACK":"1"`) {
		t.Errorf("agy entry %s", agy)
	}

	// The direct run's config file holds it next to the Colonyhall server.
	path, cleanup, err := writeMCPConfigWithServers(t.TempDir(), "http://server", "tok", map[string]mcpServerDoc{"codegraph": cg.ClaudeServer()})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var doc map[string]map[string]map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["mcpServers"]["codegraph"]["alwaysLoad"] != true || doc["mcpServers"]["colonyhall"] == nil {
		t.Fatalf("config %s", data)
	}

	// An unsandboxed work run's own config: CodeGraph only, no token.
	p2, cleanup2, err := writeServersConfig(t.TempDir(), map[string]mcpServerDoc{"codegraph": cg.ClaudeServer()})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()
	data2, _ := os.ReadFile(p2)
	if strings.Contains(string(data2), "colonyhall") || strings.Contains(string(data2), "Bearer") {
		t.Fatalf("codegraph-only config %s", data2)
	}
	if fi, _ := os.Stat(p2); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

// TestClaudeCodeGraphAllowed: the CodeGraph tools are allowed in work and
// direct runs, so a call never becomes an approval question; the docs-only
// planner gets them too and still writes nothing but Markdown.
func TestClaudeCodeGraphAllowed(t *testing.T) {
	sf := &fakeRunner{stdout: `{"type":"result","result":"ok"}`}
	c := &ClaudeCode{Run: sf.run}
	if _, err := c.Work(context.Background(), WorkRequest{Prompt: "x", Dir: t.TempDir(), NoExec: true, CodeGraph: true, MCPConfigPath: "/cfg/mcp.json"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := flagValue(sf.args, "--allowedTools"); v != CodeGraphTool {
		t.Fatalf("work --allowedTools %q (%v)", v, sf.args)
	}
	if v, _ := flagValue(sf.args, "--mcp-config"); v != "/cfg/mcp.json" {
		t.Fatalf("work --mcp-config %q", v)
	}

	sf2 := &fakeRunner{stdout: `{"type":"result","result":"ok","session_id":"s"}`}
	c2 := &ClaudeCode{Run: sf2.run}
	if _, err := c2.Direct(context.Background(), DirectRequest{Prompt: "x", Dir: t.TempDir(), SessionID: "s", MCPConfigPath: "/cfg/m.json", CodeGraph: true}); err != nil {
		t.Fatal(err)
	}
	if v, _ := flagValue(sf2.args, "--allowedTools"); v != CodeGraphTool {
		t.Fatalf("direct --allowedTools %q (%v)", v, sf2.args)
	}
	sf3 := &fakeRunner{stdout: `{"type":"result","result":"ok","session_id":"s"}`}
	c3 := &ClaudeCode{Run: sf3.run}
	if _, err := c3.Direct(context.Background(), DirectRequest{Prompt: "x", Dir: t.TempDir(), SessionID: "s"}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(sf3.args, "--allowedTools") {
		t.Fatalf("CodeGraph allowed without the server: %v", sf3.args)
	}

	f := &fakeRunner{stdout: `{"type":"result","result":"ok","session_id":"s1"}`, helpStdout: docsOnlyHelp}
	d := &ClaudeCode{Run: f.run}
	dir := t.TempDir()
	if _, err := d.Direct(context.Background(), DirectRequest{Prompt: "plan", Dir: dir, SessionID: "s1", MCPConfigPath: "/m.json", DocsOnly: true, CodeGraph: true}); err != nil {
		t.Fatal(err)
	}
	if v, _ := flagValue(f.args, "--allowedTools"); v != "mcp__colonyhall,mcp__codegraph" {
		t.Fatalf("docs-only --allowedTools %q", v)
	}
	settings, _ := flagValue(f.args, "--settings")
	var doc struct {
		Permissions struct {
			Allow       []string `json:"allow"`
			DefaultMode string   `json:"defaultMode"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(settings), &doc); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(doc.Permissions.Allow, CodeGraphTool) || doc.Permissions.DefaultMode != "dontAsk" {
		t.Fatalf("docs-only settings %s", settings)
	}
	for _, rule := range doc.Permissions.Allow {
		if (strings.HasPrefix(rule, "Edit(") || strings.HasPrefix(rule, "Write(")) && !strings.HasSuffix(rule, "/**/*.md)") {
			t.Fatalf("docs-only planner may write more than Markdown: %q", rule)
		}
	}
	if tools, _ := flagValue(f.args, "--tools"); strings.Contains(tools, "Bash") {
		t.Fatalf("command tool offered: %q", tools)
	}
}

// TestSandboxCodeGraphMounts: the bundle is mounted read-only only when the
// run has CodeGraph, and each client's own config names the server.
func TestSandboxCodeGraphMounts(t *testing.T) {
	home := t.TempDir()
	bin := installFakeCodeGraph(t, home)
	cg := &CodeGraph{Bin: bin, Bundle: filepath.Dir(filepath.Dir(bin))}
	root := filepath.Join(home, ".local", "share", "colonyhall", "sandbox")
	agentDir := filepath.Join(root, "a1")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sb := &Sandbox{Bwrap: "/usr/bin/bwrap", Home: home, Root: root, NetworkMode: "open"}
	work := t.TempDir()
	argv, _, err := sb.CommandWithOptions("claude-code", agentDir, work, []Grant{{Path: work, Write: true, Mode: "write"}}, "claude", []string{"-p", "x"}, SandboxOptions{CodeGraph: cg})
	if err != nil {
		t.Fatal(err)
	}
	if !hasMount(argv, "--ro-bind", cg.Bundle) {
		t.Fatalf("bundle not mounted read-only: %v", argv)
	}
	if hasMount(argv, "--bind", cg.Bundle) {
		t.Fatalf("bundle mounted writable: %v", argv)
	}
	data, err := os.ReadFile(filepath.Join(agentDir+".config", "claude", "mcp.json"))
	if err != nil || !strings.Contains(string(data), `"codegraph"`) || !strings.Contains(string(data), `"CODEGRAPH_NO_DAEMON": "1"`) {
		t.Fatalf("claude mcp.json %s %v", data, err)
	}

	argv, _, err = sb.CommandWithOptions("claude-code", agentDir, work, []Grant{{Path: work, Write: true, Mode: "write"}}, "claude", nil, SandboxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(argv, cg.Bundle) {
		t.Fatalf("bundle mounted without CodeGraph: %v", argv)
	}
	if _, err := os.Stat(filepath.Join(agentDir+".config", "claude", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("stale mcp.json kept: %v", err)
	}

	if _, _, err := sb.CommandWithOptions("antigravity", agentDir, work, nil, "agy", nil, SandboxOptions{CodeGraph: cg}); err != nil {
		t.Fatal(err)
	}
	gem := filepath.Join(agentDir+".config", "gemini")
	cfg, _ := os.ReadFile(filepath.Join(gem, "config", "mcp_config.json"))
	settings, _ := os.ReadFile(filepath.Join(gem, "antigravity-cli", "settings.json"))
	if !strings.Contains(string(cfg), `"codegraph"`) || !strings.Contains(string(settings), `"mcp(codegraph/*)"`) || !strings.Contains(string(settings), `"command(*)"`) {
		t.Fatalf("agy config %s settings %s", cfg, settings)
	}
}

func hasMount(argv []string, flag, path string) bool {
	for i := 0; i+2 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == path && argv[i+2] == path {
			return true
		}
	}
	return false
}

// TestIndexCommandArgs: the indexer's sandbox has no network and binds only
// the repository (read-only) and its index folder (writable).
func TestIndexCommandArgs(t *testing.T) {
	home := t.TempDir()
	bin := installFakeCodeGraph(t, home)
	cg := &CodeGraph{Bin: bin, Bundle: filepath.Dir(filepath.Dir(bin))}
	sb := &Sandbox{Bwrap: "/usr/bin/bwrap", Home: home, HasUnshareNet: true}
	repo := initTestRepo(t)
	if _, _, _, err := sb.IndexCommand(cg, repo, bin, []string{"init"}); err == nil {
		t.Fatal("no index folder yet: must be refused (it is bound writable)")
	}
	index := filepath.Join(repo, ".codegraph")
	if err := os.Mkdir(index, 0o755); err != nil {
		t.Fatal(err)
	}
	name, argv, env, err := sb.IndexCommand(cg, repo, bin, []string{"init", "--yes", repo})
	if err != nil {
		t.Fatal(err)
	}
	if name != "/usr/bin/bwrap" || !slices.Contains(argv, "--unshare-net") {
		t.Fatalf("argv %v", argv)
	}
	if !hasMount(argv, "--ro-bind", repo) || !hasMount(argv, "--bind", index) || hasMount(argv, "--bind", repo) || !hasMount(argv, "--ro-bind", cg.Bundle) {
		t.Fatalf("mounts %v", argv)
	}
	for i, a := range argv {
		if (a == "--bind") && argv[i+1] != index {
			t.Fatalf("something else writable: %s", argv[i+1])
		}
	}
	if !slices.Contains(env, "DO_NOT_TRACK=1") || !slices.Contains(env, "CODEGRAPH_NO_DAEMON=1") {
		t.Fatalf("env %v", env)
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "CG_FAKE") || strings.Contains(kv, "TOKEN") {
			t.Fatalf("env leaks %q", kv)
		}
	}

	// A link in the index folder's place is refused.
	repo2 := initTestRepo(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(repo2, ".codegraph")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := sb.IndexCommand(cg, repo2, bin, []string{"init"}); err == nil {
		t.Fatal("a symlinked index folder was accepted")
	}
}

// TestIndexInSandboxSeesOnlyRepo runs the indexer sandbox for real: the
// index folder is writable, the repository is not, and a file outside it
// (reachable through a link the agent planted) cannot be read.
func TestIndexInSandboxSeesOnlyRepo(t *testing.T) {
	sbx := DetectSandbox(context.Background(), t.TempDir(), t.TempDir())
	if sbx == nil {
		t.Skip("no working bwrap")
	}
	home := t.TempDir()
	sbx.Home = home
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := initTestRepo(t)
	if err := os.Symlink(secret, filepath.Join(repo, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(repo, ".codegraph")
	if err := os.Mkdir(index, 0o755); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(t.TempDir(), "probe.sh")
	script := fmt.Sprintf(`#!/bin/sh
echo indexed > %[1]s/out.txt && echo index-ok
touch %[2]s/new.txt 2>/dev/null && echo repo-writable
cat %[2]s/leak.txt 2>/dev/null && echo leak-readable
cat %[3]s 2>/dev/null && echo secret-readable
exit 0
`, index, repo, secret)
	if err := os.WriteFile(probe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cg := &CodeGraph{Bin: probe, Bundle: filepath.Dir(probe)}
	name, argv, env, err := sbx.IndexCommand(cg, repo, probe, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(name, argv...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "index-ok") {
		t.Fatalf("index folder not writable: %s", got)
	}
	for _, bad := range []string{"repo-writable", "leak-readable", "secret-readable", "top secret"} {
		if strings.Contains(got, bad) {
			t.Fatalf("the indexer sandbox is too wide (%s): %s", bad, got)
		}
	}
}

// TestCommitSkipsCodeGraphIndex: the guard's commit after a sandboxed run
// never takes an index folder, even when the agent rewrote CodeGraph's
// .gitignore and the database is over the 10 MB limit.
func TestCommitSkipsCodeGraphIndex(t *testing.T) {
	ctx := context.Background()
	repoDir := initTestRepo(t)
	handle := "cg-agent"
	wtPath, branch, err := EnsureWorktree(ctx, repoDir, handle)
	if err != nil {
		t.Fatal(err)
	}
	ag := &Agent{Cfg: AgentConfig{Name: handle}, Log: discardLogger(), Sandbox: true, WorktreeRepo: repoDir, WorktreeBranch: branch, RootDir: wtPath}
	for _, dir := range []string{".codegraph", filepath.Join("sub", ".codegraph-wsl")} {
		d := filepath.Join(wtPath, dir)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, ".gitignore"), []byte("# emptied by the agent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(d, "codegraph.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(11 << 20); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	if err := os.WriteFile(filepath.Join(wtPath, "notes.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	line := ag.checkAndCommitWorktree(ctx, PromptItem{Item: Item{Channel: "general", Author: "norbi"}}, "write notes", wtPath)
	if !strings.Contains(line, "(1 files)") {
		t.Fatalf("commit line %q", line)
	}
	files, err := gitExec(ctx, wtPath, "show", "--name-only", "--format=", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(files) != "notes.md" {
		t.Fatalf("committed %q", files)
	}
}

// TestPolicyCodeGraph: a run gets CodeGraph only when its working folder is
// writable (the server cannot open an index in a read-only folder); only
// writable grants are indexed, through the indexer sandbox for a sandboxed
// agent; without CodeGraph on the machine nothing changes.
func TestPolicyCodeGraph(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "projects")
	app, ref := filepath.Join(root, "app"), filepath.Join(root, "ref")
	for _, d := range []string{app, ref} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, _ := ResolveRoots([]string{root})
	cg := &CodeGraph{Bin: "/cg/bin/codegraph", Bundle: "/cg"}
	s := &Supervisor{Cfg: Config{SandboxNetwork: "filtered"}, roots: roots, Log: discardLog(), lastOK: time.Now(),
		sandbox: &Sandbox{Bwrap: "/usr/bin/bwrap", Home: base, HasUnshareNet: true}, codegraph: cg}
	agentDir := filepath.Join(base, "sb", "bot")

	m := &managed{conf: HostAgent{Handle: "bot", AllowWork: true, Sandbox: true},
		grants: []Grant{{Path: app, Write: true, Mode: "write"}, {Path: ref, Mode: "read"}}}
	p := s.policy(m, agentDir)
	if !p.CodeGraph || !slices.Equal(p.IndexRepos, []string{app}) || p.IndexWrap == nil {
		t.Fatalf("writable work folder: %+v", p)
	}

	m.grants = []Grant{{Path: ref, Mode: "read"}}
	p = s.policy(m, agentDir)
	if p.CodeGraph || len(p.IndexRepos) != 0 {
		t.Fatalf("read-only work folder got CodeGraph: %+v", p)
	}

	m.conf.Sandbox = false
	m.grants = []Grant{{Path: app, Write: true, Mode: "write"}}
	p = s.policy(m, agentDir)
	if !p.CodeGraph || p.IndexWrap != nil {
		t.Fatalf("full-access agent: indexed without a sandbox wrap: %+v", p)
	}

	s.codegraph = nil
	if p = s.policy(m, agentDir); p.CodeGraph || len(p.IndexRepos) != 0 {
		t.Fatalf("no CodeGraph on the machine: %+v", p)
	}
}
