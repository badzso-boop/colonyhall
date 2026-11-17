// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/platform/secretbox"
)

func testKey(t *testing.T) string {
	t.Helper()
	k, err := secretbox.NewKey("k1")
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// Round 51: the operator's developer access needs COLONY_ENV=development;
// a missing COLONY_ENV counts as production.
func TestOperatorDevAccessIsRefusedOutsideDevelopment(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	t.Setenv("COLONY_OPERATOR_DEV_ACCESS", "1")
	for _, env := range []string{"", "production"} {
		t.Setenv("COLONY_ENV", env)
		if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "COLONY_OPERATOR_DEV_ACCESS") {
			t.Fatalf("COLONY_ENV=%q: developer access must not start: %v", env, err)
		}
	}
	t.Setenv("COLONY_ENV", "development")
	if c, err := FromEnv(); err != nil || !c.OperatorDevAccess || c.Env != EnvDevelopment {
		t.Fatalf("development: %+v %v", c, err)
	}
	t.Setenv("COLONY_ENV", "staging")
	if _, err := FromEnv(); err == nil {
		t.Fatal("an unknown COLONY_ENV must not start")
	}
	t.Setenv("COLONY_ENV", "")
	t.Setenv("COLONY_OPERATOR_DEV_ACCESS", "")
	if c, err := FromEnv(); err != nil || c.Env != EnvProduction || c.OperatorDevAccess {
		t.Fatalf("the default is production without developer access: %+v %v", c, err)
	}
}

