package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/guard"
)

// configDir is ~/.config/colonyhall (guard.yaml, host.token, claude.token).
func (e *Env) configDir() string { return filepath.Join(e.Home, ".config", "colonyhall") }

func (e *Env) guardConfigPath() string { return filepath.Join(e.configDir(), "guard.yaml") }

// defaultRoots: ~/projects, or ~/code when only that exists.
func (e *Env) defaultRoots() []string {
	for _, d := range []string{"projects", "code", "src"} {
		p := filepath.Join(e.Home, d)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return []string{p}
		}
	}
	return []string{filepath.Join(e.Home, "projects")}
}

// askGuard fills the guard answers.
func (e *Env) askGuard() error {
	a := &e.Answers.Guard
	var err error
	if a.ServerURL == "" {
		def := ""
		if e.Answers.Mode == "both" {
			def = e.serverURL()
		}
		if a.ServerURL, err = e.UI.Ask("grdServer", def); err != nil {
			return err
		}
	}
	a.ServerURL = strings.TrimRight(a.ServerURL, "/")
	if !strings.HasPrefix(a.ServerURL, "http://") && !strings.HasPrefix(a.ServerURL, "https://") {
		return fmt.Errorf("the server address must start with http:// or https:// (got %q)", a.ServerURL)
	}
	if a.Name == "" {
		host, _ := os.Hostname()
		if a.Name, err = e.UI.Ask("grdName", strings.ToLower(strings.Split(host, ".")[0])); err != nil {
			return err
		}
	}
	if len(a.Roots) == 0 {
		if e.P.WSL {
			e.UI.Say("grdRootsWSL")
		}
		r, err := e.UI.Ask("grdRoots", strings.Join(e.defaultRoots(), ","))
		if err != nil {
			return err
		}
		for _, p := range strings.Split(r, ",") {
			if p = strings.TrimSpace(p); p != "" {
				a.Roots = append(a.Roots, p)
			}
		}
	}
	if a.AllowUnsandboxed == nil {
		if !e.P.Sandbox() {
			e.UI.Say("grdNoSandboxMac")
			t := true
			a.AllowUnsandboxed = &t
		} else {
			ok, err := e.UI.Confirm("grdFull", true)
			if err != nil {
				return err
			}
			a.AllowUnsandboxed = &ok
		}
	}
	if a.MaxConcurrentRuns == nil {
		s, err := e.UI.Ask("grdMax", "2")
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return fmt.Errorf("bad number %q", s)
		}
		a.MaxConcurrentRuns = &n
	}
	if a.PreviewPorts == "" {
		a.PreviewPorts = "41000-41999"
	}
	return nil
}

