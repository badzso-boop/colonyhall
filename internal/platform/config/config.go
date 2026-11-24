// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package config reads the server configuration from environment variables.
package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/platform/clientip"
	"colonyhall/internal/platform/egress"
	"colonyhall/internal/platform/secretbox"
)

// Config is the runtime configuration of the server.
type Config struct {
	// Role selects which parts run: api, gateway, worker, scheduler or all.
	// Phase 0 supports only "all".
	Role string
	// HTTPAddr is the listen address, e.g. ":8080".
	HTTPAddr string
	// Store selects the persistence backend: "postgres" or "memory".
	Store string
	// DatabaseURL is the Postgres connection string (Store=postgres).
	DatabaseURL string
	// DevSeed creates a demo workspace with fixed tokens on startup.
	// Never enable it on a reachable server.
	DevSeed bool
	// SecureCookies marks the session cookie HTTPS-only (default on).
	SecureCookies bool
	// FilesDir is the root directory for uploaded files (default: "./data/files").
	FilesDir string
	// MaxUploadMB is the maximum upload size in megabytes (default: 25).
	MaxUploadMB int
	// GiteaURL is the base URL of the Gitea instance (e.g. "https://git.example.com").
	GiteaURL string
	// GiteaToken is a read-only API token for fetching git metadata.
	GiteaToken string
	// SourceURL is where this server's source code can be downloaded (AGPL
	// section 13): the web app links there and GET /api/v1/version returns
	// it. Someone running a modified version points it at their own source.
	SourceURL string
	// GiteaPublicURL is the address people use in links, when it differs from
	// GiteaURL (e.g. GiteaURL on Tailscale, the public name for links).
	GiteaPublicURL string
	// GiteaWebhookSecret is the HMAC secret for verifying incoming webhooks.
	GiteaWebhookSecret string
	// PreviewAddr (COLONY_PREVIEW_ADDR, e.g. ":8081") is the preview
	// listener: the reverse proxy to running previews on its own origin
	// (round 57). Empty: no proxy, previews are links only.
	PreviewAddr string
	// PreviewOrigin (COLONY_PREVIEW_ORIGIN) is the preview listener's
	// address as browsers see it (scheme://host:port), when it is not the
	// app's host name with PreviewAddr's port (behind a reverse proxy).
	PreviewOrigin string
	// PreviewNets (COLONY_PREVIEW_NETS, comma-separated prefixes) are the
	// networks the proxy may reach previews on; empty: Tailscale's. A guard
	// cannot point the server at anything else (its own services, the
	// container network). Add the LAN (192.168.1.0/24) or 127.0.0.0/8 (one
	// machine, natively) when previews live there.
	PreviewNets []netip.Prefix
	// CalendarRefresh is how often subscribed calendars are fetched (default 15m).
	CalendarRefresh time.Duration
	// EmbedURL is the search's embedding server (the Raspberry Pi); empty:
	// keyword search only.
	EmbedURL string
	// RerankURL is the search's reranker (the homelab); empty: no rerank.
	RerankURL string
	// RerankTop is how many candidates go to the reranker (default 10).
	RerankTop int
	// RerankTimeout bounds one rerank call (default 5 s).
	RerankTimeout time.Duration
	// Multitenant turns on the subdomain per workspace (round 45). Off by
	// default: a self-hosted server behaves as before.
	Multitenant bool
	// BaseDomain is the domain the workspace subdomains live under, e.g.
	// "colonyhall.example.com" for acme.colonyhall.example.com; required
	// with Multitenant.
	BaseDomain string
	// TrustedProxies are the reverse proxies whose X-Forwarded-For is
	// believed (COLONY_TRUSTED_PROXIES, IPs or CIDRs; round 40). Empty
	// (the default): the client address is always the TCP peer.
	TrustedProxies []netip.Prefix
	// LoginAllowlist are the addresses never locked out by the login and
	// release limits (COLONY_LOGIN_ALLOWLIST, IPs or CIDRs; round 40).
	LoginAllowlist []netip.Prefix
	// Env is "production" (the default, also when COLONY_ENV is unset) or
	// "development" (round 51).
	Env string
	// OperatorDevAccess (COLONY_OPERATOR_DEV_ACCESS=1) lets the operator
	// open any workspace without a reason or the 4-hour limit, with a
	// banner on every page. Refused unless Env is "development".
	OperatorDevAccess bool
	// OperatorAllowlist are the addresses admin.<domain> answers (round 51,
	// COLONY_OPERATOR_ALLOWLIST, IPs or CIDRs); every other address gets the
	// unknown-host 404. Empty: any address (the docs advise against it).
	OperatorAllowlist []netip.Prefix
	// SecretKeys encrypt the secrets the server reads back (round 51):
	// COLONY_SECRET_KEYS, or the file COLONY_SECRET_KEYS_FILE names.
	// Required in multi-tenant mode. Nil when unset.
	SecretKeys *secretbox.Keyring
	// EgressAllow are the internal addresses the Workshop's connectors may
	// reach (round 56a, COLONY_EGRESS_ALLOW: CIDRs, addresses and host
	// names). Empty: public https only. Refused in multi-tenant mode.
	EgressAllow egress.Policy
	// Ext is the Workshop's code runner (round 56b): its address and token
	// on the server, its own settings in the ext-runner role, and the
	// limits of a call (COLONY_EXT_*; the server's owner tunes them).
	Ext Ext
}

