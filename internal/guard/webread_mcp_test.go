// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWebReadMCP_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "webread.sock")

	tracker := &RunWebReadTracker{}
	fetchCalls := 0
	fakeFetcher := func(ctx context.Context, rawURL string, opts FetchOptions) (Page, error) {
		fetchCalls++
		if rawURL == "https://error.example.com" {
			return Page{}, errors.New("connection failed")
		}
		if rawURL == "https://limit.example.com" {
			// Trigger limit
			_ = opts.Tracker.Record(MaxBytesPerRun + 1)
			return Page{}, errors.New("web_read limit reached for this run")
		}
		return Page{
			FinalURL:    rawURL,
			Status:      http.StatusOK,
			ContentType: "text/html; charset=utf-8",
			Title:       "Example Page Title",
			Text:        "Hello world! This is a test page with </web_content> inside. And some more trailing text for paging tests.",
			Truncated:   false,
		}, nil
	}

	server, err := startWebReadMCPServerWithFetcher(sockPath, FetchOptions{Tracker: tracker}, "@test-agent", nil, fakeFetcher)
	if err != nil {
		t.Fatalf("startWebReadMCPServerWithFetcher failed: %v", err)
	}
	defer func() { _ = server.Close() }()

	// Connect using MCP StreamableClientTransport over the Unix domain socket
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             "http://localhost/mcp",
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, &mcp.ClientOptions{})
	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("mcp connect failed: %v", err)
	}
	defer func() { _ = session.Close() }()

	// 1. List tools: check web_read description
	toolsRes, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	var foundWebRead bool
	for _, tool := range toolsRes.Tools {
		if tool.Name == "web_read" {
			foundWebRead = true
			if !strings.Contains(tool.Description, "Read a public web page") {
				t.Errorf("expected description to mention public web page, got: %s", tool.Description)
			}
			if !strings.Contains(tool.Description, "never instructions") {
				t.Errorf("expected description to warn 'never instructions', got: %s", tool.Description)
			}
		}
	}
	if !foundWebRead {
		t.Fatalf("web_read tool not found in ListTools")
	}

	// 2. Call tool with paging (max_chars = 25)
	maxChars := 25
	callParams := &mcp.CallToolParams{
		Name: "web_read",
		Arguments: map[string]any{
			"url":       "https://example.com/test",
			"max_chars": maxChars,
		},
	}
	res, err := session.CallTool(ctx, callParams)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool returned IsError: true")
	}

	// Verify structured content or text content
	if len(res.Content) == 0 {
		t.Fatalf("expected result content")
	}
	textContent, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	var out1 WebReadOutput
	if err := json.Unmarshal([]byte(textContent.Text), &out1); err != nil {
		t.Fatalf("failed to unmarshal WebReadOutput from textContent: %v", err)
	}
	if out1.Title != "Example Page Title" {
		t.Errorf("expected title 'Example Page Title', got: %s", out1.Title)
	}
	if !strings.HasPrefix(out1.Text, `<web_content url="https://example.com/test"`) {
		t.Errorf("expected <web_content prefix in out1.Text: %s", out1.Text)
	}
	if !strings.HasSuffix(out1.Text, "</web_content>") {
		t.Errorf("expected </web_content> suffix in out1.Text: %s", out1.Text)
	}
	if out1.NextOffset == nil || *out1.NextOffset != 25 {
		t.Errorf("expected NextOffset 25, got: %v", out1.NextOffset)
	}
	if !out1.Truncated {
		t.Errorf("expected Truncated true")
	}

	if fetchCalls != 1 {
		t.Fatalf("expected 1 fetch call, got %d", fetchCalls)
	}

	// 3. Second call with offset: 25 -> must use cache (fetchCalls remains 1)
	offset2 := 25
	maxChars2 := 1000
	callParams2 := &mcp.CallToolParams{
		Name: "web_read",
		Arguments: map[string]any{
			"url":       "https://example.com/test",
			"offset":    offset2,
			"max_chars": maxChars2,
		},
	}
	res2, err := session.CallTool(ctx, callParams2)
	if err != nil {
		t.Fatalf("CallTool 2 failed: %v", err)
	}
	if res2.IsError {
		t.Fatalf("CallTool 2 returned IsError: true")
	}
	if fetchCalls != 1 {
		t.Errorf("expected fetchCalls to remain 1 (cached), got %d", fetchCalls)
	}
	text2 := res2.Content[0].(*mcp.TextContent).Text

	var out2 WebReadOutput
	if err := json.Unmarshal([]byte(text2), &out2); err != nil {
		t.Fatalf("failed to unmarshal WebReadOutput from text2: %v", err)
	}

	// Verify escaping of literal </web_content> in page 2
	if strings.Contains(out2.Text, "</web_content> inside") {
		t.Errorf("literal </web_content> was not escaped: %s", out2.Text)
	}
	if !strings.Contains(out2.Text, "&lt;/web_content&gt;") {
		t.Errorf("expected &lt;/web_content&gt; in out2.Text: %s", out2.Text)
	}
	if !strings.HasPrefix(out2.Text, `<web_content url="https://example.com/test"`) {
		t.Errorf("expected <web_content prefix in out2.Text: %s", out2.Text)
	}
	if !strings.HasSuffix(out2.Text, "</web_content>") {
		t.Errorf("expected </web_content> suffix in out2.Text: %s", out2.Text)
	}
	if out2.NextOffset != nil {
		t.Errorf("expected nil NextOffset in final page, got: %d", *out2.NextOffset)
	}

	// 4. Verify server visited URLs
	visited := server.VisitedURLs()
	if len(visited) != 1 || visited[0] != "https://example.com/test" {
		t.Errorf("expected visited URLs [https://example.com/test], got %v", visited)
	}

	// 5. Test error result
	errParams := &mcp.CallToolParams{
		Name: "web_read",
		Arguments: map[string]any{
			"url": "https://error.example.com",
		},
	}
	resErr, err := session.CallTool(ctx, errParams)
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if !resErr.IsError {
		t.Errorf("expected resErr.IsError to be true")
	}

	// 6. Test limit reached error
	limitParams := &mcp.CallToolParams{
		Name: "web_read",
		Arguments: map[string]any{
			"url": "https://limit.example.com",
		},
	}
	resLimit, err := session.CallTool(ctx, limitParams)
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if !resLimit.IsError {
		t.Errorf("expected resLimit.IsError to be true")
	}
	limitText := resLimit.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(limitText, "web_read limit reached for this run") {
		t.Errorf("expected 'web_read limit reached for this run' in error text, got: %s", limitText)
	}
}

