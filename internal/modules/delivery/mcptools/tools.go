// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package mcptools exposes the agent inbox as MCP tools.
package mcptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// maxBodyInList keeps one inbox line from flooding the agent's context.
const maxBodyInList = 1000

// InboxInput is the input of the inbox tool.
type InboxInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum items to return (default 20, max 100)"`
}

// InboxOutput is the structured result of the inbox tool, for programmatic
// clients (the fake agent); models read the text content.
type InboxOutput struct {
	Total int       `json:"total"`
	Items []ItemOut `json:"items"`
}

// ItemOut is one inbox item in InboxOutput.
type ItemOut struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Channel   string `json:"channel"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	MessageID string `json:"message_id"`
	Thread    string `json:"thread,omitempty"`
	Mentioned bool   `json:"mentioned"`
	DM        bool   `json:"dm"`
}

// AckInput is the input of the ack tool.
type AckInput struct {
	IDs []string `json:"ids" jsonschema:"inbox item ids to mark as handled"`
}

// CurrentActor returns the calling actor with its up-to-date state; tools
// call it on every invocation so pauses and emergency stops apply at once.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// Register adds the inbox tools for the given actor.
func Register(s *mcp.Server, svc *delivery.Service, current CurrentActor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "inbox",
		Description: "List what needs your attention: mentions and direct messages you have not acknowledged yet. " +
			"Call this first when you are woken up. Acknowledge handled items with ack.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in InboxInput) (*mcp.CallToolResult, InboxOutput, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, InboxOutput{}, err
		}
		items, total, err := svc.Inbox(ctx, actor, in.Limit)
		if err != nil {
			return nil, InboxOutput{}, err
		}
		out := InboxOutput{Total: total, Items: make([]ItemOut, 0, len(items))}
		for _, it := range items {
			o := ItemOut{ID: it.ID.String(), Kind: it.ItemKind, Channel: it.ChannelName, Author: it.AuthorHandle, Body: it.Body,
				MessageID: it.ItemID.String(), Mentioned: it.Flags.Has(delivery.FlagMentioned), DM: it.Flags.Has(delivery.FlagDM)}
			if it.ThreadRootID != ids.Nil {
				o.Thread = it.ThreadRootID.String()
			}
			out.Items = append(out.Items, o)
		}
		return text(FormatInbox(actor.Handle, items, total)), out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ack",
		Description: "Mark inbox items as handled so they do not show up again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AckInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		list := make([]ids.ID, 0, len(in.IDs))
		for _, s := range in.IDs {
			id, err := ids.Parse(s)
			if err != nil {
				return nil, nil, fmt.Errorf("%q is not an inbox item id; use the id= values from inbox", s)
			}
			list = append(list, id)
		}
		n, err := svc.Ack(ctx, actor, list)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("acknowledged %d item(s)", n)), nil, nil
	})
}

// FormatInbox renders inbox items compactly for a model's context.
func FormatInbox(handle string, items []delivery.InboxItem, total int) string {
	var b strings.Builder
	if total == 0 {
		fmt.Fprintf(&b, "[inbox @%s] empty", handle)
		return b.String()
	}
	fmt.Fprintf(&b, "[inbox @%s] %d pending", handle, total)
	if len(items) < total {
		fmt.Fprintf(&b, " (showing %d, +%d more)", len(items), total-len(items))
	}
	for i, it := range items {
		body := it.Body
		if len(body) > maxBodyInList {
			body = body[:maxBodyInList] + "…"
		}
		if it.ItemKind == delivery.ItemReactionDown {
			// The body is the agent's own message; say so, or models take the
			// reactor for its author.
			fmt.Fprintf(&b, "\n%d. id=%s #%s @%s reacted 👎 (disagrees) to your message=%s", i+1, it.ID,
				it.ChannelName, it.AuthorHandle, it.ItemID)
			if it.ThreadRootID != ids.Nil {
				fmt.Fprintf(&b, " thread=%s", it.ThreadRootID)
			}
			fmt.Fprintf(&b, "\n   your message: %q", body)
			continue
		}
		reason := "mention"
		if it.Flags.Has(delivery.FlagDM) {
			reason = "dm"
		} else if it.Flags.Has(delivery.FlagThread) && !it.Flags.Has(delivery.FlagMentioned) {
			reason = "reply in your thread"
		}
		fmt.Fprintf(&b, "\n%d. id=%s #%s @%s %s (%s)", i+1, it.ID, it.ChannelName, it.AuthorHandle,
			it.MessageAt.Format("2006-01-02 15:04"), reason)
		if it.ThreadRootID != ids.Nil {
			fmt.Fprintf(&b, " thread=%s", it.ThreadRootID)
		} else {
			fmt.Fprintf(&b, " message=%s", it.ItemID)
		}
		fmt.Fprintf(&b, "\n   %q", body)
	}
	return b.String()
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
