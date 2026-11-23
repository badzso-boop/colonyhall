// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/preview"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

var (
	wsA      = ids.V7{}.New()
	wsB      = ids.V7{}.New()
	admin    = identity.Actor{ID: ids.V7{}.New(), WorkspaceID: wsA, Kind: identity.KindHuman, Role: identity.RoleAdmin}
	member   = identity.Actor{ID: ids.V7{}.New(), WorkspaceID: wsA, Kind: identity.KindHuman, Role: identity.RoleMember}
	stranger = identity.Actor{ID: ids.V7{}.New(), WorkspaceID: wsB, Kind: identity.KindHuman, Role: identity.RoleOwner}
	agent    = identity.Actor{ID: ids.V7{}.New(), WorkspaceID: wsA, Kind: identity.KindAgent, Handle: "web-agent"}
	// owned is the member's own agent; agent belongs to nobody.
	owned = identity.Actor{ID: ids.V7{}.New(), WorkspaceID: wsA, Kind: identity.KindAgent, Handle: "mine", OwnerID: member.ID}
)

// fakeAuth maps bearer tokens and session cookies to actors.
type fakeAuth struct{ by map[string]identity.Actor }

func (f fakeAuth) Authenticate(_ context.Context, secret string) (identity.Actor, error) {
	if a, ok := f.by[secret]; ok {
		return a, nil
	}
	return identity.Actor{}, identity.ErrUnauthorized
}

func (f fakeAuth) AuthenticateSession(ctx context.Context, secret string) (identity.Actor, identity.Session, error) {
	a, err := f.Authenticate(ctx, secret)
	return a, identity.Session{}, err
}

func (fakeAuth) RecordBreakGlassRead(context.Context, identity.Actor, string, string) error {
	return nil
}

type directory struct{}

func (directory) ActorByID(_ context.Context, id ids.ID) (identity.Actor, error) {
	for _, a := range []identity.Actor{agent, owned} {
		if a.ID == id {
			return a, nil
		}
	}
	return identity.Actor{}, db.ErrNotFound
}

// fakeStore is the part of preview.Store the proxy reads (the adapters
// are off limits here; the store contract is tested in storetest).
type fakeStore struct {
	preview.Store
	byID map[ids.ID]preview.Preview
}

func (f *fakeStore) Create(_ context.Context, p preview.Preview) error {
	f.byID[p.ID] = p
	return nil
}

func (f *fakeStore) ByID(_ context.Context, id ids.ID) (preview.Preview, error) {
	p, ok := f.byID[id]
	if !ok {
		return preview.Preview{}, db.ErrNotFound
	}
	return p, nil
}

func (f *fakeStore) StopActive(_ context.Context, id ids.ID, at time.Time) error {
	p := f.byID[id]
	p.State, p.UpdatedAt = preview.StateStopped, at
	f.byID[id] = p
	return nil
}

type fixture struct {
	proxy   *httptest.Server
	store   *fakeStore
	clock   *clock.Manual
	running preview.Preview
}

// newFixture: a running preview of agent pointing at backend, served by
// the proxy mux.
func newFixture(t *testing.T, backend string, owner identity.Actor) *fixture {
	t.Helper()
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	f := &fixture{store: &fakeStore{byID: map[ids.ID]preview.Preview{}}, clock: clock.NewManual(now)}
	svc := &preview.Service{Store: f.store, Directory: directory{}, Clock: f.clock}
	f.running = preview.Preview{ID: ids.V7{}.New(), WorkspaceID: wsA, AgentID: owner.ID, HostID: ids.V7{}.New(), ChannelID: ids.V7{}.New(),
		Name: "web", ComposePath: "compose.yaml", State: preview.StateRunning, URL: backend,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
	if err := f.store.Create(context.Background(), f.running); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterProxy(mux, svc, fakeAuth{by: map[string]identity.Actor{
		"admin": admin, "member": member, "stranger": stranger, "agent": agent,
	}}, slog.New(slog.DiscardHandler), testNets)
	f.proxy = httptest.NewServer(mux)
	t.Cleanup(f.proxy.Close)
	return f
}

func (f *fixture) path(rest string) string {
	return "/api/v1/previews/" + f.running.ID.String() + "/proxy" + rest
}

// result is a read and closed answer.
type result struct {
	StatusCode int
	Header     http.Header
	Body       string
	cookies    []*http.Cookie
}

func (r result) Cookies() []*http.Cookie { return r.cookies }

func (f *fixture) do(t *testing.T, method, rest, session string, body io.Reader, hdr map[string]string) result {
	t.Helper()
	req, _ := http.NewRequest(method, f.proxy.URL+f.path(rest), body)
	if session != "" {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: session})
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return result{StatusCode: res.StatusCode, Header: res.Header, Body: string(b), cookies: res.Cookies()}
}

