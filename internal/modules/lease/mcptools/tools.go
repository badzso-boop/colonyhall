// Package mcptools exposes shared resource lease tools over MCP for agents.
package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/lease"
)

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// LeaseAcquireInput is the input for lease_acquire.
type LeaseAcquireInput struct {
	Resource   string `json:"resource" jsonschema:"the shared resource to lock (e.g. 'deploy:staging', 'port:5433', 'db:shared')"`
	TTLMinutes *int   `json:"ttl_minutes,omitempty" jsonschema:"lease duration in minutes (1-240, default 30)"`
	Note       string `json:"note,omitempty" jsonschema:"optional short note on what you are using it for (max 200 chars)"`
}

// LeaseReleaseInput is the input for lease_release.
type LeaseReleaseInput struct {
	Resource string `json:"resource" jsonschema:"the shared resource to release"`
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: s},
		},
	}
}

// Register registers lease_acquire and lease_release on the MCP server.
func Register(s *mcp.Server, svc *lease.Service, current CurrentActor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "lease_acquire",
		Description: "Acquire an exclusive lease on a shared resource (a deploy, a shared database, a port, a compose stack) " +
			"before using it. Returns taken info if held by someone else (no error, no wait), extends it if held by caller, " +
			"or acquires it. Default duration 30 minutes (max 240).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in LeaseAcquireInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		ttl := 30
		if in.TTLMinutes != nil && *in.TTLMinutes > 0 {
			ttl = *in.TTLMinutes
		}

		res, err := svc.Acquire(ctx, actor, in.Resource, ttl, in.Note)
		if err != nil {
			return nil, nil, err
		}

		return textResult(res.Message), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "lease_release",
		Description: "Release an active lease you currently hold on a shared resource. " +
			"Only the holder may release the lease.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in LeaseReleaseInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		msg, err := svc.ReleaseByResource(ctx, actor, in.Resource)
		if err != nil {
			return nil, nil, err
		}

		return textResult(msg), nil, nil
	})
}
