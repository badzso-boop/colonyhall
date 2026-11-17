// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// extensionTools returns a granted live extension's tools for the gateway.
func (s *Service) extensionTools(ctx context.Context, agent identity.Actor, id ids.ID) []GatewayTool {
	e, err := s.Store.GetExtension(ctx, agent.WorkspaceID, id)
	if err != nil || e.State != StateLive || e.LiveVersion == 0 || s.checkMaintainer(ctx, e) != nil {
		return nil
	}
	v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, e.LiveVersion)
	if err != nil {
		return nil
	}
	var out []GatewayTool
	for _, t := range v.Manifest.Tools {
		desc := t.Description
		if t.Risk != RiskRead && !e.ApprovalOff {
			desc = strings.TrimSpace(desc + " (Needs a human's approval: the call waits for it.)")
		}
		out = append(out, GatewayTool{Name: ExtensionGatewayName(e.Slug, t.Name), Description: fmt.Sprintf("[%s] %s", e.Title, desc),
			InputSchema: t.Input, Risk: t.Risk})
	}
	return out
}

func (s *Service) extensionBySlug(ctx context.Context, workspaceID ids.ID, slug string) (Extension, error) {
	es, err := s.Store.ListExtensions(ctx, workspaceID)
	if err != nil {
		return Extension{}, err
	}
	for _, e := range es {
		if e.Slug == slug {
			return e, nil
		}
	}
	return Extension{}, ErrNotFound
}

// callExtension runs an extension tool for an agent: grant, live state,
// maintainer, budget and slot, approval, the run, the log and the
// automatic switch-off.
func (s *Service) callExtension(ctx context.Context, caller identity.Actor, in CallInput) (Result, error) {
	slug, tool, ok := SplitGatewayName(strings.TrimPrefix(in.Name, GatewayPrefix))
	if !ok {
		return Result{}, ErrUnknownTool
	}
	e, err := s.extensionBySlug(ctx, caller.WorkspaceID, slug)
	if err != nil {
		return Result{}, ErrUnknownTool
	}
	if !s.granted(ctx, caller, TargetExtension, e.ID) {
		return Result{}, ErrNoGrant
	}
	if e.State != StateLive || e.LiveVersion == 0 {
		return Result{}, ErrNotLive
	}
	if err := s.checkMaintainer(ctx, e); err != nil {
		return Result{}, err
	}
	v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, e.LiveVersion)
	if err != nil {
		return Result{}, ErrNoLiveVersion
	}
	mt, ok := v.Manifest.Tool(tool)
	if !ok {
		return Result{}, ErrUnknownTool
	}
	start := s.Clock.Now()
	call := ToolCall{ID: s.IDs.New(), WorkspaceID: caller.WorkspaceID, ActorID: caller.ID, TargetKind: TargetExtension,
		TargetID: e.ID, Tool: tool, Risk: mt.Risk, StartedAt: start, InBytes: len(in.Args)}
	release, err := s.acquire(caller.WorkspaceID)
	if err != nil {
		s.finishCall(ctx, &call, start, Result{}, err)
		return Result{}, err
	}
	defer release()
	if err := s.checkBudget(ctx, caller.WorkspaceID); err != nil {
		s.finishCall(ctx, &call, start, Result{}, err)
		return Result{}, err
	}
	if mt.Risk != RiskRead && !e.ApprovalOff {
		if in.ChannelID == ids.Nil {
			call.Outcome, call.Error = OutcomeDenied, "no conversation for the approval"
			s.logDenied(ctx, call, start)
			return Result{}, ErrNoChannel
		}
		req, err := s.Approvals.CreateRequest(ctx, caller, approval.CreateInput{ChannelID: in.ChannelID,
			ThreadRootID: in.ThreadRootID, ToolName: in.Name, Input: string(in.Args)})
		if err != nil {
			call.Error = errText(err)
			s.logDenied(ctx, call, start)
			return Result{}, err
		}
		call.ApprovalID = req.ID
		decided, err := s.Approvals.WaitForDecision(ctx, req, in.Current)
		if err != nil || decided.State != approval.StateAllowed {
			call.Error = decided.State
			s.logDenied(ctx, call, start)
			if err != nil {
				return Result{}, err
			}
			return Result{Text: "The call was not approved (" + decided.State + "). Do not retry it unless a person asks you to.", IsError: true}, nil
		}
		// The wait may have taken minutes: the approval is for what was
		// live and granted then (security review of round 56b).
		now, err := s.Store.GetExtension(ctx, caller.WorkspaceID, e.ID)
		if err != nil || now.State != StateLive || now.LiveVersion != e.LiveVersion || !s.granted(ctx, caller, TargetExtension, e.ID) ||
			s.checkMaintainer(ctx, now) != nil {
			call.Error = "changed while waiting for the approval"
			s.logDenied(ctx, call, start)
			return Result{}, ErrNotLive
		}
		if fresh, err := in.Current(ctx); err != nil || fresh.Halted || fresh.State != identity.StateActive {
			call.Error = "halted while waiting for the approval"
			s.logDenied(ctx, call, start)
			return Result{}, identity.ErrHalted
		}
	}
	res, _, err := s.runTool(ctx, runCtx{ext: e, version: v, tool: mt, caller: caller, input: in.Args})
	if err == nil {
		res = cutResult(res)
	}
	s.finishCall(ctx, &call, start, res, err)
	s.recordOutcome(ctx, e, err != nil || res.IsError)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %s", in.Name, errText(err))
	}
	return res, nil
}

func (s *Service) logDenied(ctx context.Context, call ToolCall, start time.Time) {
	call.Outcome = OutcomeDenied
	call.DurationMS = int(s.Clock.Now().Sub(start).Milliseconds())
	if call.DurationMS < 0 {
		call.DurationMS = 0
	}
	if err := s.Store.InsertToolCall(ctx, call); err != nil && s.Logger != nil {
		s.Logger.Warn("workshop: call log", "err", err)
	}
}
