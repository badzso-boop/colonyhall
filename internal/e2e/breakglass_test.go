// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/getkin/kin-openapi/openapi3"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
	"colonyhall/internal/storetest"
)

// Round 51b (docs/24, 6. pont): the operator's break-glass access. It
// opens on the customer's own host through a one-time link, reads the
// public channels and the workspace's settings, never a DM, a private
// channel or the search; every write is refused; every read is in the
// customer's audit; the owners are notified and may close it; it expires.

func TestBreakGlass(t *testing.T) { runBreakGlass(t, nil) }

// TestPostgresBreakGlass: the same on Postgres with row-level security, as
// the application role, where the session's connections are read-only.
func TestPostgresBreakGlass(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	app := storetest.AppRoleDatabase(t, url)
	runBreakGlass(t, func(o *server.Options) {
		o.Store, o.DatabaseURL, o.IDs, o.FilesDir = "postgres", app, ids.V7{}, t.TempDir()
	})
}

const bgReason = "Hibakeresés: az ügyfél jelezte, hogy a #general nem töltődik be"

type idOnly struct {
	ID string `json:"id"`
}

func runBreakGlass(t *testing.T, change func(*server.Options)) {
	acme, beta, secret := opEnv(t, change)
	op := acme.on("admin." + baseDomain).client()
	op.operatorLogin(secret)
	owner := server.DevAdminToken

	// acme's content: a public channel, a private one and a DM, each with
	// a message and a file.
	general := acme.generalID()
	if _, s := acme.send(owner, general, map[string]any{"body": "public note"}); s != http.StatusCreated {
		t.Fatalf("public message: %d", s)
	}
	_, pubFile := uploadMultipart(t, acme, general, "pub.txt", []byte("public file content"), owner)
	var priv, dm idOnly
	if s := acme.do("POST", "/api/v1/channels", owner, map[string]any{"name": "secret-room", "kind": "private"}, &priv); s != http.StatusCreated {
		t.Fatalf("private channel: %d", s)
	}
	privMsg, _ := acme.send(owner, priv.ID, map[string]any{"body": "private plan"})
	_, privFile := uploadMultipart(t, acme, priv.ID, "priv.txt", []byte("private file content"), owner)
	if s := acme.do("POST", "/api/v1/dms", owner, map[string]any{"actor_ids": []string{acme.agentID()}}, &dm); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("dm: %d", s)
	}
	dmMsg, _ := acme.send(owner, dm.ID, map[string]any{"body": "dm secret"})
	if pubFile == nil || privFile == nil || privMsg.ID == "" || dmMsg.ID == "" {
		t.Fatal("seed failed")
	}

	var cust apispec.CustomerList
	op.do("GET", "/api/v1/operator/workspaces", nil, &cust)
	acmeID := ""
	for _, c := range cust.Customers {
		if c.Slug == "acme" {
			acmeID = c.Id.String()
		}
	}
	bgPath := "/api/v1/operator/workspaces/" + acmeID + "/break-glass"

	// Opening needs a real reason and at most 4 hours.
	if s := op.do("POST", bgPath, map[string]any{"reason": "too short", "minutes": 30}, nil); s != http.StatusBadRequest {
		t.Fatalf("a short reason: %d", s)
	}
	if s := op.do("POST", bgPath, map[string]any{"reason": bgReason, "minutes": 300}, nil); s != http.StatusBadRequest {
		t.Fatalf("five hours: %d", s)
	}
	var opened apispec.BreakGlassOpened
	if s := op.do("POST", bgPath, map[string]any{"reason": bgReason, "minutes": 30}, &opened); s != http.StatusCreated ||
		!strings.Contains(opened.Url, "acme."+baseDomain) || !opened.BreakGlass.Open {
		t.Fatalf("open: %d %+v", s, opened)
	}
	token := opened.Url[strings.Index(opened.Url, "#")+1:]
	var c apispec.Customer
	if op.do("GET", "/api/v1/operator/workspaces/"+acmeID, nil, &c); !c.BreakGlassOpen {
		t.Fatalf("the customer does not show the open access: %+v", c)
	}

	// The owner hears of it.
	drain := func() {
		t.Helper()
		if err := acme.app.Drain(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	drain()
	eventually(t, "the owner's operator_access item", func() bool {
		var act apispec.ActivityPage
		acme.do("GET", "/api/v1/me/activity", owner, nil, &act)
		for _, it := range act.Items {
			if it.Kind == apispec.ActivityKindOperatorAccess {
				return true
			}
		}
		return false
	})

	// The link works on acme's own host, once.
	if s := beta.client().do("POST", "/api/v1/break-glass/redeem", map[string]any{"token": token}, nil); s != http.StatusGone {
		t.Fatalf("the link on another customer's host: %d", s)
	}
	bg := acme.client()
	var info apispec.SessionInfo
	if s := bg.do("POST", "/api/v1/break-glass/redeem", map[string]any{"token": token}, &info); s != http.StatusOK || info.BreakGlass == nil {
		t.Fatalf("redeem: %d %+v", s, info)
	}
	bg.csrf = info.CsrfToken
	if s := acme.client().do("POST", "/api/v1/break-glass/redeem", map[string]any{"token": token}, nil); s != http.StatusGone {
		t.Fatalf("the link a second time: %d", s)
	}
	if s := bg.do("GET", "/api/v1/auth/session", nil, &info); s != http.StatusOK || info.BreakGlass == nil || info.BreakGlass.OperatorHandle != "ops" {
		t.Fatalf("session: %d %+v", s, info)
	}

	// What it sees: the public channel, its message and file.
	var chans struct {
		Channels []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"channels"`
	}
	if s := bg.do("GET", "/api/v1/channels", nil, &chans); s != http.StatusOK || len(chans.Channels) == 0 {
		t.Fatalf("channels: %d %+v", s, chans)
	}
	for _, ch := range chans.Channels {
		if ch.Kind != "public" || ch.ID == priv.ID || ch.ID == dm.ID {
			t.Fatalf("the channel list shows a %s channel %s", ch.Kind, ch.ID)
		}
	}
	read := func(path string) (int, string) {
		res, body := bg.send("GET", path, nil)
		_ = res.Body.Close()
		return res.StatusCode, string(body)
	}
	if s, body := read("/api/v1/channels/" + general + "/messages"); s != http.StatusOK || !strings.Contains(body, "public note") {
		t.Fatalf("public messages: %d %.200s", s, body)
	}
	if s, body := read("/api/v1/files/" + pubFile.Id.String() + "/content"); s != http.StatusOK || body != "public file content" {
		t.Fatalf("public file: %d %.100s", s, body)
	}
	if s, _ := read("/api/v1/runs"); s != http.StatusOK {
		t.Fatalf("runs: %d", s)
	}
	// Never a DM, a private channel or the search.
	for _, path := range []string{
		"/api/v1/channels/" + priv.ID + "/messages", "/api/v1/channels/" + dm.ID + "/messages",
		"/api/v1/channels/" + priv.ID, "/api/v1/channels/" + dm.ID + "/members", "/api/v1/channels/" + priv.ID + "/pins",
		"/api/v1/messages/" + privMsg.ID + "/thread", "/api/v1/messages/" + dmMsg.ID + "/thread",
		"/api/v1/files/" + privFile.Id.String() + "/content", "/api/v1/files/" + privFile.Id.String(),
		"/api/v1/search?q=private", "/api/v1/admin/audit",
	} {
		s, body := read(path)
		if s < 400 || strings.Contains(body, "private plan") || strings.Contains(body, "dm secret") || strings.Contains(body, "private file content") {
			t.Fatalf("%s: %d %.200s", path, s, body)
		}
	}
	// No socket.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{}
	for name, v := range bg.cookies {
		hdr.Add("Cookie", name+"="+v)
	}
	if ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(acme.srv.URL, "http")+"/ws/ui", &websocket.DialOptions{Host: acme.host, HTTPHeader: hdr}); err == nil {
		if res.Body != nil {
			_ = res.Body.Close()
		}
		_ = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"hello"}`))
		if _, frame, err := ws.Read(ctx); err == nil && strings.Contains(string(frame), "welcome") {
			t.Fatalf("the break-glass session opened a socket: %s", frame)
		}
		_ = ws.CloseNow()
	}

	// Every write in the contract is refused, and every read outside the
	// allowed routes (both lists come from the contract, so a new route is
	// covered without touching this test).
	checkBreakGlassContract(t, bg)

	// The customer's audit has the opening, the reason and the reads.
	drain()
	opens, reads := 0, 0
	eventually(t, "the break-glass entries in the customer's audit", func() bool {
		var audit apispec.AuditPage
		if s := acme.do("GET", "/api/v1/admin/audit?limit=500", owner, nil, &audit); s != http.StatusOK {
			return false
		}
		opens, reads = 0, 0
		for _, e := range audit.Entries {
			switch e.Action {
			case identity.EventBreakGlassOpened:
				if e.Payload["reason"] == bgReason {
					opens++
				}
			case identity.EventBreakGlassRead:
				if p, _ := e.Payload["path"].(string); p == "/api/v1/channels/"+general+"/messages" {
					reads++
				}
			}
		}
		return opens == 1 && reads > 0
	})

	// The operator actor never gets a password, a token or another rank:
	// any of them would skip the break-glass rules.
	opActor := info.Actor.Id.String()
	if s := acme.do("PUT", "/api/v1/actors/"+opActor+"/password", owner, map[string]any{"new_password": "a-long-enough-password"}, nil); s != http.StatusForbidden {
		t.Fatalf("a password for the operator actor: %d", s)
	}
	if s := acme.do("POST", "/api/v1/actors/"+opActor+"/tokens", owner, map[string]any{"label": "x"}, nil); s != http.StatusForbidden {
		t.Fatalf("a token for the operator actor: %d", s)
	}
	if s := acme.do("PUT", "/api/v1/actors/"+opActor+"/role", owner, map[string]any{"role": "member"}, nil); s != http.StatusForbidden {
		t.Fatalf("another rank for the operator actor: %d", s)
	}

	// The owner closes it: the session ends at once.
	var list apispec.BreakGlassList
	if s := acme.do("GET", "/api/v1/workspace/break-glass", owner, nil, &list); s != http.StatusOK || len(list.BreakGlass) != 1 || !list.BreakGlass[0].Open {
		t.Fatalf("the owner's list: %d %+v", s, list)
	}
	if s := acme.do("DELETE", "/api/v1/workspace/break-glass/"+opened.BreakGlass.Id.String(), owner, nil, nil); s != http.StatusNoContent {
		t.Fatalf("close: %d", s)
	}
	if s := bg.do("GET", "/api/v1/auth/session", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("the session after the owner closed it: %d", s)
	}

	// Another one expires: the session ends at the time, the record later.
	var short apispec.BreakGlassOpened
	if s := op.do("POST", bgPath, map[string]any{"reason": bgReason, "minutes": 1}, &short); s != http.StatusCreated {
		t.Fatalf("open again: %d", s)
	}
	bg2 := acme.client()
	if s := bg2.do("POST", "/api/v1/break-glass/redeem", map[string]any{"token": short.Url[strings.Index(short.Url, "#")+1:]}, nil); s != http.StatusOK {
		t.Fatalf("redeem again: %d", s)
	}
	acme.clock.Advance(2 * time.Minute)
	if s := bg2.do("GET", "/api/v1/me", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("after the time: %d", s)
	}
	acme.app.Maintain(context.Background())
	var olist apispec.BreakGlassList
	op.operatorLogin(secret)
	if s := op.do("GET", bgPath, nil, &olist); s != http.StatusOK || len(olist.BreakGlass) != 2 {
		t.Fatalf("the operator's list: %d %+v", s, olist)
	}
	closedBy := map[string]string{}
	for _, b := range olist.BreakGlass {
		if b.ClosedBy != nil && !b.Open {
			closedBy[b.Id.String()] = *b.ClosedBy
		}
	}
	if closedBy[opened.BreakGlass.Id.String()] != identity.BreakGlassClosedByOwner || closedBy[short.BreakGlass.Id.String()] != identity.BreakGlassClosedByExpiry {
		t.Fatalf("closed by: %v", closedBy)
	}
}

