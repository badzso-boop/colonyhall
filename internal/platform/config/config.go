// Package config reads the server configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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
}

// FromEnv reads the configuration from COLONY_* environment variables.
func FromEnv() (Config, error) {
	maxUpload := 25
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
		GiteaWebhookSecret: os.Getenv("COLONY_GITEA_WEBHOOK_SECRET"),
		Multitenant:        os.Getenv("COLONY_MULTITENANT") == "1",
		BaseDomain:         strings.ToLower(strings.Trim(os.Getenv("COLONY_BASE_DOMAIN"), ". ")),
	}
	if c.Multitenant && c.BaseDomain == "" {
		return c, fmt.Errorf("config: COLONY_BASE_DOMAIN is required when COLONY_MULTITENANT=1")
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

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