// Ext configures the Workshop's code runner.
type Ext struct {
	RunnerURL   string // COLONY_EXT_RUNNER_URL (server): empty = no code tools
	RunnerToken string // COLONY_EXT_RUNNER_TOKEN (both): shared secret, at least 32 characters
	HostAddr    string // COLONY_EXT_HOST_ADDR (server): the internal listener the runner calls back, default :8096
	RunnerAddr  string // COLONY_EXT_RUNNER_ADDR (runner): default :8095
	ServerURL   string // COLONY_EXT_SERVER_URL (runner): the server's internal listener, e.g. http://server:8096

	MemoryMB             int           // COLONY_EXT_MEMORY_MB (64)
	Timeout              time.Duration // COLONY_EXT_TIMEOUT (10s)
	OutputKB             int           // COLONY_EXT_OUTPUT_KB (256)
	HostCalls            int           // COLONY_EXT_HOST_CALLS (50)
	ConcurrencyWorkspace int           // COLONY_EXT_CONCURRENCY_WORKSPACE (4)
	ConcurrencyTotal     int           // COLONY_EXT_CONCURRENCY_TOTAL (16)
	// COLONY_EXT_DISABLE_AFTER "5/10/10m": an extension is switched off
	// after this many failed calls in a row, or this many in the window.
	DisableConsecutive int
	DisableErrors      int
	DisableWindow      time.Duration
}

// RoleExtRunner runs only the Workshop's code runner (round 56b).
const RoleExtRunner = "ext-runner"

func extFromEnv() (Ext, error) {
	e := Ext{RunnerURL: os.Getenv("COLONY_EXT_RUNNER_URL"), RunnerToken: os.Getenv("COLONY_EXT_RUNNER_TOKEN"),
		HostAddr: getenv("COLONY_EXT_HOST_ADDR", ":8096"), RunnerAddr: getenv("COLONY_EXT_RUNNER_ADDR", ":8095"),
		ServerURL: os.Getenv("COLONY_EXT_SERVER_URL")}
	ints := []struct {
		key      string
		dst      *int
		def, max int
	}{
		{"COLONY_EXT_MEMORY_MB", &e.MemoryMB, 64, 1024},
		{"COLONY_EXT_OUTPUT_KB", &e.OutputKB, 256, 4096},
		{"COLONY_EXT_HOST_CALLS", &e.HostCalls, 50, 1000},
		{"COLONY_EXT_CONCURRENCY_WORKSPACE", &e.ConcurrencyWorkspace, 4, 256},
		{"COLONY_EXT_CONCURRENCY_TOTAL", &e.ConcurrencyTotal, 16, 1024},
	}
	for _, x := range ints {
		*x.dst = x.def
		if v := os.Getenv(x.key); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > x.max {
				return e, fmt.Errorf("config: %s must be 1–%d, got %q", x.key, x.max, v)
			}
			*x.dst = n
		}
	}
	e.Timeout = 10 * time.Second
	if v := os.Getenv("COLONY_EXT_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 100*time.Millisecond || d > 5*time.Minute {
			return e, fmt.Errorf("config: COLONY_EXT_TIMEOUT must be a duration between 100ms and 5m, got %q", v)
		}
		e.Timeout = d
	}
	e.DisableConsecutive, e.DisableErrors, e.DisableWindow = 5, 10, 10*time.Minute
	if v := os.Getenv("COLONY_EXT_DISABLE_AFTER"); v != "" {
		parts := strings.Split(v, "/")
		var a, b int
		var w time.Duration
		var err error
		if len(parts) == 3 {
			if a, err = strconv.Atoi(parts[0]); err == nil {
				if b, err = strconv.Atoi(parts[1]); err == nil {
					w, err = time.ParseDuration(parts[2])
				}
			}
		}
		if len(parts) != 3 || err != nil || a < 1 || b < 1 || w < time.Minute {
			return e, fmt.Errorf("config: COLONY_EXT_DISABLE_AFTER must look like 5/10/10m, got %q", v)
		}
		e.DisableConsecutive, e.DisableErrors, e.DisableWindow = a, b, w
	}
	if (e.RunnerURL != "" || os.Getenv("COLONY_ROLE") == RoleExtRunner) && len(e.RunnerToken) < 32 {
		return e, fmt.Errorf("config: COLONY_EXT_RUNNER_TOKEN must be at least 32 characters when the code runner is used")
	}
	return e, nil
}

