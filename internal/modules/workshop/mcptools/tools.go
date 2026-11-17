// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package mcptools puts the Workshop's tools into an agent's MCP session:
// the connector tools the agent has a grant for, next to the core tools
// (the gateway, docs/25-onfejlesztes-terv.md 13.3).
package mcptools

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/ids"
)

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// safeSchema returns the tool's input schema if it is a JSON object of
// type "object", or the empty object schema: an external server's schema
// must never make registration fail.
func safeSchema(raw json.RawMessage) json.RawMessage {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m["type"] != "object" {
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}

// Register adds the agent's connector tools. Errors only skip tools: the
// core tools must always work.
func Register(s *mcp.Server, svc *workshop.Service, actor identity.Actor, current CurrentActor, r *http.Request, logger *slog.Logger) {
	if svc == nil || actor.Kind != identity.KindAgent {
		return
	}
	tools, err := svc.ToolsFor(r.Context(), actor)
	if err != nil {
		if logger != nil {
			logger.Warn("workshop: tools for agent", "agent_id", actor.ID, "err", err)
		}
		return
	}
	for _, t := range tools {
		name := t.Name
		add(s, &mcp.Tool{Name: name, Description: t.Description, InputSchema: safeSchema(t.InputSchema),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: t.Risk == workshop.RiskRead}},
			func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				in := workshop.CallInput{Name: name, Current: current}
				if req != nil && req.Params != nil {
					in.Args = req.Params.Arguments
				}
				h := r.Header
				if req != nil && req.Extra != nil && req.Extra.Header != nil && req.Extra.Header.Get("X-Colonyhall-Channel") != "" {
					h = req.Extra.Header
				}
				if id, err := ids.Parse(h.Get("X-Colonyhall-Channel")); err == nil {
					in.ChannelID = id
				}
				if id, err := ids.Parse(h.Get("X-Colonyhall-Thread")); err == nil {
					in.ThreadRootID = id
				}
				res, err := svc.Call(ctx, in)
				if err != nil {
					// A tool error the model can read and act on, not a
					// protocol error.
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: res.Text}}, IsError: res.IsError}, nil
			}, logger)
	}
}

func add(s *mcp.Server, t *mcp.Tool, h mcp.ToolHandler, logger *slog.Logger) {
	defer func() {
		if p := recover(); p != nil && logger != nil {
			logger.Warn("workshop: tool skipped", "tool", t.Name, "reason", p)
		}
	}()
	s.AddTool(t, h)
}