// GuardYAML is the guard's commented config from the answers.
func GuardYAML(p Platform, home string, a GuardAnswers) string {
	var b strings.Builder
	cfg := filepath.Join(home, ".config", "colonyhall")
	_, _ = fmt.Fprintf(&b, `# Colonyhall guard, written by colony setup (docs/16-telepites.md).
# Edit and restart the service to apply (%s).

# The Colonyhall server this machine belongs to.
server: %s

# The machine's token (colony guard enroll writes it, 0600).
host_token_file: %s

# Folders the admin may give agents (read or write); nothing outside them.
roots:
`, restartHint(p), yamlString(a.ServerURL), yamlString(filepath.Join(cfg, "host.token")))
	for _, r := range a.Roots {
		_, _ = fmt.Fprintf(&b, "  - %s\n", yamlString(r))
	}
	if p.WSL {
		b.WriteString("  # Windows drives (/mnt/c …) are slow from WSL and Windows permissions apply; add one only if needed.\n")
	}
	b.WriteString("\n# The Claude Code login token set from the web app (Machines → Claude login).\n")
	_, _ = fmt.Fprintf(&b, "claude_token_file: %s\n\n", yamlString(filepath.Join(cfg, "claude.token")))
	if p.Sandbox() {
		b.WriteString("# Sandboxed agents run under bubblewrap; without it they do not run (require_sandbox).\n")
		b.WriteString("require_sandbox: true\n")
		b.WriteString("# The sandbox's network: filtered (only the AI providers and this server), log, or open.\n")
		b.WriteString("sandbox_network: filtered\n")
	} else {
		b.WriteString("# No sandbox on this system: only full-access agents run here.\n")
		b.WriteString("require_sandbox: false\n")
	}
	b.WriteString("\n# Full-access agents: no sandbox, this user's own login, settings and rights.\n")
	_, _ = fmt.Fprintf(&b, "allow_unsandboxed: %t\n\n", boolOr(a.AllowUnsandboxed, false))
	b.WriteString("# How many agent runs at once (0: no limit); more wait their turn.\n")
	n := 2
	if a.MaxConcurrentRuns != nil {
		n = *a.MaxConcurrentRuns
	}
	_, _ = fmt.Fprintf(&b, "max_concurrent_runs: %d\n\n", n)
	b.WriteString("# Ports for app previews (full-access agents with previews on).\n")
	_, _ = fmt.Fprintf(&b, "preview_ports: %s\n", yamlString(a.PreviewPorts))
	if a.ExchangeDir != "" {
		b.WriteString("\n# A folder for handing secrets to agents instead of the chat.\n")
		_, _ = fmt.Fprintf(&b, "exchange_dir: %s\n", yamlString(a.ExchangeDir))
	}
	return b.String()
}

func restartHint(p Platform) string {
	if p.OS == "darwin" {
		return "launchctl kickstart -k gui/$(id -u)/hu.colonyhall.guard"
	}
	return "systemctl --user restart colony-guard"
}

// yamlString quotes a scalar when YAML needs it.
func yamlString(s string) string {
	if s == "" || strings.ContainsAny(s, ":#{}[],&*!|>'\"%@`") || strings.TrimSpace(s) != s {
		raw, _ := json.Marshal(s)
		return string(raw)
	}
	return s
}

// EnrollFunc pairs the machine (guard.Enroll; tests replace it).
type EnrollFunc func(ctx context.Context, server, code, name, tokenFile string) error

// launchdPlist is a LaunchAgent: a program, run at load and kept alive, or
// (hour >= 0) run daily at that hour.
func launchdPlist(label string, args []string, path string, keepAlive bool, logDir string, hour int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + xmlEscape(label) + `</string>
  <key>ProgramArguments</key>
  <array>
`)
	for _, a := range args {
		b.WriteString("    <string>" + xmlEscape(a) + "</string>\n")
	}
	b.WriteString("  </array>\n")
	if path != "" {
		b.WriteString("  <key>EnvironmentVariables</key>\n  <dict><key>PATH</key><string>" + xmlEscape(path) + "</string></dict>\n")
	}
	if keepAlive {
		b.WriteString("  <key>RunAtLoad</key><true/>\n  <key>KeepAlive</key><true/>\n")
	}
	if hour >= 0 && !keepAlive {
		_, _ = fmt.Fprintf(&b, "  <key>StartCalendarInterval</key><dict><key>Hour</key><integer>%d</integer><key>Minute</key><integer>30</integer></dict>\n", hour)
	}
	b.WriteString("  <key>StandardOutPath</key><string>" + xmlEscape(filepath.Join(logDir, label+".log")) + "</string>\n")
	b.WriteString("  <key>StandardErrorPath</key><string>" + xmlEscape(filepath.Join(logDir, label+".log")) + "</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func (e *Env) launchAgentPath() string {
	return filepath.Join(e.Home, "Library", "LaunchAgents", "hu.colonyhall.guard.plist")
}

// servicePath is the PATH the service gets: this shell's, so it finds the
// same claude and agy (systemd and launchd start with a minimal one).
func servicePath() string {
	p := os.Getenv("PATH")
	if p == "" {
		p = "/usr/local/bin:/usr/bin:/bin"
	}
	return p
}