func TestProxyForwardsRequestAndHidesCredentials(t *testing.T) {
	var got *http.Request
	var gotBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		http.SetCookie(w, &http.Cookie{Name: httpx.SessionCookie, Value: "planted", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "app", Value: "kept", Path: "/"})
		w.Header().Set("X-Frame-Options", "DENY")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello from the preview"))
	}))
	defer backend.Close()
	f := newFixture(t, backend.URL, agent)

	res := f.do(t, "POST", "/src/main.tsx?x=1", "admin", strings.NewReader("form=1"), map[string]string{
		"Cookie":         httpx.SessionCookie + "=admin; app=theirs",
		httpx.CSRFHeader: "csrf-secret",
		"Content-Type":   "application/x-www-form-urlencoded",
	})
	if res.StatusCode != http.StatusCreated || res.Body != "hello from the preview" {
		t.Fatalf("status %d body %q", res.StatusCode, res.Body)
	}
	// The path is forwarded unchanged: the preview runs with this base.
	if got.URL.Path != f.path("/src/main.tsx") || got.URL.RawQuery != "x=1" || gotBody != "form=1" {
		t.Fatalf("forwarded %s?%s body %q", got.URL.Path, got.URL.RawQuery, gotBody)
	}
	if got.Header.Get("X-Forwarded-Prefix") != "/api/v1/previews/"+f.running.ID.String()+"/proxy" ||
		got.Header.Get("X-Forwarded-Host") == "" || got.Header.Get("X-Forwarded-Proto") != "http" {
		t.Fatalf("forwarded headers %v", got.Header)
	}
	// The person's Colonyhall credentials never reach the agent's app.
	if got.Header.Get(httpx.CSRFHeader) != "" || got.Header.Get("Authorization") != "" {
		t.Fatalf("credentials forwarded: %v", got.Header)
	}
	if c, err := got.Cookie(httpx.SessionCookie); err == nil {
		t.Fatalf("session cookie forwarded: %v", c)
	}
	if c, err := got.Cookie("app"); err != nil || c.Value != "theirs" {
		t.Fatalf("the app's own cookie was not forwarded: %v %v", c, err)
	}
	// Nor can it plant a session cookie (cookies ignore the port).
	for _, c := range res.Cookies() {
		if c.Name == httpx.SessionCookie {
			t.Fatalf("the preview set the session cookie: %v", c)
		}
	}
	if len(res.Cookies()) != 1 || res.Header.Get("X-Frame-Options") != "" {
		t.Fatalf("response cookies %v, X-Frame-Options %q", res.Cookies(), res.Header.Get("X-Frame-Options"))
	}
}

func TestProxyRootAndBearer(t *testing.T) {
	var paths []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
	}))
	defer backend.Close()
	f := newFixture(t, backend.URL, agent)
	if res := f.do(t, "GET", "", "", nil, map[string]string{"Authorization": "Bearer admin"}); res.StatusCode != http.StatusOK {
		t.Fatalf("root: %d", res.StatusCode)
	}
	if res := f.do(t, "GET", "/", "admin", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("slash: %d", res.StatusCode)
	}
	if len(paths) != 2 || paths[0] != f.path("") || paths[1] != f.path("/") {
		t.Fatalf("paths %v", paths)
	}
}

func TestProxyRefusals(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a refused request reached the preview: %s", r.URL.Path)
	}))
	defer backend.Close()
	f := newFixture(t, backend.URL, agent)
	for _, c := range []struct {
		name, session string
		want          int
	}{
		{"no session", "", http.StatusUnauthorized},
		{"bad session", "nope", http.StatusUnauthorized},
		{"other workspace", "stranger", http.StatusNotFound},
		// A member does not see an agent of nobody's (round 38).
		{"unseen agent", "member", http.StatusNotFound},
		// Agents never use the proxy: it would reach other machines' ports.
		{"agent", "agent", http.StatusForbidden},
	} {
		if res := f.do(t, "GET", "/", c.session, nil, nil); res.StatusCode != c.want {
			t.Errorf("%s: %d, want %d", c.name, res.StatusCode, c.want)
		}
	}
	res, _ := http.Get(f.proxy.URL + "/api/v1/previews/" + ids.V7{}.New().String() + "/proxy/")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("unknown preview without a session: %d", res.StatusCode)
	}
}

func TestProxyMemberSeesOwnAgentsPreview(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()
	f := newFixture(t, backend.URL, owned)
	if res := f.do(t, "GET", "/", "member", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("member, own agent: %d", res.StatusCode)
	}
}

func TestProxyOffline(t *testing.T) {
	// Unreachable: a listener that was closed.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + l.Addr().String()
	_ = l.Close()
	f := newFixture(t, dead, agent)

	res := f.do(t, "GET", "/", "admin", nil, map[string]string{"Accept": "text/html"})
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(res.Body, "Preview offline") {
		t.Fatalf("unreachable, html: %d %s", res.StatusCode, res.Body)
	}
	res = f.do(t, "GET", "/api/data", "admin", nil, nil)
	var p struct {
		Type   string
		Status int
	}
	_ = json.Unmarshal([]byte(res.Body), &p)
	if res.StatusCode != http.StatusBadGateway || p.Status != 502 || res.Header.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unreachable, json: %d %+v", res.StatusCode, p)
	}

	// Stopped, and expired: never forwarded, 502.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("an offline preview was forwarded")
	}))
	defer backend.Close()
	f = newFixture(t, backend.URL, agent)
	_ = f.store.StopActive(context.Background(), f.running.ID, f.clock.Now())
	if res := f.do(t, "GET", "/", "admin", nil, nil); res.StatusCode != http.StatusBadGateway {
		t.Fatalf("stopped: %d", res.StatusCode)
	}
	f = newFixture(t, backend.URL, agent)
	f.clock.Advance(2 * time.Hour)
	if res := f.do(t, "GET", "/", "admin", nil, nil); res.StatusCode != http.StatusBadGateway {
		t.Fatalf("expired: %d", res.StatusCode)
	}
}