func TestSecretKeys(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	t.Setenv("COLONY_MULTITENANT", "1")
	t.Setenv("COLONY_BASE_DOMAIN", "example.test")
	t.Setenv("COLONY_SECRET_KEYS", "")
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "COLONY_SECRET_KEYS") {
		t.Fatalf("multi-tenant mode needs the secret keys: %v", err)
	}
	file := filepath.Join(t.TempDir(), "keys")
	if err := os.WriteFile(file, []byte(testKey(t)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLONY_SECRET_KEYS_FILE", file)
	if c, err := FromEnv(); err != nil || c.SecretKeys == nil || c.SecretKeys.Active() != "k1" {
		t.Fatalf("keys from a file: %+v %v", c, err)
	}
	t.Setenv("COLONY_SECRET_KEYS", testKey(t))
	if _, err := FromEnv(); err == nil {
		t.Fatal("both the variable and the file must be refused")
	}
	t.Setenv("COLONY_SECRET_KEYS_FILE", "")
	t.Setenv("COLONY_SECRET_KEYS", "k1:not-a-key")
	if _, err := FromEnv(); err == nil || strings.Contains(err.Error(), "not-a-key") {
		t.Fatalf("a bad key must be refused without echoing it: %v", err)
	}
}

func TestMultitenantNeedsBaseDomain(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	t.Setenv("COLONY_MULTITENANT", "1")
	t.Setenv("COLONY_SECRET_KEYS", testKey(t))
	t.Setenv("COLONY_BASE_DOMAIN", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("multi-tenant mode without a base domain must not start")
	}
	t.Setenv("COLONY_BASE_DOMAIN", " Colonyhall.Example.com. ")
	c, err := FromEnv()
	if err != nil || !c.Multitenant || c.BaseDomain != "colonyhall.example.com" {
		t.Fatalf("config: %+v %v", c, err)
	}
	t.Setenv("COLONY_MULTITENANT", "")
	if c, err := FromEnv(); err != nil || c.Multitenant {
		t.Fatalf("single-tenant is the default: %+v %v", c, err)
	}
}

// Round 48: the AGPL source link defaults to upstream and must be a web address.
func TestSourceURL(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	t.Setenv("COLONY_SOURCE_URL", "")
	if c, err := FromEnv(); err != nil || c.SourceURL != DefaultSourceURL {
		t.Fatalf("default: %q %v", c.SourceURL, err)
	}
	t.Setenv("COLONY_SOURCE_URL", "https://git.example.com/acme/colonyhall-fork")
	if c, err := FromEnv(); err != nil || c.SourceURL != "https://git.example.com/acme/colonyhall-fork" {
		t.Fatalf("set: %q %v", c.SourceURL, err)
	}
	for _, bad := range []string{"git.example.com/fork", "javascript:alert(1)", "ftp://example.com/src"} {
		t.Setenv("COLONY_SOURCE_URL", bad)
		if _, err := FromEnv(); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLoginAllowlistAndTrustedProxies(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	c, err := FromEnv()
	if err != nil || len(c.LoginAllowlist) != 0 || len(c.TrustedProxies) != 0 {
		t.Fatalf("empty by default: %+v %v", c, err)
	}
	t.Setenv("COLONY_LOGIN_ALLOWLIST", "127.0.0.1/32, ::1/128, 192.0.2.10")
	t.Setenv("COLONY_TRUSTED_PROXIES", "10.0.0.2")
	if c, err = FromEnv(); err != nil || len(c.LoginAllowlist) != 3 || c.LoginAllowlist[2].String() != "192.0.2.10/32" || len(c.TrustedProxies) != 1 {
		t.Fatalf("parsed: %+v %v", c, err)
	}
	t.Setenv("COLONY_LOGIN_ALLOWLIST", "office")
	if _, err := FromEnv(); err == nil {
		t.Fatal("a name is not an address")
	}
	t.Setenv("COLONY_LOGIN_ALLOWLIST", "")
	t.Setenv("COLONY_TRUSTED_PROXIES", "10.0.0.0/40")
	if _, err := FromEnv(); err == nil {
		t.Fatal("a bad prefix must fail the start")
	}
}

func TestEgressAllow(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	t.Setenv("COLONY_EGRESS_ALLOW", "192.168.1.0/24, gitea.lan")
	c, err := FromEnv()
	if err != nil || len(c.EgressAllow.Prefixes) != 1 || len(c.EgressAllow.Hosts) != 1 {
		t.Fatalf("single-tenant allow list: %+v %v", c.EgressAllow, err)
	}
	t.Setenv("COLONY_EGRESS_ALLOW", "not a host/")
	if _, err := FromEnv(); err == nil {
		t.Fatal("a bad entry must be refused")
	}
	// Multi-tenant: internal addresses would be open to every customer.
	t.Setenv("COLONY_EGRESS_ALLOW", "10.0.0.0/8")
	t.Setenv("COLONY_MULTITENANT", "1")
	t.Setenv("COLONY_BASE_DOMAIN", "example.test")
	t.Setenv("COLONY_SECRET_KEYS", testKey(t))
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "COLONY_EGRESS_ALLOW") {
		t.Fatalf("multi-tenant with an allow list: %v", err)
	}
}

func TestExtRunnerSettings(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	c, err := FromEnv()
	if err != nil || c.Ext.MemoryMB != 64 || c.Ext.Timeout != 10*time.Second || c.Ext.DisableConsecutive != 5 || c.Ext.DisableWindow != 10*time.Minute {
		t.Fatalf("defaults: %+v %v", c.Ext, err)
	}
	t.Setenv("COLONY_EXT_MEMORY_MB", "128")
	t.Setenv("COLONY_EXT_TIMEOUT", "30s")
	t.Setenv("COLONY_EXT_DISABLE_AFTER", "3/6/5m")
	if c, err = FromEnv(); err != nil || c.Ext.MemoryMB != 128 || c.Ext.Timeout != 30*time.Second || c.Ext.DisableErrors != 6 {
		t.Fatalf("set: %+v %v", c.Ext, err)
	}
	for k, v := range map[string]string{"COLONY_EXT_MEMORY_MB": "0", "COLONY_EXT_TIMEOUT": "1h", "COLONY_EXT_DISABLE_AFTER": "5/10"} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := FromEnv(); err == nil {
				t.Fatalf("%s=%s accepted", k, v)
			}
		})
	}
	// A runner URL needs a long token.
	t.Setenv("COLONY_EXT_RUNNER_URL", "http://ext-runner:8095")
	t.Setenv("COLONY_EXT_RUNNER_TOKEN", "short")
	if _, err := FromEnv(); err == nil {
		t.Fatal("a short runner token accepted")
	}
	// The runner role needs no database, only the server's address.
	t.Setenv("COLONY_EXT_RUNNER_TOKEN", strings.Repeat("x", 40))
	t.Setenv("COLONY_ROLE", "ext-runner")
	t.Setenv("COLONY_STORE", "postgres")
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "COLONY_EXT_SERVER_URL") {
		t.Fatalf("runner without the server's address: %v", err)
	}
	t.Setenv("COLONY_EXT_SERVER_URL", "http://server:8096")
	if c, err := FromEnv(); err != nil || c.Role != RoleExtRunner {
		t.Fatalf("runner role: %+v %v", c.Role, err)
	}
}
