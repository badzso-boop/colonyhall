// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Release is where `colony upgrade` downloads from (install.sh's logic in
// Go; the tests point it at a fake GitHub).
type Release struct {
	Repo, API, Web string
	Token          string // never logged, only an Authorization header
	HTTP           *http.Client
}

// DefaultRelease reads COLONY_REPO, COLONY_GITHUB_API, COLONY_GITHUB_WEB and
// the token (GITHUB_TOKEN, then `gh auth token`).
func DefaultRelease(client *http.Client) Release {
	r := Release{Repo: env("COLONY_REPO", DefaultRepo), API: env("COLONY_GITHUB_API", "https://api.github.com"),
		Web: env("COLONY_GITHUB_WEB", "https://github.com"), Token: os.Getenv("GITHUB_TOKEN"), HTTP: client}
	if r.Token == "" {
		if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
			r.Token = strings.TrimSpace(string(out))
		}
	}
	return r
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (r Release) get(url, accept string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	if r.Token != "" && strings.HasPrefix(url, r.API) {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	// The token stays with the API host: a redirect to another host (or
	// port), like GitHub's storage, goes without it.
	c := *r.HTTP
	c.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if next.URL.Host != via[0].URL.Host {
			next.Header.Del("Authorization")
		}
		return nil
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 200<<20))
}

// Download returns the tag and the verified colony binary for this machine.
// Go's client drops the Authorization header on a redirect to another host.
func (r Release) Download(version string) (string, []byte, error) {
	url := r.API + "/repos/" + r.Repo + "/releases/latest"
	if version != "" {
		url = r.API + "/repos/" + r.Repo + "/releases/tags/" + version
	}
	raw, err := r.get(url, "application/vnd.github+json")
	if err != nil {
		return "", nil, err
	}
	var rel struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			URL  string `json:"url"`
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &rel); err != nil {
		return "", nil, err
	}
	asset := func(name string) ([]byte, error) {
		if r.Token != "" {
			for _, a := range rel.Assets {
				if a.Name == name {
					return r.get(a.URL, "application/octet-stream")
				}
			}
			return nil, fmt.Errorf("the release %s has no %s", rel.Tag, name)
		}
		return r.get(r.Web+"/"+r.Repo+"/releases/download/"+rel.Tag+"/"+name, "application/octet-stream")
	}
	archive := fmt.Sprintf("colony_%s_%s_%s.tar.gz", rel.Tag, runtime.GOOS, runtime.GOARCH)
	sums, err := asset("SHA256SUMS")
	if err != nil {
		return "", nil, err
	}
	tgz, err := asset(archive)
	if err != nil {
		return "", nil, err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.TrimPrefix(f[1], "*") == archive {
			want = f[0]
		}
	}
	got := sha256.Sum256(tgz)
	if want == "" || want != hex.EncodeToString(got[:]) {
		return "", nil, fmt.Errorf("checksum mismatch for %s (nothing was changed)", archive)
	}
	bin, err := untarOne(tgz, "colony")
	return rel.Tag, bin, err
}

func untarOne(tgz []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(tgz)))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("%s not in the archive: %w", name, err)
		}
		if filepath.Base(h.Name) == name {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
}

// Swap replaces the binary at path atomically; the old one stays as .prev.
func Swap(path string, bin []byte) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path + ".prev")
		if err := os.Link(path, path+".prev"); err != nil {
			raw, rerr := os.ReadFile(path)
			if rerr == nil {
				_ = os.WriteFile(path+".prev", raw, 0o755)
			}
		}
	}
	return os.Rename(tmp, path)
}

func (e *Env) restartGuard() error {
	if e.P.OS == "darwin" {
		_, err := e.Exec(Cmd{Name: "launchctl", Args: []string{"kickstart", "-k", fmt.Sprintf("gui/%d/hu.colonyhall.guard", os.Getuid())}})
		return err
	}
	_, err := e.Exec(Cmd{Name: "systemctl", Args: []string{"--user", "restart", "colony-guard"}})
	return err
}

