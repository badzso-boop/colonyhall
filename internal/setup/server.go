package setup

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultRepo is where the releases and the server image come from.
const DefaultRepo = "badzso-boop/colonyhall"

// serverDir is the server's folder (compose.yaml, .env).
func (e *Env) serverDir() string {
	if d := e.Answers.Server.Dir; d != "" {
		return d
	}
	return filepath.Join(e.Home, ".local", "share", "colonyhall", "server")
}

// imageFor is the server image of this installer's version.
func imageFor(version string) string {
	tag := "latest"
	if strings.HasPrefix(version, "v") {
		tag = version
	}
	return "ghcr.io/" + DefaultRepo + ":" + tag
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// readEnv reads an existing .env (a re-run keeps its secrets).
func readEnv(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

// composeFile is the server's compose.yaml: the server and, unless an
// existing database is used, its own Postgres on an internal network.
func composeFile(ownDB bool) string {
	var b strings.Builder
	b.WriteString(`# Colonyhall server, written by colony setup (docs/16-telepites.md).
# The settings and secrets are in .env next to this file (0600).
name: colonyhall

services:
`)
	if ownDB {
		b.WriteString(`  db:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: colony
      POSTGRES_DB: colonyhall
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?missing in .env}
    volumes:
      - db:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U colony -d colonyhall"]
      interval: 5s
      timeout: 5s
      retries: 20

`)
	}
	b.WriteString(`  server:
    image: ${COLONY_IMAGE:?missing in .env}
    restart: unless-stopped
`)
	if ownDB {
		b.WriteString(`    depends_on:
      db:
        condition: service_healthy
`)
	}
	b.WriteString(`    environment:
      COLONY_STORE: postgres
      COLONY_DATABASE_URL: ${COLONY_DATABASE_URL:?missing in .env}
      COLONY_HTTP_ADDR: ":8080"
      COLONY_SECURE_COOKIES: ${COLONY_SECURE_COOKIES:-0}
      COLONY_FILES_DIR: /data/files
      COLONY_MAX_UPLOAD_MB: ${COLONY_MAX_UPLOAD_MB:-25}
      COLONY_GITEA_URL: ${COLONY_GITEA_URL:-}
      COLONY_GITEA_PUBLIC_URL: ${COLONY_GITEA_PUBLIC_URL:-}
      COLONY_GITEA_TOKEN: ${COLONY_GITEA_TOKEN:-}
      COLONY_GITEA_WEBHOOK_SECRET: ${COLONY_GITEA_WEBHOOK_SECRET:-}
    volumes:
      - files:/data/files
    ports:
      - "${COLONY_BIND:-127.0.0.1}:${COLONY_PORT:-8080}:8080"

volumes:
`)
	if ownDB {
		b.WriteString("  db:\n")
	}
	b.WriteString("  files:\n")
	return b.String()
}

// envFile is the server's .env.
func envFile(v map[string]string) string {
	keys := []string{"COLONY_IMAGE", "COLONY_BIND", "COLONY_PORT", "COLONY_SECURE_COOKIES", "POSTGRES_PASSWORD", "COLONY_DATABASE_URL"}
	var b strings.Builder
	b.WriteString("# Colonyhall server settings and secrets (colony setup). Keep it 0600 and back it up.\n")
	for _, k := range keys {
		if val, ok := v[k]; ok {
			_, _ = fmt.Fprintf(&b, "%s=%s\n", k, val)
		}
	}
	return b.String()
}

// localAddrs finds a LAN and a Tailscale (100.64.0.0/10) IPv4 address.
func localAddrs() (lan, tailscale string) {
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || ipn.IP.IsLoopback() {
			continue
		}
		switch {
		case cgnat.Contains(ipn.IP):
			if tailscale == "" {
				tailscale = ipn.IP.String()
			}
		case ipn.IP.IsPrivate():
			if lan == "" {
				lan = ipn.IP.String()
			}
		}
	}
	return lan, tailscale
}

// docker builds a docker command; Linux runs it with sudo (Norbi's decision:
// being in the docker group is root-equivalent, so sudo is the default).
func (e *Env) docker(args ...string) Cmd {
	return Cmd{Name: "docker", Args: args, Sudo: e.P.OS == "linux"}
}

func (e *Env) compose(args ...string) Cmd {
	dir := e.serverDir()
	return e.docker(append([]string{"compose", "--project-directory", dir, "-f", filepath.Join(dir, "compose.yaml"), "--env-file", filepath.Join(dir, ".env")}, args...)...)
}

// askServer fills the server answers (an existing .env gives the defaults).
func (e *Env) askServer() error {
	a := &e.Answers.Server
	env := readEnv(filepath.Join(e.serverDir(), ".env"))
	if a.Image == "" {
		a.Image = imageFor(e.Version)
	}
	if a.Bind == "" {
		lan, ts := localAddrs()
		opts, labels := []string{"127.0.0.1"}, []string{e.UI.T("srvBindLocal")}
		if lan != "" {
			opts, labels = append(opts, lan), append(labels, e.UI.T("srvBindLAN", lan))
		}
		if ts != "" {
			opts, labels = append(opts, ts), append(labels, e.UI.T("srvBindTS", ts))
		}
		opts, labels = append(opts, "0.0.0.0"), append(labels, e.UI.T("srvBindAll"))
		def := env["COLONY_BIND"]
		if def == "" {
			def = "127.0.0.1"
		}
		b, err := e.UI.Choose("srvBind", opts, labels, def)
		if err != nil {
			return err
		}
		a.Bind = b
	}
	if a.Bind == "0.0.0.0" && !a.AllowAll {
		ok, err := e.UI.Confirm("srvBindAllWarn", false)
		if err != nil {
			return err
		}
		if !ok || e.UI.Yes {
			// --yes never opens the server to every network by itself.
			return errors.New("0.0.0.0 needs allow_all_interfaces: true in the answers file")
		}
		a.AllowAll = true
	}
	if a.Port == 0 {
		def := env["COLONY_PORT"]
		if def == "" {
			def = "8080"
		}
		p, err := e.UI.Ask("srvPort", def)
		if err != nil {
			return err
		}
		if a.Port, err = strconv.Atoi(p); err != nil || a.Port < 1 || a.Port > 65535 {
			return fmt.Errorf("bad port %q", p)
		}
	}
	if a.DatabaseURL == "" {
		a.DatabaseURL = os.Getenv("COLONY_DATABASE_URL")
	}
	if a.DatabaseURL == "" && env["POSTGRES_PASSWORD"] == "" && env["COLONY_DATABASE_URL"] == "" {
		choice, err := e.UI.Choose("srvDB", []string{"own", "external"}, []string{e.UI.T("srvDBOwn"), e.UI.T("srvDBExternal")}, "own")
		if err != nil {
			return err
		}
		if choice == "external" {
			if a.DatabaseURL, err = e.UI.Secret("srvDBURL"); err != nil {
				return err
			}
		}
	} else if a.DatabaseURL == "" && env["POSTGRES_PASSWORD"] == "" {
		a.DatabaseURL = env["COLONY_DATABASE_URL"]
	}
	if a.DatabaseURL != "" {
		u, err := url.Parse(a.DatabaseURL)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
			return errors.New("the database URL must look like postgres://user:password@host:5432/db")
		}
		e.Secret(a.DatabaseURL)
		if p, ok := u.User.Password(); ok {
			e.Secret(p)
		}
		if h := u.Hostname(); h == "localhost" || h == "127.0.0.1" {
			e.UI.Say("srvDBLocalhost")
		}
	}
	if a.AdminHandle == "" {
		h, err := e.UI.Ask("srvAdminHandle", "admin")
		if err != nil {
			return err
		}
		a.AdminHandle = h
	}
	if a.AdminName == "" {
		n, err := e.UI.Ask("srvAdminName", a.AdminHandle)
		if err != nil {
			return err
		}
		a.AdminName = n
	}
	if a.Backups == nil {
		ok, err := e.UI.Confirm("srvBackups", true, e.backupDir())
		if err != nil {
			return err
		}
		a.Backups = &ok
	}
	return nil
}

