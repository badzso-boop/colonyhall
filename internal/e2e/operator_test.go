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
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/secretbox"
	"colonyhall/internal/server"
)

// Round 51 (docs/24-hosztolt-ajanlat.md): the platform operator on
// admin.<base domain>, the owner invite and suspension.

const opTestPassword = "operator-password-for-tests"

// opEnv is mtEnv with the secret keys and an active operator; it returns
// the operator's TOTP secret.
func opEnv(t *testing.T, change func(*server.Options)) (acme, beta *env, secret string) {
	t.Helper()
	k, _ := secretbox.NewKey("k1")
	keys, _ := secretbox.Parse(k)
	acme, beta = mtEnv(t, func(o *server.Options) {
		o.SecretKeys = keys
		if change != nil {
			change(o)
		}
	})
	ctx := context.Background()
	_, en, err := acme.app.Operator.CreateOperator(ctx, "ops", "Ops", opTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acme.app.Operator.ConfirmTOTP(ctx, "ops", operator.CodeAt(en.Secret, acme.clock.Now())); err != nil {
		t.Fatal(err)
	}
	acme.clock.Advance(time.Minute)
	return acme, beta, en.Secret
}

// hostClient is a browser on one host: it keeps the cookies the server
// sets and sends the CSRF token; every response is checked against the
// contract.
type hostClient struct {
	e       *env
	cookies map[string]string
	csrf    string
	header  http.Header
}

func (e *env) client() *hostClient {
	return &hostClient{e: e, cookies: map[string]string{}, header: http.Header{}}
}

func (c *hostClient) send(method, path string, body any) (*http.Response, []byte) {
	c.e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, &buf)
	req.Host = c.e.host
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.header {
		req.Header[k] = v
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for name, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: v})
	}
	res, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed below; callers get it closed
	if err != nil {
		c.e.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	for _, ck := range res.Cookies() {
		if ck.Value == "" {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck.Value
		}
	}
	return res, raw
}

func (c *hostClient) do(method, path string, body, out any) int {
	c.e.t.Helper()
	res, raw := c.send(method, path, body)
	_ = res.Body.Close()
	if strings.HasPrefix(path, "/api/") {
		req, _ := http.NewRequest(method, c.e.srv.URL+path, nil)
		req.Host = c.e.host
		c.e.checkContract(req, res, raw)
	}
	if out != nil {
		_ = json.Unmarshal(raw, out)
	}
	return res.StatusCode
}

// operatorLogin logs the client in as the test operator.
func (c *hostClient) operatorLogin(secret string) apispec.OperatorSessionInfo {
	c.e.t.Helper()
	c.e.clock.Advance(31 * time.Second) // a fresh TOTP step
	var info apispec.OperatorSessionInfo
	if s := c.do("POST", "/api/v1/operator/auth/login", map[string]any{"handle": "ops", "password": opTestPassword,
		"code": operator.CodeAt(secret, c.e.clock.Now())}, &info); s != http.StatusOK {
		c.e.t.Fatalf("operator login: %d", s)
	}
	c.csrf = info.CsrfToken
	return info
}

