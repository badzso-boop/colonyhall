// Package mcptools exposes channel messaging as MCP tools.
package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// SendInput is the input of the send tool.
type SendInput struct {
	Channel     string `json:"channel" jsonschema:"channel name, e.g. general or #general"`
	Text        string `json:"text" jsonschema:"message text; keep it short"`
	Thread      string `json:"thread,omitempty" jsonschema:"thread root message id to reply in a thread"`
	ClientMsgID string `json:"client_msg_id,omitempty" jsonschema:"optional idempotency key; resending with the same key does not duplicate"`
}

// ReadInput is the input of the read tool.
type ReadInput struct {
	Channel string `json:"channel" jsonschema:"channel name"`
	Limit   int    `json:"limit,omitempty" jsonschema:"number of most recent messages (default 10, max 50)"`
}

// Register adds the messaging tools for the given actor.
func Register(s *mcp.Server, svc *conversation.Service, dir identity.Directory, actor identity.Actor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "send",
		Description: "Post a message to a channel, optionally as a reply in a thread. " +
			"Reply in the thread the request came from. Mention (@handle) only people you need to act.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SendInput) (*mcp.CallToolResult, any, error) {
		ch, err := resolve(ctx, svc, actor, in.Channel)
		if err != nil {
			return nil, nil, err
		}
		root := ids.Nil
		if in.Thread != "" {
			if root, err = ids.Parse(in.Thread); err != nil {
				return nil, nil, fmt.Errorf("thread must be a message id")
			}
		}
		m, err := svc.SendMessage(ctx, actor, conversation.SendInput{
			ChannelID: ch.ID, ThreadRootID: root, ClientMsgID: in.ClientMsgID, Body: in.Text,
		})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("sent message=%s to #%s", m.ID, ch.Name)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "read",
		Description: "Read the most recent messages of a channel. Prefer inbox; read only when you need more context.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, any, error) {
		ch, err := resolve(ctx, svc, actor, in.Channel)
		if err != nil {
			return nil, nil, err
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}
		msgs, err := svc.ListMessages(ctx, actor, ch.ID, ids.Nil, limit)
		if err != nil {
			return nil, nil, err
		}
		return text(formatMessages(ctx, dir, ch.Name, msgs)), nil, nil
	})
}

func resolve(ctx context.Context, svc *conversation.Service, actor identity.Actor, name string) (conversation.Channel, error) {
	ch, err := svc.ResolveChannel(ctx, actor, strings.TrimSpace(name))
	if errors.Is(err, db.ErrNotFound) || errors.Is(err, conversation.ErrForbidden) {
		views, lerr := svc.ListChannels(ctx, actor)
		if lerr != nil {
			return ch, err
		}
		names := make([]string, 0, len(views))
		for _, v := range views {
			if v.IsMember {
				names = append(names, "#"+v.Name)
			}
		}
		return ch, fmt.Errorf("unknown channel %q; available: %s", name, strings.Join(names, ", "))
	}
	return ch, err
}

func formatMessages(ctx context.Context, dir identity.Directory, channel string, msgs []conversation.Message) string {
	authorIDs := make([]ids.ID, 0, len(msgs))
	for _, m := range msgs {
		authorIDs = append(authorIDs, m.AuthorID)
	}
	handles := map[ids.ID]string{}
	if actors, err := dir.ActorsByIDs(ctx, authorIDs); err == nil {
		for _, a := range actors {
			handles[a.ID] = a.Handle
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%s, last %d message(s):", channel, len(msgs))
	for _, m := range msgs {
		fmt.Fprintf(&b, "\n- %s @%s message=%s", m.CreatedAt.Format("2006-01-02 15:04"), handles[m.AuthorID], m.ID)
		if m.ThreadRootID != ids.Nil {
			fmt.Fprintf(&b, " thread=%s", m.ThreadRootID)
		}
		fmt.Fprintf(&b, "\n  %q", m.Body)
	}
	return b.String()
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
