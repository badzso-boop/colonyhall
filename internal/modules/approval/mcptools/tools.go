// Package mcptools exposes tool approval requests as an MCP tool.
package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// ApproveInput is the input passed by Claude Code to the permission prompt tool.
type ApproveInput struct {
	ToolName string `json:"tool_name" jsonschema:"the name of the tool requesting approval"`
	Input    any    `json:"input,omitempty" jsonschema:"the arguments to the tool requesting approval"`
}

// AllowResponse is returned when the user approves the tool call.
type AllowResponse struct {
	Behavior     string `json:"behavior"`
	UpdatedInput any    `json:"updatedInput"`
}

// DenyResponse is returned when the tool call is denied or expires.
type DenyResponse struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message"`
}

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// Register adds the approve tool to the MCP server.
func Register(s *mcp.Server, svc *approval.Service, current CurrentActor, fallbackHeaders http.Header) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "approve",
		Description: "Request permission from a human in the chat before running a sensitive tool. " +
			"This blocks until the human allows or denies the request, or until it times out.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ApproveInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		if actor.Kind != identity.KindAgent {
			return nil, nil, approval.ErrForbidden
		}

		// Retrieve channel and thread headers.
		var chStr, thStr string
		if req != nil && req.Extra != nil && req.Extra.Header != nil {
			chStr = req.Extra.Header.Get("X-Colonyhall-Channel")
			thStr = req.Extra.Header.Get("X-Colonyhall-Thread")
		}
		if chStr == "" && fallbackHeaders != nil {
			chStr = fallbackHeaders.Get("X-Colonyhall-Channel")
		}
		if thStr == "" && fallbackHeaders != nil {
			thStr = fallbackHeaders.Get("X-Colonyhall-Thread")
		}
		if chStr == "" {
			return nil, nil, errors.New("missing X-Colonyhall-Channel header")
		}
		channelID, err := ids.Parse(chStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid X-Colonyhall-Channel: %w", err)
		}
		threadRootID := ids.Nil
		if thStr != "" {
			if tid, err := ids.Parse(thStr); err == nil {
				threadRootID = tid
			}
		}

		// Format input JSON.
		inputBytes, _ := json.Marshal(in.Input)
		rawInput := string(inputBytes)

		appReq, err := svc.CreateRequest(ctx, actor, approval.CreateInput{
			ChannelID:    channelID,
			ThreadRootID: threadRootID,
			ToolName:     in.ToolName,
			Input:        rawInput,
		})
		if err != nil {
			return nil, nil, err
		}

		decided, err := svc.WaitForDecision(ctx, appReq, current)
		if err != nil {
			return nil, nil, err
		}

		var respData []byte
		switch decided.State {
		case approval.StateAllowed:
			inputVal := in.Input
			if inputVal == nil {
				inputVal = map[string]any{}
			}
			respData, _ = json.Marshal(AllowResponse{
				Behavior:     "allow",
				UpdatedInput: inputVal,
			})
		case approval.StateExpired:
			respData, _ = json.Marshal(DenyResponse{
				Behavior: "deny",
				Message:  "no answer in time",
			})
		default: // StateDenied
			msg := decided.DecisionMessage
			if msg == "" {
				msg = "denied by user"
			}
			respData, _ = json.Marshal(DenyResponse{
				Behavior: "deny",
				Message:  msg,
			})
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(respData)},
			},
		}, nil, nil
	})
}
