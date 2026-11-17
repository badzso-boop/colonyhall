// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/ids"
)

// BuilderInstructions are sent with the meta-tools' descriptions.
const builderNote = " (Workshop Builder: you act for your owner; a person approves before anything goes live.)"

type createIn struct {
	Slug        string `json:"slug" jsonschema:"short name: 2-16 lower-case letters, digits and _"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	BotHandle   string `json:"bot_handle,omitempty" jsonschema:"the extension's bot's handle; default x-<slug>"`
}

type draftIn struct {
	Extension string         `json:"extension" jsonschema:"the extension's short name"`
	Manifest  map[string]any `json:"manifest" jsonschema:"the manifest: tools, connectors, egress, store, routines (docs/25 13.4, 14.3)"`
	Source    string         `json:"source,omitempty" jsonschema:"TypeScript or JavaScript of the js tools; every exported function is a tool (input, ctx)"`
}

type getIn struct {
	Extension string `json:"extension"`
	Version   int    `json:"version,omitempty" jsonschema:"0 or absent: the newest"`
}

type testIn struct {
	Extension string         `json:"extension"`
	Version   int            `json:"version"`
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input,omitempty"`
}

type versionIn struct {
	Extension string `json:"extension"`
	Version   int    `json:"version"`
}

type channelIn struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Private     bool   `json:"private,omitempty"`
}

func jsonText(v any) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}

func failed(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}
}

func diffOut(d workshop.Diff) map[string]any {
	return map[string]any{"new_connectors": d.NewConnectors, "new_egress": d.NewEgress, "new_store": d.NewStore, "new_tools": d.NewTools,
		"new_routines": d.NewRoutines, "raised_risks": d.RaisedRisks, "code_changed": d.CodeChanged}
}