// askAdminPassword asks for the owner's password (twice) or generates one.
func (e *Env) askAdminPassword() (pw string, generated bool, err error) {
	a := &e.Answers.Server
	if a.AdminPassword == "" {
		a.AdminPassword = os.Getenv("COLONY_ADMIN_PASSWORD")
	}
	if a.AdminPassword != "" {
		e.Secret(a.AdminPassword)
		return a.AdminPassword, false, nil
	}
	if e.UI.Yes {
		pw = randomHex(12)
		e.Secret(pw)
		return pw, true, nil
	}
	for {
		p1, err := e.UI.Secret("srvAdminPass")
		if err != nil {
			return "", false, err
		}
		if p1 == "" {
			pw = randomHex(12)
			e.Secret(pw)
			return pw, true, nil
		}
		p2, err := e.UI.Secret("srvAdminPass2")
		if err != nil {
			return "", false, err
		}
		if p1 == p2 && len([]rune(p1)) >= 12 {
			e.Secret(p1)
			return p1, false, nil
		}
		e.UI.Say("srvPassMismatch")
	}
}

func (e *Env) backupDir() string { return filepath.Join(e.Home, "colonyhall-backups") }

// serverURL is where this machine reaches the server.
func (e *Env) serverURL() string {
	host := e.Answers.Server.Bind
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(e.Answers.Server.Port)))
}