func TestOperatorCreatesACustomerAndTheInviteBringsTheOwner(t *testing.T) {
	acme, beta, secret := opEnv(t, nil)
	admin := acme.on("admin." + baseDomain)
	op := admin.client()

	if s := op.do("GET", "/api/v1/operator/auth/session", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("no session: %d", s)
	}
	acme.clock.Advance(31 * time.Second)
	if s := op.do("POST", "/api/v1/operator/auth/login", map[string]any{"handle": "ops", "password": opTestPassword, "code": "000000"}, nil); s != http.StatusUnauthorized {
		t.Fatalf("a wrong code: %d", s)
	}
	info := op.operatorLogin(secret)
	if info.Operator.Handle != "ops" || info.DevAccess || info.BackupCodesLeft != operator.BackupCodeCount ||
		!info.ExpiresAt.Equal(acme.clock.Now().Add(operator.SessionTTL)) {
		t.Fatalf("session: %+v", info)
	}
	if _, ok := op.cookies["colony_operator"]; !ok {
		t.Fatalf("operator cookie: %v", op.cookies)
	}

	// Unsafe requests need the CSRF token.
	csrf := op.csrf
	op.csrf = ""
	if s := op.do("POST", "/api/v1/operator/workspaces", map[string]any{"name": "Gamma", "slug": "gamma"}, nil); s != http.StatusForbidden {
		t.Fatalf("without CSRF: %d", s)
	}
	op.csrf = csrf

	var created apispec.CreatedCustomer
	if s := op.do("POST", "/api/v1/operator/workspaces", map[string]any{"name": "Gamma Kft", "slug": "gamma"}, &created); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	prefix := "http://gamma." + baseDomain + "/agent/invite#"
	token, ok := strings.CutPrefix(created.Invite.Url, prefix)
	if !ok || token == "" || created.Customer.HasOwner || created.Customer.Invite == nil || created.Customer.Invite.Used ||
		!created.Invite.ExpiresAt.Equal(acme.clock.Now().Add(identity.InviteTTL)) {
		t.Fatalf("created: %+v", created)
	}
	for slug, want := range map[string]int{"gamma": http.StatusConflict, "admin": http.StatusBadRequest, "Bad Slug": http.StatusBadRequest} {
		if s := op.do("POST", "/api/v1/operator/workspaces", map[string]any{"name": "X", "slug": slug}, nil); s != want {
			t.Fatalf("slug %q: %d, want %d", slug, s, want)
		}
	}

	// The overview: account data and numbers, no content.
	if err := acme.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, raw := op.send("GET", "/api/v1/operator/workspaces", nil)
	_ = res.Body.Close()
	var list apispec.CustomerList
	_ = json.Unmarshal(raw, &list)
	if res.StatusCode != http.StatusOK || len(list.Customers) != 3 {
		t.Fatalf("list: %d %s", res.StatusCode, raw)
	}
	for _, leak := range []string{"norbi", "bob", "general", "fake-agent"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("customer content (%q) in the overview: %s", leak, raw)
		}
	}
	for _, c := range list.Customers {
		if c.Slug == "acme" && (!c.HasOwner || c.Users == 0 || c.Agents == 0 || c.LastActivityAt == nil) {
			t.Fatalf("acme in the overview: %+v", c)
		}
	}

	// The invite works on its own subdomain only, once.
	gamma := acme.on("gamma." + baseDomain).client()
	if s := beta.client().do("POST", "/api/v1/invites/lookup", map[string]any{"token": token}, nil); s != http.StatusGone {
		t.Fatalf("the invite on another subdomain: %d", s)
	}
	var inv apispec.InviteInfo
	if s := gamma.do("POST", "/api/v1/invites/lookup", map[string]any{"token": token}, &inv); s != http.StatusOK || inv.WorkspaceName != "Gamma Kft" {
		t.Fatalf("lookup: %d %+v", s, inv)
	}
	if s := gamma.do("POST", "/api/v1/invites/accept", map[string]any{"token": token, "handle": "gina", "password": "short"}, nil); s != http.StatusBadRequest {
		t.Fatalf("a weak password: %d", s)
	}
	var sess apispec.SessionInfo
	if s := gamma.do("POST", "/api/v1/invites/accept", map[string]any{"token": token, "handle": "gina", "display_name": "Gina",
		"password": "gamma-owner-password"}, &sess); s != http.StatusOK || sess.Actor.Handle != "gina" || sess.Actor.Role != apispec.Role("owner") {
		t.Fatalf("accept: %d %+v", s, sess)
	}
	var me apispec.Actor
	if s := gamma.do("GET", "/api/v1/me", nil, &me); s != http.StatusOK || me.Handle != "gina" {
		t.Fatalf("the new owner is logged in: %d %+v", s, me)
	}
	other := acme.on("gamma." + baseDomain).client()
	if s := other.do("POST", "/api/v1/invites/accept", map[string]any{"token": token, "handle": "eve", "password": "another-long-password"}, nil); s != http.StatusGone {
		t.Fatalf("a second acceptance: %d", s)
	}
	if s := other.do("POST", "/api/v1/invites/lookup", map[string]any{"token": token}, nil); s != http.StatusGone {
		t.Fatalf("lookup after use: %d", s)
	}
	if s := op.do("POST", "/api/v1/operator/workspaces/"+created.Customer.Id.String()+"/invite", nil, nil); s != http.StatusConflict {
		t.Fatalf("an invite for a workspace with an owner: %d", s)
	}
	var cu apispec.Customer
	if s := op.do("GET", "/api/v1/operator/workspaces/"+created.Customer.Id.String(), nil, &cu); s != http.StatusOK || !cu.HasOwner || cu.Users != 1 || !cu.Invite.Used {
		t.Fatalf("after acceptance: %d %+v", s, cu)
	}

	// The operator audit has it all; the customer's audit has its part.
	var audit apispec.OperatorAuditList
	if s := op.do("GET", "/api/v1/operator/audit?limit=50", nil, &audit); s != http.StatusOK {
		t.Fatalf("audit: %d", s)
	}
	seen := map[string]bool{}
	for _, e := range audit.Entries {
		seen[e.Action] = true
	}
	for _, a := range []string{operator.ActionLogin, operator.ActionLoginFailed, operator.ActionWorkspaceCreated, operator.ActionInviteIssued} {
		if !seen[a] {
			t.Fatalf("operator audit misses %s: %+v", a, seen)
		}
	}
	if err := acme.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := acme.app.Audit.Store.Page(db.WithWorkspace(context.Background(), created.Customer.Id), created.Customer.Id, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	seen = map[string]bool{}
	for _, e := range entries {
		seen[e.Action] = true
		if strings.Contains(string(e.Payload), token) {
			t.Fatalf("the invite token in the customer's audit: %s", e.Payload)
		}
	}
	for _, a := range []string{identity.EventWorkspaceCreated, identity.EventInviteCreated, identity.EventInviteAccepted} {
		if !seen[a] {
			t.Fatalf("the customer's audit misses %s: %+v", a, seen)
		}
	}

	// Logout ends the session.
	if s := op.do("POST", "/api/v1/operator/auth/logout", nil, nil); s != http.StatusNoContent {
		t.Fatalf("logout: %d", s)
	}
	if s := op.do("GET", "/api/v1/operator/workspaces", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", s)
	}
}

