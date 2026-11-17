// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package mcpremote is the gateway's client for external MCP servers
// (streamable HTTP, the official go-sdk). Every request goes through the
// egress client, so only allowed addresses are reached; the connector's
// secret is put on the request here and nowhere else.
package mcpremote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/egress"
)

// Client implements workshop.Remote.
type Client struct {
	Policy egress.Policy
	// HTTP replaces the egress client (tests only).
	HTTP *http.Client
	// Version is sent as the client's version.
	Version string
}

type withHeader struct {
	header map[string]string
	next   http.RoundTripper
}

func (w withHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range w.header {
		r.Header.Set(k, v)
	}
	return w.next.RoundTrip(r)
}

func (c *Client) connect(ctx context.Context, ep workshop.Endpoint) (*mcp.ClientSession, error) {
	base := c.HTTP
	if base == nil {
		base = c.Policy.Client(egress.Options{Timeout: workshop.CallTimeout})
	}
	next := base.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	hc := &http.Client{Transport: withHeader{header: ep.Header, next: next}, Timeout: base.Timeout,
		CheckRedirect: base.CheckRedirect}
	client := mcp.NewClient(&mcp.Implementation{Name: "colonyhall-gateway", Version: c.Version}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ep.URL, HTTPClient: hc, MaxRetries: -1}, nil)
}

// RiskOf maps a tool's annotations to a risk: read-only → read; open
// world → external; anything else (or no annotations) → write.
func RiskOf(a *mcp.ToolAnnotations) string {
	switch {
	case a == nil:
		return workshop.RiskWrite
	case a.ReadOnlyHint:
		return workshop.RiskRead
	case a.OpenWorldHint != nil && *a.OpenWorldHint:
		return workshop.RiskExternal
	}
	return workshop.RiskWrite
}

// ListTools implements workshop.Remote.
func (c *Client) ListTools(ctx context.Context, ep workshop.Endpoint) ([]workshop.ToolInfo, error) {
	s, err := c.connect(ctx, ep)
	if err != nil {
		return nil, clean(err)
	}
	defer func() { _ = s.Close() }()
	var out []workshop.ToolInfo
	for t, err := range s.Tools(ctx, nil) {
		if err != nil {
			return nil, clean(err)
		}
		if len(out) >= workshop.MaxTools {
			break
		}
		schema, _ := json.Marshal(t.InputSchema)
		desc := t.Description
		if len(desc) > 1000 {
			desc = desc[:1000]
		}
		out = append(out, workshop.ToolInfo{Name: t.Name, Description: desc, InputSchema: schema, Risk: RiskOf(t.Annotations)})
	}
	return out, nil
}

// CallTool implements workshop.Remote.
func (c *Client) CallTool(ctx context.Context, ep workshop.Endpoint, name string, args json.RawMessage) (workshop.Result, error) {
	s, err := c.connect(ctx, ep)
	if err != nil {
		return workshop.Result{}, clean(err)
	}
	defer func() { _ = s.Close() }()
	var a any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return workshop.Result{}, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: a})
	if err != nil {
		return workshop.Result{}, clean(err)
	}
	var b strings.Builder
	for _, part := range res.Content {
		switch p := part.(type) {
		case *mcp.TextContent:
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Text)
		default:
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString("[non-text content omitted]")
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		j, _ := json.Marshal(res.StructuredContent)
		b.Write(j)
	}
	return workshop.Result{Text: b.String(), IsError: res.IsError}, nil
}

// clean keeps errors short and free of URLs (a URL may carry a token).
func clean(err error) error {
	if errors.Is(err, egress.ErrAddress) || errors.Is(err, egress.ErrScheme) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	msg := err.Error()
	if strings.Contains(msg, "://") {
		return errors.New("the connector's server could not be reached")
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return errors.New(msg)
}