// RegisterBuilder adds the Workshop's meta-tools for a Builder agent
// (round 56d): only an agent with the Workshop grant whose owner is an
// owner or an admin gets them.
func RegisterBuilder(s *mcp.Server, svc *workshop.Service, actor identity.Actor, current CurrentActor, r *http.Request) {
	if svc == nil || !svc.IsBuilder(r.Context(), actor) {
		return
	}
	conversationOf := func(req *mcp.CallToolRequest) (ids.ID, ids.ID) {
		h := r.Header
		if req != nil && req.Extra != nil && req.Extra.Header != nil && req.Extra.Header.Get("X-Colonyhall-Channel") != "" {
			h = req.Extra.Header
		}
		ch, _ := ids.Parse(h.Get("X-Colonyhall-Channel"))
		th, _ := ids.Parse(h.Get("X-Colonyhall-Thread"))
		return ch, th
	}
	me := func(ctx context.Context) (identity.Actor, error) { return current(ctx) }

	mcp.AddTool(s, &mcp.Tool{Name: "workshop_list", Description: "List the workspace's connectors (with their tools) and extensions (versions, tools, routines)." + builderNote},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			a, err := me(ctx)
			if err != nil {
				return failed(err), nil, nil
			}
			o, err := svc.BuilderOverview(ctx, a)
			if err != nil {
				return failed(err), nil, nil
			}
			var conns, exts []map[string]any
			for _, c := range o.Connectors {
				var tools []string
				for _, t := range c.Tools {
					tools = append(tools, t.Name+" ("+c.EffectiveRisk(t)+")")
				}
				conns = append(conns, map[string]any{"slug": c.Slug, "title": c.Title, "kind": c.Kind, "base_url": c.BaseURL, "tools": tools})
			}
			for _, e := range o.Extensions {
				var versions []map[string]any
				for _, v := range e.Versions {
					versions = append(versions, map[string]any{"version": v.Version, "approved": e.Approved[v.Version], "tools": len(v.Manifest.Tools), "routines": len(v.Manifest.Routines)})
				}
				exts = append(exts, map[string]any{"slug": e.Slug, "title": e.Title, "state": e.State, "live_version": e.LiveVersion, "bot": "@" + e.BotHandle, "versions": versions})
			}
			return jsonText(map[string]any{"connectors": conns, "extensions": exts,
				"note": "Connectors (and their secrets) are added by a person in the Workshop; ask for one if you need it."}), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "extension_create", Description: "Create an extension (a draft) with its own bot." + builderNote},
		func(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, any, error) {
			a, err := me(ctx)
			if err != nil {
				return failed(err), nil, nil
			}
			owner, err := svc.BuilderOwner(ctx, a)
			if err != nil {
				return failed(err), nil, nil
			}
			e, err := svc.CreateExtension(ctx, owner, workshop.ExtensionInput{Slug: in.Slug, Title: in.Title, Description: in.Description, BotHandle: in.BotHandle})
			if err != nil {
				return failed(err), nil, nil
			}
			return jsonText(map[string]any{"extension": e.Slug, "state": e.State}), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "extension_draft", Description: "Save a new version of an extension (manifest and source). It is compiled and checked; it does not go live. The answer says what it adds." + builderNote},
		func(ctx context.Context, _ *mcp.CallToolRequest, in draftIn) (*mcp.CallToolResult, any, error) {
			a, err := me(ctx)
			if err != nil {
				return failed(err), nil, nil
			}
			_, id, err := svc.BuilderExtensionID(ctx, a, in.Extension)
			if err != nil {
				return failed(err), nil, nil
			}
			var m workshop.Manifest
			raw, _ := json.Marshal(in.Manifest)
			if err := json.Unmarshal(raw, &m); err != nil {
				return failed(fmt.Errorf("invalid manifest: %w", err)), nil, nil
			}
			v, d, err := svc.BuilderDraft(ctx, a, id, workshop.VersionInput{Manifest: m, Source: in.Source})
			if err != nil {
				return failed(err), nil, nil
			}
			return jsonText(map[string]any{"version": v.Version, "adds": diffOut(d), "next": "test it with extension_test, then ask for publishing with extension_publish"}), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "extension_get", Description: "Read a version of an extension: its manifest, source and what it adds." + builderNote},
		func(ctx context.Context, _ *mcp.CallToolRequest, in getIn) (*mcp.CallToolResult, any, error) {
			a, err := me(ctx)
			if err != nil {
				return failed(err), nil, nil
			}
			owner, id, err := svc.BuilderExtensionID(ctx, a, in.Extension)
			if err != nil {
				return failed(err), nil, nil
			}
			version := in.Version
			if version == 0 {
				ev, err := svc.GetExtension(ctx, owner, id)
				if err != nil || len(ev.Versions) == 0 {
					return failed(workshop.ErrVersionNotFound), nil, nil
				}
				version = ev.Versions[0].Version
			}
			v, d, err := svc.GetVersion(ctx, owner, id, version)
			if err != nil {
				return failed(err), nil, nil
			}
			return jsonText(map[string]any{"version": v.Version, "manifest": v.Manifest, "source": v.Source, "adds": diffOut(d)}), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "extension_test", Description: "Run one tool of a version (not yet live too) as the extension's bot, to test it. A real call." + builderNote},
		func(ctx context.Context, _ *mcp.CallToolRequest, in testIn) (*mcp.CallToolResult, any, error) {
			a, err := me(ctx)
			if err != nil {
				return failed(err), nil, nil
			}
			owner, id, err := svc.BuilderExtensionID(ctx, a, in.Extension)
			if err != nil {
				return failed(err), nil, nil
			}
			input := json.RawMessage("{}")
			if in.Input != nil {
				input, _ = json.Marshal(in.Input)
			}
			res, logs, err := svc.TestTool(ctx, owner, id, in.Version, in.Tool, input)
			if err != nil {
				return failed(err), nil, nil
			}
			return jsonText(map[string]any{"output": res.Text, "is_error": res.IsError, "logs": logs}), nil, nil
		})

	publish := func(rollback bool) func(ctx context.Context, req *mcp.CallToolRequest, in versionIn) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, in versionIn) (*mcp.CallToolResult, any, error) {
			a, err := me(ctx)
			if err != nil {
				return failed(err), nil, nil
			}
			_, id, err := svc.BuilderExtensionID(ctx, a, in.Extension)
			if err != nil {
				return failed(err), nil, nil
			}
			ch, th := conversationOf(req)
			e, d, err := svc.BuilderPublish(ctx, a, id, in.Version, rollback, ch, th, me)
			if err != nil {
				return failed(err), nil, nil
			}
			return jsonText(map[string]any{"extension": e.Slug, "live_version": e.LiveVersion, "state": e.State, "added": diffOut(d)}), nil, nil
		}
	}
	mcp.AddTool(s, &mcp.Tool{Name: "extension_publish", Description: "Ask a person to make a version live: an approval card shows what it adds; the call waits for their decision." + builderNote}, publish(false))
	mcp.AddTool(s, &mcp.Tool{Name: "extension_rollback", Description: "Ask a person to make an earlier approved version live again (an approval card; waits for the decision)." + builderNote}, publish(true))

	mcp.AddTool(s, &mcp.Tool{Name: "channel_create", Description: "Create a channel for an extension's routines (as your owner; you become a member)." + builderNote},
		func(ctx context.Context, _ *mcp.CallToolRequest, in channelIn) (*mcp.CallToolResult, any, error) {
			a, err := me(ctx)
			if err != nil {
				return failed(err), nil, nil
			}
			id, err := svc.BuilderCreateChannel(ctx, a, in.Name, in.Description, in.Private)
			if err != nil {
				return failed(err), nil, nil
			}
			return jsonText(map[string]any{"channel_id": id, "name": in.Name}), nil, nil
		})
}
