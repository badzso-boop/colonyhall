package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/guard"
)

// Check statuses.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// Check is one doctor result. The same list goes into the host report
// (HostReport.doctor) for the Machines page.
type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"` // facts: a version, a path
	Fix    string `json:"fix,omitempty"`    // message key of what to do
	// fixArgs fill the fix message.
	fixArgs []any
}

// Message is the check as a line in the user's language.
func (c Check) Message(l Lang) string {
	mark := map[string]string{StatusOK: "✓", StatusWarn: "!", StatusFail: "✗"}[c.Status]
	s := fmt.Sprintf("%s %s", mark, l.T("chk."+c.ID))
	if c.Detail != "" {
		s += ": " + c.Detail
	}
	if c.Fix != "" && c.Status != StatusOK {
		s += "\n    " + l.T(c.Fix, c.fixArgs...)
	}
	return s
}

func init() {
	for k, v := range map[string][2]string{
		"chk.server":    {"server", "szerver"},
		"chk.token":     {"machine token", "géptoken"},
		"chk.config":    {"guard.yaml", "guard.yaml"},
		"chk.claude":    {"Claude Code", "Claude Code"},
		"chk.agy":       {"Antigravity (agy)", "Antigravity (agy)"},
		"chk.sandbox":   {"sandbox (bubblewrap)", "sandbox (bubblewrap)"},
		"chk.tmux":      {"tmux (agent terminals)", "tmux (agent-terminálok)"},
		"chk.git":       {"git", "git"},
		"chk.service":   {"guard service", "őr-szolgáltatás"},
		"chk.disk":      {"free disk space", "szabad hely"},
		"fix.server":    {"Check the address in guard.yaml and that the server runs (on the server: colony setup --server).", "Ellenőrizd a címet a guard.yaml-ban, és hogy fut-e a szerver (a szerveren: colony setup --server)."},
		"fix.token":     {"Pair the machine again: colony setup --guard (a new code from Machines → New machine).", "Párosítsd újra a gépet: colony setup --guard (új kód: Gépek → Új gép)."},
		"fix.config":    {"Run colony setup --guard to write it again.", "Futtasd a colony setup --guard-ot, hogy újraírja."},
		"fix.install":   {"Install it: colony setup --guard, or your package manager.", "Telepítsd: colony setup --guard, vagy a csomagkezelővel."},
		"fix.sandbox":   {"Sandboxed agents cannot run: install bubblewrap (colony setup --guard).", "Sandboxos agent nem futhat: telepítsd a bubblewrapot (colony setup --guard)."},
		"fix.noSandbox": {"No sandbox on this system: only full-access agents run here.", "Ezen a rendszeren nincs sandbox: csak teljes hozzáférésű agent futhat."},
		"fix.service":   {"Start it: %s", "Indítsd el: %s"},
		"fix.disk":      {"Free some space in %s.", "Szabadíts fel helyet itt: %s."},
	} {
		messages[k] = v
	}
}

// Login is a client's login check.
type Login struct {
	Client    string
	Installed bool
	LoggedIn  bool
	Token     bool // a claude setup-token token from the web app is set
}

// Message is the login line, with what to do when not logged in.
func (l Login) Message(lang Lang) string {
	name := map[string]string{"claude": "Claude Code", "agy": "Antigravity (agy)"}[l.Client]
	switch {
	case !l.Installed:
		return lang.T("cliMissing", name)
	case l.Token:
		return lang.T("loginToken", name)
	case l.LoggedIn:
		return lang.T("loginOK", name)
	case l.Client == "claude":
		return lang.T("loginClaude")
	default:
		return lang.T("loginAgy")
	}
}

// CheckLogins asks each installed client for its usage (no model call):
// success means it is logged in. It never logs in.
func CheckLogins(ctx context.Context, r Runner, configDir string) []Login {
	var out []Login
	for _, c := range []struct{ name, bin string }{{"claude", "claude"}, {"agy", "agy"}} {
		l := Login{Client: c.name, Installed: r.Look(c.bin)}
		if !l.Installed {
			continue
		}
		if c.name == "claude" {
			if fi, err := os.Stat(filepath.Join(configDir, "claude.token")); err == nil && fi.Size() > 0 {
				l.Token = true
			}
		}
		args := []string{"-p", "/usage", "--output-format", "json"}
		if c.name == "claude" {
			args = append(args, "--no-session-persistence", "--strict-mcp-config")
		}
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		_, err := r.Run(cctx, Cmd{Name: c.bin, Args: args})
		cancel()
		l.LoggedIn = err == nil
		out = append(out, l)
	}
	return out
}

