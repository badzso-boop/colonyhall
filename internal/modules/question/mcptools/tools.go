// Package mcptools exposes question tools (ask, answer, withdraw) over MCP for agents.
package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/ids"
)

// CurrentActor returns the calling actor with its up-to-date state.
type CurrentActor func(ctx context.Context) (identity.Actor, error)

// AskInput is the input for the ask tool.
type AskInput struct {
	Question        string   `json:"question" jsonschema:"the question text"`
	To              string   `json:"to,omitempty" jsonschema:"target: @handle or 'human' (default: human)"`
	Options         []string `json:"options,omitempty" jsonschema:"optional list of choices"`
	Default         string   `json:"default,omitempty" jsonschema:"fallback choice if unanswered at deadline"`
	DeadlineMinutes int      `json:"deadline_minutes,omitempty" jsonschema:"optional deadline in minutes"`
	Channel         string   `json:"channel,omitempty" jsonschema:"channel name (e.g. #general) or @handle; default: current channel when known"`
	Thread          string   `json:"thread,omitempty" jsonschema:"message id: thread to post in (default: current thread)"`
}

// AnswerInput is the input for the answer tool.
type AnswerInput struct {
	Question string `json:"question" jsonschema:"question identifier (e.g. Q-12 or UUID)"`
	Answer   string `json:"answer" jsonschema:"answer text or chosen option"`
}

// WithdrawInput is the input for the withdraw tool.
type WithdrawInput struct {
	Question string `json:"question" jsonschema:"question identifier (e.g. Q-12 or UUID)"`
}

// Register registers the question tools on the MCP server.
func Register(s *mcp.Server, svc *question.Service, conv *conversation.Service, current CurrentActor) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "ask",
		Description: "Ask a question in the current thread to a teammate (@handle) or to humans ('human'). " +
			"Returns the question number (e.g. Q-12). When you need a decision, use ask instead of guessing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AskInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}

		var threadRootID ids.ID
		if strings.TrimSpace(in.Thread) != "" {
			parsed, err := ids.Parse(strings.TrimSpace(in.Thread))
			if err != nil {
				return nil, nil, fmt.Errorf("invalid thread id: %w", err)
			}
			threadRootID = parsed
		}

		var channelID ids.ID
		chName := strings.TrimSpace(in.Channel)
		if chName != "" {
			if strings.HasPrefix(chName, "@") {
				dm, err := conv.OpenDMByHandle(ctx, actor, strings.TrimPrefix(chName, "@"))
				if err != nil {
					return nil, nil, err
				}
				channelID = dm.ID
			} else {
				ch, err := conv.ResolveChannel(ctx, actor, chName)
				if err != nil {
					return nil, nil, err
				}
				channelID = ch.ID
			}
		} else if threadRootID != ids.Nil {
			root, err := conv.MessageByID(ctx, threadRootID)
			if err != nil {
				return nil, nil, fmt.Errorf("could not find thread message %s: %w", threadRootID, err)
			}
			channelID = root.ChannelID
		} else {
			ch, err := conv.ResolveChannel(ctx, actor, "")
			if err != nil {
				return nil, nil, errors.New("channel or thread is required")
			}
			channelID = ch.ID
		}

		var defOpt *string
		if in.Default != "" {
			d := in.Default
			defOpt = &d
		}
		var dlMin *int
		if in.DeadlineMinutes > 0 {
			m := in.DeadlineMinutes
			dlMin = &m
		}

		toTarget := in.To
		if toTarget == "" {
			toTarget = "human"
		}

		q, err := svc.Ask(ctx, actor, question.AskInput{
			ChannelID:       channelID,
			ThreadRootID:    threadRootID,
			To:              toTarget,
			Text:            in.Question,
			Options:         in.Options,
			DefaultOption:   defOpt,
			DeadlineMinutes: dlMin,
		})
		if err != nil {
			return nil, nil, err
		}
		return text(q.FormattedNumber()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "answer",
		Description: "Answer an open or escalated question (e.g. Q-12). Posts the answer in the thread " +
			"and wakes the asker with the answer in their inbox.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AnswerInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		q, err := svc.Answer(ctx, actor, in.Question, in.Answer)
		if err != nil {
			return nil, nil, err
		}
		ans := ""
		if q.Answer != nil {
			ans = *q.Answer
		}
		return text(fmt.Sprintf("%s answered: %s", q.FormattedNumber(), ans)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "withdraw",
		Description: "Withdraw an open or escalated question you asked earlier (e.g. Q-12). Only the asker may withdraw.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in WithdrawInput) (*mcp.CallToolResult, any, error) {
		actor, err := current(ctx)
		if err != nil {
			return nil, nil, err
		}
		q, err := svc.Withdraw(ctx, actor, in.Question)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("%s withdrawn", q.FormattedNumber())), nil, nil
	})
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
