// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ErrNeedAnswer: an unattended run (--yes) reached a question that has no
// safe default (a password, a pairing code); give it as a flag or in the
// answers file.
var ErrNeedAnswer = errors.New("needs an answer")

// UI asks questions and prints messages in the user's language.
type UI struct {
	In  *bufio.Reader
	Out io.Writer
	Lang
	// Yes accepts every default and confirmation (unattended install).
	Yes bool
	// Hidden reads a line without echo (a terminal); nil reads normally.
	Hidden func() (string, error)
}

// NewTerminalUI reads from stdin and hides secrets with stty when stdin is
// a terminal.
func NewTerminalUI(lang Lang, yes bool) *UI {
	u := &UI{In: bufio.NewReader(os.Stdin), Out: os.Stdout, Lang: lang, Yes: yes}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		u.Hidden = func() (string, error) {
			_ = sttyEcho(false)
			defer func() { _ = sttyEcho(true); _, _ = fmt.Fprintln(u.Out) }()
			return u.line()
		}
	}
	return u
}

func sttyEcho(on bool) error {
	arg := "-echo"
	if on {
		arg = "echo"
	}
	c := exec.Command("stty", arg)
	c.Stdin = os.Stdin
	return c.Run()
}

func (u *UI) line() (string, error) {
	s, err := u.In.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || s == "") {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

// Say prints a message.
func (u *UI) Say(key string, args ...any) { _, _ = fmt.Fprintln(u.Out, u.T(key, args...)) }

// Sayf prints text that needs no translation (a path, a command).
func (u *UI) Sayf(format string, args ...any) { _, _ = fmt.Fprintf(u.Out, format+"\n", args...) }

// Ask asks for a value; empty input (or --yes) takes def.
func (u *UI) Ask(key, def string, args ...any) (string, error) {
	if u.Yes {
		if def == "" {
			return "", fmt.Errorf("%s: %w", u.T(key, args...), ErrNeedAnswer)
		}
		return def, nil
	}
	q := u.T(key, args...)
	if def != "" {
		q += " [" + def + "]"
	}
	_, _ = fmt.Fprint(u.Out, q+": ")
	s, err := u.line()
	if err != nil {
		return "", err
	}
	if s == "" {
		s = def
	}
	return s, nil
}

// Secret asks for a value without echo. It is never logged.
func (u *UI) Secret(key string, args ...any) (string, error) {
	if u.Yes {
		return "", fmt.Errorf("%s: %w", u.T(key, args...), ErrNeedAnswer)
	}
	_, _ = fmt.Fprint(u.Out, u.T(key, args...)+": ")
	if u.Hidden != nil {
		return u.Hidden()
	}
	return u.line()
}

// Confirm asks a yes/no question; --yes answers yes.
func (u *UI) Confirm(key string, def bool, args ...any) (bool, error) {
	if u.Yes {
		return true, nil
	}
	hint := " [y/N]"
	if def {
		hint = " [Y/n]"
	}
	if u.Lang == Hungarian {
		hint = strings.NewReplacer("y", "i", "Y", "I").Replace(hint)
	}
	_, _ = fmt.Fprint(u.Out, u.T(key, args...)+hint+" ")
	s, err := u.line()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(s) {
	case "":
		return def, nil
	case "y", "yes", "i", "igen":
		return true, nil
	default:
		return false, nil
	}
}

// Choose asks for one of the options (by number or value).
func (u *UI) Choose(key string, options []string, labels []string, def string) (string, error) {
	if u.Yes {
		return def, nil
	}
	_, _ = fmt.Fprintln(u.Out, u.T(key))
	for i, o := range options {
		mark := " "
		if o == def {
			mark = "*"
		}
		_, _ = fmt.Fprintf(u.Out, " %s %d) %s\n", mark, i+1, labels[i])
	}
	for {
		_, _ = fmt.Fprint(u.Out, "> ")
		s, err := u.line()
		if err != nil {
			return "", err
		}
		if s == "" {
			return def, nil
		}
		for i, o := range options {
			if s == o || s == fmt.Sprint(i+1) {
				return o, nil
			}
		}
	}
}
