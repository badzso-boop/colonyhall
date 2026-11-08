package setup

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// dep is a program and its package per package manager.
type dep struct {
	Bin  string
	Pkgs map[string]string // apt, dnf, brew → package ("" = not on this manager)
}

var (
	depGit    = dep{Bin: "git", Pkgs: map[string]string{"apt": "git", "dnf": "git", "brew": "git"}}
	depCurl   = dep{Bin: "curl", Pkgs: map[string]string{"apt": "curl", "dnf": "curl", "brew": "curl"}}
	depTmux   = dep{Bin: "tmux", Pkgs: map[string]string{"apt": "tmux", "dnf": "tmux", "brew": "tmux"}}
	depBwrap  = dep{Bin: "bwrap", Pkgs: map[string]string{"apt": "bubblewrap", "dnf": "bubblewrap"}}
	depDocker = dep{Bin: "docker", Pkgs: map[string]string{"apt": "docker.io docker-compose-v2", "dnf": "moby-engine docker-compose"}}
)

// guardDeps are the guard's programs on this platform.
func guardDeps(p Platform) []dep {
	d := []dep{depGit, depCurl, depTmux}
	if p.Sandbox() {
		d = append(d, depBwrap)
	}
	return d
}

func missing(r Runner, deps []dep) []dep {
	var out []dep
	for _, d := range deps {
		if !r.Look(d.Bin) {
			out = append(out, d)
		}
	}
	return out
}

// installCmds are the package manager commands for the missing programs.
func installCmds(p Platform, deps []dep) ([]Cmd, []string) {
	var pkgs, manual []string
	for _, d := range deps {
		pkg := d.Pkgs[p.Manager]
		if pkg == "" {
			manual = append(manual, d.Bin)
			continue
		}
		pkgs = append(pkgs, strings.Fields(pkg)...)
	}
	slices.Sort(pkgs)
	pkgs = slices.Compact(pkgs)
	if len(pkgs) == 0 {
		return nil, manual
	}
	switch p.Manager {
	case "apt":
		return []Cmd{
			{Name: "apt-get", Args: []string{"update", "-q"}, Sudo: true},
			{Name: "apt-get", Args: append([]string{"install", "-y", "-q"}, pkgs...), Sudo: true, Env: []string{"DEBIAN_FRONTEND=noninteractive"}},
		}, manual
	case "dnf":
		return []Cmd{{Name: "dnf", Args: append([]string{"install", "-y"}, pkgs...), Sudo: true}}, manual
	case "brew":
		return []Cmd{{Name: "brew", Args: append([]string{"install"}, pkgs...)}}, manual
	}
	return nil, append(manual, pkgs...)
}

var errDepsMissing = errors.New("programs are missing")

// depsStep installs the missing programs after a confirmation.
func depsStep(id string, list func(e *Env) []dep) Step {
	return Step{
		ID:    id,
		Title: "",
		Done:  func(e *Env) (bool, error) { return len(missing(e.Run, list(e))) == 0, nil },
		Plan: func(e *Env) []string {
			cmds, _ := installCmds(e.P, missing(e.Run, list(e)))
			var out []string
			for _, c := range cmds {
				out = append(out, c.String())
			}
			return out
		},
		Confirm: "proceed",
		Apply: func(e *Env) error {
			miss := missing(e.Run, list(e))
			names := make([]string, 0, len(miss))
			for _, d := range miss {
				names = append(names, d.Bin)
			}
			e.UI.Say("depsMissing", strings.Join(names, ", "))
			cmds, manual := installCmds(e.P, miss)
			if len(manual) > 0 {
				if e.P.Manager == "" {
					e.UI.Say("depsNoManager")
				}
				e.UI.Say("depsManual", strings.Join(manual, ", "))
				return errDepsMissing
			}
			for _, c := range cmds {
				if _, err := e.Exec(c); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
