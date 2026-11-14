// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package mcptools exposes git operations as MCP tools for agents.
package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/git"
	"colonyhall/internal/modules/identity"
)

// StatusInput is the input for the git_status tool.
type StatusInput struct {
	Ref string `json:"ref" jsonschema:"the PR/issue URL or shorthand e.g. owner/repo#12 or owner/repo@sha"`
}

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// Register adds the git MCP tools to the MCP server.
func Register(s *mcp.Server, svc *git.Service, current CurrentActor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "git_status",
		Description: "Check the status of a git pull request, issue, repository, or commit. " +
			"Accepts a full URL or shorthand like owner/repo#12 or owner/repo@sha. " +
			"Returns a compact single-line summary with title, state, and author.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in StatusInput) (*mcp.CallToolResult, any, error) {
		a, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		status, err := svc.Status(ctx, a.WorkspaceID, in.Ref)
		if err != nil {
			return nil, nil, err
		}
		return text(status), nil, nil
	})
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
