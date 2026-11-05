package setup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Cmd is one outside command. Stdin carries secrets (a password, a token)
// so they never become arguments; String never shows it.
type Cmd struct {
	Name  string
	Args  []string
	Stdin string
	Sudo  bool // run through sudo when not root
	Dir   string
	Env   []string // added to the environment
}

// String is the command as the plan and the log show it (no stdin).
func (c Cmd) String() string {
	parts := make([]string, 0, len(c.Args)+2)
	if c.Sudo {
		parts = append(parts, "sudo")
	}
	parts = append(parts, c.Name)
	for _, a := range c.Args {
		if a == "" || strings.ContainsAny(a, " \t\"'$`\\|&;<>()*?[]#~") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// Runner runs outside commands; the tests use FakeRunner.
type Runner interface {
	// Run returns stdout; on failure the error carries stderr's tail.
	Run(ctx context.Context, c Cmd) (string, error)
	// Look reports whether a program is on PATH.
	Look(name string) bool
}

// ExecRunner runs real commands.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, c Cmd) (string, error) {
	name, args := c.Name, c.Args
	if c.Sudo && os.Geteuid() != 0 {
		name, args = "sudo", append([]string{c.Name}, c.Args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	} else if c.Sudo {
		cmd.Stdin = os.Stdin // sudo may ask for the password
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if c.Sudo {
		cmd.Stderr = os.Stderr // the sudo password prompt must show
	}
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(errb.String())
		if len(tail) > 400 {
			tail = tail[len(tail)-400:]
		}
		return out.String(), fmt.Errorf("%s: %w: %s", c.Name, err, tail)
	}
	return out.String(), nil
}

// Look implements Runner.
func (ExecRunner) Look(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// FakeRunner records commands and answers them from a table (tests).
type FakeRunner struct {
	mu    sync.Mutex
	Cmds  []Cmd
	Have  map[string]bool // programs on PATH
	Reply func(c Cmd) (string, error)
}

// Run implements Runner.
func (f *FakeRunner) Run(_ context.Context, c Cmd) (string, error) {
	f.mu.Lock()
	f.Cmds = append(f.Cmds, c)
	f.mu.Unlock()
	if f.Reply != nil {
		return f.Reply(c)
	}
	return "", nil
}

// Look implements Runner.
func (f *FakeRunner) Look(name string) bool { return f.Have[name] }

// Lines is the recorded commands as strings.
func (f *FakeRunner) Lines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.Cmds))
	for _, c := range f.Cmds {
		out = append(out, c.String())
	}
	return out
}