// pairingCode asks the server for a code as the owner just created (setup
// "both"): it signs in and asks POST /hosts/pairing-codes.
func (e *Env) pairingCode(server, handle, password string) (string, error) {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Timeout: 15 * time.Second, Jar: jar}
	body, _ := json.Marshal(map[string]string{"handle": handle, "password": password})
	res, err := c.Post(server+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sign-in as @%s: %s", handle, res.Status)
	}
	var sess struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&sess); err != nil {
		return "", err
	}
	req, _ := http.NewRequestWithContext(e.Ctx, http.MethodPost, server+"/api/v1/hosts/pairing-codes", nil)
	req.Header.Set("X-CSRF-Token", sess.CSRF)
	res2, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("pairing code: %s", res2.Status)
	}
	var pc struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&pc); err != nil {
		return "", err
	}
	return pc.Code, nil
}

// GuardSteps install (or repair) the guard.
func (e *Env) GuardSteps(enroll EnrollFunc) []Step {
	if enroll == nil {
		enroll = guard.Enroll
	}
	a := &e.Answers.Guard
	tokenFile := filepath.Join(e.configDir(), "host.token")
	steps := []Step{
		depsStep("grd.deps", func(e *Env) []dep { return guardDeps(e.P) }),
		{
			ID: "grd.config", Title: e.guardConfigPath(),
			Apply: func(e *Env) error {
				return e.WriteFile(e.guardConfigPath(), []byte(GuardYAML(e.P, e.Home, *a)), 0o600)
			},
		},
		{
			ID: "grd.enroll", Title: e.UI.T("grdEnrolled"),
			Done: func(e *Env) (bool, error) {
				_, err := os.Stat(tokenFile)
				return err == nil, nil
			},
			Apply: func(e *Env) error {
				if e.DryRun {
					e.plan = append(e.plan, "colony guard enroll -server "+a.ServerURL+" -code … -name "+a.Name)
					return nil
				}
				if a.PairCode == "" && e.Answers.Mode == "both" && e.ownerPassword != "" {
					code, err := e.pairingCode(a.ServerURL, e.Answers.Server.AdminHandle, e.ownerPassword)
					if err != nil {
						return err
					}
					a.PairCode = code
				}
				if a.PairCode == "" {
					code, err := e.UI.Ask("grdCode", "")
					if err != nil {
						return err
					}
					a.PairCode = code
				}
				e.Secret(a.PairCode)
				return enroll(e.Ctx, a.ServerURL, a.PairCode, a.Name, tokenFile)
			},
		},
	}
	steps[0].Title = e.UI.T("depsTitle")
	steps = append(steps, e.serviceSteps()...)
	steps = append(steps, e.clientSteps()...)
	steps = append(steps, Step{
		ID: "grd.summary", Title: e.UI.T("finished"),
		Done: func(*Env) (bool, error) { return false, nil },
		Apply: func(e *Env) error {
			e.UI.Say("grdNextAgent")
			e.UI.Say("grdColleague")
			return nil
		},
	})
	return steps
}