var anyParam = regexp.MustCompile(`\{\w+\}`)

// checkBreakGlassContract calls every operation of the contract with the
// break-glass session: a write must be refused (403 read_only_access, or
// 401 where the route takes another credential), and a read outside
// identity.BreakGlassRoutes too (403 break_glass_scope, or 401).
func checkBreakGlassContract(t *testing.T, bg *hostClient) {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for p := range identity.BreakGlassRoutes {
		allowed[anyParam.ReplaceAllString(p, "{}")] = true
	}
	writes, reads := 0, 0
	for path, item := range doc.Paths.Map() {
		if strings.HasPrefix(path, "/operator/") {
			continue // the admin host's
		}
		for method, o := range item.Operations() {
			if o.Security != nil && len(*o.Security) == 0 {
				continue // public: no session involved
			}
			key := method + " /api/v1" + anyParam.ReplaceAllString(path, "{}")
			if allowed[key] {
				continue
			}
			p := "/api/v1" + anyParam.ReplaceAllStringFunc(path, func(string) string { return ids.V7{}.New().String() })
			var body any
			if method != http.MethodGet && method != http.MethodDelete {
				body = map[string]any{}
			}
			res, raw := bg.send(method, p, body)
			_ = res.Body.Close()
			var prob struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(raw, &prob)
			want := "break_glass_scope"
			if method != http.MethodGet {
				want = "read_only_access"
				writes++
			} else {
				reads++
			}
			if res.StatusCode != http.StatusUnauthorized && (res.StatusCode != http.StatusForbidden || prob.Type != want) {
				t.Errorf("%s %s with the break-glass session: %d %s, want 403 %s (or 401)", method, path, res.StatusCode, prob.Type, want)
			}
		}
	}
	t.Logf("break-glass: %d writes and %d reads outside the allowed routes refused", writes, reads)
}
