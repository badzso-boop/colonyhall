package e2e

import (
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/config"
	"colonyhall/internal/server"
)

// Round 48 (AGPL section 13): anyone who can reach the server, signed in or
// not, can find its version, licence and source.
func TestVersionOffersTheSource(t *testing.T) {
	e := newEnv(t)
	var v apispec.VersionInfo
	if code := e.do("GET", "/api/v1/version", "", nil, &v); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if v.License != "AGPL-3.0-only" || v.SourceUrl != config.DefaultSourceURL || v.Version == "" {
		t.Fatalf("version: %+v", v)
	}

	fork := newEnvWith(t, func(o *server.Options) { o.SourceURL = "https://git.example.com/acme/colonyhall-fork" })
	if fork.do("GET", "/api/v1/version", "", nil, &v); v.SourceUrl != "https://git.example.com/acme/colonyhall-fork" {
		t.Fatalf("a modified version's source: %+v", v)
	}
}