// Doctor checks this machine's guard.
func Doctor(ctx context.Context, e *Env) []Check {
	var out []Check
	add := func(c Check) { out = append(out, c) }

	cfg, err := guard.LoadConfig(e.guardConfigPath())
	if err != nil {
		add(Check{ID: "config", Status: StatusFail, Detail: e.Mask(err.Error()), Fix: "fix.config"})
	} else {
		add(Check{ID: "config", Status: StatusOK, Detail: e.guardConfigPath()})
		res, err := e.HTTP.Get(cfg.Server + "/readyz")
		switch {
		case err != nil:
			add(Check{ID: "server", Status: StatusFail, Detail: e.Mask(err.Error()), Fix: "fix.server"})
		default:
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				add(Check{ID: "server", Status: StatusOK, Detail: cfg.Server})
			} else {
				add(Check{ID: "server", Status: StatusFail, Detail: res.Status, Fix: "fix.server"})
			}
		}
		add(e.tokenCheck(ctx, cfg))
	}
	for _, l := range CheckLogins(ctx, e.Run, e.configDir()) {
		c := Check{ID: l.Client, Status: StatusOK, Detail: "logged in"}
		if l.Token {
			c.Detail = "token from the web app"
		} else if !l.LoggedIn {
			c.Status, c.Detail = StatusWarn, "not logged in"
			c.Fix = map[string]string{"claude": "loginClaude", "agy": "loginAgy"}[l.Client]
		}
		add(c)
	}
	switch {
	case !e.P.Sandbox():
		add(Check{ID: "sandbox", Status: StatusWarn, Detail: "none", Fix: "fix.noSandbox"})
	case e.Run.Look("bwrap"):
		add(Check{ID: "sandbox", Status: StatusOK})
	default:
		add(Check{ID: "sandbox", Status: StatusWarn, Detail: "bwrap missing", Fix: "fix.sandbox"})
	}
	for _, p := range []string{"tmux", "git"} {
		if e.Run.Look(p) {
			add(Check{ID: p, Status: StatusOK})
		} else {
			add(Check{ID: p, Status: StatusWarn, Detail: "missing", Fix: "fix.install"})
		}
	}
	add(e.serviceCheck(ctx))
	add(diskCheck(e.Home))
	return out
}

func (e *Env) tokenCheck(ctx context.Context, cfg guard.Config) Check {
	raw, err := os.ReadFile(cfg.HostTokenFile)
	if err != nil {
		return Check{ID: "token", Status: StatusFail, Detail: "missing", Fix: "fix.token"}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.Server+"/api/v1/host/config", nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(raw)))
	res, err := e.HTTP.Do(req)
	if err != nil {
		return Check{ID: "token", Status: StatusWarn, Detail: "server unreachable"}
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return Check{ID: "token", Status: StatusFail, Detail: "refused (revoked?)", Fix: "fix.token"}
	}
	return Check{ID: "token", Status: StatusOK}
}

func (e *Env) serviceCheck(ctx context.Context) Check {
	if e.P.OS == "darwin" {
		_, err := e.Run.Run(ctx, Cmd{Name: "launchctl", Args: []string{"print", fmt.Sprintf("gui/%d/hu.colonyhall.guard", os.Getuid())}})
		if err != nil {
			return Check{ID: "service", Status: StatusFail, Detail: "not loaded", Fix: "fix.service", fixArgs: []any{"colony setup --guard"}}
		}
		return Check{ID: "service", Status: StatusOK, Detail: "launchd"}
	}
	out, err := e.Run.Run(ctx, Cmd{Name: "systemctl", Args: []string{"--user", "is-active", "colony-guard"}})
	if err != nil || strings.TrimSpace(out) != "active" {
		return Check{ID: "service", Status: StatusFail, Detail: strings.TrimSpace(out), Fix: "fix.service", fixArgs: []any{"systemctl --user restart colony-guard"}}
	}
	return Check{ID: "service", Status: StatusOK, Detail: "systemd"}
}

func diskCheck(home string) Check {
	var st syscall.Statfs_t
	if err := syscall.Statfs(home, &st); err != nil {
		return Check{ID: "disk", Status: StatusWarn, Detail: err.Error()}
	}
	free := st.Bavail * uint64(st.Bsize)
	gb := float64(free) / (1 << 30)
	c := Check{ID: "disk", Status: StatusOK, Detail: fmt.Sprintf("%.1f GB", gb)}
	if gb < 2 {
		c.Status, c.Fix, c.fixArgs = StatusWarn, "fix.disk", []any{home}
	}
	return c
}

// DoctorJSON is the doctor's result for --json and the host report.
func DoctorJSON(checks []Check) ([]byte, error) { return json.MarshalIndent(checks, "", "  ") }

// ReportChecks runs the doctor for the guard's host report
// (guard.DoctorHook): ids, statuses and facts, never secrets.
func ReportChecks(ctx context.Context) []apispec.DoctorCheck {
	e, err := newEnv(ctx)
	if err != nil {
		return nil
	}
	e.HTTP = &http.Client{Timeout: 10 * time.Second}
	return toReport(Doctor(ctx, e))
}

func toReport(checks []Check) []apispec.DoctorCheck {
	out := make([]apispec.DoctorCheck, 0, len(checks))
	for _, c := range checks {
		d := apispec.DoctorCheck{Id: c.ID, Status: apispec.DoctorCheckStatus(c.Status)}
		if c.Detail != "" {
			detail := c.Detail
			d.Detail = &detail
		}
		out = append(out, d)
	}
	return out
}
