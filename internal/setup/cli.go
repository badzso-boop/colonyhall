// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"colonyhall/internal/guard"
)

// Main runs `colony setup|doctor|upgrade|uninstall`; it returns the exit code.
func Main(ctx context.Context, cmd string, args []string, stdout io.Writer) int {
	e, err := newEnv(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "colony:", err)
		return 1
	}
	switch cmd {
	case "setup":
		err = runSetup(e, args)
	case "doctor":
		err = runDoctor(e, args, stdout)
	case "upgrade":
		err = runUpgrade(e, args)
	case "uninstall":
		err = runUninstall(e, args)
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	e.Close()
	if err != nil {
		if !errors.Is(err, errRestartWSL) && !errors.Is(err, errDepsMissing) {
			_, _ = fmt.Fprintln(os.Stderr, "colony "+cmd+":", e.Mask(err.Error()))
		}
		return 1
	}
	return 0
}

func newEnv(ctx context.Context) (*Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if s, err := filepath.EvalSymlinks(self); err == nil {
		self = s
	}
	r := ExecRunner{}
	return &Env{Ctx: ctx, Run: r, Home: home, Self: self, Version: guard.Version, P: Detect(r, nil, nil)}, nil
}

// setupFlags are `colony setup`'s flags; they override the answers file.
type setupFlags struct {
	server, guardOnly, both, yes, dry, upgrade bool
	answers, lang                              string
	serverURL, pairCode, name, roots, dir      string
}

func parseSetup(args []string) (setupFlags, error) {
	var f setupFlags
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.BoolVar(&f.server, "server", false, "install the server only")
	fs.BoolVar(&f.guardOnly, "guard", false, "install the guard only (for an existing server)")
	fs.BoolVar(&f.both, "both", false, "install the server and the guard on this machine")
	fs.BoolVar(&f.yes, "yes", false, "accept every default and confirmation (unattended)")
	fs.BoolVar(&f.dry, "dry-run", false, "print the plan, change nothing")
	fs.BoolVar(&f.upgrade, "upgrade", false, "with --server: move the server to this version (pull, restart; migrations run on start)")
	fs.StringVar(&f.answers, "answers", "", "a YAML answers file (see docs/16-telepites.md)")
	fs.StringVar(&f.lang, "lang", "", "en or hu (default: from the locale)")
	fs.StringVar(&f.serverURL, "server-url", "", "guard: the server's address")
	fs.StringVar(&f.pairCode, "pair-code", "", "guard: the pairing code from the Machines page")
	fs.StringVar(&f.name, "name", "", "guard: this machine's name")
	fs.StringVar(&f.roots, "roots", "", "guard: folders agents may be given, comma-separated")
	fs.StringVar(&f.dir, "server-dir", "", "server: its folder (compose.yaml, .env)")
	return f, fs.Parse(args)
}

func runSetup(e *Env, args []string) error {
	f, err := parseSetup(args)
	if err != nil {
		return err
	}
	lang := DetectLang(nil)
	if f.lang == "hu" || f.lang == "en" {
		lang = Lang(f.lang)
	}
	e.UI = NewTerminalUI(lang, f.yes)
	e.DryRun = f.dry
	if f.answers != "" {
		if e.Answers, err = LoadAnswers(f.answers, func(p string) { e.UI.Say("answersPerms", p) }); err != nil {
			return err
		}
	}
	a := &e.Answers
	switch {
	case f.server:
		a.Mode = "server"
	case f.guardOnly:
		a.Mode = "guard"
	case f.both:
		a.Mode = "both"
	}
	if f.serverURL != "" {
		a.Guard.ServerURL = f.serverURL
	}
	if f.pairCode != "" {
		a.Guard.PairCode = f.pairCode
	}
	if f.name != "" {
		a.Guard.Name = f.name
	}
	if f.roots != "" {
		a.Guard.Roots = strings.Split(f.roots, ",")
	}
	if f.dir != "" {
		a.Server.Dir = f.dir
	}
	if err := e.Open(); err != nil {
		return err
	}
	e.UI.Say("welcome", e.Version, platformName(e.P))
	if a.Mode == "" {
		if a.Mode, err = e.UI.Choose("mode", []string{"guard", "server", "both"},
			[]string{e.UI.T("modeGuard"), e.UI.T("modeServer"), e.UI.T("modeBoth")}, "guard"); err != nil {
			return err
		}
	}
	var steps []Step
	if a.Mode == "server" || a.Mode == "both" {
		if err := e.askServer(); err != nil {
			return err
		}
		steps = append(steps, e.ServerSteps(f.upgrade)...)
	}
	if a.Mode == "guard" || a.Mode == "both" {
		if err := e.askGuard(); err != nil {
			return err
		}
		steps = append(steps, e.GuardSteps(nil)...)
	}
	if err := e.Do(steps); err != nil {
		return err
	}
	if !e.DryRun {
		e.UI.Say("finished")
	}
	return nil
}

func platformName(p Platform) string {
	s := p.OS + "/" + p.Arch
	if p.WSL {
		s += ", WSL"
	}
	return s
}

func runDoctor(e *Env, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	e.UI = NewTerminalUI(DetectLang(nil), false)
	if err := e.Open(); err != nil {
		return err
	}
	checks := Doctor(e.Ctx, e)
	if *asJSON {
		raw, err := DoctorJSON(checks)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(raw))
		return err
	}
	failed := false
	for _, c := range checks {
		_, _ = fmt.Fprintln(stdout, c.Message(e.UI.Lang))
		failed = failed || c.Status == StatusFail
	}
	if failed {
		return errors.New("some checks failed")
	}
	return nil
}
