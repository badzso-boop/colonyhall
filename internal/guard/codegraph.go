package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"colonyhall/internal/apispec"
)

// CodeGraph (round 39) builds a local graph of a repository's symbols and
// calls (https://github.com/colbymchenry/codegraph) and answers "how does X
// work / where is Y / what does changing Z affect" in one MCP call. The guard
// wires it in itself: it indexes the agents' git repositories and gives every
// work and direct run the MCP server. `codegraph install` is never run (it
// would write into the user's own Claude and agent settings).

// CodeGraphConfig is guard.yaml's codegraph section.
type CodeGraphConfig struct {
	// Bin is the codegraph launcher; default ~/.local/bin/codegraph when it
	// exists (the reviewed install.sh puts it there).
	Bin string `yaml:"bin"`
	// Enabled switches CodeGraph off with false; default on when Bin runs.
	Enabled *bool `yaml:"enabled"`
	// MaxFiles skips repositories with more files (git ls-files); 0 means
	// the default of 20 000.
	MaxFiles int `yaml:"max_files"`
}

const (
	defaultCodeGraphMaxFiles = 20000
	// codeGraphIndexDir is the index folder in a repository. CODEGRAPH_DIR
	// pins it: on a Windows drive under WSL CodeGraph would otherwise pick
	// .codegraph-wsl for a new project.
	codeGraphIndexDir = ".codegraph"
	// codeGraphExclude keeps every CodeGraph index folder (.codegraph,
	// .codegraph-wsl, ...) out of git, at any depth.
	codeGraphExclude = ".codegraph*/"
)

// codeGraphEnv is set for every CodeGraph process the guard starts, whatever
// the machine's own settings: no telemetry, no update check, no background
// daemon outliving the run (one server process per client, ended with it),
// and one fixed index folder name.
var codeGraphEnv = []string{
	"CODEGRAPH_TELEMETRY=0",
	"DO_NOT_TRACK=1",
	"CODEGRAPH_NO_UPDATE_CHECK=1",
	"CODEGRAPH_NO_DAEMON=1",
	"CODEGRAPH_DIR=" + codeGraphIndexDir,
}

// CodeGraph is the machine's CodeGraph: how to start it, and the indexer.
type CodeGraph struct {
	// Bin is the launcher as resolved (symlinks followed): the bundle's own
	// bin/codegraph, which also works in a sandbox that sees the bundle only.
	Bin string
	// Bundle is the folder of that version (bin, lib, its own node), mounted
	// read-only into sandboxes.
	Bundle   string
	Version  string
	MaxFiles int
	Run      Runner
	Log      *slog.Logger
	Now      func() time.Time
	// RetryAfter is the pause after a failed index before the repository
	// is tried again (never in a loop); default 1 hour.
	RetryAfter time.Duration
	// MinSync is the shortest time between two syncs of one repository;
	// default 1 minute.
	MinSync time.Duration

	mu     sync.Mutex
	queue  []codeGraphJob
	queued map[string]bool
	repos  map[string]*codeGraphRepo
	wake   chan struct{}
	// OnIndexed is called after each index job (the supervisor reports).
	OnIndexed func()
}

// IndexWrap runs an index command in a sandbox: for a sandboxed agent's
// repository, so the indexer sees that repository only (see
// Sandbox.IndexCommand). It returns the command, its arguments and its
// environment.
type IndexWrap func(repo, name string, args []string) (string, []string, []string, error)

type codeGraphJob struct {
	repo string
	wrap IndexWrap
}

type codeGraphRepo struct {
	path      string
	status    apispec.CodegraphRepoStatus
	files     int
	indexedAt time.Time
	triedAt   time.Time
	detail    string
}

// DetectCodeGraph returns the machine's CodeGraph, or nil when it is
// switched off or not installed (nothing changes then, no error).
func DetectCodeGraph(ctx context.Context, cfg CodeGraphConfig, home string, run Runner, log *slog.Logger) *CodeGraph {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	bin := cfg.Bin
	if bin == "" {
		bin = filepath.Join(home, ".local", "bin", "codegraph")
		if !exists(bin) {
			return nil
		}
	} else if strings.HasPrefix(bin, "~/") {
		bin = filepath.Join(home, bin[2:])
	}
	real, err := filepath.EvalSymlinks(bin)
	if err != nil {
		log.Warn("guard: codegraph not found", "bin", bin, "err", err)
		return nil
	}
	if run == nil {
		run = ExecRunner
	}
	vctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, _, err := run(vctx, home, real, []string{"--version"}, append(os.Environ(), codeGraphEnv...))
	if err != nil {
		log.Warn("guard: codegraph does not run; it stays off", "bin", real, "err", err)
		return nil
	}
	cg := &CodeGraph{
		Bin:      real,
		Bundle:   filepath.Dir(filepath.Dir(real)),
		Version:  clip(strings.TrimSpace(string(out)), 40),
		MaxFiles: cfg.MaxFiles,
		Run:      run,
		Log:      log,
	}
	return cg
}

