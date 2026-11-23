// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
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

	"colonyhall/internal/platform/clientip"
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

// randomSecretKey is a COLONY_SECRET_KEYS entry: id:<base64 of 32 bytes>
// (round 51; the same form as `colonyhall secret-key`).
func randomSecretKey(id string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return id + ":" + base64.StdEncoding.EncodeToString(b)
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
    image: pgvector/pgvector:pg17
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
	b.WriteString(serverHardening)
	b.WriteString(`    environment:
      COLONY_STORE: postgres
      COLONY_DATABASE_URL: ${COLONY_DATABASE_URL:?missing in .env}
      COLONY_HTTP_ADDR: ":8080"
      COLONY_SECURE_COOKIES: ${COLONY_SECURE_COOKIES:-0}
      COLONY_FILES_DIR: /data/files
      TMPDIR: /data/files/.tmp
      # The Go runtime collects before the container limit (mem_limit) is reached.
      GOMEMLIMIT: ${COLONY_GOMEMLIMIT:-768MiB}
      COLONY_MAX_UPLOAD_MB: ${COLONY_MAX_UPLOAD_MB:-25}
      COLONY_GITEA_URL: ${COLONY_GITEA_URL:-}
      COLONY_GITEA_PUBLIC_URL: ${COLONY_GITEA_PUBLIC_URL:-}
      # Round 57: the preview reverse proxy on its own port (its own origin).
      COLONY_PREVIEW_ADDR: ":${COLONY_PREVIEW_PORT:-8081}"
      COLONY_PREVIEW_ORIGIN: ${COLONY_PREVIEW_ORIGIN:-}
      COLONY_PREVIEW_NETS: ${COLONY_PREVIEW_NETS:-}
      # Where this server's source can be downloaded (AGPL section 13); empty: upstream.
      COLONY_SOURCE_URL: ${COLONY_SOURCE_URL:-}
      COLONY_GITEA_TOKEN: ${COLONY_GITEA_TOKEN:-}
      COLONY_GITEA_WEBHOOK_SECRET: ${COLONY_GITEA_WEBHOOK_SECRET:-}
      # Round 40: reverse proxies whose X-Forwarded-For is believed, and the
      # addresses never locked out by the login limits (IPs or CIDRs).
      COLONY_TRUSTED_PROXIES: ${COLONY_TRUSTED_PROXIES:-}
      COLONY_LOGIN_ALLOWLIST: ${COLONY_LOGIN_ALLOWLIST:-}
      # Search (docs/11-rag-terv.md): the embedding server and the reranker; empty: keyword search only / no rerank.
      COLONY_EMBED_URL: ${COLONY_EMBED_URL:-}
      COLONY_RERANK_URL: ${COLONY_RERANK_URL:-}
      COLONY_RERANK_TOP: ${COLONY_RERANK_TOP:-}
      COLONY_RERANK_TIMEOUT: ${COLONY_RERANK_TIMEOUT:-}
      # Several customers on one server, one subdomain each (docs/23-elkulonites.md); off by default.
      COLONY_MULTITENANT: ${COLONY_MULTITENANT:-0}
      COLONY_BASE_DOMAIN: ${COLONY_BASE_DOMAIN:-}
      # Round 51 (docs/24-hosztolt-ajanlat.md): production unless set to
      # development; the operator's developer access is refused in production.
      # COLONY_SECRET_KEYS encrypts the secrets the server reads back (the
      # operators' TOTP); required in multi-tenant mode. Or a file: _FILE.
      COLONY_ENV: ${COLONY_ENV:-production}
      COLONY_OPERATOR_DEV_ACCESS: ${COLONY_OPERATOR_DEV_ACCESS:-0}
      COLONY_SECRET_KEYS: ${COLONY_SECRET_KEYS:-}
      COLONY_SECRET_KEYS_FILE: ${COLONY_SECRET_KEYS_FILE:-}
      # The addresses admin.<domain> (the operator) answers; empty: any.
      COLONY_OPERATOR_ALLOWLIST: ${COLONY_OPERATOR_ALLOWLIST:-}
      # Round 56a (docs/25-onfejlesztes-terv.md 13.7): internal addresses the
      # Workshop's connectors may reach (CIDRs, addresses, host names, e.g.
      # a Gitea on the LAN); empty: public https only. Refused in
      # multi-tenant mode.
      COLONY_EGRESS_ALLOW: ${COLONY_EGRESS_ALLOW:-}
      # Round 56b (docs/25-onfejlesztes-terv.md 13.5): the Workshop's code
      # runner. The server sends jobs to COLONY_EXT_RUNNER_URL and answers
      # their host calls on COLONY_EXT_HOST_ADDR (internal network only);
      # the token is shared with the ext-runner service. Empty URL: no code
      # tools (the rest of the Workshop works). The limits of a call are
      # the server owner's (defaults in .env.prod.example).
      COLONY_EXT_RUNNER_URL: ${COLONY_EXT_RUNNER_URL:-}
      COLONY_EXT_RUNNER_TOKEN: ${COLONY_EXT_RUNNER_TOKEN:-}
      COLONY_EXT_HOST_ADDR: ${COLONY_EXT_HOST_ADDR:-:8096}
      COLONY_EXT_TIMEOUT: ${COLONY_EXT_TIMEOUT:-10s}
      COLONY_EXT_OUTPUT_KB: ${COLONY_EXT_OUTPUT_KB:-256}
      COLONY_EXT_HOST_CALLS: ${COLONY_EXT_HOST_CALLS:-50}
      COLONY_EXT_CONCURRENCY_WORKSPACE: ${COLONY_EXT_CONCURRENCY_WORKSPACE:-4}
      COLONY_EXT_DISABLE_AFTER: ${COLONY_EXT_DISABLE_AFTER:-5/10/10m}
    volumes:
      - files:/data/files
    ports:
      - "${COLONY_BIND:-127.0.0.1}:${COLONY_PORT:-8080}:8080"
      - "${COLONY_BIND:-127.0.0.1}:${COLONY_PREVIEW_PORT:-8081}:${COLONY_PREVIEW_PORT:-8081}"

  # Round 56b: the Workshop's code runner (COLONY_ROLE=ext-runner): the same
  # image, its own process, no database, no secrets, no published port; it
  # only talks to the server on the internal network. In the "workshop"
  # profile (COMPOSE_PROFILES=workshop in .env), which the installer turns on.
  ext-runner:
    image: ${COLONY_IMAGE:?missing in .env}
    restart: unless-stopped
    profiles: [workshop]
    read_only: true
    tmpfs:
      - /tmp:size=64m,mode=1777
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    mem_limit: ${COLONY_EXT_MEM_LIMIT:-1g}
    cpus: ${COLONY_EXT_CPUS:-2}
    pids_limit: 64
    environment:
      COLONY_ROLE: ext-runner
      COLONY_EXT_RUNNER_TOKEN: ${COLONY_EXT_RUNNER_TOKEN:-}
      COLONY_EXT_SERVER_URL: http://server:8096
      COLONY_EXT_RUNNER_ADDR: ":8095"
      COLONY_EXT_MEMORY_MB: ${COLONY_EXT_MEMORY_MB:-64}
      COLONY_EXT_CONCURRENCY_TOTAL: ${COLONY_EXT_CONCURRENCY_TOTAL:-16}

volumes:
`)
	if ownDB {
		b.WriteString("  db:\n")
	}
	b.WriteString("  files:\n")
	return b.String()
}

