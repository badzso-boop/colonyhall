// Package mcptools exposes channel messaging as MCP tools.
package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// SendInput is the input of the send tool.
type SendInput struct {
	Channel     string   `json:"channel" jsonschema:"channel name (general or #general), or @handle for a direct message"`
	Text        string   `json:"text" jsonschema:"message text; keep it short (agents: max 600 characters)"`
	Thread      string   `json:"thread,omitempty" jsonschema:"thread root message id to reply in a thread"`
	ClientMsgID string   `json:"client_msg_id,omitempty" jsonschema:"optional idempotency key; resending with the same key does not duplicate"`
	Files       []string `json:"files,omitempty" jsonschema:"optional file IDs to attach to the message"`
	Elaborate   bool     `json:"elaborate,omitempty" jsonschema:"allow a long answer; set only when someone asked you to elaborate"`
}

// ReadInput is the input of the read tool.
type ReadInput struct {
	Channel string `json:"channel,omitempty" jsonschema:"channel name; omit when reading a thread"`
	Thread  string `json:"thread,omitempty" jsonschema:"message id: read that message's whole thread instead of the channel"`
	Limit   int    `json:"limit,omitempty" jsonschema:"number of most recent channel messages (default 10, max 50)"`
}

// ReactInput is the input of the react tool.
type ReactInput struct {
	Message string `json:"message" jsonschema:"message id"`
	Emoji   string `json:"emoji" jsonschema:"use 👍 to acknowledge or agree, 👎 to disagree"`
	Remove  bool   `json:"remove,omitempty" jsonschema:"remove your reaction instead of adding it"`
}

// CurrentActor returns the calling actor with its up-to-date state; tools
// call it on every invocation so pauses and emergency stops apply at once.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// Register adds the messaging tools.
func Register(s *mcp.Server, svc *conversation.Service, dir identity.Directory, current CurrentActor, r *http.Request) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "send",
		Description: "Post a message to a channel or a direct message (@handle), optionally as a thread reply. " +
			"Reply in the thread the request came from. Mention (@handle) only people who must act. " +
			"Never send acknowledgements or thanks: use react with 👍 instead.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SendInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		ch, err := resolveTarget(ctx, svc, actor, in.Channel)
		if err != nil {
			return nil, nil, err
		}
		root := ids.Nil
		if in.Thread != "" {
			if root, err = ids.Parse(in.Thread); err != nil {
				return nil, nil, fmt.Errorf("thread must be a message id")
			}
		}
		var fileIDs []ids.ID
		for _, fidStr := range in.Files {
			fid, err := ids.Parse(fidStr)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid file id %q: %w", fidStr, err)
			}
			fileIDs = append(fileIDs, fid)
		}
		var origChanID, origThreadID ids.ID
		if r != nil {
			if cStr := strings.TrimSpace(r.Header.Get("X-Colonyhall-Channel")); cStr != "" {
				if parsed, err := ids.Parse(cStr); err == nil {
					origChanID = parsed
				}
			}
			if tStr := strings.TrimSpace(r.Header.Get("X-Colonyhall-Thread")); tStr != "" {
				if parsed, err := ids.Parse(tStr); err == nil {
					origThreadID = parsed
				}
			}
		}
		res, err := svc.SendMessage(ctx, actor, conversation.SendInput{
			ChannelID: ch.ID, ThreadRootID: root, ClientMsgID: in.ClientMsgID, Body: in.Text, Elaborate: in.Elaborate,
			FileIDs: fileIDs, OriginChannelID: origChanID, OriginThreadID: origThreadID,
		})
		if err != nil {
			return nil, nil, err
		}
		if r := res.ConvertedToReaction; r != nil {
			return text(fmt.Sprintf("not posted: acknowledgements become a %s reaction (added to message=%s)", r.Emoji, r.MessageID)), nil, nil
		}
		return text(fmt.Sprintf("sent message=%s to %s", res.Message.ID, label(ch))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "read",
		Description: "Read recent messages of a channel, or one whole thread. Prefer inbox; read only when you need " +
			"more context.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		if in.Thread != "" {
			id, err := ids.Parse(in.Thread)
			if err != nil {
				return nil, nil, fmt.Errorf("thread must be a message id")
			}
			root, replies, err := svc.Thread(ctx, actor, id)
			if err != nil {
				return nil, nil, err
			}
			return text(formatMessages(ctx, dir, "thread "+root.ID.String(), append([]conversation.Message{root}, replies...))), nil, nil
		}
		ch, err := resolveTarget(ctx, svc, actor, in.Channel)
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
		return text(formatMessages(ctx, dir, label(ch), msgs)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "react",
		Description: "React to a message: 👍 to acknowledge or agree, 👎 to disagree. Reactions never wake anyone.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ReactInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		id, err := ids.Parse(in.Message)
		if err != nil {
			return nil, nil, fmt.Errorf("message must be a message id")
		}
		if in.Remove {
			err = svc.Unreact(ctx, actor, id, in.Emoji)
		} else {
			err = svc.React(ctx, actor, id, in.Emoji)
		}
		if err != nil {
			return nil, nil, err
		}
		return text("ok"), nil, nil
	})
}

// resolveTarget accepts "general", "#general" or "@handle" (a DM).
func resolveTarget(ctx context.Context, svc *conversation.Service, actor identity.Actor, name string) (conversation.Channel, error) {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "@") {
		ch, err := svc.OpenDMByHandle(ctx, actor, strings.TrimPrefix(name, "@"))
		if errors.Is(err, conversation.ErrBadDM) {
			return ch, fmt.Errorf("unknown person %q", name)
		}
		return ch, err
	}
	ch, err := svc.ResolveChannel(ctx, actor, name)
	if errors.Is(err, db.ErrNotFound) || errors.Is(err, conversation.ErrForbidden) {
		views, lerr := svc.ListChannels(ctx, actor)
		if lerr != nil {
			return ch, err
		}
		names := make([]string, 0, len(views))
		for _, v := range views {
			if v.IsMember && v.Kind != conversation.KindDM {
				names = append(names, "#"+v.Name)
			}
		}
		return ch, fmt.Errorf("unknown channel %q; available: %s (or @handle for a direct message)", name, strings.Join(names, ", "))
	}
	return ch, err
}

func label(c conversation.Channel) string {
	if c.Kind == conversation.KindDM {
		return "direct message"
	}
	return "#" + c.Name
}

func formatMessages(ctx context.Context, dir identity.Directory, title string, msgs []conversation.Message) string {
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
	fmt.Fprintf(&b, "%s, %d message(s):", title, len(msgs))
	for _, m := range msgs {
		fmt.Fprintf(&b, "\n- %s @%s message=%s", m.CreatedAt.Format("2006-01-02 15:04"), handles[m.AuthorID], m.ID)
		if m.ThreadRootID != ids.Nil {
			fmt.Fprintf(&b, " thread=%s", m.ThreadRootID)
		}
		for _, r := range m.Reactions {
			fmt.Fprintf(&b, " %s%d", r.Emoji, r.Count)
		}
		if len(m.Attachments) > 0 {
			var atts []string
			for _, a := range m.Attachments {
				atts = append(atts, fmt.Sprintf("%s (%s, %d bytes, id=%s)", a.Name, a.Mime, a.Size, a.ID))
			}
			fmt.Fprintf(&b, " [files: %s]", strings.Join(atts, ", "))
		}
		fmt.Fprintf(&b, "\n  %q", m.Body)
	}
	return b.String()
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
