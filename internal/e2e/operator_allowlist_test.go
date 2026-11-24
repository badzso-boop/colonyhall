// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"net/http"
	"net/netip"
	"testing"

	"colonyhall/internal/server"
)

// Round 51: admin.<domain> answers only the operator's addresses
// (COLONY_OPERATOR_ALLOWLIST). From anywhere else it does not exist (the
// unknown-host 404), so nobody outside can try the operator's password or
// lock the operator out with failed logins; the customers' hosts are not
// affected.
func TestOperatorConsoleAnswersOnlyTheAllowlist(t *testing.T) {
	acme, _, secret := opEnv(t, func(o *server.Options) {
		o.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
		o.OperatorAllowlist = []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}
	})
	admin := acme.on("admin." + baseDomain)

	outsider := admin.client()
	outsider.header.Set("X-Forwarded-For", "203.0.113.9")
	unknown := func() string {
		_, body := acme.on("nosuch."+baseDomain).raw("GET", "/api/v1/operator/auth/session", "", nil)
		return body
	}()
	for _, p := range []string{"/api/v1/operator/auth/session", "/agent/", "/"} {
		res, body := outsider.send("GET", p, nil)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound || string(body) != unknown {
			t.Fatalf("%s from outside the allowlist: %d %s", p, res.StatusCode, body)
		}
	}
	// Ten failed logins from outside: none reaches the login (5 would lock
	// the handle out without the allowlist).
	for range 10 {
		res, _ := outsider.send("POST", "/api/v1/operator/auth/login", map[string]any{"handle": "ops", "password": "wrong password!!", "code": "000000"})
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("a login from outside the allowlist: %d", res.StatusCode)
		}
	}

	op := admin.client()
	op.header.Set("X-Forwarded-For", "198.51.100.7")
	if info := op.operatorLogin(secret); info.Operator.Handle != "ops" {
		t.Fatalf("the operator from an allowed address: %+v", info)
	}
	if s := op.do("GET", "/api/v1/operator/workspaces", nil, nil); s != http.StatusOK {
		t.Fatalf("the console from an allowed address: %d", s)
	}

	// A customer's host is reachable from anywhere, as before.
	cust := acme.client()
	cust.header.Set("X-Forwarded-For", "203.0.113.9")
	res, _ := cust.send("GET", "/healthz", nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("a customer host from outside the operator's allowlist: %d", res.StatusCode)
	}
}
