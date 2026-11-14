// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package fakeagent is a deterministic stand-in for an AI agent. It talks to
// Colonyhall over the same MCP endpoint as a real client and answers inbox
// items from a YAML scenario, so whole flows can be tested without any AI.
package fakeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// Scenario is a list of rules; the first matching rule handles an item.
type Scenario struct {
	Agent string `yaml:"agent"`
	Steps []Step `yaml:"steps"`
}

// Step reacts to an inbox item.
type Step struct {
	On    Match  `yaml:"on"`
	Reply string `yaml:"reply"` // empty: acknowledge without replying
}

// Match selects inbox items.
type Match struct {
	Kind         string `yaml:"kind"`          // "mention", "dm" or "any"
	TextContains string `yaml:"text_contains"` // case-insensitive substring; empty matches all
}

// Load reads a scenario file.
func Load(path string) (Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	var s Scenario
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return Scenario{}, fmt.Errorf("fakeagent: %s: %w", path, err)
	}
	return s, nil
}

// Connect opens an MCP session with a bearer token.
func Connect(ctx context.Context, endpoint, token string, base http.RoundTripper) (*mcp.ClientSession, error) {
	if base == nil {
		base = http.DefaultTransport
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "colonyhall-fake-agent", Version: "0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearer{token: token, base: base}},
	}
	return client.Connect(ctx, transport, nil)
}

type bearer struct {
	token string
	base  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// Agent runs a scenario over an MCP session.
type Agent struct {
	Session  *mcp.ClientSession
	Scenario Scenario
}

type inboxOutput struct {
	Total int `json:"total"`
	Items []struct {
		ID        string `json:"id"`
		Channel   string `json:"channel"`
		Body      string `json:"body"`
		MessageID string `json:"message_id"`
		Thread    string `json:"thread"`
		Mentioned bool   `json:"mentioned"`
		DM        bool   `json:"dm"`
	} `json:"items"`
}

// RunOnce reads the inbox once, answers what the scenario covers and
// acknowledges every item it saw. It returns how many items it handled.
func (a *Agent) RunOnce(ctx context.Context) (int, error) {
	res, err := a.Session.CallTool(ctx, &mcp.CallToolParams{Name: "inbox", Arguments: map[string]any{"limit": 50}})
	if err != nil {
		return 0, err
	}
	if res.IsError {
		return 0, fmt.Errorf("fakeagent: inbox: %s", contentText(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return 0, err
	}
	var inbox inboxOutput
	if err := json.Unmarshal(raw, &inbox); err != nil {
		return 0, fmt.Errorf("fakeagent: inbox output: %w", err)
	}
	var handled []string
	for _, it := range inbox.Items {
		kind := "mention"
		if it.DM {
			kind = "dm"
		}
		if step, ok := a.match(kind, it.Body); ok && step.Reply != "" {
			thread := it.Thread
			if thread == "" {
				thread = it.MessageID
			}
			// The item id as idempotency key: a retried run never posts twice.
			out, err := a.Session.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{
				"channel": it.Channel, "text": step.Reply, "thread": thread, "client_msg_id": "fake-" + it.ID,
			}})
			if err != nil {
				return len(handled), err
			}
			if out.IsError {
				return len(handled), fmt.Errorf("fakeagent: send: %s", contentText(out))
			}
		}
		handled = append(handled, it.ID)
	}
	if len(handled) == 0 {
		return 0, nil
	}
	ack, err := a.Session.CallTool(ctx, &mcp.CallToolParams{Name: "ack", Arguments: map[string]any{"ids": handled}})
	if err != nil {
		return len(handled), err
	}
	if ack.IsError {
		return len(handled), fmt.Errorf("fakeagent: ack: %s", contentText(ack))
	}
	return len(handled), nil
}

func (a *Agent) match(kind, body string) (Step, bool) {
	for _, s := range a.Scenario.Steps {
		if s.On.Kind != "" && s.On.Kind != "any" && s.On.Kind != kind {
			continue
		}
		if s.On.TextContains != "" && !strings.Contains(strings.ToLower(body), strings.ToLower(s.On.TextContains)) {
			continue
		}
		return s, true
	}
	return Step{}, false
}

func contentText(r *mcp.CallToolResult) string {
	var parts []string
	for _, c := range r.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, " ")
}
