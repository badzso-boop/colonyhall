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
}

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
	c := Config{
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
	if u, err := url.Parse(c.SourceURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return c, fmt.Errorf("config: COLONY_SOURCE_URL must be an http(s) address, got %q", c.SourceURL)
	}
	if c.Role != "all" {
		return c, fmt.Errorf("config: COLONY_ROLE=%q is not supported yet; use \"all\"", c.Role)
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

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