// Environments.
const (
	EnvProduction  = "production"
	EnvDevelopment = "development"
)

// FromEnv reads the configuration from COLONY_* environment variables.
func FromEnv() (Config, error) {
	maxUpload := 25
	trusted, err := clientip.ParseList(os.Getenv("COLONY_TRUSTED_PROXIES"))
	if err != nil {
		return Config{}, fmt.Errorf("config: COLONY_TRUSTED_PROXIES: %w", err)
	}
	allow, err := clientip.ParseList(os.Getenv("COLONY_LOGIN_ALLOWLIST"))
	if err != nil {
		return Config{}, fmt.Errorf("config: COLONY_LOGIN_ALLOWLIST: %w", err)
	}
	if v := os.Getenv("COLONY_MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxUpload = n
		}
	}
	calRefresh := 15 * time.Minute
	if v := os.Getenv("COLONY_CALENDAR_REFRESH"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= time.Minute {
			calRefresh = d
		}
	}
	rerankTop := 0
	if v := os.Getenv("COLONY_RERANK_TOP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2 || n > 100 {
			return Config{}, fmt.Errorf("config: COLONY_RERANK_TOP must be 2–100, got %q", v)
		}
		rerankTop = n
	}
	var rerankTimeout time.Duration
	if v := os.Getenv("COLONY_RERANK_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 100*time.Millisecond || d > time.Minute {
			return Config{}, fmt.Errorf("config: COLONY_RERANK_TIMEOUT must be a duration of 100ms–1m, got %q", v)
		}
		rerankTimeout = d
	}
	keys, err := secretKeys()
	if err != nil {
		return Config{}, err
	}
	ext, err := extFromEnv()
	if err != nil {
		return Config{}, err
	}
	egressAllow, err := egress.ParsePolicy(os.Getenv("COLONY_EGRESS_ALLOW"))
	if err != nil {
		return Config{}, fmt.Errorf("config: COLONY_EGRESS_ALLOW: %w", err)
	}
	opAllow, err := clientip.ParseList(os.Getenv("COLONY_OPERATOR_ALLOWLIST"))
	if err != nil {
		return Config{}, fmt.Errorf("config: COLONY_OPERATOR_ALLOWLIST: %w", err)
	}
	previewNets, err := clientip.ParseList(os.Getenv("COLONY_PREVIEW_NETS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: COLONY_PREVIEW_NETS: %w", err)
	}
	c := Config{
		Env:                getenv("COLONY_ENV", EnvProduction),
		OperatorDevAccess:  os.Getenv("COLONY_OPERATOR_DEV_ACCESS") == "1",
		SecretKeys:         keys,
		EgressAllow:        egressAllow,
		Ext:                ext,
		OperatorAllowlist:  opAllow,
		PreviewNets:        previewNets,
		EmbedURL:           os.Getenv("COLONY_EMBED_URL"),
		RerankURL:          os.Getenv("COLONY_RERANK_URL"),
		RerankTop:          rerankTop,
		RerankTimeout:      rerankTimeout,
		CalendarRefresh:    calRefresh,
		Role:               getenv("COLONY_ROLE", "all"),
		HTTPAddr:           getenv("COLONY_HTTP_ADDR", ":8080"),
		Store:              getenv("COLONY_STORE", "postgres"),
		DatabaseURL:        os.Getenv("COLONY_DATABASE_URL"),
		DevSeed:            os.Getenv("COLONY_DEV_SEED") == "1",
		SecureCookies:      os.Getenv("COLONY_SECURE_COOKIES") != "0",
		FilesDir:           getenv("COLONY_FILES_DIR", "./data/files"),
		MaxUploadMB:        maxUpload,
		GiteaURL:           os.Getenv("COLONY_GITEA_URL"),
		GiteaToken:         os.Getenv("COLONY_GITEA_TOKEN"),
		GiteaPublicURL:     os.Getenv("COLONY_GITEA_PUBLIC_URL"),
		PreviewAddr:        os.Getenv("COLONY_PREVIEW_ADDR"),
		PreviewOrigin:      os.Getenv("COLONY_PREVIEW_ORIGIN"),
		SourceURL:          getenv("COLONY_SOURCE_URL", DefaultSourceURL),
		GiteaWebhookSecret: os.Getenv("COLONY_GITEA_WEBHOOK_SECRET"),
		Multitenant:        os.Getenv("COLONY_MULTITENANT") == "1",
		BaseDomain:         strings.ToLower(strings.Trim(os.Getenv("COLONY_BASE_DOMAIN"), ". ")),
		TrustedProxies:     trusted,
		LoginAllowlist:     allow,
	}
	if c.Multitenant && c.BaseDomain == "" {
		return c, fmt.Errorf("config: COLONY_BASE_DOMAIN is required when COLONY_MULTITENANT=1")
	}
	if c.Env != EnvProduction && c.Env != EnvDevelopment {
		return c, fmt.Errorf("config: COLONY_ENV must be %q or %q, got %q", EnvProduction, EnvDevelopment, c.Env)
	}
	if c.OperatorDevAccess && c.Env != EnvDevelopment {
		return c, fmt.Errorf("config: COLONY_OPERATOR_DEV_ACCESS=1 lets the operator read every workspace; it needs COLONY_ENV=development and is refused in production")
	}
	if c.Multitenant && !c.EgressAllow.Empty() {
		return c, fmt.Errorf("config: COLONY_EGRESS_ALLOW opens internal addresses to every customer's connectors; it is refused when COLONY_MULTITENANT=1")
	}
	if c.Multitenant && c.SecretKeys == nil {
		return c, fmt.Errorf("config: COLONY_SECRET_KEYS (or COLONY_SECRET_KEYS_FILE) is required when COLONY_MULTITENANT=1; create one with `colonyhall secret-key`")
	}
	if u, err := url.Parse(c.SourceURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return c, fmt.Errorf("config: COLONY_SOURCE_URL must be an http(s) address, got %q", c.SourceURL)
	}
	switch c.Role {
	case "all":
	case RoleExtRunner:
		// The code runner needs no database: only the server's internal
		// address and the shared token (round 56b).
		if c.Ext.ServerURL == "" {
			return c, fmt.Errorf("config: COLONY_EXT_SERVER_URL is required when COLONY_ROLE=ext-runner")
		}
		return c, nil
	default:
		return c, fmt.Errorf("config: COLONY_ROLE=%q is not supported; use \"all\" or \"ext-runner\"", c.Role)
	}
	switch c.Store {
	case "memory":
	case "postgres":
		if c.DatabaseURL == "" {
			return c, fmt.Errorf("config: COLONY_DATABASE_URL is required when COLONY_STORE=postgres")
		}
	default:
		return c, fmt.Errorf("config: COLONY_STORE must be \"postgres\" or \"memory\", got %q", c.Store)
	}
	return c, nil
}

// DefaultSourceURL is the upstream repository, the source of an unmodified
// build.
const DefaultSourceURL = "https://github.com/badzso-boop/colonyhall"

// secretKeys reads COLONY_SECRET_KEYS or the file COLONY_SECRET_KEYS_FILE.
func secretKeys() (*secretbox.Keyring, error) {
	v, file := os.Getenv("COLONY_SECRET_KEYS"), os.Getenv("COLONY_SECRET_KEYS_FILE")
	if v != "" && file != "" {
		return nil, fmt.Errorf("config: set COLONY_SECRET_KEYS or COLONY_SECRET_KEYS_FILE, not both")
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("config: COLONY_SECRET_KEYS_FILE: %w", err)
		}
		v = strings.TrimSpace(string(b))
	}
	if v == "" {
		return nil, nil
	}
	k, err := secretbox.Parse(v)
	if err != nil {
		return nil, fmt.Errorf("config: COLONY_SECRET_KEYS: %w", err)
	}
	return k, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
