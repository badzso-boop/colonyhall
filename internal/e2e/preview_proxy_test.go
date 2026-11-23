// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/fakeagent"
	"colonyhall/internal/server"
)

// Round 57: the full preview lifecycle through the reverse proxy on the
// preview listener. The app's own listener never proxies (its origin
// carries the person's session).
func TestPreviewProxyLifecycle(t *testing.T) {
	e := newEnvWith(t, func(o *server.Options) { o.PreviewAddr, o.PreviewAllowLoopback = ":18081", true })
	if e.app.PreviewHandler == nil {
		t.Fatal("no preview handler with PreviewAddr set")
	}
	proxy := httptest.NewServer(e.app.PreviewHandler)
	defer proxy.Close()

	var seen string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<h1>agent app</h1>")
	}))
	defer backend.Close()

	agent := e.agentID()
	general := e.generalID()
	hostID, hostToken := e.enroll("dev-box")
	e.configure(agent, map[string]any{"host_id": hostID, "client": "claude-code", "model": "haiku",
		"allow_work": true, "root_dir": "/app", "allow_preview": true})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "preview_start", Arguments: map[string]any{
		"name": "web", "compose_path": "compose.yaml", "channel_id": general}}); err != nil {
		t.Fatal(err)
	}
	var list struct {
		Previews []struct {
			ID       string `json:"id"`
			ProxyURL string `json:"proxy_url"`
		} `json:"previews"`
	}
	e.do("GET", "/api/v1/previews", server.DevAdminToken, nil, &list)
	if len(list.Previews) != 1 {
		t.Fatalf("previews: %+v", list)
	}
	id := list.Previews[0].ID
	path := "/api/v1/previews/" + id + "/proxy/"
	if !strings.HasSuffix(list.Previews[0].ProxyURL, ":18081"+path) {
		t.Fatalf("proxy_url %q", list.Previews[0].ProxyURL)
	}

	get := func(base, p string) (int, string) {
		req, _ := http.NewRequest("GET", base+p, nil)
		req.Header.Set("Authorization", "Bearer "+server.DevAdminToken)
		req.Header.Set("Accept", "text/html")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		if strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") && base == proxy.URL {
			t.Errorf("the preview listener refuses frames: %v", res.Header)
		}
		return res.StatusCode, string(b)
	}
	// Requested, not running yet: 502, the offline page.
	if s, body := get(proxy.URL, path); s != http.StatusBadGateway || !strings.Contains(body, "Preview offline") {
		t.Fatalf("requested: %d %s", s, body)
	}
	if s := e.do("POST", "/api/v1/host/previews/"+id, hostToken, map[string]any{"state": "running", "url": backend.URL}, nil); s != http.StatusNoContent {
		t.Fatalf("report running: %d", s)
	}
	if s, body := get(proxy.URL, path+"index.html"); s != http.StatusOK || body != "<h1>agent app</h1>" || seen != path+"index.html" {
		t.Fatalf("running: %d %q, backend saw %q", s, body, seen)
	}
	// The app's listener does not proxy.
	if s, body := get(e.srv.URL, path); s == http.StatusOK && strings.Contains(body, "agent app") {
		t.Fatal("the app's own origin proxied the preview")
	}
	if s := e.do("POST", "/api/v1/previews/"+id+"/stop", server.DevAdminToken, nil, nil); s != http.StatusOK {
		t.Fatalf("stop: %d", s)
	}
	if s, _ := get(proxy.URL, path); s != http.StatusBadGateway {
		t.Fatalf("stopped: %d", s)
	}
}

// Without a preview listener previews have no proxy_url and no handler.
func TestPreviewProxyDisabledByDefault(t *testing.T) {
	e := newEnv(t)
	if e.app.PreviewHandler != nil {
		t.Fatal("preview handler without PreviewAddr")
	}
}