// serverEnv is the .env content: secrets kept from an existing file.
func (e *Env) serverEnv() (map[string]string, bool) {
	a := e.Answers.Server
	old := readEnv(filepath.Join(e.serverDir(), ".env"))
	v := map[string]string{
		"COLONY_IMAGE": a.Image, "COLONY_BIND": a.Bind, "COLONY_PORT": strconv.Itoa(a.Port), "COLONY_SECURE_COOKIES": "0",
	}
	if a.SecureCookies {
		v["COLONY_SECURE_COOKIES"] = "1"
	}
	ownDB := a.DatabaseURL == ""
	if ownDB {
		pw := old["POSTGRES_PASSWORD"]
		if pw == "" {
			pw = randomHex(24)
		}
		e.Secret(pw)
		v["POSTGRES_PASSWORD"] = pw
		v["COLONY_DATABASE_URL"] = "postgres://colony:" + pw + "@db:5432/colonyhall?sslmode=disable"
	} else {
		v["COLONY_DATABASE_URL"] = a.DatabaseURL
	}
	e.Secret(v["COLONY_DATABASE_URL"])
	return v, ownDB
}

// ServerSteps install (or repair, or upgrade) the server.
func (e *Env) ServerSteps(upgrade bool) []Step {
	if upgrade {
		e.Forget("srv.files", "srv.up", "srv.ready")
	}
	var generated string
	return []Step{
		{
			ID: "srv.deps", Title: e.UI.T("depsTitle") + ": Docker",
			Done: func(e *Env) (bool, error) {
				if !e.Run.Look("docker") {
					return false, nil
				}
				_, err := e.Run.Run(e.Ctx, Cmd{Name: "docker", Args: []string{"compose", "version"}})
				return err == nil, nil
			},
			Confirm: "proceed",
			Plan: func(e *Env) []string {
				cmds, _ := installCmds(e.P, []dep{depDocker})
				var out []string
				for _, c := range cmds {
					out = append(out, c.String())
				}
				return append(out, e.docker("info").String())
			},
			Apply: func(e *Env) error {
				if e.P.OS == "darwin" || e.P.WSL && !e.P.Systemd {
					e.UI.Say("depsDocker")
					return errDepsMissing
				}
				cmds, manual := installCmds(e.P, []dep{depDocker})
				if len(manual) > 0 || len(cmds) == 0 {
					e.UI.Say("depsManual", "docker, docker compose (https://docs.docker.com/engine/install/)")
					return errDepsMissing
				}
				for _, c := range cmds {
					if _, err := e.Exec(c); err != nil {
						return err
					}
				}
				_, err := e.Exec(Cmd{Name: "systemctl", Args: []string{"enable", "--now", "docker"}, Sudo: true})
				return err
			},
		},
		{
			ID: "srv.files", Title: "compose.yaml, .env",
			Apply: func(e *Env) error {
				if e.P.OS == "linux" {
					e.UI.Say("srvSudoDocker")
				}
				v, ownDB := e.serverEnv()
				dir := e.serverDir()
				if err := e.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(composeFile(ownDB)), 0o644); err != nil {
					return err
				}
				return e.WriteFile(filepath.Join(dir, ".env"), []byte(envFile(v)), 0o600)
			},
		},
		{
			ID: "srv.up", Title: "docker compose up",
			Apply: func(e *Env) error {
				if _, err := e.Exec(e.compose("pull")); err != nil {
					return err
				}
				_, err := e.Exec(e.compose("up", "-d"))
				return err
			},
		},
		{
			ID: "srv.ready", Title: e.UI.T("srvWaiting"),
			Apply: func(e *Env) error {
				if e.DryRun {
					e.plan = append(e.plan, "GET "+e.serverURL()+"/readyz")
					return nil
				}
				return e.waitReady(e.serverURL(), 3*time.Minute)
			},
		},
		{
			ID: "srv.owner", Title: e.UI.T("srvAdminHandle") + ": @" + e.Answers.Server.AdminHandle,
			Apply: func(e *Env) error {
				pw, gen, err := e.askAdminPassword()
				if err != nil {
					return err
				}
				a := e.Answers.Server
				_, err = e.Exec(Cmd{Name: e.compose().Name, Sudo: e.compose().Sudo,
					Args:  append(e.compose().Args, "run", "--rm", "-T", "server", "bootstrap", "-handle", a.AdminHandle, "-name", a.AdminName),
					Stdin: pw + "\n"})
				if err != nil && strings.Contains(err.Error(), "not empty") {
					return nil // an owner exists already: a repair run
				}
				if err == nil {
					e.ownerPassword = pw
					if gen {
						generated = pw
					}
				}
				return err
			},
		},
		{
			ID: "srv.backups", Title: e.UI.T("srvBackups", e.backupDir()),
			Done:  func(e *Env) (bool, error) { return !boolOr(e.Answers.Server.Backups, false), nil },
			Apply: func(e *Env) error { return e.installBackups() },
		},
		{
			ID: "srv.summary", Title: e.UI.T("finished"),
			Done: func(*Env) (bool, error) { return false, nil },
			Apply: func(e *Env) error {
				e.UI.Say("srvReady", e.serverURL()+"/app/")
				if generated != "" {
					e.UI.Say("srvPassGenerated", generated)
				}
				e.UI.Say("srvOpen", e.serverURL()+"/app/", e.Answers.Server.AdminHandle)
				e.UI.Say("srvNoTLS")
				return nil
			},
		},
	}
}

