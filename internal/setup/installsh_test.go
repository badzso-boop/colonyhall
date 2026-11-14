// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The install.sh tests run the real script with sh and curl against a fake
// GitHub (API, web downloads and a separate "storage" host the asset
// downloads redirect to). No real GitHub call is made.

const fakeVersion = "v1.2.3"

// fakeColony is the "binary" in the release: it answers --version and
// records the setup call.
const fakeColony = `#!/bin/sh
case "$1" in
  --version) echo "colony v1.2.3" ;;
  setup) shift; echo "$*" > "$HOME/setup-args" ;;
esac
`

func tarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type fakeGitHub struct {
	api, storage *httptest.Server
	private      bool
	token        string
	files        map[string][]byte

	mu           sync.Mutex
	storageAuth  []string // Authorization headers the storage host saw
	unauthorized int
}

func newFakeGitHub(t *testing.T, private bool, corrupt bool) *fakeGitHub {
	t.Helper()
	archive := fmt.Sprintf("colony_%s_%s_%s.tar.gz", fakeVersion, runtime.GOOS, runtime.GOARCH)
	tgz := tarGz(t, "colony", []byte(fakeColony))
	sum := sha256.Sum256(tgz)
	if corrupt {
		sum[0] ^= 0xff
	}
	f := &fakeGitHub{private: private, token: "ghp_testTOKEN_0123456789", files: map[string][]byte{
		archive:      tgz,
		"SHA256SUMS": []byte(hex.EncodeToString(sum[:]) + "  " + archive + "\n"),
	}}
	f.storage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.storageAuth = append(f.storageAuth, r.Header.Get("Authorization"))
		f.mu.Unlock()
		body, ok := f.files[strings.TrimPrefix(r.URL.Path, "/blob/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(f.storage.Close)
	// Started only after f.api is set: serveAPI reads f.api.URL.
	f.api = httptest.NewUnstartedServer(http.HandlerFunc(f.serveAPI))
	f.api.Start()
	t.Cleanup(f.api.Close)
	return f
}

func (f *fakeGitHub) authorized(r *http.Request) bool {
	if !f.private {
		return true
	}
	ok := r.Header.Get("Authorization") == "Bearer "+f.token
	if !ok {
		f.mu.Lock()
		f.unauthorized++
		f.mu.Unlock()
	}
	return ok
}

func (f *fakeGitHub) serveAPI(w http.ResponseWriter, r *http.Request) {
	const repo = "/api/repos/owner/colonyhall"
	switch {
	case r.URL.Path == repo+"/releases/latest" || r.URL.Path == repo+"/releases/tags/"+fakeVersion:
		if !f.authorized(r) {
			http.NotFound(w, r) // GitHub hides private repos
			return
		}
		type asset struct {
			URL  string `json:"url"`
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		var assets []asset
		i := 0
		for name := range f.files {
			i++
			assets = append(assets, asset{URL: fmt.Sprintf("%s%s/releases/assets/%s", f.api.URL, repo, name), ID: i, Name: name})
		}
		out, _ := json.MarshalIndent(map[string]any{"url": f.api.URL + repo + "/releases/1", "tag_name": fakeVersion, "assets": assets}, "", "  ")
		_, _ = w.Write(out)
	case strings.HasPrefix(r.URL.Path, repo+"/releases/assets/"):
		if !f.authorized(r) || r.Header.Get("Accept") != "application/octet-stream" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, f.storage.URL+"/blob/"+strings.TrimPrefix(r.URL.Path, repo+"/releases/assets/"), http.StatusFound)
	case strings.HasPrefix(r.URL.Path, "/web/owner/colonyhall/releases/download/"+fakeVersion+"/"):
		if f.private {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, f.storage.URL+"/blob/"+filepath.Base(r.URL.Path), http.StatusFound)
	default:
		http.NotFound(w, r)
	}
}

type installRun struct {
	home, prefix, out string
	err               error
}

func runInstall(t *testing.T, f *fakeGitHub, env []string, args ...string) installRun {
	t.Helper()
	for _, tool := range []string{"sh", "curl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	home := t.TempDir()
	prefix := filepath.Join(home, ".local", "bin")
	script, err := filepath.Abs("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append([]string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"GH_CONFIG_DIR=" + filepath.Join(home, "no-gh"), // never the developer's own gh login
		"COLONY_REPO=owner/colonyhall",
		"COLONY_GITHUB_API=" + f.api.URL + "/api",
		"COLONY_GITHUB_WEB=" + f.api.URL + "/web",
		"COLONY_PREFIX=" + prefix,
		"COLONY_INSECURE_HTTP=1",
	}, env...)
	out, err := cmd.CombinedOutput()
	return installRun{home: home, prefix: prefix, out: string(out), err: err}
}

func TestInstallShPublicRelease(t *testing.T) {
	f := newFakeGitHub(t, false, false)
	r := runInstall(t, f, nil, "--", "--guard", "--yes")
	if r.err != nil {
		t.Fatalf("install: %v\n%s", r.err, r.out)
	}
	if fi, err := os.Stat(filepath.Join(r.prefix, "colony")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("colony not installed: %v %v\n%s", fi, err, r.out)
	}
	args, err := os.ReadFile(filepath.Join(r.home, "setup-args"))
	if err != nil || strings.TrimSpace(string(args)) != "--guard --yes" {
		t.Fatalf("setup not started with the arguments: %q %v\n%s", args, err, r.out)
	}
}

func TestInstallShPrivateReleaseWithToken(t *testing.T) {
	f := newFakeGitHub(t, true, false)
	r := runInstall(t, f, []string{"GITHUB_TOKEN=" + f.token}, "--no-setup")
	if r.err != nil {
		t.Fatalf("install: %v\n%s", r.err, r.out)
	}
	if _, err := os.Stat(filepath.Join(r.prefix, "colony")); err != nil {
		t.Fatalf("colony not installed: %v\n%s", err, r.out)
	}
	// The token goes to the API only, never on to the storage host.
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.storageAuth) != 2 {
		t.Fatalf("storage downloads: %d", len(f.storageAuth))
	}
	for _, a := range f.storageAuth {
		if a != "" {
			t.Fatalf("the token was forwarded to the storage host: %q", a)
		}
	}
	if strings.Contains(r.out, f.token) {
		t.Fatal("the token was printed")
	}
}

func TestInstallShPrivateReleaseWithoutTokenFails(t *testing.T) {
	f := newFakeGitHub(t, true, false)
	r := runInstall(t, f, nil, "--no-setup")
	if r.err == nil || !strings.Contains(r.out, "cannot read the release") {
		t.Fatalf("expected a clear failure: %v\n%s", r.err, r.out)
	}
	if _, err := os.Stat(filepath.Join(r.prefix, "colony")); !os.IsNotExist(err) {
		t.Fatal("something was installed")
	}
}

func TestInstallShRefusesABadChecksum(t *testing.T) {
	f := newFakeGitHub(t, false, true)
	r := runInstall(t, f, nil, "--no-setup")
	if r.err == nil || !strings.Contains(r.out, "checksum mismatch") {
		t.Fatalf("expected a checksum failure: %v\n%s", r.err, r.out)
	}
	if _, err := os.Stat(filepath.Join(r.prefix, "colony")); !os.IsNotExist(err) {
		t.Fatal("a binary with a bad checksum was installed")
	}
}

func TestInstallShNeverPutsTheTokenOnACommandLine(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "curl") && strings.Contains(line, "$token") {
			t.Fatalf("install.sh:%d puts the token on curl's command line: %s", i+1, line)
		}
	}
}