// Vite's live reload: the WebSocket upgrade is forwarded both ways.
func TestProxyWebSocket(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(httpx.SessionCookie); err == nil {
			t.Errorf("session cookie reached the preview's socket")
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, Subprotocols: []string{"vite-hmr"}})
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer func() { _ = c.CloseNow() }()
		ctx := r.Context()
		_, msg, err := c.Read(ctx)
		if err != nil {
			return
		}
		_ = c.Write(ctx, websocket.MessageText, []byte("echo:"+string(msg)))
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"full-reload"}`))
	}))
	defer backend.Close()
	f := newFixture(t, backend.URL, agent)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{}
	hdr.Set("Cookie", httpx.SessionCookie+"=admin")
	c, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.proxy.URL, "http")+f.path("/"), &websocket.DialOptions{
		HTTPHeader: hdr, Subprotocols: []string{"vite-hmr"}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = c.CloseNow() }()
	if res.StatusCode != http.StatusSwitchingProtocols || c.Subprotocol() != "vite-hmr" {
		t.Fatalf("upgrade: %d %q", res.StatusCode, c.Subprotocol())
	}
	if err := c.Write(ctx, websocket.MessageText, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"echo:ping", `{"type":"full-reload"}`} {
		_, msg, err := c.Read(ctx)
		if err != nil || string(msg) != want {
			t.Fatalf("read %q %v, want %q", msg, err, want)
		}
	}

	// Without a session the upgrade is refused before the preview.
	_, res, err = websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.proxy.URL, "http")+f.path("/"), nil)
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	if err == nil || res == nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated upgrade: %v %v", res, err)
	}
}

func TestLinksURL(t *testing.T) {
	r := httptest.NewRequest("GET", "http://colony.example.com:8080/api/v1/previews", nil)
	if got := (Links{}).URL(r, "p1"); got != "" {
		t.Fatalf("disabled: %q", got)
	}
	if got := (Links{Port: "8081"}).URL(r, "p1"); got != "http://colony.example.com:8081/api/v1/previews/p1/proxy/" {
		t.Fatalf("derived: %q", got)
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := (Links{Port: "8081"}).URL(r, "p1"); !strings.HasPrefix(got, "https://colony.example.com:8081/") {
		t.Fatalf("https: %q", got)
	}
	if got := (Links{Origin: "https://preview.example.com/", Port: "8081"}).URL(r, "p1"); got != "https://preview.example.com/api/v1/previews/p1/proxy/" {
		t.Fatalf("origin: %q", got)
	}
}

// testNets: the httptest backends listen on loopback.
var testNets = append([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, DefaultNets...)

// The proxy dials only the allowed networks, and never link-local (cloud
// metadata) or unspecified addresses, whatever the guard reported.
func TestDialGuard(t *testing.T) {
	strict := dialGuard(DefaultNets)
	for addr, ok := range map[string]bool{
		"100.64.0.5:41000": true, "[fd7a:115c:a1e0::5]:80": true,
		"192.168.1.10:41000": false, "172.17.0.2:5432": false, "10.0.0.1:80": false,
		"127.0.0.1:41000": false, "[::1]:80": false, "169.254.169.254:80": false,
		"[fe80::1]:80": false, "0.0.0.0:80": false, "[::ffff:169.254.169.254]:80": false, "224.0.0.1:80": false,
	} {
		if err := strict("tcp", addr, nil); (err == nil) != ok {
			t.Errorf("default %s: %v", addr, err)
		}
	}
	all := dialGuard([]netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")})
	if err := all("tcp", "127.0.0.1:41000", nil); err != nil {
		t.Errorf("listed loopback refused: %v", err)
	}
	if err := all("tcp", "169.254.169.254:80", nil); err == nil {
		t.Error("metadata address dialled inside an allowed network")
	}
}

func TestProxyRefusesBlockedTarget(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a loopback preview was dialled")
	}))
	defer backend.Close()
	f := newFixture(t, backend.URL, agent)
	mux := http.NewServeMux()
	RegisterProxy(mux, &preview.Service{Store: f.store, Directory: directory{}, Clock: f.clock},
		fakeAuth{by: map[string]identity.Actor{"admin": admin}}, slog.New(slog.DiscardHandler), DefaultNets)
	f.proxy.Config.Handler = mux
	if res := f.do(t, "GET", "/", "admin", nil, nil); res.StatusCode != http.StatusBadGateway {
		t.Fatalf("loopback target: %d", res.StatusCode)
	}
}
