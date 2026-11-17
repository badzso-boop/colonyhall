// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/workshop"
)

// RegisterCore adds Colonyhall's own tools for an actor (the server's).
type RegisterCore func(s *mcp.Server, actor identity.Actor, r *http.Request)

// CoreCaller lets an extension's code call Colonyhall's own tools
// (ctx.colony.<tool>) as the calling actor: a fresh in-memory MCP session
// with exactly the tools and checks an agent gets, never the Workshop's
// own tools (no loops through the gateway).
func CoreCaller(version string, register RegisterCore) workshop.CoreTool {
	return func(ctx context.Context, as identity.Actor, tool string, input json.RawMessage) (string, error) {
		srv := mcp.NewServer(&mcp.Implementation{Name: "colonyhall-core", Version: version}, nil)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp", nil)
		if err != nil {
			return "", err
		}
		register(srv, as, req)
		ct, st := mcp.NewInMemoryTransports()
		ss, err := srv.Connect(ctx, st, nil)
		if err != nil {
			return "", err
		}
		defer func() { _ = ss.Close() }()
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "colonyhall-extension", Version: version}, nil).Connect(ctx, ct, nil)
		if err != nil {
			return "", err
		}
		defer func() { _ = cs.Close() }()
		var args any = map[string]any{}
		if len(input) > 0 && string(input) != "null" {
			if err := json.Unmarshal(input, &args); err != nil {
				return "", errors.New("the tool's input must be an object")
			}
		}
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, c := range res.Content {
			if t, ok := c.(*mcp.TextContent); ok {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(t.Text)
			}
		}
		if res.IsError {
			return "", errors.New(b.String())
		}
		return b.String(), nil
	}
}
