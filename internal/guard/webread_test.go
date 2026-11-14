// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type testMockResolver struct {
	mapping map[string][]net.IP
}

func (m *testMockResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if ips, ok := m.mapping[host]; ok {
		var res []net.IPAddr
		for _, ip := range ips {
			res = append(res, net.IPAddr{IP: ip})
		}
		return res, nil
	}
	// Fallback for IP strings or unknown
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	return nil, fmt.Errorf("unknown host: %s", host)
}

func setupTestServer(t *testing.T, handler http.Handler) (*httptest.Server, *testMockResolver) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	u, _ := url.Parse(ts.URL)
	port := u.Port()

	res := &testMockResolver{
		mapping: map[string][]net.IP{
			"example.test":     {net.ParseIP("93.184.216.34")},
			"sub.example.test": {net.ParseIP("93.184.216.35")},
			"other.test":       {net.ParseIP("93.184.216.36")},
			"private.test":     {net.ParseIP("192.168.1.1")},
			"loopback.test":    {net.ParseIP("127.0.0.1")},
		},
	}

	_ = port
	return ts, res
}

func TestWebRead_HTMLToText(t *testing.T) {
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>My Sample Page</title>
    <style>body { color: red; }</style>
    <script>alert("evil");</script>
</head>
<body>
    <nav><a href="/nav">Nav Link</a></nav>
    <h1>Main Heading</h1>
    <p>This is a paragraph with <a href="/docs/guide">a guide link</a> and <b>bold text</b>.</p>
    <h2>Subheading</h2>
    <ul>
        <li>First item</li>
        <li>Second item</li>
    </ul>
    <pre><code>func Hello() {
    println("world")
}</code></pre>
    <p>Inline <code>variable_name</code> here.</p>
    <table>
        <tr><th>Name</th><th>Role</th></tr>
        <tr><td>Alice</td><td>Engineer</td></tr>
        <tr><td>Bob</td><td>Designer</td></tr>
    </table>
    <form><input type="text" name="secret"/></form>
    <footer>Copyright 2026</footer>
</body>
</html>`

	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlContent))
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}

	u, _ := url.Parse(ts.URL)
	port := u.Port()

	targetURL := fmt.Sprintf("http://example.test:%s/my/page", port)
	page, err := Fetch(context.Background(), targetURL, FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if page.Title != "My Sample Page" {
		t.Errorf("expected title 'My Sample Page', got %q", page.Title)
	}
	if page.Status != 200 {
		t.Errorf("expected status 200, got %d", page.Status)
	}
	if page.ContentType != "text/html" {
		t.Errorf("expected content-type text/html, got %q", page.ContentType)
	}

	text := page.Text
	// Headings
	if !strings.Contains(text, "# Main Heading") {
		t.Errorf("missing H1 in text: %s", text)
	}
	if !strings.Contains(text, "## Subheading") {
		t.Errorf("missing H2 in text: %s", text)
	}
	// Absolute link: /docs/guide relative to http://example.test:port/my/page -> http://example.test:port/docs/guide
	expectedLink := fmt.Sprintf("a guide link (http://example.test:%s/docs/guide)", port)
	if !strings.Contains(text, expectedLink) {
		t.Errorf("expected absolute link %q in text, got: %s", expectedLink, text)
	}
	// Lists
	if !strings.Contains(text, "- First item") || !strings.Contains(text, "- Second item") {
		t.Errorf("missing list items in text: %s", text)
	}
	// Fenced code block
	if !strings.Contains(text, "```\nfunc Hello() {\n    println(\"world\")\n}\n```") {
		t.Errorf("missing fenced code block in text: %s", text)
	}
	// Inline code
	if !strings.Contains(text, "`variable_name`") {
		t.Errorf("missing inline code in text: %s", text)
	}
	// Table
	if !strings.Contains(text, "| Name | Role |") || !strings.Contains(text, "| Alice | Engineer |") {
		t.Errorf("missing table in text: %s", text)
	}
	// Dropped elements
	if strings.Contains(text, "alert(\"evil\")") {
		t.Errorf("script was not dropped: %s", text)
	}
	if strings.Contains(text, "color: red") {
		t.Errorf("style was not dropped: %s", text)
	}
	if strings.Contains(text, "Nav Link") {
		t.Errorf("nav was not dropped: %s", text)
	}
	if strings.Contains(text, "Copyright 2026") {
		t.Errorf("footer was not dropped: %s", text)
	}
	if strings.Contains(text, "secret") {
		t.Errorf("form was not dropped: %s", text)
	}
}

func TestWebRead_PlainTextAndJSON(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plain":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("plain text content line 1\nline 2"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"hello": "world", "count": 42}`))
		}
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	// 1. Plain text
	p1, err := Fetch(context.Background(), fmt.Sprintf("http://example.test:%s/plain", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p1.ContentType != "text/plain" || p1.Text != "plain text content line 1\nline 2" {
		t.Fatalf("unexpected plain text result: %+v", p1)
	}

	// 2. JSON
	p2, err := Fetch(context.Background(), fmt.Sprintf("http://example.test:%s/json", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p2.ContentType != "application/json" || !strings.Contains(p2.Text, `"hello": "world"`) {
		t.Fatalf("unexpected json result: %+v", p2)
	}
}

func TestWebRead_BinaryNote(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		// Send 1.2 MB of dummy bytes
		size := int(1258291)
		_, _ = w.Write(make([]byte, size))
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	page, err := Fetch(context.Background(), fmt.Sprintf("http://example.test:%s/file.pdf", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.ContentType != "application/pdf" {
		t.Errorf("expected application/pdf, got %s", page.ContentType)
	}
	expectedNote := "not a text page: application/pdf, 1.2 MB"
	if page.Text != expectedNote {
		t.Fatalf("expected %q, got %q", expectedNote, page.Text)
	}
}

func TestWebRead_SizeCapAndTruncated(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Write 6 MB (> 5 MB limit)
		chunk := strings.Repeat("a", 1024*1024)
		for i := 0; i < 6; i++ {
			_, _ = w.Write([]byte(chunk))
		}
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	page, err := Fetch(context.Background(), fmt.Sprintf("http://example.test:%s/huge.txt", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Truncated {
		t.Fatal("expected Truncated=true for >5MB page")
	}
	if len(page.Text) != MaxBodyBytes {
		t.Fatalf("expected text length %d, got %d", MaxBodyBytes, len(page.Text))
	}
}

func TestWebRead_RedirectToPrivateRefused(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to private IP host
		http.Redirect(w, r, "http://private.test/secret", http.StatusFound)
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	_, err := Fetch(context.Background(), fmt.Sprintf("http://example.test:%s/start", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err == nil {
		t.Fatal("expected error on redirect to private host, got nil")
	}
	if !strings.Contains(err.Error(), "blocked address") && !strings.Contains(err.Error(), "private") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestWebRead_RedirectChainOver5Refused(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step := r.URL.Query().Get("step")
		n := 0
		if step != "" {
			_, _ = fmt.Sscanf(step, "%d", &n)
		}
		http.Redirect(w, r, fmt.Sprintf("/redir?step=%d", n+1), http.StatusFound)
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	_, err := Fetch(context.Background(), fmt.Sprintf("http://example.test:%s/redir?step=0", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err == nil {
		t.Fatal("expected error on redirect chain > 5, got nil")
	}
	if !strings.Contains(err.Error(), "stopped after 5 redirects") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestWebRead_Refusals(t *testing.T) {
	res := &testMockResolver{
		mapping: map[string][]net.IP{
			"example.test": {net.ParseIP("93.184.216.34")},
		},
	}

	ctx := context.Background()

	// 1. IP literal refused
	if _, err := Fetch(ctx, "http://93.184.216.34/path", FetchOptions{Resolver: res}); err == nil {
		t.Fatal("expected error for IP literal")
	}
	if _, err := Fetch(ctx, "http://127.0.0.1/path", FetchOptions{Resolver: res}); err == nil {
		t.Fatal("expected error for loopback IP literal")
	}

	// 2. user:pass@ refused
	if _, err := Fetch(ctx, "http://user:pass@example.test/path", FetchOptions{Resolver: res}); err == nil {
		t.Fatal("expected error for user:pass@")
	}

	// 3. non-http schemes refused
	for _, scheme := range []string{"file:///etc/passwd", "ftp://example.test/file", "gopher://example.test/"} {
		if _, err := Fetch(ctx, scheme, FetchOptions{Resolver: res}); err == nil {
			t.Fatalf("expected error for scheme %s", scheme)
		}
	}

	// 4. URL over 2048 chars refused
	longURL := "http://example.test/" + strings.Repeat("a", 2040)
	if _, err := Fetch(ctx, longURL, FetchOptions{Resolver: res}); err == nil {
		t.Fatal("expected error for url > 2048 chars")
	}
}

func TestWebRead_AllowAndDenyLists(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	ctx := context.Background()

	// 1. Deny list match
	_, err := Fetch(ctx, fmt.Sprintf("http://example.test:%s/", port), FetchOptions{
		Deny:        []string{"example.test"},
		Resolver:    res,
		DialContext: dialContext,
	})
	if err == nil || !strings.Contains(err.Error(), "denied by configuration") {
		t.Fatalf("expected denied by configuration error, got: %v", err)
	}

	// 2. Wildcard deny list (*.example.test matches sub.example.test)
	_, err = Fetch(ctx, fmt.Sprintf("http://sub.example.test:%s/", port), FetchOptions{
		Deny:        []string{"*.example.test"},
		Resolver:    res,
		DialContext: dialContext,
	})
	if err == nil || !strings.Contains(err.Error(), "denied by configuration") {
		t.Fatalf("expected wildcard deny error, got: %v", err)
	}

	// 3. Allow list set, but host not in it
	_, err = Fetch(ctx, fmt.Sprintf("http://other.test:%s/", port), FetchOptions{
		Allow:       []string{"example.test"},
		Resolver:    res,
		DialContext: dialContext,
	})
	if err == nil || !strings.Contains(err.Error(), "not in the allowlist") {
		t.Fatalf("expected not in allowlist error, got: %v", err)
	}

	// 4. Allow list set and matches wildcard
	p, err := Fetch(ctx, fmt.Sprintf("http://sub.example.test:%s/", port), FetchOptions{
		Allow:       []string{"*.example.test"},
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatalf("expected wildcard allow to succeed, got: %v", err)
	}
	if p.Text != "ok" {
		t.Fatalf("expected 'ok', got %q", p.Text)
	}
}

func TestWebRead_PerRunLimits(t *testing.T) {
	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	tracker := &RunWebReadTracker{}
	ctx := context.Background()

	// Simulate 30 fetches
	for i := 0; i < 30; i++ {
		_, err := Fetch(ctx, fmt.Sprintf("http://example.test:%s/", port), FetchOptions{
			Resolver:    res,
			DialContext: dialContext,
			Tracker:     tracker,
		})
		if err != nil {
			t.Fatalf("fetch %d failed: %v", i, err)
		}
	}

	// 31st fetch must fail with per-run limit error
	_, err := Fetch(ctx, fmt.Sprintf("http://example.test:%s/", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
		Tracker:     tracker,
	})
	if err == nil || !strings.Contains(err.Error(), "web_read limit reached for this run") {
		t.Fatalf("expected per-run limit reached error, got: %v", err)
	}
}

func TestWebRead_CharsetHandling(t *testing.T) {
	// ISO-8859-1 encoded text with non-ASCII accented characters
	// In ISO-8859-1: 'é' is byte 0xe9, 'á' is 0xe1
	isoBytes := []byte{0x4b, 0x61, 0x76, 0xe1, 0x7a, 0x20, 0xe9, 0x73, 0x20, 0x74, 0x65, 0x61} // "Kaváz és tea"

	ts, res := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=iso-8859-1")
		_, _ = w.Write(isoBytes)
	}))

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("tcp", ts.Listener.Addr().String())
	}
	u, _ := url.Parse(ts.URL)
	port := u.Port()

	page, err := Fetch(context.Background(), fmt.Sprintf("http://example.test:%s/iso", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Text, "Kaváz és tea") {
		t.Fatalf("expected charset decoded text 'Kaváz és tea', got %q", page.Text)
	}
}

type rebindingResolver struct {
	callCount int
}

func (r *rebindingResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	r.callCount++
	if r.callCount == 1 {
		// First answer: public IP
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	// Second answer: attempted rebind to private loopback IP!
	return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
}

func TestWebRead_DNSRebinding(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("rebind-safe"))
	}))
	t.Cleanup(ts.Close)

	u, _ := url.Parse(ts.URL)
	port := u.Port()

	res := &rebindingResolver{}
	var dialedAddr string
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialedAddr = addr
		return net.Dial("tcp", ts.Listener.Addr().String())
	}

	page, err := Fetch(context.Background(), fmt.Sprintf("http://rebind.test:%s/", port), FetchOptions{
		Resolver:    res,
		DialContext: dialContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Text != "rebind-safe" {
		t.Fatalf("expected 'rebind-safe', got %q", page.Text)
	}
	// Verify that the dialer dialed the first resolved IP (93.184.216.34), NOT the rebound 127.0.0.1
	expectedDial := fmt.Sprintf("93.184.216.34:%s", port)
	if dialedAddr != expectedDial {
		t.Fatalf("expected dialed target to be pinned to first answer %s, got: %s", expectedDial, dialedAddr)
	}
}
