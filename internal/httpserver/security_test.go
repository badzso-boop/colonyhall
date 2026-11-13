package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestAppCSPAllowsOnlyTheInlineThemeScriptByHash(t *testing.T) {
	script := "\n      (function () { document.documentElement.classList.add('x'); })();\n    "
	dist := fstest.MapFS{"index.html": {Data: []byte("<html><head><script>" + script + "</script><script type=\"module\" src=\"/app/assets/a.js\"></script></head></html>")}}
	sum := sha256.Sum256([]byte(script))
	csp := appCSP(dist)
	for _, want := range []string{
		"script-src 'self' 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'",
		"object-src 'none'", "frame-ancestors 'none'", "base-uri 'self'", "default-src 'self'",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q misses %q", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe-inline' ;") || strings.Contains(strings.Split(csp, "; ")[1], "unsafe-inline") {
		t.Fatalf("scripts must not be unsafe-inline: %s", csp)
	}
}

func TestSecurityHeadersPerArea(t *testing.T) {
	mux := http.NewServeMux()
	h := NewWithTimeouts(mux, func(context.Context) error { return nil }, slog.Default(), Timeouts{})
	for path, wantCSP := range map[string]string{
		"/app/":      "script-src 'self'",
		"/healthz":   devPageCSP,
		"/api/v1/me": apiCSP,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, wantCSP) {
			t.Errorf("%s: CSP %q, want %q", path, got, wantCSP)
		}
		for k, v := range map[string]string{"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "same-origin"} {
			if rec.Header().Get(k) != v {
				t.Errorf("%s: %s = %q", path, k, rec.Header().Get(k))
			}
		}
	}
	// Round 40: API answers are never cached; the PWA's assets may be.
	ok := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) }
	mux.HandleFunc("GET /api/v1/auth/session", ok)
	mux.HandleFunc("GET /mcp", ok)
	for path, want := range map[string]string{"/api/v1/auth/session": "no-store", "/mcp": "no-store", "/healthz": ""} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control %q, want %q", path, got, want)
		}
	}
}

// A client that stops sending its body is cut off; only a real WebSocket
// route (and the MCP stream) is long-lived.
func TestReadDeadlineSparesLongLivedConnections(t *testing.T) {
	mux := http.NewServeMux()
	readErr := make(chan error, 2)
	mux.HandleFunc("POST /api/v1/upload", func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErr <- err
	})
	h := NewWithTimeouts(mux, func(context.Context) error { return nil }, slog.Default(), Timeouts{Read: 150 * time.Millisecond, Write: time.Second})
	srv := httptest.NewServer(h)
	defer srv.Close()

	send := func(extra string) error {
		c, err := net.Dial("tcp", srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		_, _ = io.WriteString(c, "POST /api/v1/upload HTTP/1.1\r\nHost: x\r\nContent-Length: 10\r\n"+extra+"\r\n12345")
		time.Sleep(400 * time.Millisecond) // stall past the deadline, then finish
		_, _ = io.WriteString(c, "67890")
		select {
		case err := <-readErr:
			return err
		case <-time.After(3 * time.Second):
			t.Fatal("handler never finished")
			return nil
		}
	}
	if err := send(""); err == nil {
		t.Fatal("a stalled body must hit the read deadline")
	}
	// Round 40: an Upgrade header on an ordinary route does not lift the
	// deadline (a slow body would hold the connection without login).
	if err := send("Connection: Upgrade\r\nUpgrade: websocket\r\n"); err == nil {
		t.Fatal("an Upgrade header on a POST route lifted the read deadline")
	}
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{"GET", "/ws/ui", true}, {"GET", "/ws/host", true}, {"POST", "/ws/ui", false}, {"GET", "/api/v1/me", false}, {"GET", "/ws/ui/x", false},
	} {
		r := httptest.NewRequest(c.method, c.path, nil)
		r.Header.Set("Upgrade", "WebSocket")
		if got := longLived(r); got != c.want {
			t.Errorf("%s %s with Upgrade: long-lived %v, want %v", c.method, c.path, got, c.want)
		}
	}
	if !longLived(httptest.NewRequest("POST", "/mcp", nil)) {
		t.Error("the MCP stream keeps no deadline")
	}
}
