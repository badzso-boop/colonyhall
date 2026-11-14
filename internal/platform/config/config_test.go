// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package config

import "testing"

func TestMultitenantNeedsBaseDomain(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	t.Setenv("COLONY_MULTITENANT", "1")
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