func TestOperatorAndCustomerCredentialsDoNotCross(t *testing.T) {
	acme, _, secret := opEnv(t, nil)
	admin := acme.on("admin." + baseDomain)
	op := admin.client()
	op.operatorLogin(secret)
	opCookie := op.cookies["colony_operator"]
	_, unknown := acme.on("nope."+baseDomain).raw("GET", "/api/v1/me", "", nil)

	// A customer's token or session does not open the operator API.
	if s := admin.do("GET", "/api/v1/operator/workspaces", server.DevAdminToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("a customer token on the operator API: %d", s)
	}
	cust := acme.client()
	if s := cust.do("POST", "/api/v1/auth/login", map[string]any{"handle": "norbi", "password": server.DevAdminPassword}, nil); s != http.StatusOK {
		t.Fatalf("customer login: %d", s)
	}
	stolen := admin.client()
	stolen.cookies = cust.cookies
	if s := stolen.do("GET", "/api/v1/operator/workspaces", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("a customer session on the operator API: %d", s)
	}
	// The customers' API is not on the admin host: the unknown-host answer.
	for _, path := range []string{"/api/v1/me", "/api/v1/channels", "/mcp", "/ws/ui"} {
		if s, body := admin.raw("GET", path, server.DevAdminToken, nil); s != http.StatusNotFound || body != unknown {
			t.Fatalf("%s on the admin host: %d %s", path, s, body)
		}
	}
	// The operator's cookie does nothing on a customer's host, and the
	// operator API is not there.
	carried := acme.client()
	carried.cookies["colony_operator"] = opCookie
	if s := carried.do("GET", "/api/v1/me", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("the operator cookie on a customer host: %d", s)
	}
	if s, body := acme.raw("GET", "/api/v1/operator/workspaces", "", nil); s != http.StatusNotFound || strings.Contains(body, "acme") {
		t.Fatalf("the operator API on a customer host: %d %s", s, body)
	}
}

