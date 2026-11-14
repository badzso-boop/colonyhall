// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package mcptools exposes agent notebook tools over MCP.
package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/notes"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// NoteGetInput is the input for the note_get tool.
type NoteGetInput struct {
	Thread string `json:"thread,omitempty" jsonschema:"message id in the thread (default: current thread)"`
}

// NoteSetInput is the input for the note_set tool.
type NoteSetInput struct {
	Text   string `json:"text" jsonschema:"note content for this thread (max 2000 characters)"`
	Thread string `json:"thread,omitempty" jsonschema:"message id in the thread (default: current thread)"`
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: s},
		},
	}
}

func resolveThreadID(inThread string, r *http.Request) (ids.ID, error) {
	threadStr := strings.TrimSpace(inThread)
	if threadStr == "" && r != nil {
		threadStr = strings.TrimSpace(r.Header.Get("X-Colonyhall-Thread"))
	}
	if threadStr == "" {
		return ids.Nil, errors.New("thread is required: pass thread or run within a thread context")
	}
	parsed, err := ids.Parse(threadStr)
	if err != nil {
		return ids.Nil, fmt.Errorf("invalid thread id: %w", err)
	}
	return parsed, nil
}

// Register registers note_get and note_set on the MCP server.
func Register(s *mcp.Server, svc *notes.Service, current CurrentActor, r *http.Request) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "note_get",
		Description: "Get your current notebook entry for this thread. " +
			"Returns the latest note you saved for this thread. Agents see only their own notes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in NoteGetInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		threadID, err := resolveThreadID(in.Thread, r)
		if err != nil {
			return nil, nil, err
		}
		note, err := svc.GetLatestNote(ctx, actor, threadID)
		if errors.Is(err, db.ErrNotFound) {
			return textResult("No note for this thread yet."), nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		return textResult(note.Text), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "note_set",
		Description: "Save your notebook entry for this thread (task, state, next step; max 2000 chars). " +
			"Overwrites the current note by appending a new version. Kept across session restarts.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in NoteSetInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		threadID, err := resolveThreadID(in.Thread, r)
		if err != nil {
			return nil, nil, err
		}
		note, err := svc.SetNote(ctx, actor, threadID, in.Text)
		if err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("Note saved (created at %s).", note.CreatedAt.Format("15:04:05"))), nil, nil
	})
}
