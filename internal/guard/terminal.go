package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"colonyhall/internal/platform/ids"
)

const (
	// DefaultTmuxSocket is the dedicated tmux socket used by Colonyhall.
	DefaultTmuxSocket = "colonyhall"

	// MaxTerminalsPerAgent is the maximum concurrent running terminals per agent.
	MaxTerminalsPerAgent = 5

	// MaxTerminalsPerHost is the maximum concurrent running terminals on this machine.
	MaxTerminalsPerHost = 20

	// MaxOutputFileSize caps output.log at 1 MB.
	MaxOutputFileSize = 1024 * 1024

	// MaxOutputLines is the maximum number of lines returned by term_output.
	MaxOutputLines = 1000

	// DefaultOutputLines is the default number of lines returned by term_output.
	DefaultOutputLines = 100

	// TerminalRetention is the time after which finished terminals are removed.
	TerminalRetention = 7 * 24 * time.Hour
)

var (
	// termNameRe matches ^[a-z0-9][a-z0-9-]{0,31}$
	termNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

	// ansiRegex matches ANSI escape codes (CSI, OSC, etc.)
	ansiRegex = regexp.MustCompile(`(\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[PX^_].*?\x1b\\|\x1b.)`)
)

// TerminalMeta contains persistent metadata for an agent terminal.
type TerminalMeta struct {
	AgentID       string     `json:"agent_id"`
	Handle        string     `json:"handle"`
	Name          string     `json:"name"`
	Command       string     `json:"command"`
	Cwd           string     `json:"cwd"`
	ChannelID     string     `json:"channel_id"`
	ThreadRootID  string     `json:"thread_root_id,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	KilledBy      string     `json:"killed_by,omitempty"`
	StartNotified bool       `json:"start_notified,omitempty"`
}

// IsRunning reports whether the terminal is currently active.
func (m *TerminalMeta) IsRunning() bool {
	return m.FinishedAt == nil
}

// Status returns one of: "running", "finished", "killed", "error".
func (m *TerminalMeta) Status() string {
	if m.FinishedAt == nil {
		return "running"
	}
	if m.KilledBy != "" {
		return "killed"
	}
	if m.ExitCode != nil && *m.ExitCode != 0 {
		return "error"
	}
	return "finished"
}

// Duration returns the total run duration of the terminal.
func (m *TerminalMeta) Duration(now time.Time) time.Duration {
	if m.StartedAt.IsZero() {
		return 0
	}
	end := now
	if m.FinishedAt != nil {
		end = *m.FinishedAt
	}
	return end.Sub(m.StartedAt)
}

// FilterTerminalOutput strips ANSI escape sequences and non-printable control
// characters, keeping newlines, carriage returns, and tabs.
func FilterTerminalOutput(s string) string {
	s = ansiRegex.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(r)
		} else if unicode.IsControl(r) {
			continue
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TerminalManager manages long-running agent tmux sessions, their state files,
// periodic watching, cleanup, and halt lifecycle.
type TerminalManager struct {
	mu         sync.Mutex
	stateDir   string
	socketName string
	roots      []string
	home       string
	clock      func() time.Time
	onStart    func(m *TerminalMeta)
	onFinish   func(m *TerminalMeta)
	running    map[string]*TerminalMeta // key: handle + "/" + name
}

// NewTerminalManager creates a manager storing state under stateDir/terminals.
func NewTerminalManager(stateDir string, roots []string, home string) *TerminalManager {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return &TerminalManager{
		stateDir:   stateDir,
		socketName: DefaultTmuxSocket,
		roots:      roots,
		home:       home,
		clock:      time.Now,
		running:    make(map[string]*TerminalMeta),
	}
}

// SetSocketName configures the tmux socket name (default "colonyhall").
func (tm *TerminalManager) SetSocketName(name string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.socketName = name
}

// SetOnStart registers a callback invoked whenever a terminal starts.
func (tm *TerminalManager) SetOnStart(fn func(m *TerminalMeta)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onStart = fn
}

// SetOnFinish registers a callback invoked whenever a terminal completes or is killed.
func (tm *TerminalManager) SetOnFinish(fn func(m *TerminalMeta)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onFinish = fn
}

// SetClock sets a custom clock for testing.
func (tm *TerminalManager) SetClock(fn func() time.Time) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.clock = fn
}

func (tm *TerminalManager) now() time.Time {
	if tm.clock != nil {
		return tm.clock()
	}
	return time.Now()
}

func cleanHandle(h string) string {
	return strings.TrimPrefix(strings.TrimSpace(h), "@")
}

func (tm *TerminalManager) terminalsDir() string {
	base := filepath.Clean(tm.stateDir)
	if filepath.Base(base) == "terminals" {
		return base
	}
	return filepath.Join(base, "terminals")
}

func (tm *TerminalManager) terminalDir(handle, name string) string {
	return filepath.Join(tm.terminalsDir(), cleanHandle(handle), name)
}

func (tm *TerminalManager) sessionName(handle, name string) string {
	return fmt.Sprintf("colony-%s-%s", cleanHandle(handle), name)
}

func (tm *TerminalManager) hasSession(session string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "-L", tm.socketName, "has-session", "-t", session)
	return cmd.Run() == nil
}

func (tm *TerminalManager) killSession(session string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "-L", tm.socketName, "kill-session", "-t", session)
	return cmd.Run()
}

// Start launches a new tmux terminal for the agent.
func (tm *TerminalManager) Start(agentID, handle, name, command, cwd, channelID, threadRootID string, env []string) (*TerminalMeta, error) {
	handle = cleanHandle(handle)
	if handle == "" {
		return nil, errors.New("agent handle is required")
	}
	if !termNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid terminal name %q: must match ^[a-z0-9][a-z0-9-]{0,31}$", name)
	}
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("command is required")
	}

	// Validate cwd against allowed roots
	if cwd == "" {
		cwd = tm.home
	}
	realCwd, reason := CheckGrant(cwd, tm.roots, tm.home)
	if reason != "" {
		return nil, fmt.Errorf("cwd %q outside allowed scope: %s", cwd, reason)
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check limits
	totalRunning := len(tm.running)
	if totalRunning >= MaxTerminalsPerHost {
		return nil, fmt.Errorf("machine limit reached: maximum %d running terminals", MaxTerminalsPerHost)
	}

	key := handle + "/" + name
	if _, exists := tm.running[key]; exists {
		return nil, fmt.Errorf("terminal %q is already running", name)
	}

	agentRunning := 0
	for _, m := range tm.running {
		if m.Handle == handle {
			agentRunning++
		}
	}
	if agentRunning >= MaxTerminalsPerAgent {
		return nil, fmt.Errorf("agent limit reached: maximum %d running terminals", MaxTerminalsPerAgent)
	}

	termDir := tm.terminalDir(handle, name)
	// If a previous finished terminal exists with the same name, replace it and its files.
	if fi, err := os.Stat(termDir); err == nil && fi.IsDir() {
		_ = os.RemoveAll(termDir)
	}
	if err := os.MkdirAll(termDir, 0o700); err != nil {
		return nil, fmt.Errorf("create terminal directory: %w", err)
	}

	now := tm.now()
	meta := &TerminalMeta{
		AgentID:      agentID,
		Handle:       handle,
		Name:         name,
		Command:      command,
		Cwd:          realCwd,
		ChannelID:    channelID,
		ThreadRootID: threadRootID,
		StartedAt:    now,
	}

	if err := tm.saveMetaLocked(meta); err != nil {
		_ = os.RemoveAll(termDir)
		return nil, fmt.Errorf("save meta: %w", err)
	}

	session := tm.sessionName(handle, name)
	_ = tm.killSession(session) // ensure clean state

	exitPath := filepath.Join(termDir, "exit")
	logPath := filepath.Join(termDir, "output.log")
	shellCmd := fmt.Sprintf("trap 'echo $? > %s' EXIT; (%s)", exitPath, command)

	tmuxArgs := []string{
		"-L", tm.socketName,
		"new-session", "-d", "-s", session, "-c", realCwd, "-x", "200", "-y", "50",
		"--", "sh", "-c", shellCmd,
		";", "pipe-pane", "-o", "-t", session, "cat >> " + logPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", tmuxArgs...)
	if len(env) > 0 {
		cmd.Env = env
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(termDir)
		return nil, fmt.Errorf("tmux start failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	tm.running[key] = meta
	if tm.onStart != nil && !meta.StartNotified {
		meta.StartNotified = true
		_ = tm.saveMetaLocked(meta)
		cb := tm.onStart
		go cb(meta)
	}
	return meta, nil
}

func (tm *TerminalManager) saveMetaLocked(m *TerminalMeta) error {
	termDir := tm.terminalDir(m.Handle, m.Name)
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	metaPath := filepath.Join(termDir, "meta.json")
	return os.WriteFile(metaPath, raw, 0o600)
}

func (tm *TerminalManager) loadMeta(handle, name string) (*TerminalMeta, error) {
	metaPath := filepath.Join(tm.terminalDir(handle, name), "meta.json")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var m TerminalMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Get returns the metadata for a specific terminal.
func (tm *TerminalManager) Get(handle, name string) (*TerminalMeta, error) {
	handle = cleanHandle(handle)
	tm.mu.Lock()
	key := handle + "/" + name
	if m, ok := tm.running[key]; ok {
		cp := *m
		tm.mu.Unlock()
		return &cp, nil
	}
	tm.mu.Unlock()

	return tm.loadMeta(handle, name)
}

// List returns all terminals for a handle, or all terminals if handle is empty.
func (tm *TerminalManager) List(handle string) ([]*TerminalMeta, error) {
	handle = cleanHandle(handle)
	var handles []string

	root := tm.terminalsDir()
	if handle != "" {
		handles = []string{handle}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				handles = append(handles, e.Name())
			}
		}
	}

	var result []*TerminalMeta
	for _, h := range handles {
		hDir := filepath.Join(root, h)
		entries, err := os.ReadDir(hDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			m, err := tm.Get(h, e.Name())
			if err == nil && m != nil {
				result = append(result, m)
			}
		}
	}

	// Sort by StartedAt desc
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result, nil
}

// Output reads up to lines (default 100, max 1000) from output.log, filtered.
func (tm *TerminalManager) Output(handle, name string, lines int) (string, error) {
	handle = cleanHandle(handle)
	logPath := filepath.Join(tm.terminalDir(handle, name), "output.log")
	raw, err := readLastLines(logPath, lines)
	if err != nil {
		return "", err
	}
	return FilterTerminalOutput(raw), nil
}

// Send types text into the running terminal followed by an Enter key.
func (tm *TerminalManager) Send(handle, name, text string) error {
	handle = cleanHandle(handle)
	session := tm.sessionName(handle, name)
	if !tm.hasSession(session) {
		return fmt.Errorf("terminal %q is not running", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Type text literally
	cmd := exec.CommandContext(ctx, "tmux", "-L", tm.socketName, "send-keys", "-l", "-t", session, "--", text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	// Send enter
	cmdEnter := exec.CommandContext(ctx, "tmux", "-L", tm.socketName, "send-keys", "-t", session, "Enter")
	if out, err := cmdEnter.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys Enter failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Kill terminates a running terminal and records killed_by.
func (tm *TerminalManager) Kill(handle, name, killedBy string) error {
	handle = cleanHandle(handle)
	session := tm.sessionName(handle, name)
	_ = tm.killSession(session)

	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := handle + "/" + name
	now := tm.now()

	meta, ok := tm.running[key]
	if !ok {
		var err error
		meta, err = tm.loadMeta(handle, name)
		if err != nil {
			return err
		}
	}

	if meta.FinishedAt == nil {
		meta.FinishedAt = &now
		meta.KilledBy = killedBy
		_ = tm.saveMetaLocked(meta)
		_ = trimOutputFile(filepath.Join(tm.terminalDir(handle, name), "output.log"), MaxOutputFileSize)
	}
	delete(tm.running, key)

	if tm.onFinish != nil {
		cb := tm.onFinish
		go cb(meta)
	}
	return nil
}

// KillAllForHandle stops all active terminals belonging to an agent (e.g. on unassign or halt).
func (tm *TerminalManager) KillAllForHandle(handle, killedBy string) {
	handle = cleanHandle(handle)
	tm.mu.Lock()
	var targets []*TerminalMeta
	for _, m := range tm.running {
		if m.Handle == handle {
			targets = append(targets, m)
		}
	}
	tm.mu.Unlock()

	for _, m := range targets {
		_ = tm.Kill(m.Handle, m.Name, killedBy)
	}
}

// KillAll terminates all running terminals across all agents (emergency halt).
func (tm *TerminalManager) KillAll(killedBy string) {
	tm.mu.Lock()
	var targets []*TerminalMeta
	for _, m := range tm.running {
		targets = append(targets, m)
	}
	tm.mu.Unlock()

	for _, m := range targets {
		_ = tm.Kill(m.Handle, m.Name, killedBy)
	}
}

// CheckHalted kills terminals of any agent present in the halted map.
func (tm *TerminalManager) CheckHalted(haltedAgents map[string]bool) {
	tm.mu.Lock()
	var targets []*TerminalMeta
	for _, m := range tm.running {
		if haltedAgents[m.AgentID] {
			targets = append(targets, m)
		}
	}
	tm.mu.Unlock()

	for _, m := range targets {
		_ = tm.Kill(m.Handle, m.Name, "halt")
	}
}

// AdoptExisting discovers existing terminals on disk upon guard startup.
func (tm *TerminalManager) AdoptExisting() error {
	root := tm.terminalsDir()
	hEntries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := tm.now()
	var finishedReports []*TerminalMeta

	for _, he := range hEntries {
		if !he.IsDir() {
			continue
		}
		hDir := filepath.Join(root, he.Name())
		tEntries, err := os.ReadDir(hDir)
		if err != nil {
			continue
		}
		for _, te := range tEntries {
			if !te.IsDir() {
				continue
			}
			meta, err := tm.loadMeta(he.Name(), te.Name())
			if err != nil || meta == nil {
				continue
			}
			if meta.FinishedAt != nil {
				continue
			}

			session := tm.sessionName(meta.Handle, meta.Name)
			termDir := tm.terminalDir(meta.Handle, meta.Name)
			exitPath := filepath.Join(termDir, "exit")
			exitData, exitErr := os.ReadFile(exitPath)
			hasExit := exitErr == nil
			alive := tm.hasSession(session)

			if alive && !hasExit {
				// Still running: adopt into watcher
				tm.running[meta.Handle+"/"+meta.Name] = meta
			} else {
				// Finished while guard was offline
				meta.FinishedAt = &now
				if hasExit {
					if code, parseErr := strconv.Atoi(strings.TrimSpace(string(exitData))); parseErr == nil {
						meta.ExitCode = &code
					}
				}
				_ = tm.killSession(session)
				_ = trimOutputFile(filepath.Join(termDir, "output.log"), MaxOutputFileSize)
				_ = tm.saveMetaLocked(meta)
				finishedReports = append(finishedReports, meta)
			}
		}
	}

	if tm.onFinish != nil && len(finishedReports) > 0 {
		cb := tm.onFinish
		go func() {
			for _, m := range finishedReports {
				cb(m)
			}
		}()
	}
	return nil
}

func (tm *TerminalManager) scanRunningLocked() []*TerminalMeta {
	root := tm.terminalsDir()
	hEntries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var started []*TerminalMeta
	for _, he := range hEntries {
		if !he.IsDir() {
			continue
		}
		hDir := filepath.Join(root, he.Name())
		tEntries, err := os.ReadDir(hDir)
		if err != nil {
			continue
		}
		for _, te := range tEntries {
			if !te.IsDir() {
				continue
			}
			key := he.Name() + "/" + te.Name()
			if _, exists := tm.running[key]; exists {
				continue
			}
			meta, err := tm.loadMeta(he.Name(), te.Name())
			if err != nil || meta == nil || meta.FinishedAt != nil {
				continue
			}
			session := tm.sessionName(meta.Handle, meta.Name)
			termDir := tm.terminalDir(meta.Handle, meta.Name)
			exitPath := filepath.Join(termDir, "exit")
			_, exitErr := os.ReadFile(exitPath)
			hasExit := exitErr == nil
			alive := tm.hasSession(session)
			if alive && !hasExit {
				tm.running[key] = meta
				if !meta.StartNotified {
					meta.StartNotified = true
					_ = tm.saveMetaLocked(meta)
					started = append(started, meta)
				}
			}
		}
	}
	return started
}

// WatchTick checks active terminals once and cleans up aged records.
func (tm *TerminalManager) WatchTick() {
	tm.mu.Lock()
	newStarted := tm.scanRunningLocked()
	var active []*TerminalMeta
	for _, m := range tm.running {
		active = append(active, m)
	}
	now := tm.now()
	tm.mu.Unlock()

	if tm.onStart != nil && len(newStarted) > 0 {
		cb := tm.onStart
		for _, m := range newStarted {
			cb(m)
		}
	}

	var finished []*TerminalMeta

	for _, meta := range active {
		termDir := tm.terminalDir(meta.Handle, meta.Name)
		exitPath := filepath.Join(termDir, "exit")
		logPath := filepath.Join(termDir, "output.log")
		session := tm.sessionName(meta.Handle, meta.Name)

		exitData, exitErr := os.ReadFile(exitPath)
		hasExit := exitErr == nil
		alive := tm.hasSession(session)

		// Trim active log if it exceeded 1 MB
		_ = trimOutputFile(logPath, MaxOutputFileSize)

		if hasExit || !alive {
			tm.mu.Lock()
			meta.FinishedAt = &now
			if hasExit {
				if code, parseErr := strconv.Atoi(strings.TrimSpace(string(exitData))); parseErr == nil {
					meta.ExitCode = &code
				}
			}
			_ = tm.killSession(session)
			_ = tm.saveMetaLocked(meta)
			delete(tm.running, meta.Handle+"/"+meta.Name)
			tm.mu.Unlock()

			finished = append(finished, meta)
		}
	}

	if tm.onFinish != nil && len(finished) > 0 {
		cb := tm.onFinish
		for _, m := range finished {
			cb(m)
		}
	}

	// 7-day retention cleanup
	tm.cleanupAged(now)
}

func (tm *TerminalManager) cleanupAged(now time.Time) {
	root := tm.terminalsDir()
	hEntries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, he := range hEntries {
		if !he.IsDir() {
			continue
		}
		hDir := filepath.Join(root, he.Name())
		tEntries, err := os.ReadDir(hDir)
		if err != nil {
			continue
		}
		for _, te := range tEntries {
			if !te.IsDir() {
				continue
			}
			tDir := filepath.Join(hDir, te.Name())
			meta, err := tm.loadMeta(he.Name(), te.Name())
			if err != nil || meta == nil {
				continue
			}
			if meta.FinishedAt != nil && now.Sub(*meta.FinishedAt) > TerminalRetention {
				_ = os.RemoveAll(tDir)
			}
		}
	}
}

// StartWatcher starts a background ticker running WatchTick every interval.
func (tm *TerminalManager) StartWatcher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tm.WatchTick()
			}
		}
	}()
}

func trimOutputFile(path string, maxSize int64) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Size() <= maxSize {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	offset := fi.Size() - maxSize
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if idx := bytes.IndexByte(buf, '\n'); idx >= 0 && idx < len(buf)-1 {
		buf = buf[idx+1:]
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, buf, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readLastLines(path string, maxLines int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if maxLines <= 0 {
		maxLines = DefaultOutputLines
	}
	if maxLines > MaxOutputLines {
		maxLines = MaxOutputLines
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// HandleTerminalFinish posts the completion message into the chat thread and wakes the agent.
func HandleTerminalFinish(ctx context.Context, client *Client, tm *TerminalManager, meta *TerminalMeta) error {
	if client == nil || meta == nil {
		return nil
	}
	now := time.Now()
	dur := meta.Duration(now)
	totalSecs := int(dur.Seconds())
	if totalSecs < 0 {
		totalSecs = 0
	}
	mins := totalSecs / 60
	secs := totalSecs % 60
	durStr := fmt.Sprintf("%02d:%02d", mins, secs)
	if mins >= 60 {
		hours := mins / 60
		mins = mins % 60
		durStr = fmt.Sprintf("%d:%02d:%02d", hours, mins, secs)
	}

	var body string
	if meta.KilledBy != "" {
		body = fmt.Sprintf("⛔ %s leállítva", meta.Name)
	} else if meta.ExitCode != nil && *meta.ExitCode == 0 {
		body = fmt.Sprintf("⚙️ %s kész · exit 0 · %s", meta.Name, durStr)
	} else if meta.ExitCode != nil {
		body = fmt.Sprintf("⚠️ %s hibával állt le · exit %d · %s", meta.Name, *meta.ExitCode, durStr)
	} else {
		body = fmt.Sprintf("⚠️ %s hibával állt le · %s", meta.Name, durStr)
	}

	clientMsgID := ids.V7{}.New().String()
	_ = client.Send(ctx, meta.ChannelID, meta.ThreadRootID, body, clientMsgID)

	tail := ""
	if tm != nil {
		out, err := tm.Output(meta.Handle, meta.Name, 40)
		if err == nil {
			tail = out
		}
	}

	return client.CreateTerminalEvent(ctx, meta.Name, meta.ChannelID, meta.ThreadRootID, meta.ExitCode, dur.Milliseconds(), tail)
}

// HandleTerminalStart posts the start message into the chat thread with silent=true (wakes nobody).
func HandleTerminalStart(ctx context.Context, client *Client, meta *TerminalMeta) error {
	if client == nil || meta == nil {
		return nil
	}
	return client.CreateTerminalStart(ctx, meta.Name, Redact(clip(meta.Command, 120)), meta.ChannelID, meta.ThreadRootID)
}