func (cg *CodeGraph) now() time.Time {
	if cg.Now != nil {
		return cg.Now()
	}
	return time.Now()
}

func (cg *CodeGraph) maxFiles() int {
	if cg.MaxFiles > 0 {
		return cg.MaxFiles
	}
	return defaultCodeGraphMaxFiles
}

// serveArgs start the MCP server: stdio, no file watcher (the index is
// caught up when the server connects; a watcher on a Windows drive is slow).
var codeGraphServeArgs = []string{"serve", "--mcp", "--no-watch"}

func codeGraphEnvMap() map[string]string {
	m := make(map[string]string, len(codeGraphEnv))
	for _, kv := range codeGraphEnv {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

// ClaudeServer is the Claude Code MCP config entry. alwaysLoad makes Claude
// Code wait for the server's tools at start, so codegraph_explore is in the
// first prompt instead of behind a tool search.
func (cg *CodeGraph) ClaudeServer() mcpServerDoc {
	return mcpServerDoc{Type: "stdio", Command: cg.Bin, Args: slices.Clone(codeGraphServeArgs),
		Env: codeGraphEnvMap(), AlwaysLoad: true}
}

// AgyServer is the Antigravity MCP config entry.
func (cg *CodeGraph) AgyServer() map[string]any {
	return map[string]any{"command": cg.Bin, "args": slices.Clone(codeGraphServeArgs), "env": codeGraphEnvMap()}
}

// CodeGraphTool is the Claude Code permission rule for every CodeGraph tool.
const CodeGraphTool = "mcp__codegraph"

// CodeGraphHint is added to the agent rules of a run that has the server:
// a hint, not a rule (round 39, part 4).
const CodeGraphHint = `## Finding code: CodeGraph

You have the codegraph MCP server. For "how does X work", "where is Y" or "what breaks if I change Z", call codegraph_explore once with a narrow question, then read only the exact lines it points to; prefer it over a series of grep calls and whole-file reads. Its answer stays in your context: keep questions narrow and do not repeat an explore whose answer you already have. If it says the project is not indexed, search as usual.`

// WithCodeGraphHint appends the hint to the rules when the run has CodeGraph.
func WithCodeGraphHint(rules string, on bool) string {
	if !on {
		return rules
	}
	if strings.TrimSpace(rules) == "" {
		return CodeGraphHint
	}
	return strings.TrimRight(rules, "\n") + "\n\n" + CodeGraphHint
}

// Start runs the indexer until ctx ends: one job at a time per guard.
func (cg *CodeGraph) Start(ctx context.Context) {
	cg.mu.Lock()
	if cg.wake == nil {
		cg.wake = make(chan struct{}, 1)
	}
	wake := cg.wake
	cg.mu.Unlock()
	go func() {
		for {
			job, ok := cg.next()
			if !ok {
				select {
				case <-ctx.Done():
					return
				case <-wake:
				}
				continue
			}
			cg.index(ctx, job)
			cg.mu.Lock()
			delete(cg.queued, job.repo)
			cg.mu.Unlock()
			if cg.OnIndexed != nil {
				cg.OnIndexed()
			}
		}
	}()
}

func (cg *CodeGraph) next() (codeGraphJob, bool) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	if len(cg.queue) == 0 {
		return codeGraphJob{}, false
	}
	j := cg.queue[0]
	cg.queue = cg.queue[1:]
	return j, true
}

// Ensure asks for repo to be indexed (or synced, when it has an index). It
// never blocks: a run without an index simply has the tool answer "not
// indexed". Only a git repository's root is ever indexed, never a folder of
// projects; a failed repository waits RetryAfter.
func (cg *CodeGraph) Ensure(repo string, wrap IndexWrap) {
	if cg == nil || repo == "" || !filepath.IsAbs(repo) {
		return
	}
	repo = filepath.Clean(repo)
	if !exists(filepath.Join(repo, ".git")) {
		return
	}
	retry, minSync := cg.RetryAfter, cg.MinSync
	if retry <= 0 {
		retry = time.Hour
	}
	if minSync <= 0 {
		minSync = time.Minute
	}
	cg.mu.Lock()
	defer cg.mu.Unlock()
	if cg.queued[repo] {
		return
	}
	if r := cg.repos[repo]; r != nil {
		now := cg.now()
		switch {
		case r.status != apispec.CodegraphRepoStatusIndexed && now.Sub(r.triedAt) < retry:
			return
		case r.status == apispec.CodegraphRepoStatusIndexed && now.Sub(r.triedAt) < minSync:
			return
		}
	}
	if cg.queued == nil {
		cg.queued = map[string]bool{}
	}
	cg.queued[repo] = true
	cg.queue = append(cg.queue, codeGraphJob{repo: repo, wrap: wrap})
	if cg.wake == nil {
		cg.wake = make(chan struct{}, 1)
	}
	select {
	case cg.wake <- struct{}{}:
	default:
	}
}

// index builds or syncs one repository's index.
func (cg *CodeGraph) index(ctx context.Context, job codeGraphJob) {
	repo := job.repo
	files, args, err := cg.prepare(ctx, repo, job.wrap != nil)
	status, detail := apispec.CodegraphRepoStatusIndexed, ""
	var skip errSkip
	switch {
	case errors.As(err, &skip):
		status, detail = apispec.CodegraphRepoStatusSkipped, skip.reason
		cg.Log.Info("guard: codegraph skipped a repository", "repo", repo, "reason", skip.reason)
	case err != nil:
		status, detail = apispec.CodegraphRepoStatusFailed, err.Error()
	default:
		if err := cg.exec(ctx, repo, args, job.wrap); err != nil {
			status, detail = apispec.CodegraphRepoStatusFailed, err.Error()
		}
	}
	if status == apispec.CodegraphRepoStatusFailed {
		cg.Log.Warn("guard: codegraph index failed; retried later", "repo", repo, "err", detail)
	}
	cg.mu.Lock()
	defer cg.mu.Unlock()
	if cg.repos == nil {
		cg.repos = map[string]*codeGraphRepo{}
	}
	r := cg.repos[repo]
	if r == nil {
		r = &codeGraphRepo{path: repo}
		cg.repos[repo] = r
	}
	now := cg.now()
	r.triedAt, r.status, r.detail = now, status, clip(detail, 200)
	if files > 0 {
		r.files = files
	}
	if status == apispec.CodegraphRepoStatusIndexed {
		r.indexedAt = now
	}
}

type errSkip struct{ reason string }

func (e errSkip) Error() string { return e.reason }

// prepare checks the repository and returns its file count and the
// codegraph arguments (init for a new index, sync for an existing one).
func (cg *CodeGraph) prepare(ctx context.Context, repo string, sandboxed bool) (int, []string, error) {
	gitDir := ""
	if sandboxed {
		d, err := trustedGitDir(repo)
		if err != nil {
			return 0, nil, errSkip{err.Error()}
		}
		gitDir = d
	}
	top, err := gitExec(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return 0, nil, errSkip{"not a git repository"}
	}
	realRepo, err1 := filepath.EvalSymlinks(repo)
	realTop, err2 := filepath.EvalSymlinks(top)
	if err1 != nil || err2 != nil || realRepo != realTop {
		return 0, nil, errSkip{"not the root of a git repository"}
	}
	raw, err := gitExecRaw(ctx, repo, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return 0, nil, fmt.Errorf("git ls-files: %w", err)
	}
	files := bytes.Count(raw, []byte{0})
	if files > cg.maxFiles() {
		return files, nil, errSkip{fmt.Sprintf("%d files, more than codegraph.max_files (%d)", files, cg.maxFiles())}
	}
	if err := excludeCodeGraph(ctx, repo, gitDir); err != nil {
		return files, nil, fmt.Errorf("git exclude: %w", err)
	}
	dir := filepath.Join(repo, codeGraphIndexDir)
	fi, err := os.Lstat(dir)
	switch {
	case err == nil && (!fi.IsDir() || fi.Mode()&os.ModeSymlink != 0):
		return files, nil, errSkip{codeGraphIndexDir + " is not a folder"}
	case errors.Is(err, os.ErrNotExist) && sandboxed:
		// The indexer's sandbox binds it writable, so it must exist first
		// (Mkdir never follows a link put in its place).
		if err := os.Mkdir(dir, 0o755); err != nil {
			return files, nil, err
		}
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return files, nil, err
	}
	if exists(filepath.Join(dir, "codegraph.db")) {
		return files, []string{"sync", "--quiet", repo}, nil
	}
	return files, []string{"init", "--yes", repo}, nil
}

// exec runs codegraph at low CPU and IO priority, in the indexer sandbox
// when wrap is set.
func (cg *CodeGraph) exec(ctx context.Context, repo string, args []string, wrap IndexWrap) error {
	timeout := 10 * time.Minute
	if args[0] == "init" {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	name, argv, env := cg.Bin, args, append(os.Environ(), codeGraphEnv...)
	if wrap != nil {
		var err error
		if name, argv, env, err = wrap(repo, cg.Bin, args); err != nil {
			return fmt.Errorf("sandbox: %w", err)
		}
	}
	name, argv = lowPriority(name, argv)
	run := cg.Run
	if run == nil {
		run = ExecRunner
	}
	_, _, err := run(ctx, repo, name, argv, env)
	if err != nil {
		return fmt.Errorf("codegraph %s: %w", args[0], err)
	}
	return nil
}

// lowPriority prefixes nice and ionice when the machine has them (ionice
// only where it works: some containers refuse the idle class).
func lowPriority(name string, args []string) (string, []string) {
	if p := ionicePath(); p != "" {
		args = append([]string{"-c", "3", name}, args...)
		name = p
	}
	if p, err := exec.LookPath("nice"); err == nil {
		args = append([]string{"-n", "19", name}, args...)
		name = p
	}
	return name, args
}

var ionicePath = sync.OnceValue(func() string {
	p, err := exec.LookPath("ionice")
	if err != nil || exec.Command(p, "-c", "3", "true").Run() != nil {
		return ""
	}
	return p
})

// idle reports whether no index job is queued or running.
func (cg *CodeGraph) idle() bool {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	return len(cg.queue) == 0 && len(cg.queued) == 0
}

// excludeCodeGraph adds the index folders to the repository's local
// exclude file (.git/info/exclude, shared by its worktrees): CodeGraph's own
// .codegraph/.gitignore keeps the database out of git but not that file
// itself, so `git add -A` by an agent would commit it. gitDir, when set, is
// the checked git folder (trustedGitDir) of a sandboxed agent's repository:
// the file is then written there only, never where git or a link points.
func excludeCodeGraph(ctx context.Context, repo, gitDir string) error {
	var p string
	if gitDir != "" {
		info := filepath.Join(gitDir, "info")
		switch fi, err := os.Lstat(info); {
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(info, 0o755); err != nil {
				return err
			}
		case err != nil:
			return err
		case !fi.IsDir():
			return fmt.Errorf("%s is not a folder", info)
		}
		p = filepath.Join(info, "exclude")
	} else {
		out, err := gitExec(ctx, repo, "rev-parse", "--git-path", "info/exclude")
		if err != nil {
			return err
		}
		p = out
		if !filepath.IsAbs(p) {
			p = filepath.Join(repo, p)
		}
	}
	raw, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) == codeGraphExclude {
			return nil
		}
	}
	if fi, err := os.Lstat(p); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	add := "# Colonyhall guard: CodeGraph indexes\n" + codeGraphExclude + "\n"
	if len(raw) > 0 && !bytes.HasSuffix(raw, []byte("\n")) {
		add = "\n" + add
	}
	if _, err := f.WriteString(add); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// maxReportedRepos caps the Machines page list (the contract allows 50).
const maxReportedRepos = 50

// Report is the host report's codegraph field.
func (cg *CodeGraph) Report() *apispec.CodegraphReport {
	if cg == nil {
		return nil
	}
	cg.mu.Lock()
	list := make([]codeGraphRepo, 0, len(cg.repos))
	for _, r := range cg.repos {
		list = append(list, *r)
	}
	cg.mu.Unlock()
	slices.SortFunc(list, func(a, b codeGraphRepo) int { return b.triedAt.Compare(a.triedAt) })
	if len(list) > maxReportedRepos {
		list = list[:maxReportedRepos]
	}
	out := &apispec.CodegraphReport{Version: cg.Version, Repos: make([]apispec.CodegraphRepo, 0, len(list))}
	for _, r := range list {
		rr := apispec.CodegraphRepo{Path: clip(r.path, 1024), Status: r.status}
		if r.files > 0 {
			n := r.files
			rr.Files = &n
		}
		if !r.indexedAt.IsZero() {
			t := r.indexedAt
			rr.IndexedAt = &t
		}
		if r.detail != "" {
			d := r.detail
			rr.Detail = &d
		}
		out.Repos = append(out.Repos, rr)
	}
	return out
}

// reposFromSteps finds the git repositories under root that a run's steps
// touched (step paths are relative to root), at most limit of them.
func reposFromSteps(root string, steps []Step, limit int) []string {
	if root == "" {
		return nil
	}
	root = filepath.Clean(root)
	seen := map[string]bool{}
	var out []string
	for _, s := range steps {
		if s.Path == "" || filepath.IsAbs(s.Path) || !strings.Contains(s.Path, "/") {
			continue // outside root (RelPath gives a bare name then), or root's own files
		}
		p := filepath.Join(root, s.Path)
		if !within(p, root) {
			continue
		}
		for d := filepath.Dir(p); d != root && within(d, root); d = filepath.Dir(d) {
			if seen[d] {
				break
			}
			if exists(filepath.Join(d, ".git")) {
				seen[d] = true
				out = append(out, d)
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

// IndexCommand runs codegraph (bin, args) for a sandboxed agent's
// repository in a sandbox of its own: no network, the system read-only, an
// empty home, the CodeGraph bundle and the repository (with its git
// metadata) read-only, and only the repository's index folder writable. The
// agent can write the repository and its index, so the indexer must not see
// anything else: a link the agent planted cannot pull an outside file into
// the index, and a tampered database cannot reach beyond this sandbox.
func (s *Sandbox) IndexCommand(cg *CodeGraph, repo, bin string, args []string) (string, []string, []string, error) {
	repo = filepath.Clean(repo)
	index := filepath.Join(repo, codeGraphIndexDir)
	if fi, err := os.Lstat(index); err != nil || !fi.IsDir() {
		return "", nil, nil, fmt.Errorf("%s is not a folder", index)
	}
	if real, err := filepath.EvalSymlinks(repo); err != nil || real != repo {
		return "", nil, nil, fmt.Errorf("%s is not a real path", repo)
	}
	a := []string{"--die-with-parent", "--new-session", "--unshare-pid", "--unshare-ipc", "--unshare-uts"}
	if s.HasUnshareNet {
		a = append(a, "--unshare-net")
	}
	a = append(a, "--ro-bind", "/usr", "/usr", "--ro-bind", "/etc", "/etc",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--tmpfs", "/run")
	for _, d := range []string{"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/opt"} {
		a = append(a, systemDir(d)...)
	}
	for _, f := range []string{"/etc/shadow", "/etc/gshadow", "/etc/sudoers"} {
		if _, err := os.Stat(f); err == nil {
			a = append(a, "--ro-bind", "/dev/null", f)
		}
	}
	a = append(a, "--tmpfs", s.Home)
	if cg != nil && cg.Bundle != "" {
		a = append(a, "--ro-bind", cg.Bundle, cg.Bundle)
	}
	a = append(a, "--ro-bind", repo, repo)
	for _, p := range gitMetadata([]string{repo}) {
		a = append(a, "--ro-bind", p, p)
	}
	for _, d := range deniedInside(repo, s.Home) {
		a = append(a, hide(d)...)
	}
	a = append(a, "--bind", index, index, "--chdir", repo, "--", bin)
	a = append(a, args...)
	env := []string{"HOME=" + s.Home}
	for _, k := range []string{"PATH", "LANG", "LC_ALL", "TZ"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	env = append(env, codeGraphEnv...)
	return s.Bwrap, a, env, nil
}

// writeServersConfig writes a temporary 0600 MCP config with only the given
// servers (no Colonyhall server, no token) and returns its path and cleanup.
func writeServersConfig(dir string, servers map[string]mcpServerDoc) (string, func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	raw, err := json.Marshal(mcpConfigDoc{MCPServers: servers})
	if err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(dir, "mcp-*.json")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

// trustedGitDir returns the git folder of a sandboxed agent's repository:
// repo/.git when it is a real folder, or the repository's .git folder when
// repo is a worktree whose pointer file passes worktreeCommonDir's back-link
// check. The agent can write the repository, so a .git pointer it planted
// (to a folder of its own with a crafted config, or elsewhere on the
// machine) is refused instead of being followed by git and by the exclude
// write.
func trustedGitDir(repo string) (string, error) {
	gf := filepath.Join(repo, ".git")
	fi, err := os.Lstat(gf)
	switch {
	case err != nil:
		return "", errors.New("not a git repository")
	case fi.IsDir():
		return gf, nil
	case !fi.Mode().IsRegular():
		return "", errors.New(".git is neither a folder nor a worktree link")
	}
	raw, err := os.ReadFile(gf)
	if err != nil {
		return "", err
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
	if !ok {
		return "", errors.New(".git is not a worktree link")
	}
	common := worktreeCommonDir(gf, strings.TrimSpace(gitdir))
	if common == "" {
		return "", errors.New(".git points to a folder that is not this worktree's")
	}
	return common, nil
}
