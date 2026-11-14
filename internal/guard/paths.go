// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Grant is a folder the server lets an agent use on this machine.
type Grant struct {
	Path  string `json:"path"`
	Mode  string `json:"mode"` // read | write
	Write bool   `json:"-"`
}

// deniedHome lists folders under the user's home that no agent may see,
// whatever the server grants: keys, cloud credentials, other CLIs' logins.
// Round 40 added the guard's own folder (the host token, the Claude login
// token, the agents' Gitea tokens) and the AI clients' login files: they
// were hidden inside the sandbox only, so a grant of ~/.config or a file
// fetch on a full-access agent could still read them.
var deniedHome = []string{".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker", ".config/gh", ".config/gcloud",
	".netrc", ".git-credentials", ".password-store", ".local/share/keyrings", ".colonyhall",
	".config/colonyhall", ".codex", ".pgpass", ".npmrc", ".pypirc"}

// ResolveRoots cleans the machine owner's root folders and resolves
// symlinks; a root that does not exist is an error (a typo must not
// silently grant nothing or something else).
func ResolveRoots(roots []string) ([]string, error) {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if !filepath.IsAbs(r) {
			return nil, fmt.Errorf("guard: root %q must be an absolute path", r)
		}
		real, err := filepath.EvalSymlinks(filepath.Clean(r))
		if err != nil {
			return nil, fmt.Errorf("guard: root %q: %w", r, err)
		}
		out = append(out, real)
	}
	return out, nil
}

func within(p, root string) bool {
	return p == root || root == "/" || strings.HasPrefix(p, root+string(filepath.Separator))
}

// CheckGrant resolves a granted path on this machine and checks it against
// the roots and the deny list. It returns the real path or a reason.
func CheckGrant(path string, roots []string, home string) (string, string) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", "not an absolute, clean path"
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "does not exist on this machine"
	}
	inside := false
	for _, r := range roots {
		if within(real, r) {
			inside = true
			break
		}
	}
	if !inside {
		return "", "outside the root folders of this machine"
	}
	for _, d := range deniedPaths(home) {
		if within(real, d) {
			return "", "on the deny list (" + d + ")"
		}
	}
	return real, ""
}

// deniedPaths returns the deny list as absolute, resolved paths.
func deniedPaths(home string) []string {
	if home == "" {
		return nil
	}
	out := make([]string, 0, len(deniedHome)+len(clientConfig))
	for _, d := range append(append([]string{}, deniedHome...), clientConfig...) {
		p := filepath.Join(home, d)
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		out = append(out, p)
	}
	return out
}

// deniedInside returns the existing denied folders and files that lie
// inside a granted folder; the sandbox hides them.
func deniedInside(grant, home string) []string {
	var out []string
	for _, d := range deniedPaths(home) {
		if within(d, grant) && d != grant {
			if _, err := os.Lstat(d); err == nil {
				out = append(out, d)
			}
		}
	}
	// Secrets at the top of a granted folder (.env, .env.local, …).
	if matches, err := filepath.Glob(filepath.Join(grant, ".env*")); err == nil {
		out = append(out, matches...)
	}
	return out
}

// CheckExchangeDir verifies that dir exists, is a directory, and is writable
// by creating and removing a temporary test file.
func CheckExchangeDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("exchange_dir is empty")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	f, err := os.CreateTemp(dir, ".colony-exchange-check-*")
	if err != nil {
		return fmt.Errorf("exchange_dir is not writable: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}