func TestSuspendedWorkspaceIsRefusedEverywhere(t *testing.T) {
	acme, beta, secret := opEnv(t, nil)
	op := acme.on("admin." + baseDomain).client()
	op.operatorLogin(secret)
	var list apispec.CustomerList
	op.do("GET", "/api/v1/operator/workspaces", nil, &list)
	var betaID string
	for _, c := range list.Customers {
		if c.Slug == "beta" {
			betaID = c.Id.String()
		}
	}
	_, unknown := acme.on("nope."+baseDomain).raw("GET", "/api/v1/me", "", nil)

	// beta's owner has a socket open.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(beta.srv.URL, "http")+"/ws/ui", &websocket.DialOptions{Host: beta.host})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = ws.CloseNow() }()
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": betaOwnerToken}); err != nil {
		t.Fatal(err)
	}
	expectFrame(t, ctx, ws, "welcome")

	if s := op.do("POST", "/api/v1/operator/workspaces/"+betaID+"/suspend", map[string]any{"reason": ""}, nil); s != http.StatusBadRequest {
		t.Fatalf("a suspension without a reason: %d", s)
	}
	var cu apispec.Customer
	if s := op.do("POST", "/api/v1/operator/workspaces/"+betaID+"/suspend", map[string]any{"reason": "unpaid invoice 2026-11"}, &cu); s != http.StatusOK ||
		cu.State != apispec.CustomerStateSuspended || cu.SuspendedReason == nil || *cu.SuspendedReason != "unpaid invoice 2026-11" {
		t.Fatalf("suspend: %d %+v", s, cu)
	}

	// The open socket is closed.
	for {
		var f map[string]any
		if err := wsjson.Read(ctx, ws, &f); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("the socket ended with %v, want a policy close", err)
			}
			break
		}
	}
	// The API, MCP, the guard's routes and sockets: the unknown-host answer.
	for _, r := range [][2]string{{"GET", "/api/v1/me"}, {"POST", "/mcp"}, {"GET", "/api/v1/host/config"}, {"GET", "/ws/ui"}, {"GET", "/ws/host"}} {
		if s, body := beta.raw(r[0], r[1], betaOwnerToken, nil); s != http.StatusNotFound || body != unknown {
			t.Fatalf("%s %s while suspended: %d %s", r[0], r[1], s, body)
		}
	}
	// A browser gets the paused page, in its language, without the reason.
	page := beta.client()
	page.header.Set("Accept", "text/html,application/xhtml+xml")
	for lang, want := range map[string]string{"hu-HU,hu;q=0.9": "A munkaterület szünetel", "en-GB,en;q=0.8": "This workspace is paused", "": "A munkaterület szünetel"} {
		page.header.Set("Accept-Language", lang)
		res, raw := page.send("GET", "/agent/", nil)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusForbidden || !strings.Contains(string(raw), want) || strings.Contains(string(raw), "unpaid") ||
			!strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
			t.Fatalf("paused page (%q): %d %s", lang, res.StatusCode, raw)
		}
	}
	// The customer's audit says it was suspended, not why.
	if err := acme.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	betaWS := betaWorkspace(t, acme)
	entries, _ := acme.app.Audit.Store.Page(db.WithWorkspace(context.Background(), betaWS), betaWS, 0, 100)
	found := false
	for _, e := range entries {
		if e.Action == identity.EventWorkspaceSuspended {
			found = true
			if strings.Contains(string(e.Payload), "unpaid") {
				t.Fatalf("the reason in the customer's audit: %s", e.Payload)
			}
		}
	}
	if !found {
		t.Fatal("no workspace.suspended in the customer's audit")
	}

	// Restored: everything works again, the data is there.
	var restored apispec.Customer
	if s := op.do("POST", "/api/v1/operator/workspaces/"+betaID+"/restore", nil, &restored); s != http.StatusOK || restored.State != apispec.CustomerStateActive ||
		restored.SuspendedReason != nil || restored.SuspendedAt != nil {
		t.Fatalf("restore: %d %+v", s, restored)
	}
	var me apispec.Actor
	if s := beta.do("GET", "/api/v1/me", betaOwnerToken, nil, &me); s != http.StatusOK || me.Handle != "bob" {
		t.Fatalf("after restore: %d %+v", s, me)
	}
}

func TestOperatorDevAccessIsShown(t *testing.T) {
	acme, _, secret := opEnv(t, func(o *server.Options) { o.OperatorDevAccess = true })
	info := acme.on("admin." + baseDomain).client().operatorLogin(secret)
	if !info.DevAccess {
		t.Fatal("developer access must be shown to the operator")
	}
}