func runUpgrade(e *Env, args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	version := fs.String("version", "", "the version (default: the latest)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	e.UI = NewTerminalUI(DetectLang(nil), false)
	if err := e.Open(); err != nil {
		return err
	}
	rel := DefaultRelease(e.HTTP)
	e.Secret(rel.Token)
	tag, bin, err := rel.Download(*version)
	if err != nil {
		return err
	}
	if tag == e.Version {
		e.UI.Sayf("colony %s is current.", tag)
		return nil
	}
	if err := Swap(e.Self, bin); err != nil {
		return err
	}
	e.UI.Sayf("colony %s → %s (%s; the previous one is %s.prev)", e.Version, tag, e.Self, e.Self)
	if err := e.restartGuard(); err != nil {
		return err
	}
	for _, c := range Doctor(e.Ctx, e) {
		e.UI.Sayf("%s", c.Message(e.UI.Lang))
	}
	return nil
}

// Uninstall removes the guard's service (and with server the server stack);
// data and config stay unless purge.
func (e *Env) Uninstall(server, purge bool) error {
	if server {
		if _, err := os.Stat(filepath.Join(e.serverDir(), "compose.yaml")); err == nil {
			args := []string{"down"}
			if purge {
				args = append(args, "--volumes")
			}
			if _, err := e.Exec(e.compose(args...)); err != nil {
				return err
			}
			if purge {
				if err := e.remove(e.serverDir()); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if e.P.OS == "darwin" {
		_, _ = e.Exec(Cmd{Name: "launchctl", Args: []string{"bootout", fmt.Sprintf("gui/%d/hu.colonyhall.guard", os.Getuid())}})
		if err := e.remove(e.launchAgentPath()); err != nil {
			return err
		}
	} else {
		_, _ = e.Exec(Cmd{Name: "systemctl", Args: []string{"--user", "disable", "--now", "colony-guard"}})
		if err := e.remove(filepath.Join(e.Home, ".config", "systemd", "user", "colony-guard.service")); err != nil {
			return err
		}
		_, _ = e.Exec(Cmd{Name: "systemctl", Args: []string{"--user", "daemon-reload"}})
	}
	if purge {
		for _, p := range []string{e.configDir(), stateDir(e.Home), filepath.Join(e.Home, ".local", "share", "colonyhall", "sandbox")} {
			if err := e.remove(p); err != nil {
				return err
			}
		}
	}
	if err := e.remove(e.Self + ".prev"); err != nil {
		return err
	}
	return e.remove(e.Self)
}

func (e *Env) remove(path string) error {
	if e.DryRun {
		e.plan = append(e.plan, "remove "+path)
		return nil
	}
	e.Logf("remove: %s", path)
	if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func runUninstall(e *Env, args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	server := fs.Bool("server", false, "remove the server stack instead of the guard")
	purge := fs.Bool("purge", false, "also delete the data (server) or the config and token (guard)")
	yes := fs.Bool("yes", false, "do not ask")
	dry := fs.Bool("dry-run", false, "print what would be removed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	e.UI = NewTerminalUI(DetectLang(nil), *yes)
	e.DryRun = *dry
	if err := e.Open(); err != nil {
		return err
	}
	what := "the guard service and binary (config kept)"
	switch {
	case *server && *purge:
		what = "the server stack AND ITS DATA (database, files)"
	case *server:
		what = "the server stack (data kept)"
	case *purge:
		what = "the guard service, binary, config and machine token"
	}
	e.UI.Sayf("Removing %s.", what)
	if ok, err := e.UI.Confirm("proceed", false); err != nil || !ok {
		return err
	}
	if err := e.Uninstall(*server, *purge); err != nil {
		return err
	}
	for _, line := range e.plan {
		e.UI.Sayf("    %s", line)
	}
	if !*server {
		e.UI.Sayf("Revoke the machine on the Machines page too.")
	}
	return nil
}
