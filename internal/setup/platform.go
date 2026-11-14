// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"os"
	"runtime"
	"strings"
)

// Platform is what the installer needs to know about the machine.
type Platform struct {
	OS      string // linux, darwin
	Arch    string
	WSL     bool
	Distro  string // WSL_DISTRO_NAME
	Manager string // apt, dnf, brew, or "" (print the commands)
	Systemd bool   // systemd runs as PID 1 (Linux, WSL with systemd)
	Root    bool
}

// FS is the read access Detect needs (tests give a map).
type FS interface {
	ReadFile(path string) ([]byte, error)
	Exists(path string) bool
}

type osFS struct{}

func (osFS) ReadFile(p string) ([]byte, error) { return os.ReadFile(p) }
func (osFS) Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// MapFS is an in-memory FS for tests.
type MapFS map[string]string

// ReadFile implements FS.
func (m MapFS) ReadFile(p string) ([]byte, error) {
	if s, ok := m[p]; ok {
		return []byte(s), nil
	}
	return nil, os.ErrNotExist
}

// Exists implements FS.
func (m MapFS) Exists(p string) bool { _, ok := m[p]; return ok }

// Detect looks at the machine.
func Detect(r Runner, fsys FS, getenv func(string) string) Platform {
	if fsys == nil {
		fsys = osFS{}
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	p := Platform{OS: runtime.GOOS, Arch: runtime.GOARCH, Root: os.Geteuid() == 0}
	if p.OS == "linux" {
		p.Distro = getenv("WSL_DISTRO_NAME")
		p.WSL = p.Distro != "" || fsys.Exists("/proc/sys/fs/binfmt_misc/WSLInterop")
		if comm, err := fsys.ReadFile("/proc/1/comm"); err == nil {
			p.Systemd = strings.TrimSpace(string(comm)) == "systemd"
		}
	}
	switch {
	case p.OS == "darwin" && r.Look("brew"):
		p.Manager = "brew"
	case r.Look("apt-get"):
		p.Manager = "apt"
	case r.Look("dnf"):
		p.Manager = "dnf"
	}
	return p
}

// ServiceKind is the service manager the guard uses here.
func (p Platform) ServiceKind() string {
	if p.OS == "darwin" {
		return "launchd"
	}
	return "systemd"
}

// Sandbox reports whether agents can be sandboxed here (bwrap on Linux).
func (p Platform) Sandbox() bool { return p.OS == "linux" }
