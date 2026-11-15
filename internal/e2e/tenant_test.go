// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// Multi-tenant mode (round 45, docs/23-elkulonites.md): the subdomain picks
// the workspace; another workspace's credential is refused like an unknown
// one, revoked and recorded.

const baseDomain = "colonyhall.test"

const betaOwnerToken = "col_beta_owner_token_for_tenant_tests_only"

// mtEnv is a multi-tenant server with two workspaces: acme (the dev seed,
// its owner holds server.DevAdminToken) and beta (betaOwnerToken).
func mtEnv(t *testing.T, change func(*server.Options)) (acme, beta *env) {
	t.Helper()
	e := newEnvWith(t, func(o *server.Options) {
		o.Multitenant, o.BaseDomain = true, baseDomain
		if change != nil {
			change(o)
		}
	})
	ctx := system.Context(context.Background()) // as the set-slug command does
	if _, err := e.app.Identity.SetWorkspaceSlug(ctx, "colonyhall", "acme"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := e.app.Identity.Bootstrap(ctx, "Beta", "beta", "bob", "Bob", betaOwnerToken); err != nil {
		t.Fatal(err)
	}
	return e.on("acme." + baseDomain), e.on("beta." + baseDomain)
}

// raw does a request on the env's host and returns the status and body.
func (e *env) raw(method, path, token string, body any) (int, string) {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
	req.Host = e.host
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestTenantFromSubdomain(t *testing.T) {
	acme, beta := mtEnv(t, nil)
	var me struct{ Handle string }
	if s := acme.do("GET", "/api/v1/me", server.DevAdminToken, nil, &me); s != http.StatusOK || me.Handle != "norbi" {
		t.Fatalf("acme owner on acme: %d %+v", s, me)
	}
	if s := beta.do("GET", "/api/v1/me", betaOwnerToken, nil, &me); s != http.StatusOK || me.Handle != "bob" {
		t.Fatalf("beta owner on beta: %d %+v", s, me)
	}

	// Every host that is not an active workspace's subdomain gets one answer.
	_, want := acme.on("nope."+baseDomain).raw("GET", "/api/v1/me", server.DevAdminToken, nil)
	for _, host := range []string{"nope." + baseDomain, baseDomain, "evil.example.com", "a.b." + baseDomain, "admin." + baseDomain, "ACME." + baseDomain + ".evil"} {
		s, body := acme.on(host).raw("GET", "/api/v1/me", server.DevAdminToken, nil)
		if s != http.StatusNotFound || body != want {
			t.Errorf("%s: %d %s", host, s, body)
		}
	}
	// Host names are case-insensitive and may carry a port.
	if s, _ := acme.on("ACME."+baseDomain+":8080").raw("GET", "/api/v1/me", server.DevAdminToken, nil); s != http.StatusOK {
		t.Fatalf("upper case host with port: %d", s)
	}
	// The bare domain serves the health checks only.
	if s, _ := acme.on(baseDomain).raw("GET", "/readyz", "", nil); s != http.StatusOK {
		t.Fatalf("readyz on the base domain: %d", s)
	}
	// A suspended workspace looks like an unknown one.
	ctx := context.Background()
	if err := acme.app.Identity.Store.SetWorkspaceState(ctx, betaWorkspace(t, acme), identity.WorkspaceSuspended, "test", nil); err != nil {
		t.Fatal(err)
	}
	if s, body := beta.raw("GET", "/api/v1/me", betaOwnerToken, nil); s != http.StatusNotFound || body != want {
		t.Fatalf("suspended workspace: %d %s", s, body)
	}
}

func TestCrossTenantTokenIsRefusedRevokedAndAudited(t *testing.T) {
	acme, beta := mtEnv(t, nil)
	_, unknown := beta.raw("GET", "/api/v1/me", "col_not_a_real_token_anywhere_at_all", nil)
	s, body := beta.raw("GET", "/api/v1/me", server.DevAdminToken, nil)
	if s != http.StatusUnauthorized || body != unknown {
		t.Fatalf("acme's token on beta must look like an unknown token: %d %s (unknown: %s)", s, body, unknown)
	}
	// The token leaked: it no longer works anywhere.
	if s := acme.do("GET", "/api/v1/me", server.DevAdminToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("a token used on another subdomain must be revoked: %d", s)
	}
	// acme's audit log has it (read by another acme admin).
	if err := acme.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	found := false
	entries, err := acme.app.Audit.Store.Page(context.Background(), acmeWorkspace(t, acme), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range entries {
		if en.Action == identity.EventCrossTenant {
			found = true
			var p identity.CrossTenantUse
			_ = json.Unmarshal(en.Payload, &p)
			if p.Credential != "token" || p.UsedOn != "beta" || strings.Contains(string(en.Payload), server.DevAdminToken) {
				t.Fatalf("audit payload: %s", en.Payload)
			}
		}
	}
	if !found {
		t.Fatal("no auth.cross_tenant entry in acme's audit log")
	}
	// beta saw nothing of it.
	betaEntries, _ := acme.app.Audit.Store.Page(context.Background(), betaWorkspace(t, acme), 0, 100)
	for _, en := range betaEntries {
		if en.Action == identity.EventCrossTenant {
			t.Fatal("the event must stay in the credential's own workspace")
		}
	}
}

func TestCrossTenantSessionAndLogin(t *testing.T) {
	acme, beta := mtEnv(t, func(o *server.Options) { o.SecureCookies = true })
	login := func(e *env, handle string) (int, *http.Cookie) {
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/auth/login",
			strings.NewReader(`{"handle":"`+handle+`","password":"`+server.DevAdminPassword+`"}`))
		req.Host = e.host
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		for _, c := range res.Cookies() {
			return res.StatusCode, c
		}
		return res.StatusCode, nil
	}
	// norbi exists in acme only: on beta's subdomain the login just fails.
	if s, _ := login(beta, "norbi"); s != http.StatusUnauthorized {
		t.Fatalf("login on another subdomain: %d", s)
	}
	s, c := login(acme, "norbi")
	if s != http.StatusOK || c == nil || c.Name != "__Host-colony_session" || c.Domain != "" || !c.Secure || c.Path != "/" {
		t.Fatalf("session cookie on HTTPS in multi-tenant mode: %d %+v", s, c)
	}
	call := func(e *env) int {
		req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/me", nil)
		req.Host = e.host
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	if s := call(acme); s != http.StatusOK {
		t.Fatalf("session on its own subdomain: %d", s)
	}
	if s := call(beta); s != http.StatusUnauthorized {
		t.Fatalf("session on another subdomain: %d", s)
	}
	if s := call(acme); s != http.StatusUnauthorized {
		t.Fatalf("a session used on another subdomain must be revoked: %d", s)
	}
}

func TestCrossTenantHostTokenRevokesTheHost(t *testing.T) {
	acme, beta := mtEnv(t, nil)
	_, hostToken := acme.enroll("acme-pc")
	if s := acme.do("GET", "/api/v1/host/config", hostToken, nil, nil); s != http.StatusOK {
		t.Fatalf("host on its own subdomain: %d", s)
	}
	if s := beta.do("GET", "/api/v1/host/config", hostToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("host token on another subdomain: %d", s)
	}
	if s := acme.do("GET", "/api/v1/host/config", hostToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("the host must be revoked: %d", s)
	}
	// A pairing code works on its own subdomain only.
	var pc struct{ Code string }
	acme.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	if s := beta.do("POST", "/api/v1/hosts/enroll", "", map[string]any{"code": pc.Code, "name": "x"}, nil); s != http.StatusUnauthorized {
		t.Fatalf("acme's pairing code on beta: %d", s)
	}
	if s := acme.do("POST", "/api/v1/hosts/enroll", "", map[string]any{"code": pc.Code, "name": "x"}, nil); s != http.StatusCreated {
		t.Fatalf("the code is still good on acme: %d", s)
	}
}

func acmeWorkspace(t *testing.T, e *env) ids.ID {
	t.Helper()
	w, err := e.app.Identity.Store.WorkspaceBySlug(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	return w.ID
}

func betaWorkspace(t *testing.T, e *env) ids.ID {
	t.Helper()
	w, err := e.app.Identity.Store.WorkspaceBySlug(context.Background(), "beta")
	if err != nil {
		t.Fatal(err)
	}
	return w.ID
}