// serverHardening is the server service's container hardening (round 40):
// a read-only root with a small /tmp in memory (large temporary files go
// to the files volume through TMPDIR), no capabilities, no privilege gain,
// and memory and process limits measured on the dev stack (docs/02).
// Postgres is left alone.
const serverHardening = `    read_only: true
    tmpfs:
      - /tmp:size=16m,mode=1777
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    mem_limit: ${COLONY_MEM_LIMIT:-1g}
    pids_limit: ${COLONY_PIDS_LIMIT:-256}
`

// envFile is the server's .env. A value with a line break would add lines
// of its own (another variable), so such a value is dropped (round 40); the
// installer's own values never have one, and the answers file's are
// checked before they get here.
func envFile(v map[string]string) string {
	keys := []string{"COLONY_IMAGE", "COLONY_BIND", "COLONY_PORT", "COLONY_SECURE_COOKIES", "COLONY_ENV", "COLONY_SECRET_KEYS", "COLONY_OPERATOR_ALLOWLIST",
		"COMPOSE_PROFILES", "COLONY_EXT_RUNNER_URL", "COLONY_EXT_RUNNER_TOKEN",
		"POSTGRES_PASSWORD", "COLONY_APP_DB_PASSWORD", "COLONY_DATABASE_URL"}
	var b strings.Builder
	b.WriteString("# Colonyhall server settings and secrets (colony setup). Keep it 0600 and back it up.\n")
	for _, k := range keys {
		if val, ok := v[k]; ok && !strings.ContainsAny(val, "\r\n") {
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
	// The answers file can name any address (round 40): it must be an IP
	// address, and every "all interfaces" spelling (0.0.0.0, ::, 0) needs
	// the explicit allow_all_interfaces, not only the literal 0.0.0.0.
	ip := net.ParseIP(a.Bind)
	if ip == nil {
		return fmt.Errorf("bind %q is not an IP address", a.Bind)
	}
	if ip.IsUnspecified() && !a.AllowAll {
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
	if a.OperatorAllowlist == "" {
		// Round 51: who may reach the operator's console. Kept across
		// re-runs; asked only in multi-tenant mode (a single workspace has
		// no admin.<domain>), with Tailscale as the default when present.
		a.OperatorAllowlist = env["COLONY_OPERATOR_ALLOWLIST"]
		if a.OperatorAllowlist == "" && env["COLONY_MULTITENANT"] == "1" {
			def := "-"
			if _, ts := localAddrs(); ts != "" {
				def = "100.64.0.0/10"
			}
			s, err := e.UI.Ask("srvOpAllow", def)
			if err != nil {
				return err
			}
			a.OperatorAllowlist = s
		}
	}
	if v := strings.TrimSpace(a.OperatorAllowlist); v != "-" {
		if _, err := clientip.ParseList(v); err != nil {
			return fmt.Errorf("operator allowlist: %w", err)
		}
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

// backupDir is where the nightly backups go. On Linux the job runs as root
// (Docker needs sudo), so it writes to a root-owned folder: in the user's home
// a symlink at the predictable file name would make root overwrite any file.
func (e *Env) backupDir() string {
	if e.P.OS == "linux" {
		return "/var/backups/colonyhall"
	}
	return filepath.Join(e.Home, "colonyhall-backups")
}

// rootBackupScript is the Linux backup script: root-owned, because root runs
// it and a script the user can edit would give the user root.
const rootBackupScript = "/usr/local/lib/colonyhall/backup.sh"

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
	// Round 51: an installed server is production (the operator's
	// developer access is refused), and it has a key for the secrets it
	// encrypts, kept across re-runs.
	v["COLONY_ENV"] = "production"
	if allow := strings.TrimSpace(a.OperatorAllowlist); allow != "" && allow != "-" {
		v["COLONY_OPERATOR_ALLOWLIST"] = strings.ReplaceAll(allow, " ", "")
	}
	key := old["COLONY_SECRET_KEYS"]
	if key == "" {
		key = randomSecretKey("k1")
	}
	e.Secret(key)
	v["COLONY_SECRET_KEYS"] = key
	// Round 56b: the Workshop's code runner, with a token kept across re-runs.
	runnerToken := old["COLONY_EXT_RUNNER_TOKEN"]
	if len(runnerToken) < 32 {
		runnerToken = randomHex(24)
	}
	e.Secret(runnerToken)
	v["COLONY_EXT_RUNNER_TOKEN"] = runnerToken
	v["COLONY_EXT_RUNNER_URL"] = "http://ext-runner:8095"
	v["COMPOSE_PROFILES"] = "workshop"
	ownDB := a.DatabaseURL == ""
	if ownDB {
		pw := old["POSTGRES_PASSWORD"]
		// An install from before round 45 has a database password but no
		// application role: it keeps connecting as the superuser (the
		// server warns). A new install gets the role (docs/23-elkulonites.md).
		legacy := pw != "" && old["COLONY_APP_DB_PASSWORD"] == ""
		if pw == "" {
			pw = randomHex(24)
		}
		e.Secret(pw)
		v["POSTGRES_PASSWORD"] = pw
		v["COLONY_DATABASE_URL"] = "postgres://colony:" + pw + "@db:5432/colonyhall?sslmode=disable"
		if !legacy {
			app := old["COLONY_APP_DB_PASSWORD"]
			if app == "" {
				app = randomHex(24)
			}
			e.Secret(app)
			v["COLONY_APP_DB_PASSWORD"] = app
			v["COLONY_DATABASE_URL"] = "postgres://" + AppRole + ":" + app + "@db:5432/colonyhall?sslmode=disable"
		}
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
				// The application role first (new installs): start the
				// database alone, create the role, then the server.
				if app := readEnv(filepath.Join(e.serverDir(), ".env"))["COLONY_APP_DB_PASSWORD"]; app != "" || e.DryRun {
					if _, err := e.Exec(e.compose("up", "-d", "--wait", "db")); err != nil {
						return err
					}
					_, err := e.Exec(Cmd{Name: e.compose().Name, Sudo: e.compose().Sudo,
						Args:  append(e.compose().Args, "exec", "-T", "db", "psql", "-q", "-U", "colony", "-d", "colonyhall"),
						Stdin: appRoleSQL("colonyhall", app)})
					if err != nil {
						return err
					}
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
// volume into the backup folder, keeping 14 days. It reads nothing the user
// can write: the database container is found by its compose labels, not
// through the compose.yaml and .env in the user's home.
func (e *Env) backupScript() string {
	return fmt.Sprintf(`#!/bin/sh
# Colonyhall nightly backup (colony setup). Restore: docs/16-telepites.md.
set -eu
umask 077
out=%q
mkdir -p "$out"
stamp=$(date +%%Y%%m%%d-%%H%%M)
db=$(docker ps -q --filter label=com.docker.compose.project=colonyhall --filter label=com.docker.compose.service=db)
if [ -n "$db" ]; then
  docker exec "$db" pg_dump -U colony -d colonyhall -Fc > "$out/colonyhall-$stamp.dump"
fi
docker run --rm -v colonyhall_files:/data:ro -v "$out":/backup %s tar czf "/backup/files-$stamp.tar.gz" -C /data .
find "$out" -name 'colonyhall-*.dump' -mtime +14 -delete
find "$out" -name 'files-*.tar.gz' -mtime +14 -delete
`, e.backupDir(), backupImage)
}

// backupImage archives the files volume. Root runs the backup every night,
// so the image is pinned by digest (round 40), not taken from whatever
// "busybox" (latest) the registry serves that day.
const backupImage = "busybox:1.37@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec"

// installBackups writes the script and schedules it nightly: a root
// systemd timer on Linux (Docker runs with sudo there), a launchd agent on
// macOS.
func (e *Env) installBackups() error {
	if e.P.OS == "darwin" {
		script := filepath.Join(e.serverDir(), "backup.sh")
		if err := e.WriteFile(script, []byte(e.backupScript()), 0o700); err != nil {
			return err
		}
		plist := launchdPlist("hu.colonyhall.backup", []string{"/bin/sh", script}, "", false, filepath.Join(e.Home, "Library", "Logs", "colonyhall"), 3)
		path := filepath.Join(e.Home, "Library", "LaunchAgents", "hu.colonyhall.backup.plist")
		if err := e.WriteFile(path, []byte(plist), 0o644); err != nil {
			return err
		}
		_, err := e.Exec(Cmd{Name: "launchctl", Args: []string{"bootstrap", "gui/" + strconv.Itoa(os.Getuid()), path}})
		return err
	}
	if _, err := e.Exec(Cmd{Name: "install", Args: []string{"-d", "-o", "root", "-g", "root", "-m", "0755", filepath.Dir(rootBackupScript)}, Sudo: true}); err != nil {
		return err
	}
	if _, err := e.Exec(Cmd{Name: "tee", Args: []string{rootBackupScript}, Stdin: e.backupScript(), Sudo: true}); err != nil {
		return err
	}
	unit := fmt.Sprintf("[Unit]\nDescription=Colonyhall backup\n\n[Service]\nType=oneshot\nExecStart=/bin/sh %s\n", rootBackupScript)
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
