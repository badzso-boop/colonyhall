// Package mcptools exposes file upload and download as MCP tools for agents.
package mcptools

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// CurrentActor returns the calling actor with fresh state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// ChannelResolver resolves channel names or DM handles.
type ChannelResolver interface {
	ResolveChannel(ctx context.Context, by identity.Actor, name string) (conversation.Channel, error)
	OpenDMByHandle(ctx context.Context, by identity.Actor, handle string) (conversation.Channel, error)
}

// UploadInput is the input schema for the upload tool.
type UploadInput struct {
	Channel string `json:"channel" jsonschema:"channel name (e.g. general, #general) or @handle for a direct message"`
	Name    string `json:"name" jsonschema:"filename for the uploaded file (e.g. data.csv, report.pdf)"`
	Content string `json:"content" jsonschema:"base64-encoded file content"`
}

// DownloadInput is the input schema for the download tool.
type DownloadInput struct {
	FileID string `json:"file_id" jsonschema:"UUID of the file to download"`
}

// Register adds the files MCP tools (upload, download) to the server.
func Register(s *mcp.Server, svc *files.Service, resolver ChannelResolver, current CurrentActor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "upload",
		Description: "Upload a file to a channel using base64-encoded content. " +
			"Returns the file ID, which can then be attached to messages using the send tool's files parameter. " +
			"When the workspace storage is full the upload fails: do not retry it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UploadInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		ch, err := resolveChannel(ctx, resolver, actor, in.Channel)
		if err != nil {
			return nil, nil, err
		}

		raw, err := base64.StdEncoding.DecodeString(in.Content)
		if err != nil {
			var err2 error
			raw, err2 = base64.URLEncoding.DecodeString(in.Content)
			if err2 != nil {
				return nil, nil, fmt.Errorf("invalid base64 content: %w", err)
			}
		}

		f, err := svc.Upload(ctx, actor, ch.ID, in.Name, bytes.NewReader(raw))
		if errors.Is(err, files.ErrStorageFull) {
			// Round 54: a full storage does not empty itself; retrying only
			// spends the agent's quota.
			return nil, nil, fmt.Errorf("%w Do not retry this upload; tell the person who asked that the workspace storage is full", err)
		}
		if err != nil {
			return nil, nil, err
		}

		return text(fmt.Sprintf("uploaded file id=%s name=%s (%d bytes, %s)", f.ID, f.Name, f.Size, f.Mime)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "download",
		Description: "Download a file by its ID. Small text files (up to 64KB) are returned as text directly. " +
			"Other files return metadata and the download URL (/api/v1/files/{id}/content).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DownloadInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		fid, err := ids.Parse(in.FileID)
		if err != nil {
			return nil, nil, fmt.Errorf("file_id must be a UUID: %w", err)
		}

		f, err := svc.Get(ctx, actor, fid)
		if err != nil {
			return nil, nil, err
		}

		if isTextMime(f.Mime) && f.Size <= files.MaxTextDownloadSize {
			_, rc, err := svc.OpenContent(ctx, actor, fid)
			if err != nil {
				return nil, nil, err
			}
			defer func() { _ = rc.Close() }()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, nil, fmt.Errorf("read file content: %w", err)
			}
			return text(string(data)), nil, nil
		}

		return text(fmt.Sprintf("file %s (%s, %d bytes): download url is /api/v1/files/%s/content", f.Name, f.Mime, f.Size, f.ID)), nil, nil
	})
}

func resolveChannel(ctx context.Context, resolver ChannelResolver, actor identity.Actor, name string) (conversation.Channel, error) {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "@") {
		handle := strings.TrimPrefix(name, "@")
		ch, err := resolver.OpenDMByHandle(ctx, actor, handle)
		if errors.Is(err, conversation.ErrBadDM) {
			return ch, fmt.Errorf("unknown person %q", name)
		}
		return ch, err
	}
	return resolver.ResolveChannel(ctx, actor, name)
}

func isTextMime(m string) bool {
	m = strings.ToLower(m)
	if strings.HasPrefix(m, "text/") {
		return true
	}
	switch m {
	case "application/json", "application/xml", "application/javascript",
		"application/x-yaml", "application/yaml", "application/toml",
		"application/sql", "application/csv":
		return true
	}
	return false
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
