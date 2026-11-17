// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package mcpremote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/egress"
)

type searchIn struct {
	Q string `json:"q"`
}

// fakeServer is an external MCP server with one read tool and one write
// tool; it records the Authorization headers it saw.
func fakeServer(t *testing.T) (string, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "fake-jira"}, nil)
		mcp.AddTool(s, &mcp.Tool{Name: "search", Description: "Search issues", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
			func(_ context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "found: " + in.Q}}}, nil, nil
			})
		yes := true
		mcp.AddTool(s, &mcp.Tool{Name: "post", Description: "Post outside", Annotations: &mcp.ToolAnnotations{OpenWorldHint: &yes}},
			func(_ context.Context, _ *mcp.CallToolRequest, _ searchIn) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
			})
		mcp.AddTool(s, &mcp.Tool{Name: "edit", Description: "Edit"},
			func(_ context.Context, _ *mcp.CallToolRequest, _ searchIn) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
			})
		return s
	}, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/mcp", &seen
}

func TestListAndCallThroughEgress(t *testing.T) {
	url, seen := fakeServer(t)
	pol, _ := egress.ParsePolicy("127.0.0.1")
	c := &Client{Policy: pol}
	ep := workshop.Endpoint{URL: url, Header: map[string]string{"Authorization": "Bearer s3cret"}}
	tools, err := c.ListTools(context.Background(), ep)
	if err != nil {
		t.Fatal(err)
	}
	risks := map[string]string{}
	for _, tl := range tools {
		risks[tl.Name] = tl.Risk
	}
	if risks["search"] != workshop.RiskRead || risks["post"] != workshop.RiskExternal || risks["edit"] != workshop.RiskWrite {
		t.Fatalf("risks: %v", risks)
	}
	res, err := c.CallTool(context.Background(), ep, "search", json.RawMessage(`{"q":"bug"}`))
	if err != nil || res.Text != "found: bug" {
		t.Fatalf("call: %+v %v", res, err)
	}
	for _, h := range *seen {
		if h != "Bearer s3cret" {
			t.Fatalf("a request without the secret: %q", h)
		}
	}
}

// Without the operator's allow rule the loopback server is not reached.
func TestInternalAddressRefused(t *testing.T) {
	url, seen := fakeServer(t)
	c := &Client{}
	if _, err := c.ListTools(context.Background(), workshop.Endpoint{URL: url}); err == nil {
		t.Fatal("reached a loopback MCP server without an allow rule")
	}
	if len(*seen) != 0 {
		t.Fatalf("the server saw %d requests", len(*seen))
	}
}

// A redirect to another host never carries the connector's secret.
func TestSecretNotSentAcrossRedirect(t *testing.T) {
	var got []string
	var mu sync.Mutex
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Get("Authorization"))
		mu.Unlock()
	}))
	t.Cleanup(other.Close)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer s3cret" {
			t.Errorf("the endpoint itself did not get the secret")
		}
		http.Redirect(w, r, other.URL+"/elsewhere", http.StatusFound)
	}))
	t.Cleanup(first.Close)
	c := &http.Client{Transport: withHeader{header: map[string]string{"Authorization": "Bearer s3cret"},
		origin: "http://" + first.Listener.Addr().String(), next: http.DefaultTransport}}
	res, err := c.Get(first.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("the other host saw %q", got)
	}
}
