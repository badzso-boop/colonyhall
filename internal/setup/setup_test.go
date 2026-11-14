// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"flag"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/guard"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var (
	linux  = Platform{OS: "linux", Arch: "amd64", Manager: "apt", Systemd: true}
	wsl    = Platform{OS: "linux", Arch: "amd64", Manager: "apt", Systemd: true, WSL: true, Distro: "Ubuntu-24.04"}
	wslOld = Platform{OS: "linux", Arch: "amd64", Manager: "apt", WSL: true, Distro: "Ubuntu-24.04"}
	mac    = Platform{OS: "darwin", Arch: "arm64", Manager: "brew"}
)

// testEnv is an Env with a FakeRunner, a temporary home, scripted input
// and an output buffer.
func testEnv(t *testing.T, p Platform, input string, yes bool) (*Env, *FakeRunner, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	out := &bytes.Buffer{}
	fr := &FakeRunner{Have: map[string]bool{}}
	e := &Env{Ctx: context.Background(), Run: fr, P: p, Home: home, Self: filepath.Join(home, ".local", "bin", "colony"), Version: "v1.2.3",
		UI: &UI{In: bufio.NewReader(strings.NewReader(input)), Out: out, Lang: English, Yes: yes}, HTTP: &http.Client{Timeout: 5 * time.Second}}
	if err := e.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e, fr, out
}

