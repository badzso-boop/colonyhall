// Package mcptools exposes decision logging tools over MCP.
package mcptools

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// DecisionAddInput is the input for decision_add.
type DecisionAddInput struct {
	Text       string `json:"text" jsonschema:"one-line decision statement (max 300 chars)"`
	Rationale  string `json:"rationale" jsonschema:"why this decision was made (max 500 chars)"`
	Supersedes any    `json:"supersedes,omitempty" jsonschema:"optional number of the decision this replaces (e.g. 7 or 'D-7')"`
	Thread     string `json:"thread,omitempty" jsonschema:"optional message ID in the thread (default: current thread)"`
}

// DecisionListInput is the input for decision_list.
type DecisionListInput struct {
	Limit *int   `json:"limit,omitempty" jsonschema:"maximum number of decisions to return (default 5, max 50)"`
	Query string `json:"query,omitempty" jsonschema:"optional keyword to search decision text and rationale"`
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: s},
		},
	}
}

func parseSupersedes(v any) (*int, error) {
	if v == nil {
		return nil, nil
	}
	switch val := v.(type) {
	case float64:
		n := int(val)
		return &n, nil
	case int:
		return &val, nil
	case int32:
		n := int(val)
		return &n, nil
	case int64:
		n := int(val)
		return &n, nil
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil, nil
		}
		s = strings.TrimPrefix(s, "D-")
		s = strings.TrimPrefix(s, "d-")
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("invalid superseded decision reference: %q", val)
		}
		return &n, nil
	default:
		return nil, fmt.Errorf("unsupported type for supersedes: %T", v)
	}
}

func resolveThreadID(inThread string, r *http.Request) *ids.ID {
	threadStr := strings.TrimSpace(inThread)
	if threadStr == "" && r != nil {
		threadStr = strings.TrimSpace(r.Header.Get("X-Colonyhall-Thread"))
	}
	if threadStr == "" {
		return nil
	}
	parsed, err := ids.Parse(threadStr)
	if err != nil {
		return nil
	}
	return &parsed
}

func resolveChannelID(r *http.Request) *ids.ID {
	if r == nil {
		return nil
	}
	chStr := strings.TrimSpace(r.Header.Get("X-Colonyhall-Channel"))
	if chStr == "" {
		return nil
	}
	parsed, err := ids.Parse(chStr)
	if err != nil {
		return nil
	}
	return &parsed
}

// Register registers decision_add and decision_list tools on the MCP server.
func Register(s *mcp.Server, svc *decision.Service, current CurrentActor, r *http.Request) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "decision_add",
		Description: "Record an architectural or operational decision made in the conversation (D-12). " +
			"Appends to the 'Döntések' wiki page and records the rationale. Max text 300 chars, max rationale 500 chars.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DecisionAddInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		sup, err := parseSupersedes(in.Supersedes)
		if err != nil {
			return nil, nil, err
		}

		trID := resolveThreadID(in.Thread, r)
		chID := resolveChannelID(r)

		d, err := svc.AddDecision(ctx, actor, decision.AddDecisionInput{
			Text:        in.Text,
			Rationale:   in.Rationale,
			Supersedes:  sup,
			ThreadMsgID: trID,
			ChannelID:   chID,
		})
		if err != nil {
			return nil, nil, err
		}

		return textResult(fmt.Sprintf("D-%d", d.Number)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "decision_list",
		Description: "List recent valid (not superseded) decisions in the workspace. " +
			"Returns one line per decision. Cheap and fast to consult before starting work.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DecisionListInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		limit := 5
		if in.Limit != nil && *in.Limit > 0 {
			limit = *in.Limit
		}

		decisions, err := svc.ListDecisions(ctx, actor, limit, in.Query, true)
		if err != nil {
			return nil, nil, err
		}

		if len(decisions) == 0 {
			return textResult("no decisions found"), nil, nil
		}

		var b strings.Builder
		for i, d := range decisions {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "D-%d — %s. Why: %s", d.Number, d.Text, d.Rationale)
		}
		return textResult(b.String()), nil, nil
	})
}