// waitReady polls /readyz until it answers 200.
func (e *Env) waitReady(base string, limit time.Duration) error {
	deadline := e.Now().Add(limit)
	for {
		res, err := e.HTTP.Get(base + "/readyz")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		if e.Now().After(deadline) {
			return fmt.Errorf("the server did not become ready at %s (see `docker compose logs server` in %s)", base, e.serverDir())
		}
		time.Sleep(2 * time.Second)
	}
}

// backupScript dumps the database (own Postgres) and archives the files
// volume into the backup folder, keeping 14 days.
func (e *Env) backupScript() string {
	dir := e.serverDir()
	c := e.compose()
	base := strings.TrimPrefix(c.String(), "sudo ")
	return fmt.Sprintf(`#!/bin/sh
# Colonyhall nightly backup (colony setup). Restore: docs/16-telepites.md.
set -eu
out=%q
mkdir -p "$out"
stamp=$(date +%%Y%%m%%d-%%H%%M)
cd %q
if grep -q '^POSTGRES_PASSWORD=' .env; then
  %s exec -T db pg_dump -U colony -d colonyhall -Fc > "$out/colonyhall-$stamp.dump"
fi
docker run --rm -v colonyhall_files:/data:ro -v "$out":/backup busybox tar czf "/backup/files-$stamp.tar.gz" -C /data .
find "$out" -name 'colonyhall-*.dump' -mtime +14 -delete
find "$out" -name 'files-*.tar.gz' -mtime +14 -delete
`, e.backupDir(), dir, base)
}

// installBackups writes the script and schedules it nightly: a root
// systemd timer on Linux (Docker runs with sudo there), a launchd agent on
// macOS.
func (e *Env) installBackups() error {
	script := filepath.Join(e.serverDir(), "backup.sh")
	if err := e.WriteFile(script, []byte(e.backupScript()), 0o700); err != nil {
		return err
	}
	if e.P.OS == "darwin" {
		plist := launchdPlist("hu.colonyhall.backup", []string{"/bin/sh", script}, "", false, filepath.Join(e.Home, "Library", "Logs", "colonyhall"), 3)
		path := filepath.Join(e.Home, "Library", "LaunchAgents", "hu.colonyhall.backup.plist")
		if err := e.WriteFile(path, []byte(plist), 0o644); err != nil {
			return err
		}
		_, err := e.Exec(Cmd{Name: "launchctl", Args: []string{"bootstrap", "gui/" + strconv.Itoa(os.Getuid()), path}})
		return err
	}
	unit := fmt.Sprintf("[Unit]\nDescription=Colonyhall backup\n\n[Service]\nType=oneshot\nExecStart=/bin/sh %s\n", script)
	timer := "[Unit]\nDescription=Colonyhall nightly backup\n\n[Timer]\nOnCalendar=*-*-* 03:30\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n"
	for name, body := range map[string]string{"colonyhall-backup.service": unit, "colonyhall-backup.timer": timer} {
		if _, err := e.Exec(Cmd{Name: "tee", Args: []string{"/etc/systemd/system/" + name}, Stdin: body, Sudo: true}); err != nil {
			return err
		}
	}
	if _, err := e.Exec(Cmd{Name: "systemctl", Args: []string{"daemon-reload"}, Sudo: true}); err != nil {
		return err
	}
	_, err := e.Exec(Cmd{Name: "systemctl", Args: []string{"enable", "--now", "colonyhall-backup.timer"}, Sudo: true})
	return err
}
