// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package mcptools exposes wiki operations as MCP tools for agents.
package mcptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/wiki"
)

// IndexInput is the input for the wiki_index tool.
type IndexInput struct {
	Query string `json:"q,omitempty" jsonschema:"optional search term to filter pages by title, slug, tags, or content"`
}

// ReadInput is the input for the wiki_read tool.
type ReadInput struct {
	Slug string `json:"slug" jsonschema:"the slug of the wiki page to read"`
}

// CreateInput is the input for the wiki_create tool.
type CreateInput struct {
	Title string `json:"title" jsonschema:"page title, max 120 characters"`
	Body  string `json:"body" jsonschema:"markdown content of the page, max 50000 characters"`
	Tags  string `json:"tags,omitempty" jsonschema:"optional comma-separated tags, e.g. 'architecture, backend'"`
}

// AppendInput is the input for the wiki_append tool.
type AppendInput struct {
	Slug string `json:"slug" jsonschema:"slug of the wiki page to append to"`
	Text string `json:"text" jsonschema:"markdown text to append, max 8000 characters"`
}

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// Register adds the wiki MCP tools to the MCP server.
func Register(s *mcp.Server, svc *wiki.Service, current CurrentActor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "wiki_index",
		Description: "Search or list the workspace wiki pages. Pass an optional query 'q' to search by " +
			"title, slug, tags, or content.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in IndexInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		pages, err := svc.List(ctx, actor, in.Query, false)
		if err != nil {
			return nil, nil, err
		}
		if len(pages) == 0 {
			return text("no wiki pages found"), nil, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d wiki page(s):", len(pages))
		for _, p := range pages {
			tagsStr := ""
			if len(p.Tags) > 0 {
				tagsStr = " [" + strings.Join(p.Tags, ", ") + "]"
			}
			fmt.Fprintf(&b, "\n- %s (%s)%s: %s", p.Slug, p.Title, tagsStr, p.Excerpt)
		}
		return text(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wiki_read",
		Description: "Read the markdown content and metadata of a wiki page by its slug.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		page, err := svc.Get(ctx, actor, in.Slug)
		if err != nil {
			return nil, nil, err
		}
		body := page.Body
		runes := []rune(body)
		if len(runes) > 8000 {
			body = string(runes[:8000]) + "\n\n[TRUNCATED: page exceeds 8000 characters. Ask a human if you need more.]"
		}
		tagsStr := ""
		if len(page.Tags) > 0 {
			tagsStr = strings.Join(page.Tags, ", ")
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# %s\nSlug: %s | Rev: %d | Updated: %s", page.Title, page.Slug, page.Rev, page.UpdatedAt.Format("2006-01-02 15:04"))
		if tagsStr != "" {
			fmt.Fprintf(&b, " | Tags: %s", tagsStr)
		}
		fmt.Fprintf(&b, "\n\n%s", body)
		return text(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "wiki_create",
		Description: "Create a new wiki page with a title and initial markdown body. " +
			"Use for persistent, workspace-wide documentation.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		var tags []string
		if in.Tags != "" {
			for _, part := range strings.Split(in.Tags, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					tags = append(tags, part)
				}
			}
		}
		page, err := svc.Create(ctx, actor, wiki.CreateInput{
			Title: in.Title,
			Body:  in.Body,
			Tags:  tags,
		})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("created wiki page %q (slug: %s, rev: %d)", page.Title, page.Slug, page.Rev)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "wiki_append",
		Description: "Append a section or note to an existing wiki page. " +
			"Your note will automatically include your handle and timestamp header.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AppendInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		page, err := svc.Append(ctx, actor, in.Slug, in.Text)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("appended to wiki page %q (slug: %s, rev: %d)", page.Title, page.Slug, page.Rev)), nil, nil
	})
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