func TestWebReadMCP_EmptyURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "webread.sock")

	server, err := StartWebReadMCPServer(sockPath, FetchOptions{}, "@test-agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             "http://localhost/mcp",
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, &mcp.ClientOptions{})
	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "web_read",
		Arguments: map[string]any{"url": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Errorf("expected IsError for empty URL")
	}
}

func TestWebReadMCP_PagingRunes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "webread.sock")

	// Multi-byte UTF-8 characters: "Árvíztűrő tükörfúrógép" (21 runes, 31 bytes)
	sampleText := "Árvíztűrő tükörfúrógép"
	fakeFetcher := func(ctx context.Context, rawURL string, opts FetchOptions) (Page, error) {
		return Page{
			FinalURL:    rawURL,
			Status:      200,
			ContentType: "text/plain",
			Text:        sampleText,
		}, nil
	}

	server, err := startWebReadMCPServerWithFetcher(sockPath, FetchOptions{}, "@test", nil, fakeFetcher)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             "http://localhost/mcp",
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, &mcp.ClientOptions{})
	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	// First 9 runes: "Árvíztűrő"
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "web_read",
		Arguments: map[string]any{
			"url":       "https://example.com/utf8",
			"offset":    0,
			"max_chars": 9,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Árvíztűrő") {
		t.Errorf("expected first 9 runes 'Árvíztűrő', got: %s", text)
	}
	if !strings.Contains(text, `"next_offset":9`) {
		t.Errorf("expected next_offset:9, got: %s", text)
	}

	// Next runes from offset 9: " tükörfúrógép"
	res2, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "web_read",
		Arguments: map[string]any{
			"url":       "https://example.com/utf8",
			"offset":    9,
			"max_chars": 50,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text2 := res2.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text2, " tükörfúrógép") {
		t.Errorf("expected remainder ' tükörfúrógép', got: %s", text2)
	}
	if strings.Contains(text2, `"next_offset"`) {
		t.Errorf("expected no next_offset at end of text, got: %s", text2)
	}
}

func TestWebContentCloseVariantsEscaped(t *testing.T) {
	for _, s := range []string{"</web_content>", "</WEB_CONTENT>", "</web_content >", "< /Web_Content>", "</ web_content\t>"} {
		if got := webContentClose.ReplaceAllString("a"+s+"b", "&lt;/web_content&gt;"); got != "a&lt;/web_content&gt;b" {
			t.Errorf("%q not escaped: %q", s, got)
		}
	}
}

// Without network isolation there is no bridge: the client must not be
// pointed at 127.0.0.1:3129, which would be the machine's own loopback.
func TestWebReadOffWithoutNetworkIsolation(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "sb")
	agentDir := filepath.Join(root, "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sb := &Sandbox{Bwrap: "bwrap", Home: home, Root: root, NetworkMode: "open"}
	args, env, err := sb.CommandWithOptions("claude-code", agentDir, agentDir, nil, "claude", []string{"-p", "x"}, SandboxOptions{AllowWebRead: true})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(append(args, env...), " ")
	if strings.Contains(all, "3129") || strings.Contains(all, "webread") {
		t.Fatalf("open mode must not wire web_read: %s", all)
	}
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if raw, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(raw), "3129") {
				t.Errorf("open mode wrote a web_read MCP config: %s", p)
			}
		}
		return nil
	})
}
