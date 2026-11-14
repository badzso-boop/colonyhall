// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
)

func mkdirs(t *testing.T, base string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// hostScope builds a scope with one root inside a temporary tree and an
// "outside" folder next to it.
func hostScope(t *testing.T) (HostFsScope, string, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	mkdirs(t, base, "root", "outside/inner")
	return HostFsScope{Roots: []string{root}, Home: filepath.Join(base, "home")}, root, outside
}

func byName(l apispec.HostDirListing) map[string]apispec.HostDirEntry {
	m := map[string]apispec.HostDirEntry{}
	for _, e := range l.Entries {
		m[e.Name] = e
	}
	return m
}

func flag(p *bool) bool { return p != nil && *p }

func TestHostListDirsRoots(t *testing.T) {
	sc, root, _ := hostScope(t)
	mkdirs(t, root, "proj")
	l := sc.ListDirs("")
	if l.ErrorCode != nil || len(l.Entries) != 1 {
		t.Fatalf("roots: %+v", l)
	}
	e := l.Entries[0]
	if e.Name != root || e.Path != root || !flag(e.HasChildren) {
		t.Fatalf("root entry: %+v", e)
	}
}

func TestHostListDirsOnlyInsideRoots(t *testing.T) {
	sc, root, outside := hostScope(t)
	for _, p := range []string{outside, filepath.Dir(root), "/", "relative/path", root + "/../outside", filepath.Join(outside, "missing")} {
		l := sc.ListDirs(p)
		if l.ErrorCode == nil || *l.ErrorCode != apispec.HostDirListingErrorCodeDenied || len(l.Entries) != 0 {
			t.Errorf("ListDirs(%q) = %+v, want denied", p, l)
		}
	}
	l := sc.ListDirs(filepath.Join(root, "missing"))
	if l.ErrorCode == nil || *l.ErrorCode != apispec.HostDirListingErrorCodeNotFound {
		t.Errorf("missing inside root: %+v", l)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	l = sc.ListDirs(filepath.Join(root, "file.txt"))
	if l.ErrorCode == nil || *l.ErrorCode != apispec.HostDirListingErrorCodeNotADirectory {
		t.Errorf("file: %+v", l)
	}
}

func TestHostListDirsFoldersOnlyAndFlags(t *testing.T) {
	sc, root, _ := hostScope(t)
	mkdirs(t, root, "repo/.git", "repo/src", ".hidden", "node_modules/x", "empty")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "empty", "only-a-file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := sc.ListDirs(root)
	if l.ErrorCode != nil {
		t.Fatalf("list: %+v", l)
	}
	m := byName(l)
	if _, ok := m["README.md"]; ok || len(m) != 4 {
		t.Fatalf("want folders only, got %+v", m)
	}
	if e := m["repo"]; !flag(e.IsGitRepo) || !flag(e.HasChildren) || flag(e.Hidden) || flag(e.Heavy) || e.Path != filepath.Join(root, "repo") {
		t.Errorf("repo: %+v", e)
	}
	if e := m[".hidden"]; !flag(e.Hidden) || flag(e.HasChildren) {
		t.Errorf(".hidden: %+v", e)
	}
	if e := m["node_modules"]; !flag(e.Heavy) {
		t.Errorf("node_modules: %+v", e)
	}
	if e := m["empty"]; flag(e.HasChildren) || flag(e.IsGitRepo) {
		t.Errorf("empty: %+v", e)
	}
	g := byName(sc.ListDirs(filepath.Join(root, "repo")))
	if e := g[".git"]; !flag(e.Heavy) || !flag(e.Hidden) {
		t.Errorf(".git: %+v", e)
	}
}

func TestHostListDirsDenied(t *testing.T) {
	sc, root, _ := hostScope(t)
	sc.ExchangeDir = filepath.Join(root, "exchange")
	mkdirs(t, root, ".ssh/keys", ".gnupg", ".aws", ".config/colonyhall", ".config/other", "exchange", "ok")
	l := sc.ListDirs(root)
	m := byName(l)
	for _, n := range []string{".ssh", ".gnupg", ".aws", "exchange"} {
		if e := m[n]; !flag(e.Denied) || e.HasChildren != nil || e.IsGitRepo != nil {
			t.Errorf("%s: want denied without details, got %+v", n, e)
		}
	}
	if e := m["ok"]; flag(e.Denied) {
		t.Errorf("ok: %+v", e)
	}
	c := byName(sc.ListDirs(filepath.Join(root, ".config")))
	if !flag(c["colonyhall"].Denied) || flag(c["other"].Denied) {
		t.Errorf(".config: %+v", c)
	}
	for _, p := range []string{filepath.Join(root, ".ssh"), filepath.Join(root, ".ssh", "keys"), filepath.Join(root, ".config", "colonyhall")} {
		l := sc.ListDirs(p)
		if l.ErrorCode == nil || *l.ErrorCode != apispec.HostDirListingErrorCodeDenied {
			t.Errorf("ListDirs(%q) = %+v, want denied", p, l)
		}
	}
}

func TestHostListDirsSymlinks(t *testing.T) {
	sc, root, outside := hostScope(t)
	mkdirs(t, root, "real/sub")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	m := byName(sc.ListDirs(root))
	if _, ok := m["escape"]; ok {
		t.Errorf("a symlink leaving the roots must not be listed: %+v", m)
	}
	if e, ok := m["inside"]; !ok || !flag(e.HasChildren) || e.Path != filepath.Join(root, "inside") {
		t.Errorf("a symlink inside the roots: %+v", m)
	}
	l := sc.ListDirs(filepath.Join(root, "escape"))
	if l.ErrorCode == nil || *l.ErrorCode != apispec.HostDirListingErrorCodeDenied {
		t.Errorf("following an escaping symlink: %+v", l)
	}
	if l := sc.ListDirs(filepath.Join(root, "inside")); l.ErrorCode != nil || len(l.Entries) != 1 {
		t.Errorf("listing through an inside symlink: %+v", l)
	}
}

func TestHostListDirsTruncates(t *testing.T) {
	sc, root, _ := hostScope(t)
	for i := 0; i < 510; i++ {
		mkdirs(t, root, fmt.Sprintf("d%03d", i))
	}
	l := sc.ListDirs(root)
	if len(l.Entries) != 500 || !flag(l.Truncated) {
		t.Fatalf("got %d entries, truncated=%v", len(l.Entries), flag(l.Truncated))
	}
	if l.Entries[0].Name != "d000" || l.Entries[499].Name != "d499" {
		t.Errorf("not sorted: %s … %s", l.Entries[0].Name, l.Entries[499].Name)
	}
}

// TestHostWSServesFsRequest runs the host WebSocket against a fake server:
// the hello carries the host token, a host.fs.request frame makes the guard
// fetch the request and post the listing with the host token.
func TestHostWSServesFsRequest(t *testing.T) {
	sc, root, _ := hostScope(t)
	mkdirs(t, root, "proj")
	const token = "colh_test_host_token"
	hello := make(chan map[string]any, 1)
	result := make(chan apispec.HostDirListing, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/host", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		var h map[string]any
		if err := wsjson.Read(r.Context(), c, &h); err != nil {
			return
		}
		hello <- h
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "welcome", "protocol": 1, "host_id": "h1"})
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "host.fs.request", "request_id": "req-1"})
		<-c.CloseRead(r.Context()).Done()
	})
	mux.HandleFunc("GET /api/v1/host/fs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token || r.PathValue("id") != "req-1" {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "req-1", "op": "list_dirs", "path": root})
	})
	mux.HandleFunc("POST /api/v1/host/fs/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var l apispec.HostDirListing
		_ = json.NewDecoder(r.Body).Decode(&l)
		result <- l
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hw := &HostWS{Client: &Client{BaseURL: srv.URL, Token: token, HTTP: &http.Client{Timeout: 5 * time.Second}},
		Scope: sc, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Backoff: func(int) time.Duration { return 10 * time.Millisecond }}
	go hw.Run(ctx)

	select {
	case h := <-hello:
		if h["type"] != "hello" || h["token"] != token {
			t.Fatalf("hello: %+v", h)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no hello")
	}
	select {
	case l := <-result:
		if l.ErrorCode != nil || len(l.Entries) != 1 || l.Entries[0].Name != "proj" {
			t.Fatalf("result: %+v", l)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
}
