// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// loginFrom posts a login with an optional X-Forwarded-For header and
// returns the status and the Retry-After header.
func (e *env) loginFrom(handle, password, xff string) (int, string) {
	e.t.Helper()
	body, _ := json.Marshal(map[string]any{"handle": handle, "password": password})
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if e.host != "" {
		req.Host = e.host
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	e.checkContract(req, res, raw)
	return res.StatusCode, res.Header.Get("Retry-After")
}

func TestLoginLockoutHasRetryAfter(t *testing.T) {
	e := newEnv(t)
	for range 5 {
		if s, _ := e.loginFrom("norbi", "wrong password!!", ""); s != http.StatusUnauthorized {
			t.Fatalf("status %d", s)
		}
	}
	e.clock.Advance(5 * time.Minute)
	s, ra := e.loginFrom("norbi", server.DevAdminPassword, "")
	if s != http.StatusTooManyRequests {
		t.Fatalf("status %d", s)
	}
	if secs, err := strconv.Atoi(ra); err != nil || secs != 600 {
		t.Fatalf("Retry-After %q, want 600 seconds", ra)
	}
}

func TestForgedForwardedForDoesNotDodgeTheIPLimit(t *testing.T) {
	e := newEnv(t) // no trusted proxies: the header is ignored
	for i := range 20 {
		// A different handle and a different forged address every time.
		_, _ = e.loginFrom("nobody"+strconv.Itoa(i), "wrong password!!", "203.0.113."+strconv.Itoa(i))
	}
	if s, _ := e.loginFrom("norbi", server.DevAdminPassword, "198.51.100.1"); s != http.StatusTooManyRequests {
		t.Fatalf("the forged header moved the requests to other addresses: status %d", s)
	}
}

func TestTrustedProxyForwardsTheClientAddress(t *testing.T) {
	e := newEnvWith(t, func(o *server.Options) {
		o.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	})
	for i := range 20 {
		_, _ = e.loginFrom("nobody"+strconv.Itoa(i), "wrong password!!", "203.0.113.5")
	}
	if s, _ := e.loginFrom("norbi", server.DevAdminPassword, "203.0.113.5"); s != http.StatusTooManyRequests {
		t.Fatalf("the proxied client is locked out: status %d", s)
	}
	// Another client behind the same proxy is not.
	if s, _ := e.loginFrom("norbi", server.DevAdminPassword, "203.0.113.6"); s != http.StatusOK {
		t.Fatalf("another client of the proxy: status %d", s)
	}
}

func TestAllowlistedAddressLogsInDuringALockout(t *testing.T) {
	e := newEnvWith(t, func(o *server.Options) {
		o.LoginAllowlist = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	})
	for range 10 {
		if s, _ := e.loginFrom("norbi", "wrong password!!", ""); s != http.StatusUnauthorized {
			t.Fatalf("an allowlisted address still gets its password checked: %d", s)
		}
	}
	if s, _ := e.loginFrom("norbi", server.DevAdminPassword, ""); s != http.StatusOK {
		t.Fatalf("an allowlisted address is never locked out: status %d", s)
	}
}

func securityCard(t *testing.T, e *env, token string) apispec.SecurityStatus {
	t.Helper()
	var st apispec.AdminStatus
	if s := e.do("GET", "/api/v1/admin/status", token, nil, &st); s != http.StatusOK || st.System == nil {
		t.Fatalf("status: %d", s)
	}
	return st.System.Security
}

func TestSystemStatusSecurityCard(t *testing.T) {
	e := newEnvWith(t, func(o *server.Options) {
		o.LoginAllowlist = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
	})
	if c := securityCard(t, e, server.DevAdminToken); c.Lockouts24h != 0 || c.TrustedProxies || len(c.Allowlist) != 1 || c.Allowlist[0] != "192.0.2.0/24" {
		t.Fatalf("empty card: %+v", c)
	}
	for range 5 {
		_, _ = e.loginFrom("norbi", "wrong password!!", "")
		_, _ = e.loginFrom("ghost", "wrong password!!", "") // an unknown handle: no workspace
	}
	c := securityCard(t, e, server.DevAdminToken)
	if c.Lockouts24h != 2 || len(c.Lockouts) != 2 {
		t.Fatalf("single-tenant mode shows both lockouts: %+v", c)
	}
	for _, l := range c.Lockouts {
		if l.Ip != "127.0.0.1" || l.Scope != apispec.Login || !l.Until.Equal(l.At.Add(15*time.Minute)) {
			t.Fatalf("lockout row %+v", l)
		}
	}
	e.clock.Advance(25 * time.Hour)
	if c := securityCard(t, e, server.DevAdminToken); c.Lockouts24h != 0 {
		t.Fatalf("only the last 24 hours: %+v", c)
	}
}

func TestSecurityCardIsPerWorkspace(t *testing.T) {
	acme, beta := mtEnv(t, nil)
	for range 5 {
		_, _ = beta.loginFrom("bob", "wrong password!!", "")
		_, _ = beta.loginFrom("ghost", "wrong password!!", "") // the subdomain's workspace owns it
	}
	if c := securityCard(t, beta, betaOwnerToken); c.Lockouts24h != 2 {
		t.Fatalf("beta sees its lockouts: %+v", c)
	}
	if c := securityCard(t, acme, server.DevAdminToken); c.Lockouts24h != 0 || len(c.Lockouts) != 0 {
		t.Fatalf("acme sees beta's lockouts: %+v", c)
	}
}

// The hourly maintenance prunes failure rows after a week (round 40).
func TestMaintenancePrunesOldLoginFailures(t *testing.T) {
	e := newEnv(t)
	for range 3 {
		_, _ = e.loginFrom("norbi", "wrong password!!", "")
	}
	count := func() int {
		times, err := e.app.Identity.Store.AuthFailures(context.Background(), "login", "norbi", "", e.clock.Now().Add(-30*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		return len(times.ByHandle)
	}
	e.app.Maintain(context.Background())
	if n := count(); n != 3 {
		t.Fatalf("recent failures must stay: %d", n)
	}
	e.clock.Advance(8 * 24 * time.Hour)
	e.app.Maintain(context.Background())
	if n := count(); n != 0 {
		t.Fatalf("week-old failures must go: %d", n)
	}
}