// serviceSteps start the guard automatically: systemd (Linux, WSL) or
// launchd (macOS), and on WSL the systemd switch and the keepalive.
func (e *Env) serviceSteps() []Step {
	a := &e.Answers.Guard
	if e.P.OS == "darwin" {
		return []Step{{
			ID: "grd.launchd", Title: e.UI.T("grdService", "launchd"),
			Apply: func(e *Env) error {
				plist := launchdPlist("hu.colonyhall.guard", []string{e.Self, "guard", "-config", e.guardConfigPath()}, servicePath(), true,
					filepath.Join(e.Home, "Library", "Logs", "colonyhall"), -1)
				if err := e.WriteFile(e.launchAgentPath(), []byte(plist), 0o644); err != nil {
					return err
				}
				uid := strconv.Itoa(os.Getuid())
				_, _ = e.Exec(Cmd{Name: "launchctl", Args: []string{"bootout", "gui/" + uid + "/hu.colonyhall.guard"}})
				_, err := e.Exec(Cmd{Name: "launchctl", Args: []string{"bootstrap", "gui/" + uid, e.launchAgentPath()}})
				return err
			},
		}}
	}
	var steps []Step
	if e.P.WSL && !e.P.Systemd {
		steps = append(steps, Step{
			ID: "grd.wslsystemd", Title: "/etc/wsl.conf: systemd=true",
			Confirm: "grdWSLSystemd",
			Plan:    func(*Env) []string { return []string{"sudo tee -a /etc/wsl.conf  ([boot] systemd=true)"} },
			Apply: func(e *Env) error {
				conf, _ := os.ReadFile("/etc/wsl.conf")
				if _, err := e.Exec(Cmd{Name: "tee", Args: []string{"/etc/wsl.conf"}, Stdin: wslConfWithSystemd(string(conf)), Sudo: true}); err != nil {
					return err
				}
				e.UI.Say("grdWSLRestart")
				return errRestartWSL
			},
		})
		return steps // the service needs systemd: the next run continues
	}
	unitPath := filepath.Join(e.Home, ".config", "systemd", "user", "colony-guard.service")
	steps = append(steps, Step{
		ID: "grd.systemd", Title: e.UI.T("grdService", "systemd"),
		Apply: func(e *Env) error {
			unit, err := guard.GenerateUnit(e.Self, e.guardConfigPath(), servicePath())
			if err != nil {
				return err
			}
			if err := e.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
				return err
			}
			if _, err := e.Exec(Cmd{Name: "systemctl", Args: []string{"--user", "daemon-reload"}}); err != nil {
				return err
			}
			_, err = e.Exec(Cmd{Name: "systemctl", Args: []string{"--user", "enable", "--now", "colony-guard"}})
			return err
		},
	}, Step{
		ID: "grd.linger", Title: "loginctl enable-linger",
		Done: func(e *Env) (bool, error) {
			if !boolOr(a.Linger, true) {
				return true, nil
			}
			user := os.Getenv("USER")
			_, err := os.Stat("/var/lib/systemd/linger/" + user)
			return user != "" && err == nil, nil
		},
		Confirm: "grdLinger",
		Plan:    func(*Env) []string { return []string{"sudo loginctl enable-linger " + os.Getenv("USER")} },
		Apply: func(e *Env) error {
			_, err := e.Exec(Cmd{Name: "loginctl", Args: []string{"enable-linger", os.Getenv("USER")}, Sudo: true})
			return err
		},
	})
	if e.P.WSL {
		steps = append(steps, e.keepaliveStep())
	}
	return steps
}

var errRestartWSL = fmt.Errorf("restart WSL (wsl --shutdown), then run colony setup again")

// wslConfWithSystemd turns systemd on in an /etc/wsl.conf.
func wslConfWithSystemd(conf string) string {
	lines := strings.Split(strings.TrimRight(conf, "\n"), "\n")
	inBoot, set := false, false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			if inBoot && !set {
				lines = append(lines[:i], append([]string{"systemd=true"}, lines[i:]...)...)
				set = true
				break
			}
			inBoot = t == "[boot]"
			continue
		}
		if inBoot && strings.HasPrefix(strings.ReplaceAll(t, " ", ""), "systemd=") {
			lines[i] = "systemd=true"
			set = true
		}
	}
	out := strings.Join(lines, "\n")
	if strings.TrimSpace(conf) == "" {
		out = ""
	}
	if !set {
		if inBoot {
			out += "\nsystemd=true"
		} else {
			if out != "" {
				out += "\n\n"
			}
			out += "[boot]\nsystemd=true"
		}
	}
	return out + "\n"
}

