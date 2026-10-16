// Package config reads the server configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
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
	// GiteaWebhookSecret is the HMAC secret for verifying incoming webhooks.
	GiteaWebhookSecret string
}

// FromEnv reads the configuration from COLONY_* environment variables.
func FromEnv() (Config, error) {
	maxUpload := 25
	if v := os.Getenv("COLONY_MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxUpload = n
		}
	}
	c := Config{
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
		GiteaWebhookSecret: os.Getenv("COLONY_GITEA_WEBHOOK_SECRET"),
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
