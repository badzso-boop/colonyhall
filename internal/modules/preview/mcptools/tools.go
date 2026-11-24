// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package mcptools exposes preview tools for AI agents over MCP.
package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/preview"
	"colonyhall/internal/platform/ids"
)

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// StartInput is the input schema for preview_start.
type StartInput struct {
	Name         string `json:"name" jsonschema:"preview name (alphanumeric/hyphens, 1-50 chars, e.g. web)"`
	ComposePath  string `json:"compose_path" jsonschema:"relative path to the compose file (e.g. compose.yaml or docker-compose.yml)"`
	TTLMinutes   *int   `json:"ttl_minutes,omitempty" jsonschema:"lifetime in minutes (default 240, max 1440)"`
	ChannelID    string `json:"channel_id,omitempty" jsonschema:"channel to announce the preview in (optional, defaults to current thread)"`
	ThreadRootID string `json:"thread_root_id,omitempty" jsonschema:"thread root message id (optional, defaults to current thread)"`
}

// StopInput is the input schema for preview_stop.
type StopInput struct {
	Name string `json:"name" jsonschema:"name of the active preview to stop"`
}

// StartResult is the output of preview_start.
type StartResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ComposePath string `json:"compose_path"`
	State       string `json:"state"`
	ExpiresAt   string `json:"expires_at"`
	Message     string `json:"message"`
}

// StopResult is the output of preview_stop.
type StopResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// Register adds preview tools to the MCP server.
func Register(s *mcp.Server, svc *preview.Service, current CurrentActor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "preview_start",
		Description: "Start a preview application defined in a docker compose file. " +
			"The host machine will validate the compose file, start containers with safety overrides, " +
			"and share a link in the chat thread once ready; people open it inside Colonyhall, next to the conversation. " +
			preview.TemplateGuide,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StartInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		var chStr, thStr string
		if req != nil && req.Extra != nil && req.Extra.Header != nil {
			chStr = req.Extra.Header.Get("X-Colonyhall-Channel")
			thStr = req.Extra.Header.Get("X-Colonyhall-Thread")
		}
		if chStr == "" {
			chStr = in.ChannelID
		}
		if thStr == "" {
			thStr = in.ThreadRootID
		}

		var chID, thID ids.ID
		if chStr != "" {
			var err error
			chID, err = ids.Parse(chStr)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid channel ID: %w", err)
			}
		}
		if thStr != "" {
			var err error
			thID, err = ids.Parse(thStr)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid thread root ID: %w", err)
			}
		}

		ttl := preview.DefaultTTL
		if in.TTLMinutes != nil && *in.TTLMinutes > 0 {
			ttl = time.Duration(*in.TTLMinutes) * time.Minute
		}

		p, err := svc.Start(ctx, actor, preview.StartInput{
			Name:         in.Name,
			ComposePath:  in.ComposePath,
			TTL:          ttl,
			ChannelID:    chID,
			ThreadRootID: thID,
		})
		if err != nil {
			return nil, nil, err
		}

		return nil, StartResult{
			ID:          p.ID.String(),
			Name:        p.Name,
			ComposePath: p.ComposePath,
			State:       p.State,
			ExpiresAt:   p.ExpiresAt.Format(time.RFC3339),
			Message:     fmt.Sprintf("Preview '%s' requested. The host will start it and post the URL here.", p.Name),
		}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "preview_stop",
		Description: "Stop a running preview application.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StopInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		p, err := svc.Stop(ctx, actor, in.Name)
		if err != nil {
			return nil, nil, err
		}
		return nil, StopResult{
			ID:      p.ID.String(),
			Name:    p.Name,
			State:   p.State,
			Message: fmt.Sprintf("Preview '%s' stopped.", p.Name),
		}, nil
	})
}
