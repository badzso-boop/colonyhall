package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/server"
)

// browser is an HTTP client with a cookie jar, like a logged-in browser tab.
type browser struct {
	e      *env
	client *http.Client
	csrf   string
}

func (e *env) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{e: e, client: &http.Client{Jar: sessionJar{jar}}}
}

// sessionJar keeps every cookie for the browser's lifetime. The server
// stamps expiry dates from the tests' manual clock (2026-10-15 plus 30
// days), but cookiejar compares them with the real time, so from
// 2026-11-14 it threw the login cookie away and every browser test failed.
// Expiry is the server's job and is tested there, not the jar's.
type sessionJar struct{ http.CookieJar }

func (j sessionJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	kept := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		if c.MaxAge < 0 { // a logout deletes the cookie: keep that
			kept = append(kept, c)
			continue
		}
		cc := *c
		cc.Expires, cc.MaxAge = time.Time{}, 0
		kept = append(kept, &cc)
	}
	j.CookieJar.SetCookies(u, kept)
}

func (b *browser) do(method, path string, body any, out any) int {
	b.e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, b.e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if b.csrf != "" {
		req.Header.Set("X-CSRF-Token", b.csrf)
	}
	res, err := b.client.Do(req)
	if err != nil {
		b.e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	b.e.checkContract(req, res, raw)
	if out != nil {
		_ = json.Unmarshal(raw, out)
	}
	return res.StatusCode
}

func (b *browser) login(handle, password string) int {
	var info struct {
		CsrfToken string `json:"csrf_token"`
	}
	s := b.do("POST", "/api/v1/auth/login", map[string]any{"handle": handle, "password": password}, &info)
	b.csrf = info.CsrfToken
	return s
}

func TestBrowserLoginWithCSRF(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	b := e.browser()
	if s := b.login("norbi", server.DevAdminPassword); s != http.StatusOK {
		t.Fatalf("login: status %d", s)
	}
	if b.csrf == "" {
		t.Fatal("login must return a CSRF token")
	}
	var me struct{ Handle string }
	if s := b.do("GET", "/api/v1/me", nil, &me); s != http.StatusOK || me.Handle != "norbi" {
		t.Fatalf("me: %d %+v", s, me)
	}

	// A state-changing request without the CSRF header is rejected.
	csrf := b.csrf
	b.csrf = ""
	if s := b.do("POST", "/api/v1/channels/"+general+"/messages", map[string]any{"body": "x"}, nil); s != http.StatusForbidden {
		t.Fatalf("POST without CSRF: status %d", s)
	}
	b.csrf = "wrong"
	if s := b.do("POST", "/api/v1/channels/"+general+"/messages", map[string]any{"body": "x"}, nil); s != http.StatusForbidden {
		t.Fatalf("POST with wrong CSRF: status %d", s)
	}
	b.csrf = csrf
	if s := b.do("POST", "/api/v1/channels/"+general+"/messages", map[string]any{"body": "hello from the browser"}, nil); s != http.StatusCreated {
		t.Fatalf("POST with CSRF: status %d", s)
	}

	// After a reload the page gets the CSRF token back from the session.
	var info struct {
		CsrfToken string `json:"csrf_token"`
	}
	b.do("GET", "/api/v1/auth/session", nil, &info)
	if info.CsrfToken != csrf {
		t.Fatalf("session CSRF token changed: %q", info.CsrfToken)
	}

	// The WebSocket accepts the session cookie when the hello has no token.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui",
		&websocket.DialOptions{HTTPClient: b.client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = ws.CloseNow() }()
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello"}); err != nil {
		t.Fatal(err)
	}
	expectFrame(t, ctx, ws, "welcome")

	// Logout ends the session.
	if s := b.do("POST", "/api/v1/auth/logout", nil, nil); s != http.StatusNoContent {
		t.Fatalf("logout: status %d", s)
	}
	if s := b.do("GET", "/api/v1/me", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("after logout: status %d", s)
	}
}

func TestLoginRejections(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	if s := b.login("fake-agent", "whatever-password"); s != http.StatusUnauthorized {
		t.Fatalf("agents cannot log in with a password: status %d", s)
	}
	if s := b.login("nobody", "whatever-password"); s != http.StatusUnauthorized {
		t.Fatalf("unknown handle: status %d", s)
	}
	for i := 0; i < 5; i++ {
		if s := b.login("norbi", "wrong-password-123"); s != http.StatusUnauthorized {
			t.Fatalf("wrong password #%d: status %d", i+1, s)
		}
	}
	if s := b.login("norbi", server.DevAdminPassword); s != http.StatusTooManyRequests {
		t.Fatalf("after 5 failures even the right password is blocked: status %d", s)
	}
}

func TestChangePassword(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	b.login("norbi", server.DevAdminPassword)
	var me struct{ ID string }
	b.do("GET", "/api/v1/me", nil, &me)
	path := "/api/v1/actors/" + me.ID + "/password"
	if s := b.do("PUT", path, map[string]any{"current_password": "not-the-password", "new_password": "a-brand-new-password"}, nil); s != http.StatusUnauthorized {
		t.Fatalf("wrong current password: status %d", s)
	}
	if s := b.do("PUT", path, map[string]any{"current_password": server.DevAdminPassword, "new_password": "short"}, nil); s != http.StatusBadRequest {
		t.Fatalf("weak password: status %d", s)
	}
	if s := b.do("PUT", path, map[string]any{"current_password": server.DevAdminPassword, "new_password": "a-brand-new-password"}, nil); s != http.StatusNoContent {
		t.Fatalf("change: status %d", s)
	}
	if s := e.browser().login("norbi", "a-brand-new-password"); s != http.StatusOK {
		t.Fatalf("login with the new password: status %d", s)
	}
}
