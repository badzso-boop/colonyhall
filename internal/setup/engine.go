// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Env is everything a step may use. Tests fill it with a FakeRunner, a
// scripted UI and a temporary home.
type Env struct {
	Ctx     context.Context
	Run     Runner
	UI      *UI
	P       Platform
	DryRun  bool
	Home    string
	Self    string // this colony binary
	Version string
	HTTP    *http.Client
	Now     func() time.Time
	LogPath string
	state   *State
	secrets []string
	logFile *os.File
	plan    []string
	Answers Answers
	// ownerPassword is the owner's password while setup runs (setup "both"
	// pairs the machine as them); never written anywhere.
	ownerPassword string
}

// Step is one idempotent piece of the install.
type Step struct {
	ID    string
	Title string
	// Done reports whether the step's result is already in place.
	Done func(e *Env) (bool, error)
	// Plan lists what Apply would do (commands, files), for --dry-run and
	// the confirmation.
	Plan func(e *Env) []string
	// Apply does it.
	Apply func(e *Env) error
	// Confirm: ask before applying (it installs packages, uses sudo, …).
	Confirm string
}

// State remembers the finished steps, so a run after a failure continues.
type State struct {
	Path string          `json:"-"`
	Done map[string]bool `json:"done"`
}

func loadState(path string) *State {
	s := &State{Path: path, Done: map[string]bool{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, s)
		if s.Done == nil {
			s.Done = map[string]bool{}
		}
	}
	return s
}

func (s *State) save() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(s.Path, raw, 0o600)
}

// stateDir is ~/.local/state/colonyhall.
func stateDir(home string) string { return filepath.Join(home, ".local", "state", "colonyhall") }

// Open prepares the log and the state.
func (e *Env) Open() error {
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.HTTP == nil {
		e.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	dir := stateDir(e.Home)
	e.state = loadState(filepath.Join(dir, "setup-state.json"))
	if e.DryRun {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	e.LogPath = filepath.Join(dir, "setup.log")
	f, err := os.OpenFile(e.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	e.logFile = f
	e.Logf("---- colony setup %s, %s", e.Version, e.Now().Format(time.RFC3339))
	return nil
}

// Close closes the log.
func (e *Env) Close() {
	if e.logFile != nil {
		_ = e.logFile.Close()
	}
}

// Secret registers a value the log must never show.
func (e *Env) Secret(s string) {
	if len(s) >= 4 {
		e.secrets = append(e.secrets, s)
	}
}

var tokenLike = regexp.MustCompile(`(col_|colh_|sk-ant-[a-z0-9]*-|ghp_|github_pat_|gho_)[A-Za-z0-9_\-]{6,}`)

// Mask hides the registered secrets and anything token-like.
func (e *Env) Mask(s string) string {
	for _, sec := range e.secrets {
		s = strings.ReplaceAll(s, sec, "***")
	}
	return tokenLike.ReplaceAllString(s, "$1***")
}

// Logf writes a masked line to the log file.
func (e *Env) Logf(format string, args ...any) {
	if e.logFile == nil {
		return
	}
	_, _ = fmt.Fprintf(e.logFile, "%s %s\n", e.Now().Format("15:04:05"), e.Mask(fmt.Sprintf(format, args...)))
}

// Exec runs a command (logged, masked); in a dry run it only plans it.
func (e *Env) Exec(c Cmd) (string, error) {
	if e.DryRun {
		e.plan = append(e.plan, c.String())
		return "", nil
	}
	e.Logf("run: %s", c.String())
	out, err := e.Run.Run(e.Ctx, c)
	if err != nil {
		e.Logf("failed: %s", err)
	}
	return out, err
}

// WriteFile writes a file with perm (logged without its content).
func (e *Env) WriteFile(path string, data []byte, perm os.FileMode) error {
	if e.DryRun {
		e.plan = append(e.plan, fmt.Sprintf("write %s (%o)", path, perm))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// A new temporary file (created 0600) instead of path+".new": an
	// existing .new kept its old mode while the secret was written into it,
	// and a symlink there was followed (round 40).
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	e.Logf("write: %s", path)
	return os.Rename(tmp, path)
}

// Do runs the steps in order: a finished step is skipped, a step that needs
// confirmation asks first, a failure stops the run (the state lets the next
// run continue there).
func (e *Env) Do(steps []Step) error {
	if e.DryRun {
		e.UI.Say("dryRun")
	}
	for _, s := range steps {
		done := e.state.Done[s.ID]
		if s.Done != nil {
			d, err := s.Done(e)
			if err != nil {
				return e.fail(s, err)
			}
			done = d
		}
		if done {
			e.UI.Say("stepDone", s.Title)
			continue
		}
		if e.DryRun {
			// Apply runs with Exec and WriteFile only recording; a step with
			// other side effects checks e.DryRun itself.
			e.UI.Say("stepApply", s.Title)
			before := len(e.plan)
			if s.Apply != nil {
				if err := s.Apply(e); err != nil && !errors.Is(err, ErrNeedAnswer) {
					return e.fail(s, err)
				}
			}
			for _, line := range e.plan[before:] {
				e.UI.Sayf("    %s", line)
			}
			continue
		}
		if s.Confirm != "" {
			if lines := planOf(e, s); len(lines) > 0 {
				e.UI.Say("willRun")
				for _, line := range lines {
					e.UI.Sayf("    %s", line)
				}
			}
			ok, err := e.UI.Confirm(s.Confirm, true)
			if err != nil {
				return e.fail(s, err)
			}
			if !ok {
				e.UI.Say("skipped", s.Title)
				e.Logf("skipped: %s", s.ID)
				continue
			}
		}
		e.UI.Say("stepApply", s.Title)
		if s.Apply != nil {
			if err := s.Apply(e); err != nil {
				return e.fail(s, err)
			}
		}
		e.state.Done[s.ID] = true
		if err := e.state.save(); err != nil {
			return err
		}
		e.Logf("done: %s", s.ID)
	}
	return nil
}

func planOf(e *Env, s Step) []string {
	if s.Plan == nil {
		return nil
	}
	return s.Plan(e)
}

func (e *Env) fail(s Step, err error) error {
	e.Logf("step %s failed: %v", s.ID, err)
	e.UI.Say("stepFailed", s.Title, e.Mask(err.Error()))
	if e.LogPath != "" {
		e.UI.Say("resume", e.LogPath)
	}
	return fmt.Errorf("%s: %w", s.ID, err)
}

// Forget clears steps from the state (a re-run of an upgrade, for example).
func (e *Env) Forget(ids ...string) {
	for k := range e.state.Done {
		if slices.Contains(ids, k) {
			delete(e.state.Done, k)
		}
	}
}