// keepaliveStep adds a Windows logon task that keeps this distribution
// running (conhost --headless wsl.exe -d <distro> -- sleep infinity).
func (e *Env) keepaliveStep() Step {
	a := &e.Answers.Guard
	distro := e.P.Distro
	task := "Colonyhall WSL keepalive (" + distro + ")"
	ps := keepalivePowerShell(task, distro)
	return Step{
		ID: "grd.keepalive", Title: task,
		Done: func(e *Env) (bool, error) {
			if a.Keepalive != nil && !*a.Keepalive {
				e.UI.Say("grdNoKeepalive")
				return true, nil
			}
			return distro == "", nil
		},
		Confirm: "grdKeepalive",
		Plan:    func(*Env) []string { return []string{"powershell.exe -NoProfile -Command " + ps} },
		Apply: func(e *Env) error {
			_, err := e.Exec(Cmd{Name: "powershell.exe", Args: []string{"-NoProfile", "-NonInteractive", "-Command", ps}})
			return err
		},
	}
}

func keepalivePowerShell(task, distro string) string {
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	return "$a = New-ScheduledTaskAction -Execute 'conhost.exe' -Argument " + q("--headless wsl.exe -d "+distro+" -- sleep infinity") +
		"; $t = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME" +
		"; $s = New-ScheduledTaskSettingsSet -ExecutionTimeLimit 0 -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries" +
		"; Register-ScheduledTask -TaskName " + q(task) + " -Action $a -Trigger $t -Settings $s -Force | Out-Null" +
		"; Start-ScheduledTask -TaskName " + q(task)
}

// clientInstallers are the AI clients' own installers. Antigravity's is not
// known yet (Norbi, 2026-11-05): another agent fills it in.
var clientInstallers = map[string]string{
	"claude": "https://claude.ai/install.sh",
	"agy":    "",
}

// clientSteps install the chosen AI clients (download, show, confirm, run)
// and check their login.
func (e *Env) clientSteps() []Step {
	a := &e.Answers.Guard
	return []Step{{
		ID: "grd.clients", Title: "AI",
		Done: func(*Env) (bool, error) { return false, nil },
		Apply: func(e *Env) error {
			if a.Clients == nil && !e.UI.Yes {
				s, err := e.UI.Ask("cliPick", "claude")
				if err != nil {
					return err
				}
				for _, c := range strings.Split(s, ",") {
					if c = strings.TrimSpace(c); c != "" {
						a.Clients = append(a.Clients, c)
					}
				}
			}
			for _, c := range a.Clients {
				if e.Run.Look(c) {
					continue
				}
				if err := e.installClient(c); err != nil {
					return err
				}
			}
			if e.DryRun {
				return nil
			}
			for _, r := range CheckLogins(e.Ctx, e.Run, e.configDir()) {
				e.UI.Sayf("%s", r.Message(e.UI.Lang))
			}
			return nil
		},
	}}
}

// installClient downloads a client's installer, shows its address, size
// and SHA-256 (and, on request, the script), and runs it after a yes.
func (e *Env) installClient(name string) error {
	src := clientInstallers[name]
	if src == "" {
		e.UI.Say("cliNoAgy")
		return nil
	}
	e.UI.Say("cliMissing", name)
	if e.DryRun {
		e.plan = append(e.plan, "download "+src+", show its SHA-256, run it with sh after a confirmation")
		return nil
	}
	res, err := e.HTTP.Get(src)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", src, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	e.UI.Say("cliInstaller", name, src, len(body), hex.EncodeToString(sum[:]))
	if show, err := e.UI.Confirm("cliShow", false); err != nil {
		return err
	} else if show {
		e.UI.Sayf("%s", body)
	}
	if ok, err := e.UI.Confirm("cliRun", false, name); err != nil || !ok {
		return err
	}
	f, err := os.CreateTemp("", "colony-"+name+"-install-*.sh")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(body); err != nil {
		return err
	}
	_ = f.Close()
	_, err = e.Exec(Cmd{Name: "sh", Args: []string{f.Name()}})
	return err
}
