// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Round 40: the guard's own tokens and the AI clients' login files are on
// the one deny list that grants, the sandbox, file fetch and the browser
// share; before, they were hidden inside the sandbox only.
func secretHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, f := range []string{".config/colonyhall/host.token", ".config/colonyhall/gitea/token", ".claude/.credentials.json",
		".claude.json", ".gemini/antigravity-cli/antigravity-oauth-token", ".codex/auth.json", "project/main.go", ".config/other/x"} {
		p := filepath.Join(home, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestGuardSecretsCannotBeGranted(t *testing.T) {
	home := secretHome(t)
	roots := []string{home}
	for _, p := range []string{".config/colonyhall", ".config/colonyhall/gitea", ".claude", ".gemini", ".codex"} {
		if _, reason := CheckGrant(filepath.Join(home, p), roots, home); !strings.Contains(reason, "deny list") {
			t.Errorf("a grant of ~/%s must be refused, got %q", p, reason)
		}
	}
	// A broad grant is allowed, but the secrets inside it are masked.
	if _, reason := CheckGrant(filepath.Join(home, ".config"), roots, home); reason != "" {
		t.Fatalf("~/.config itself: %s", reason)
	}
	inside := deniedInside(filepath.Join(home, ".config"), home)
	if !slices.Contains(inside, filepath.Join(home, ".config/colonyhall")) {
		t.Fatalf("~/.config/colonyhall is not masked in a grant of ~/.config: %v", inside)
	}
}

func TestClientLoginFilesCannotBeFetched(t *testing.T) {
	home := secretHome(t)
	roots := []string{home}
	for _, p := range []string{".claude/.credentials.json", ".claude.json", ".gemini/antigravity-cli/antigravity-oauth-token",
		".codex/auth.json", ".config/colonyhall/host.token", ".config/colonyhall/gitea/token"} {
		// A full-access agent: its scope is every root of the machine.
		if _, _, _, code := CheckFileFetch(filepath.Join(home, p), roots, nil, false, home, nil, ""); code == "" {
			t.Errorf("~/%s was fetchable", p)
		}
	}
	if _, _, _, code := CheckFileFetch(filepath.Join(home, "project/main.go"), roots, nil, false, home, nil, ""); code != "" {
		t.Fatalf("an ordinary file: %s", code)
	}
}

func TestSecretNamesIgnoreCase(t *testing.T) {
	for _, n := range []string{".ENV", ".Env.Local", "Credentials.json", ".credentials.json", "ID_RSA", "server.PEM", "id_ecdsa", "vault.kdbx", ".pgpass", ".npmrc"} {
		if !isSecretName(n) {
			t.Errorf("%s is a secret name", n)
		}
	}
	for _, n := range []string{"main.go", "README.md", "environment.go"} {
		if isSecretName(n) {
			t.Errorf("%s is not a secret name", n)
		}
	}
}

func TestSandboxHidesMovedTokenFiles(t *testing.T) {
	home := secretHome(t)
	moved := filepath.Join(home, "project", "host.token") // guard.yaml host_token_file elsewhere
	if err := os.WriteFile(moved, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sb := &Sandbox{Bwrap: "/usr/bin/bwrap", Home: home, Root: filepath.Join(home, "sandbox"), Hide: []string{moved, ""}}
	agentDir := filepath.Join(home, "sandbox", "bot")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	args, _, err := sb.Command("claude-code", agentDir, filepath.Join(home, "project"), []Grant{{Path: home, Write: true}}, "/bin/true", nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--ro-bind /dev/null " + moved, "--tmpfs " + filepath.Join(home, ".config/colonyhall"), "--tmpfs " + filepath.Join(home, ".claude")} {
		if !strings.Contains(joined, want) {
			t.Errorf("sandbox arguments lack %q", want)
		}
	}
	// The hiding comes after the grant, or the grant would reveal it again.
	if strings.LastIndex(joined, "--bind "+home+" "+home) > strings.LastIndex(joined, "--ro-bind /dev/null "+moved) {
		t.Fatal("the token file is hidden before the grant is mounted")
	}
}

// Round 40: a rewritten token file is 0600 even when the old one was not,
// and a symlink at the path is replaced, not followed.
func TestWritePrivateFixesModeAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	tok := filepath.Join(dir, "host.token")
	if err := os.WriteFile(tok, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(tok, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(tok); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.token")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(link, []byte("secret\n")); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(victim); string(raw) != "keep" {
		t.Fatal("the symlink was followed")
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the token is not a private regular file: %v", fi.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}