func TestEveryMessageHasBothLanguages(t *testing.T) {
	for k, v := range messages {
		if strings.TrimSpace(v[0]) == "" || strings.TrimSpace(v[1]) == "" {
			t.Errorf("message %q lacks a language", k)
		}
		if strings.Count(v[0], "%") != strings.Count(v[1], "%") {
			t.Errorf("message %q: the two languages take different arguments", k)
		}
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for want, m := range map[Lang]map[string]string{
		English:   {},
		Hungarian: {"LANG": "hu_HU.UTF-8"},
	} {
		if got := DetectLang(env(m)); got != want {
			t.Errorf("%v: %s", m, got)
		}
	}
	if DetectLang(env(map[string]string{"LC_ALL": "C", "LANG": "hu_HU.UTF-8"})) != English {
		t.Error("LC_ALL wins over LANG")
	}
}

func TestDetectPlatform(t *testing.T) {
	fr := &FakeRunner{Have: map[string]bool{"apt-get": true}}
	p := Detect(fr, MapFS{"/proc/sys/fs/binfmt_misc/WSLInterop": "", "/proc/1/comm": "init\n"}, func(string) string { return "" })
	if p.OS == "linux" && (!p.WSL || p.Systemd || p.Manager != "apt") {
		t.Fatalf("WSL without systemd: %+v", p)
	}
}

func TestInstallCommandsPerManager(t *testing.T) {
	deps := []dep{depGit, depTmux, depBwrap}
	cases := map[string]string{
		"apt":  "sudo apt-get install -y -q bubblewrap git tmux",
		"dnf":  "sudo dnf install -y bubblewrap git tmux",
		"brew": "brew install git tmux",
	}
	for m, want := range cases {
		cmds, manual := installCmds(Platform{OS: "linux", Manager: m}, deps)
		if got := cmds[len(cmds)-1].String(); got != want {
			t.Errorf("%s: %q", m, got)
		}
		if m == "brew" && !slices.Equal(manual, []string{"bwrap"}) {
			t.Errorf("brew has no bubblewrap: %v", manual)
		}
	}
	if cmds, manual := installCmds(Platform{OS: "linux"}, deps); cmds != nil || len(manual) != 3 {
		t.Errorf("no manager: print, never run: %v %v", cmds, manual)
	}
}

func TestGuardYAMLGolden(t *testing.T) {
	two, zero := 2, 0
	yes, no := true, false
	cases := map[string]struct {
		p Platform
		a GuardAnswers
	}{
		"linux":  {linux, GuardAnswers{ServerURL: "http://192.168.1.10:8080", Roots: []string{"/home/anna/projects"}, AllowUnsandboxed: &yes, MaxConcurrentRuns: &two, PreviewPorts: "41000-41999"}},
		"wsl":    {wsl, GuardAnswers{ServerURL: "http://100.85.0.1:8080", Roots: []string{"/home/anna/projects", "/home/anna/my code"}, AllowUnsandboxed: &no, MaxConcurrentRuns: &zero, PreviewPorts: "42000-42999", ExchangeDir: "/home/anna/exchange"}},
		"darwin": {mac, GuardAnswers{ServerURL: "https://colony.example.com", Roots: []string{"/Users/anna/projects"}, AllowUnsandboxed: &yes, MaxConcurrentRuns: &two, PreviewPorts: "41000-41999"}},
	}
	for name, c := range cases {
		home := map[string]string{"linux": "/home/anna", "wsl": "/home/anna", "darwin": "/Users/anna"}[name]
		got := GuardYAML(c.p, home, c.a)
		golden := filepath.Join("testdata", "guard-"+name+".yaml")
		if *update {
			if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("%s: %v (run go test ./internal/setup -update)", name, err)
		}
		if got != string(want) {
			t.Errorf("%s: guard.yaml differs from %s:\n%s", name, golden, got)
		}
		// The guard itself accepts it.
		path := filepath.Join(t.TempDir(), "guard.yaml")
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := guard.LoadConfig(path); err != nil {
			t.Errorf("%s: the guard refuses it: %v", name, err)
		}
	}
}

func TestWSLConfSystemd(t *testing.T) {
	for in, want := range map[string]string{
		"":                                     "[boot]\nsystemd=true\n",
		"[network]\nhostname=x\n":              "[network]\nhostname=x\n\n[boot]\nsystemd=true\n",
		"[boot]\nsystemd=false\n":              "[boot]\nsystemd=true\n",
		"[boot]\ncommand=x\n[user]\ndefault=a": "[boot]\ncommand=x\nsystemd=true\n[user]\ndefault=a\n",
	} {
		if got := wslConfWithSystemd(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestLaunchdPlistIsValidXML(t *testing.T) {
	p := launchdPlist("hu.colonyhall.guard", []string{"/Users/a&b/.local/bin/colony", "guard", "-config", "/Users/a&b/.config/colonyhall/guard.yaml"},
		"/opt/homebrew/bin:/usr/bin", true, "/Users/a&b/Library/Logs/colonyhall", -1)
	dec := xml.NewDecoder(strings.NewReader(p))
	dec.Strict = false
	for {
		if _, err := dec.Token(); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("plist: %v\n%s", err, p)
		}
	}
	for _, want := range []string{"<key>KeepAlive</key><true/>", "a&amp;b", "<string>guard</string>", "/opt/homebrew/bin"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q", want)
		}
	}
}

// readyServer answers /readyz; the server answers are bound to its port.
func readyServer(t *testing.T) (host string, port int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	h, p, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	n, _ := strconv.Atoi(p)
	return h, n
}

func TestServerInstallCommandsAndFiles(t *testing.T) {
	e, fr, out := testEnv(t, linux, "", true)
	fr.Have["docker"] = true
	host, port := readyServer(t)
	no := false
	e.Answers = Answers{Mode: "server", Server: ServerAnswers{Bind: host, Port: port, AdminHandle: "anna", AdminName: "Anna", AdminPassword: "correct-horse-battery", Backups: &no}}
	if err := e.askServer(); err != nil {
		t.Fatal(err)
	}
	if err := e.Do(e.ServerSteps(false)); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	dir := e.serverDir()
	lines := fr.Lines()
	compose := "sudo docker compose --project-directory " + dir + " -f " + dir + "/compose.yaml --env-file " + dir + "/.env"
	want := []string{
		"docker compose version",
		compose + " pull",
		compose + " up -d --wait db",
		compose + " exec -T db psql -q -U colony -d colonyhall",
		compose + " up -d",
		compose + " run --rm -T server bootstrap -handle anna -name Anna",
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("commands:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	// The password goes on stdin only.
	last := fr.Cmds[len(fr.Cmds)-1]
	if last.Stdin != "correct-horse-battery\n" || strings.Contains(last.String(), "correct-horse") {
		t.Fatalf("bootstrap password: %+v", last)
	}
	envPath := filepath.Join(dir, ".env")
	fi, err := os.Stat(envPath)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf(".env must be 0600: %v %v", fi, err)
	}
	env := readEnv(envPath)
	if env["COLONY_IMAGE"] != "ghcr.io/badzso-boop/colonyhall:v1.2.3" || env["COLONY_BIND"] != host || len(env["POSTGRES_PASSWORD"]) != 48 {
		t.Fatalf(".env: %v", env)
	}
	// Round 45: a new install's server connects as the application role,
	// whose password goes to psql on stdin only.
	app := env["COLONY_APP_DB_PASSWORD"]
	if len(app) != 48 || !strings.HasPrefix(env["COLONY_DATABASE_URL"], "postgres://"+AppRole+":"+app+"@db:5432/") {
		t.Fatalf("application role: %v", env)
	}
	psql := fr.Cmds[3]
	if !strings.Contains(psql.Stdin, "PASSWORD '"+app+"'") || strings.Contains(psql.String(), app) {
		t.Fatalf("the role's password must go on stdin only: %+v", psql)
	}
	compYAML, _ := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	if !strings.Contains(string(compYAML), "pgvector/pgvector:pg17") || strings.Contains(string(compYAML), env["POSTGRES_PASSWORD"]) {
		t.Fatal("compose.yaml: its own Postgres, and no secret in it")
	}
	// Secrets never reach the log.
	logRaw, _ := os.ReadFile(e.LogPath)
	for _, s := range []string{env["POSTGRES_PASSWORD"], app, "correct-horse-battery"} {
		if strings.Contains(string(logRaw), s) || strings.Contains(out.String(), s) {
			t.Fatalf("a secret is in the log or the output")
		}
	}

	// A second run keeps the database password and runs no bootstrap again.
	pw := env["POSTGRES_PASSWORD"]
	e2, fr2, _ := testEnv(t, linux, "", true)
	e2.Home = e.Home
	if err := e2.Open(); err != nil {
		t.Fatal(err)
	}
	fr2.Have["docker"] = true
	e2.Answers = e.Answers
	if err := e2.Do(e2.ServerSteps(false)); err != nil {
		t.Fatal(err)
	}
	if again := readEnv(envPath); again["POSTGRES_PASSWORD"] != pw || again["COLONY_APP_DB_PASSWORD"] != app {
		t.Fatal("a re-run changed a database password")
	}
	for _, l := range fr2.Lines() {
		if strings.Contains(l, "bootstrap") {
			t.Fatal("a re-run bootstrapped again")
		}
	}
}

func TestServerWithAnExistingDatabase(t *testing.T) {
	e, _, out := testEnv(t, mac, "", true)
	e.Answers.Server = ServerAnswers{Bind: "127.0.0.1", Port: 8080, DatabaseURL: "postgres://colony:s3cr3t-pw@db.lan:5432/colonyhall", AdminHandle: "a", AdminName: "A"}
	if err := e.askServer(); err != nil {
		t.Fatal(err)
	}
	v, own := e.serverEnv()
	if own || v["COLONY_DATABASE_URL"] != e.Answers.Server.DatabaseURL || v["POSTGRES_PASSWORD"] != "" {
		t.Fatalf("external: %v", v)
	}
	if strings.Contains(composeFile(false), "pgvector/pgvector") {
		t.Fatal("no own Postgres with an existing database")
	}
	if e.docker("ps").Sudo {
		t.Fatal("macOS runs docker without sudo")
	}
	if strings.Contains(e.Mask("url "+e.Answers.Server.DatabaseURL), "s3cr3t") {
		t.Fatal("the database URL is not masked")
	}
	_ = out
}

func TestUnattendedNeverOpensTheServerToEveryNetwork(t *testing.T) {
	e, _, _ := testEnv(t, linux, "", true)
	// Round 40: every spelling of "all interfaces", and no non-address.
	for _, bind := range []string{"0.0.0.0", "::", "0:0:0:0:0:0:0:0", "localhost\nCOLONY_PORT=80", "my-server"} {
		e.Answers.Server = ServerAnswers{Bind: bind, Port: 8080, AdminHandle: "a", AdminName: "A"}
		if err := e.askServer(); err == nil {
			t.Fatalf("--yes accepted bind %q without allow_all_interfaces", bind)
		}
	}
}

func TestEnvFileDropsValuesWithLineBreaks(t *testing.T) {
	got := envFile(map[string]string{"COLONY_IMAGE": "img:1\nCOLONY_DEV_SEED=1", "COLONY_BIND": "127.0.0.1"})
	if strings.Contains(got, "COLONY_DEV_SEED") || !strings.Contains(got, "COLONY_BIND=127.0.0.1\n") {
		t.Fatalf("a line break added a variable:\n%s", got)
	}
}

func TestGuardInstallOnLinux(t *testing.T) {
	e, fr, out := testEnv(t, linux, "", true)
	for _, p := range []string{"git", "curl", "tmux", "bwrap", "claude"} {
		fr.Have[p] = true
	}
	no := false
	e.Answers = Answers{Mode: "guard", Guard: GuardAnswers{ServerURL: "http://127.0.0.1:8080", PairCode: "K7Q2-MX4P", Name: "pc", Roots: []string{"/tmp"}, Linger: &no, Clients: []string{}}}
	if err := e.askGuard(); err != nil {
		t.Fatal(err)
	}
	var enrolled []string
	fake := func(_ context.Context, server, code, name, tokenFile string) error {
		enrolled = []string{server, code, name}
		return os.WriteFile(tokenFile, []byte("colh_x\n"), 0o600)
	}
	if err := e.Do(e.GuardSteps(fake)); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !slices.Equal(enrolled, []string{"http://127.0.0.1:8080", "K7Q2-MX4P", "pc"}) {
		t.Fatalf("enroll: %v", enrolled)
	}
	cfg, err := guard.LoadConfig(e.guardConfigPath())
	if err != nil || cfg.MaxConcurrentRuns != 2 || !*cfg.RequireSandbox {
		t.Fatalf("guard.yaml: %+v %v", cfg, err)
	}
	unit, err := os.ReadFile(filepath.Join(e.Home, ".config", "systemd", "user", "colony-guard.service"))
	if err != nil || !strings.Contains(string(unit), e.Self+" guard -config "+e.guardConfigPath()) {
		t.Fatalf("unit: %v\n%s", err, unit)
	}
	lines := fr.Lines()
	for _, want := range []string{"systemctl --user daemon-reload", "systemctl --user enable --now colony-guard", "claude -p /usage --output-format json --no-session-persistence --strict-mcp-config"} {
		if !slices.Contains(lines, want) {
			t.Errorf("missing %q in\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if strings.Contains(string(mustRead(t, e.LogPath)), "K7Q2-MX4P") {
		t.Fatal("the pairing code is in the log")
	}
	// Running it again does nothing new: the machine is paired.
	enrolled = nil
	if err := e.Do(e.GuardSteps(fake)); err != nil || enrolled != nil {
		t.Fatalf("re-run: %v %v", err, enrolled)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestGuardOnWSLWithoutSystemdStopsAfterEnablingIt(t *testing.T) {
	e, fr, out := testEnv(t, wslOld, "", true)
	for _, p := range []string{"git", "curl", "tmux", "bwrap"} {
		fr.Have[p] = true
	}
	e.Answers = Answers{Mode: "guard", Guard: GuardAnswers{ServerURL: "http://x:1", PairCode: "C", Name: "w", Roots: []string{"/tmp"}, Clients: []string{}}}
	if err := e.askGuard(); err != nil {
		t.Fatal(err)
	}
	enroll := func(_ context.Context, _, _, _, tokenFile string) error {
		return os.WriteFile(tokenFile, []byte("t"), 0o600)
	}
	err := e.Do(e.GuardSteps(enroll))
	if err == nil || !strings.Contains(out.String(), "wsl --shutdown") {
		t.Fatalf("expected a stop with the restart hint: %v\n%s", err, out)
	}
	tee := fr.Cmds[len(fr.Cmds)-1]
	if tee.String() != "sudo tee /etc/wsl.conf" || !strings.Contains(tee.Stdin, "systemd=true") {
		t.Fatalf("wsl.conf: %+v", tee)
	}
}

func TestGuardOnWSLAddsTheKeepalive(t *testing.T) {
	e, fr, _ := testEnv(t, wsl, "", true)
	for _, p := range []string{"git", "curl", "tmux", "bwrap"} {
		fr.Have[p] = true
	}
	no := false
	e.Answers = Answers{Mode: "guard", Guard: GuardAnswers{ServerURL: "http://x:1", PairCode: "C", Name: "w", Roots: []string{"/tmp"}, Linger: &no, Clients: []string{}}}
	if err := e.askGuard(); err != nil {
		t.Fatal(err)
	}
	enroll := func(_ context.Context, _, _, _, tokenFile string) error {
		return os.WriteFile(tokenFile, []byte("t"), 0o600)
	}
	if err := e.Do(e.GuardSteps(enroll)); err != nil {
		t.Fatal(err)
	}
	var ps string
	for _, c := range fr.Cmds {
		if c.Name == "powershell.exe" {
			ps = c.Args[len(c.Args)-1]
		}
	}
	if !strings.Contains(ps, "conhost.exe") || !strings.Contains(ps, "--headless wsl.exe -d Ubuntu-24.04 -- sleep infinity") || !strings.Contains(ps, "-AtLogOn") {
		t.Fatalf("keepalive: %q", ps)
	}
}

func TestGuardOnMacUsesLaunchdAndNoSandbox(t *testing.T) {
	e, fr, out := testEnv(t, mac, "", true)
	for _, p := range []string{"git", "curl", "tmux"} {
		fr.Have[p] = true
	}
	e.Answers = Answers{Mode: "guard", Guard: GuardAnswers{ServerURL: "http://x:1", PairCode: "C", Name: "m", Roots: []string{"/tmp"}, Clients: []string{}}}
	if err := e.askGuard(); err != nil {
		t.Fatal(err)
	}
	if !*e.Answers.Guard.AllowUnsandboxed || !strings.Contains(out.String(), "macOS has no sandbox") {
		t.Fatal("macOS: full access only, and it says so")
	}
	enroll := func(_ context.Context, _, _, _, tokenFile string) error {
		return os.WriteFile(tokenFile, []byte("t"), 0o600)
	}
	if err := e.Do(e.GuardSteps(enroll)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.launchAgentPath()); err != nil {
		t.Fatal("no LaunchAgent")
	}
	found := false
	for _, l := range fr.Lines() {
		found = found || strings.HasPrefix(l, "launchctl bootstrap gui/")
		if strings.Contains(l, "bwrap") || strings.Contains(l, "systemctl") {
			t.Fatalf("not on macOS: %s", l)
		}
	}
	if !found {
		t.Fatal("launchctl bootstrap missing")
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	e, fr, out := testEnv(t, linux, "", true)
	e.DryRun = true
	no := false
	e.Answers = Answers{Mode: "both", Server: ServerAnswers{Bind: "127.0.0.1", Port: 8080, AdminHandle: "a", AdminName: "A", AdminPassword: "long-enough-pw", Backups: &no},
		Guard: GuardAnswers{ServerURL: "http://127.0.0.1:8080", PairCode: "C", Name: "n", Roots: []string{"/tmp"}, Clients: []string{}}}
	if err := e.askServer(); err != nil {
		t.Fatal(err)
	}
	if err := e.askGuard(); err != nil {
		t.Fatal(err)
	}
	if err := e.Do(append(e.ServerSteps(false), e.GuardSteps(nil)...)); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(fr.Cmds) != 0 {
		t.Fatalf("a dry run ran commands: %v", fr.Lines())
	}
	for _, p := range []string{".config", ".local/share", "Library"} {
		if _, err := os.Stat(filepath.Join(e.Home, p)); !os.IsNotExist(err) {
			t.Fatalf("a dry run wrote %s", p)
		}
	}
	for _, want := range []string{"Dry run", "compose.yaml (644)", "up -d", "colony guard enroll", "systemctl --user enable --now colony-guard"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the plan lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out.String(), "long-enough-pw") {
		t.Fatal("the plan shows the password")
	}
}

func TestFailedStepIsResumed(t *testing.T) {
	e, fr, _ := testEnv(t, linux, "", true)
	calls := 0
	fail := true
	steps := []Step{
		{ID: "a", Title: "a", Apply: func(e *Env) error { calls++; return nil }},
		{ID: "b", Title: "b", Apply: func(e *Env) error {
			if fail {
				return os.ErrPermission
			}
			return nil
		}},
	}
	if err := e.Do(steps); err == nil {
		t.Fatal("b should fail")
	}
	fail = false
	e2, _, _ := testEnv(t, linux, "", true)
	e2.Home = e.Home
	if err := e2.Open(); err != nil {
		t.Fatal(err)
	}
	if err := e2.Do(steps); err != nil || calls != 1 {
		t.Fatalf("resume: %v, a ran %d times", err, calls)
	}
	_ = fr
}

func TestUpgradeVerifiesAndSwaps(t *testing.T) {
	for _, private := range []bool{false, true} {
		f := newFakeGitHub(t, private, false)
		rel := Release{Repo: "owner/colonyhall", API: f.api.URL + "/api", Web: f.api.URL + "/web", HTTP: f.api.Client()}
		if private {
			rel.Token = f.token
		}
		tag, bin, err := rel.Download("")
		if err != nil || tag != fakeVersion || !strings.Contains(string(bin), "colony v1.2.3") {
			t.Fatalf("private=%v: %v %q", private, err, tag)
		}
		for _, a := range f.storageAuth {
			if a != "" {
				t.Fatal("the token reached the storage host")
			}
		}
		path := filepath.Join(t.TempDir(), "colony")
		_ = os.WriteFile(path, []byte("old"), 0o755)
		if err := Swap(path, bin); err != nil {
			t.Fatal(err)
		}
		if string(mustRead(t, path+".prev")) != "old" || !strings.Contains(string(mustRead(t, path)), "colony v1.2.3") {
			t.Fatal("swap")
		}
	}
	bad := newFakeGitHub(t, false, true)
	rel := Release{Repo: "owner/colonyhall", API: bad.api.URL + "/api", Web: bad.api.URL + "/web", HTTP: bad.api.Client()}
	if _, _, err := rel.Download(""); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("bad checksum: %v", err)
	}
}

func TestUninstallKeepsTheConfig(t *testing.T) {
	e, fr, _ := testEnv(t, linux, "", true)
	unit := filepath.Join(e.Home, ".config", "systemd", "user", "colony-guard.service")
	for _, p := range []string{e.Self, unit, e.guardConfigPath(), filepath.Join(e.configDir(), "host.token")} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(p, []byte("x"), 0o600)
	}
	if err := e.Uninstall(false, false); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{e.Self, unit} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s still there", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(e.configDir(), "host.token")); err != nil {
		t.Error("the config was removed without --purge")
	}
	if !slices.Contains(fr.Lines(), "systemctl --user disable --now colony-guard") {
		t.Errorf("commands: %v", fr.Lines())
	}
}

func TestDoctorReportsAndExplains(t *testing.T) {
	e, fr, _ := testEnv(t, linux, "", true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/host/config":
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	two := 2
	yes := true
	_ = os.MkdirAll(e.configDir(), 0o700)
	_ = os.WriteFile(e.guardConfigPath(), []byte(GuardYAML(linux, e.Home, GuardAnswers{ServerURL: srv.URL, Roots: []string{e.Home}, AllowUnsandboxed: &yes, MaxConcurrentRuns: &two, PreviewPorts: "41000-41999"})), 0o600)
	_ = os.WriteFile(filepath.Join(e.configDir(), "host.token"), []byte("colh_revoked_token_1234\n"), 0o600)
	fr.Have = map[string]bool{"claude": true, "git": true}
	fr.Reply = func(c Cmd) (string, error) {
		if c.Name == "claude" {
			return "", os.ErrPermission // not logged in
		}
		if c.Name == "systemctl" {
			return "inactive\n", os.ErrProcessDone
		}
		return "", nil
	}
	got := map[string]string{}
	var text strings.Builder
	for _, c := range Doctor(context.Background(), e) {
		got[c.ID] = c.Status
		text.WriteString(c.Message(English) + "\n")
	}
	want := map[string]string{"config": StatusOK, "server": StatusOK, "token": StatusFail, "claude": StatusWarn, "sandbox": StatusWarn, "tmux": StatusWarn, "git": StatusOK, "service": StatusFail}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %s, want %s\n%s", k, got[k], v, text.String())
		}
	}
	for _, hint := range []string{"Pair the machine again", "run `claude` and sign in", "never asks for your AI account password", "systemctl --user restart colony-guard"} {
		if !strings.Contains(text.String(), hint) {
			t.Errorf("no fix %q in\n%s", hint, text.String())
		}
	}
}

func TestDoctorForTheHostReportCarriesNoFixText(t *testing.T) {
	got := toReport([]Check{{ID: "claude", Status: StatusWarn, Detail: "not logged in", Fix: "loginClaude"}, {ID: "git", Status: StatusOK}})
	if len(got) != 2 || got[0].Id != "claude" || string(got[0].Status) != "warn" || *got[0].Detail != "not logged in" || got[1].Detail != nil {
		t.Fatalf("%+v", got)
	}
}

// The Linux backup job runs as root, so nothing it runs, reads or writes may
// be in the user's home: a user-writable script or backup folder would give
// the user (or anything running as the user) root.
func TestRootBackupUsesOnlyRootOwnedPaths(t *testing.T) {
	e, fr, _ := testEnv(t, linux, "", true)
	if err := e.installBackups(); err != nil {
		t.Fatal(err)
	}
	// Round 40: root runs it nightly; the image is pinned by digest.
	if !strings.Contains(e.backupScript(), " busybox:1.37@sha256:") || strings.Contains(e.backupScript(), " busybox tar") {
		t.Errorf("the backup image is not pinned by digest:\n%s", e.backupScript())
	}
	if strings.Contains(e.backupScript(), e.Home) {
		t.Errorf("the root backup script refers to the user's home:\n%s", e.backupScript())
	}
	var script, unit bool
	for _, c := range fr.Cmds {
		if c.Name == "tee" && slices.Equal(c.Args, []string{rootBackupScript}) {
			script = c.Sudo && c.Stdin == e.backupScript()
		}
		if c.Name == "tee" && slices.Equal(c.Args, []string{"/etc/systemd/system/colonyhall-backup.service"}) {
			unit = strings.Contains(c.Stdin, "ExecStart=/bin/sh "+rootBackupScript+"\n")
		}
		if strings.Contains(c.Stdin, e.Home) || slices.ContainsFunc(c.Args, func(a string) bool { return strings.HasPrefix(a, e.Home) }) {
			t.Errorf("a root step touches the user's home: %s", c.String())
		}
	}
	if !script || !unit {
		t.Errorf("want the script written by root to %s and run from there (script %v, unit %v):\n%s", rootBackupScript, script, unit, strings.Join(fr.Lines(), "\n"))
	}
	if _, err := os.Stat(filepath.Join(e.serverDir(), "backup.sh")); err == nil {
		t.Error("a backup.sh in the user's home on Linux")
	}
}

// An install from before round 45 (a database password, no application
// role) keeps its superuser connection on upgrade: the role is not created
// under a running database, and the server only warns.
func TestServerUpgradeKeepsTheOldDatabaseRole(t *testing.T) {
	e, fr, out := testEnv(t, linux, "", true)
	fr.Have["docker"] = true
	host, port := readyServer(t)
	no := false
	e.Answers = Answers{Mode: "server", Server: ServerAnswers{Bind: host, Port: port, AdminHandle: "anna", AdminName: "Anna", AdminPassword: "correct-horse-battery", Backups: &no}}
	if err := e.askServer(); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(e.serverDir(), ".env")
	old := strings.Repeat("ab", 24)
	if err := os.MkdirAll(e.serverDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("POSTGRES_PASSWORD="+old+"\nCOLONY_DATABASE_URL=postgres://colony:"+old+"@db:5432/colonyhall?sslmode=disable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.Do(e.ServerSteps(true)); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	env := readEnv(envPath)
	if env["COLONY_APP_DB_PASSWORD"] != "" || env["COLONY_DATABASE_URL"] != "postgres://colony:"+old+"@db:5432/colonyhall?sslmode=disable" {
		t.Fatalf("an old install must keep its connection: %v", env)
	}
	for _, l := range fr.Lines() {
		if strings.Contains(l, "psql") || strings.Contains(l, "--wait db") {
			t.Fatalf("no role change on an old install: %s", l)
		}
	}
}

// Round 40: an existing installation gets the server's container hardening
// on `colony setup --server --upgrade` (the compose file is rewritten);
// the database service stays as it was.
func TestServerUpgradeAddsContainerHardening(t *testing.T) {
	e, fr, out := testEnv(t, linux, "", true)
	fr.Have["docker"] = true
	host, port := readyServer(t)
	no := false
	e.Answers = Answers{Mode: "server", Server: ServerAnswers{Bind: host, Port: port, AdminHandle: "anna", AdminName: "Anna", AdminPassword: "correct-horse-battery", Backups: &no}}
	if err := e.askServer(); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(e.serverDir(), "compose.yaml")
	if err := os.MkdirAll(e.serverDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	// A compose file from before round 40.
	if err := os.WriteFile(composePath, []byte("services:\n  server:\n    image: ${COLONY_IMAGE}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Do(e.ServerSteps(true)); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	raw, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	c := string(raw)
	others, server, _ := strings.Cut(c, "  server:\n")
	for _, want := range []string{"read_only: true", "- /tmp:size=16m,mode=1777", "cap_drop: [ALL]", `security_opt: ["no-new-privileges:true"]`,
		"mem_limit: ${COLONY_MEM_LIMIT:-1g}", "pids_limit: ${COLONY_PIDS_LIMIT:-256}", "TMPDIR: /data/files/.tmp", "GOMEMLIMIT: ${COLONY_GOMEMLIMIT:-768MiB}",
		"COLONY_LOGIN_ALLOWLIST: ${COLONY_LOGIN_ALLOWLIST:-}", "COLONY_TRUSTED_PROXIES: ${COLONY_TRUSTED_PROXIES:-}"} {
		if !strings.Contains(server, want) {
			t.Errorf("the upgraded server service lacks %q", want)
		}
	}
	if !strings.Contains(others, "  db:\n") || strings.Contains(others, "read_only") || strings.Contains(others, "cap_drop") {
		t.Errorf("the database service must be there and not hardened by this round:\n%s", others)
	}
}

// Round 40: a secret is written into a new 0600 temporary file; a stale
// "<file>.new" with a loose mode or a symlink there plays no part.
func TestWriteFileIgnoresStaleNewFile(t *testing.T) {
	e, _, _ := testEnv(t, linux, "", true)
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(dir, ".env")
	if err := os.Symlink(victim, env+".new"); err != nil {
		t.Fatal(err)
	}
	if err := e.WriteFile(env, []byte("POSTGRES_PASSWORD=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(victim); string(raw) != "keep" {
		t.Fatal("the secret went through a symlink")
	}
	if fi, _ := os.Stat(env); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}
