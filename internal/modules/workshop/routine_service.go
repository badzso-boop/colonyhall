// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// ConsumerName is the Workshop's event consumer (routines on messages and
// clicks, round 56c).
const ConsumerName = "workshop"

// Channels is what the routines need from the conversation module.
type Channels interface {
	ChannelIDByName(ctx context.Context, by identity.Actor, name string) (ids.ID, error)
	AddMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID, canWrite bool) error
	IsMember(ctx context.Context, channelID, actorID ids.ID) bool
	SendMessage(ctx context.Context, by identity.Actor, in conversation.SendInput) (conversation.SendResult, error)
	ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error)
	CreateChannel(ctx context.Context, by identity.Actor, name, description string, kind conversation.ChannelKind) (conversation.Channel, error)
}

// Errors of the routines.
var (
	ErrNoRoutine     = errors.New("no such routine in the live version")
	ErrNotWebhook    = errors.New("the routine is not triggered by a webhook")
	ErrBadWebhook    = errors.New("unknown webhook")
	ErrRoutineTarget = errors.New("the routine's channel does not exist")
)

// syncRoutines prepares a version's routines when it goes live: their
// schedule, and the bot in their channels (by the approving admin, who may
// manage members). Called inside the approval's transaction.
func (s *Service) syncRoutines(ctx context.Context, by identity.Actor, e Extension, v Version) error {
	now := s.Clock.Now()
	for _, r := range v.Manifest.Routines {
		st := RoutineState{ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, ExtensionID: e.ID, Routine: r.Name}
		if r.Trigger.Kind() == TriggerCron {
			next, err := schedule.Next(r.Trigger.Cron, r.Trigger.Timezone, now)
			if err != nil {
				return fmt.Errorf("%w: routine %q: %s", ErrBadManifest, r.Name, err.Error())
			}
			st.NextRunAt = &next
		}
		if err := s.Store.UpsertRoutineState(ctx, st); err != nil {
			return err
		}
		if s.Channels == nil {
			continue
		}
		var names []string
		if r.Output != "" {
			names = append(names, r.Output)
		}
		if r.Trigger.Message != nil {
			names = append(names, r.Trigger.Message.Channel)
		}
		for _, name := range names {
			ch, err := s.Channels.ChannelIDByName(ctx, by, name)
			if err != nil {
				return fmt.Errorf("%w: #%s (routine %q)", ErrRoutineTarget, name, r.Name)
			}
			if !s.Channels.IsMember(ctx, ch, e.BotID) {
				if err := s.Channels.AddMember(ctx, by, ch, e.BotID, true); err != nil && !errors.Is(err, db.ErrConflict) {
					return err
				}
			}
		}
	}
	// Routines no longer in the live version stop being due.
	sts, err := s.Store.ListRoutineStates(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return err
	}
	for _, st := range sts {
		r, ok := v.Manifest.Routine(st.Routine)
		if !ok && st.NextRunAt != nil {
			if err := s.Store.SetRoutineNext(ctx, e.WorkspaceID, st.ID, nil, st.LastRunAt); err != nil {
				return err
			}
		}
		// A webhook's address belongs to the routine it was made for: gone
		// or no longer a webhook, the address stops working.
		if (!ok || r.Trigger.Kind() != TriggerWebhook) && len(st.WebhookTokenHash) > 0 {
			if err := s.Store.SetRoutineWebhook(ctx, e.WorkspaceID, st.ID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// clearWebhooks revokes every webhook address of an extension (switched
// off or deleted).
func (s *Service) clearWebhooks(ctx context.Context, e Extension) error {
	sts, err := s.Store.ListRoutineStates(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return err
	}
	for _, st := range sts {
		if len(st.WebhookTokenHash) > 0 {
			if err := s.Store.SetRoutineWebhook(ctx, e.WorkspaceID, st.ID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// TickRoutines queues the cron routines that are due (the background
// loop, system scope; each item in its workspace).
func (s *Service) TickRoutines(ctx context.Context) (int, error) {
	now := s.Clock.Now()
	due, err := s.Store.DueRoutines(ctx, now, 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, st := range due {
		wctx := db.WithWorkspace(ctx, st.WorkspaceID)
		err := s.Tx.InTx(wctx, func(ctx context.Context) error {
			var next *time.Time
			e, err := s.Store.GetExtension(ctx, st.WorkspaceID, st.ExtensionID)
			if err == nil && e.State == StateLive && e.LiveVersion > 0 {
				v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, e.LiveVersion)
				if err != nil {
					return err
				}
				if r, ok := v.Manifest.Routine(st.Routine); ok && r.Trigger.Kind() == TriggerCron {
					if t, err := schedule.Next(r.Trigger.Cron, r.Trigger.Timezone, now); err == nil {
						next = &t
					}
					in, _ := json.Marshal(map[string]any{"kind": TriggerCron, "at": now})
					if err := s.Store.InsertRoutineRun(ctx, RoutineRun{ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, ExtensionID: e.ID,
						Routine: r.Name, Trigger: TriggerCron, Input: in, CreatedAt: now}); err != nil {
						return err
					}
					n++
				}
			}
			return s.Store.SetRoutineNext(ctx, st.WorkspaceID, st.ID, next, &now)
		})
		if err != nil && s.Logger != nil {
			s.Logger.Warn("workshop: routine tick", "routine", st.Routine, "err", err)
		}
	}
	return n, nil
}

// WorkRoutines runs the queued routine runs (the background loop, system
// scope). Each run goes in its own goroutine (an approval may take
// minutes); done is called when it ends (tests wait on it).
func (s *Service) WorkRoutines(ctx context.Context, done func()) (int, error) {
	runs, err := s.Store.ClaimRoutineRuns(ctx, s.Clock.Now(), 20)
	if err != nil {
		return 0, err
	}
	for _, run := range runs {
		go func(run RoutineRun) {
			wctx := db.WithWorkspace(ctx, run.WorkspaceID)
			state, msg := RunDone, ""
			if err := s.executeRun(wctx, run); err != nil {
				state, msg = RunFailed, errText(err)
				if s.Logger != nil {
					s.Logger.Info("workshop: routine failed", "routine", run.Routine, "err", msg)
				}
			}
			if err := s.Store.FinishRoutineRun(wctx, run.WorkspaceID, run.ID, state, msg, s.Clock.Now()); err != nil && s.Logger != nil {
				s.Logger.Warn("workshop: routine finish", "err", err)
			}
			if done != nil {
				done()
			}
		}(run)
	}
	return len(runs), nil
}

// RoutineLoop ticks the cron routines and works the queue.
func (s *Service) RoutineLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := s.TickRoutines(ctx); err != nil && ctx.Err() == nil && s.Logger != nil {
			s.Logger.Warn("workshop: routine tick", "err", err)
		}
		if _, err := s.WorkRoutines(ctx, nil); err != nil && ctx.Err() == nil && s.Logger != nil {
			s.Logger.Warn("workshop: routine work", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

type triggerData struct {
	Kind         string          `json:"kind"`
	MessageID    string          `json:"message_id,omitempty"`
	ThreadRootID string          `json:"thread_root_id,omitempty"`
	ChannelID    string          `json:"channel_id,omitempty"`
	Channel      string          `json:"channel,omitempty"`
	Author       string          `json:"author,omitempty"`
	Text         string          `json:"text,omitempty"`
	ActionID     string          `json:"action_id,omitempty"`
	Value        string          `json:"value,omitempty"`
	Label        string          `json:"label,omitempty"`
	Actor        string          `json:"actor,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

// executeRun runs one routine: a tool as the extension's bot (its answer
// posted by the bot), or the bot asking an agent.
func (s *Service) executeRun(ctx context.Context, run RoutineRun) error {
	e, err := s.Store.GetExtension(ctx, run.WorkspaceID, run.ExtensionID)
	if err != nil || e.State != StateLive || e.LiveVersion == 0 {
		return ErrNotLive
	}
	if err := s.checkMaintainer(ctx, e); err != nil {
		return err
	}
	v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, e.LiveVersion)
	if err != nil {
		return ErrNoLiveVersion
	}
	r, ok := v.Manifest.Routine(run.Routine)
	if !ok {
		return ErrNoRoutine
	}
	bot, err := s.Directory.ActorByID(ctx, e.BotID)
	if err != nil || bot.ArchivedAt != nil || bot.Halted {
		return identity.ErrHalted
	}
	maintainer, err := s.Directory.ActorByID(ctx, e.MaintainerID)
	if err != nil {
		return ErrMaintainerGone
	}
	var trig triggerData
	_ = json.Unmarshal(run.Input, &trig)
	// Where the bot answers: the output channel, or the triggering thread.
	var channelID, threadID ids.ID
	if r.Output != "" {
		if channelID, err = s.Channels.ChannelIDByName(ctx, maintainer, r.Output); err != nil {
			return ErrRoutineTarget
		}
	} else {
		channelID, _ = ids.Parse(trig.ChannelID)
		if threadID, err = ids.Parse(trig.ThreadRootID); err != nil || threadID == ids.Nil {
			threadID, _ = ids.Parse(trig.MessageID)
		}
	}
	if channelID == ids.Nil {
		return ErrRoutineTarget
	}
	if r.Agent != "" {
		return s.askAgent(ctx, bot, r, trig, channelID, threadID)
	}
	mt, ok := v.Manifest.Tool(r.Tool)
	if !ok {
		return ErrUnknownTool
	}
	input := map[string]any{}
	if len(r.Input) > 0 {
		_ = json.Unmarshal(r.Input, &input)
	}
	input["trigger"] = trig
	raw, _ := json.Marshal(input)
	current := func(ctx context.Context) (identity.Actor, error) { return s.Directory.ActorByID(ctx, bot.ID) }
	res, err := s.runAsBot(ctx, e, v, mt, bot, raw, channelID, threadID, current)
	if err != nil {
		return err
	}
	return s.postResult(ctx, bot, channelID, threadID, res)
}

// runAsBot is callExtension for a routine: the bot is the caller.
func (s *Service) runAsBot(ctx context.Context, e Extension, v Version, mt ManifestTool, bot identity.Actor, input []byte,
	channelID, threadID ids.ID, current func(context.Context) (identity.Actor, error)) (Result, error) {
	start := s.Clock.Now()
	call := ToolCall{ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, ActorID: bot.ID, TargetKind: TargetExtension, TargetID: e.ID,
		Tool: mt.Name, Risk: mt.Risk, StartedAt: start, InBytes: len(input)}
	release, err := s.acquire(e.WorkspaceID)
	if err != nil {
		s.finishCall(ctx, &call, start, Result{}, err)
		return Result{}, err
	}
	defer release()
	if err := s.checkBudget(ctx, e.WorkspaceID); err != nil {
		s.finishCall(ctx, &call, start, Result{}, err)
		return Result{}, err
	}
	if mt.Risk != RiskRead && !e.ApprovalOff {
		req, err := s.Approvals.CreateRequest(ctx, bot, approval.CreateInput{ChannelID: channelID, ThreadRootID: threadID,
			ToolName: ExtensionGatewayName(e.Slug, mt.Name), Input: string(input)})
		if err != nil {
			call.Error = errText(err)
			s.logDenied(ctx, call, start)
			return Result{}, err
		}
		call.ApprovalID = req.ID
		decided, err := s.Approvals.WaitForDecision(ctx, req, current)
		if err != nil || decided.State != approval.StateAllowed {
			call.Error = decided.State
			s.logDenied(ctx, call, start)
			return Result{}, fmt.Errorf("not approved (%s)", decided.State)
		}
		now, err := s.Store.GetExtension(ctx, e.WorkspaceID, e.ID)
		if err != nil || now.State != StateLive || now.LiveVersion != e.LiveVersion {
			call.Error = "changed while waiting for the approval"
			s.logDenied(ctx, call, start)
			return Result{}, ErrNotLive
		}
	}
	res, _, err := s.runTool(ctx, runCtx{ext: e, version: v, tool: mt, caller: bot, input: input})
	if err == nil {
		res = cutResult(res)
	}
	s.finishCall(ctx, &call, start, res, err)
	s.recordOutcome(ctx, e, err != nil || res.IsError)
	return res, err
}

// postResult posts a tool's answer as the bot: a JSON object with
// "blocks" (and "text") becomes an interactive message; empty posts
// nothing.
func (s *Service) postResult(ctx context.Context, bot identity.Actor, channelID, threadID ids.ID, res Result) error {
	text := strings.TrimSpace(res.Text)
	if text == "" || text == "null" || text == `""` {
		return nil
	}
	in := conversation.SendInput{ChannelID: channelID, ThreadRootID: threadID, Body: text, Elaborate: true}
	var obj struct {
		Text   string          `json:"text"`
		Blocks json.RawMessage `json:"blocks"`
	}
	if json.Unmarshal([]byte(text), &obj) == nil && (len(obj.Blocks) > 0 || obj.Text != "") {
		blocks, err := conversation.ParseBlocks(obj.Blocks)
		if err != nil {
			return err
		}
		in.Blocks, in.Body = blocks, obj.Text
	}
	if res.IsError {
		in.Body = "⚠ " + clip(text, 2000)
		in.Blocks = nil
	}
	_, err := s.Channels.SendMessage(ctx, bot, in)
	return err
}

// askAgent posts the routine's prompt as the bot, mentioning the agent;
// the agent wakes the usual way (and the usual rules of who may task whom
// apply to the bot). What triggered it goes along as data.
func (s *Service) askAgent(ctx context.Context, bot identity.Actor, r Routine, trig triggerData, channelID, threadID ids.ID) error {
	body := "@" + r.Agent + " " + strings.TrimSpace(r.Prompt)
	if trig.Kind != TriggerCron {
		data, _ := json.MarshalIndent(trig, "", "  ")
		body += "\n\n```json\n" + clip(string(data), 3000) + "\n```"
	}
	_, err := s.Channels.SendMessage(ctx, bot, conversation.SendInput{ChannelID: channelID, ThreadRootID: threadID, Body: body, Elaborate: true})
	return err
}

// HandleEvent queues routines on messages and clicks, and passes a click
// on an agent's message to that agent (the event consumer, in the event's
// workspace and transaction).
func (s *Service) HandleEvent(ctx context.Context, ev eventlog.Event) error {
	switch ev.Type {
	case conversation.EventMessageCreated:
		var p conversation.MessageCreated
		if json.Unmarshal(ev.Payload, &p) != nil || p.Silent || p.Kind != "text" {
			return nil
		}
		return s.onMessage(ctx, ev.WorkspaceID, p)
	case conversation.EventMessageAction:
		var p conversation.MessageAction
		if json.Unmarshal(ev.Payload, &p) != nil {
			return nil
		}
		return s.onAction(ctx, ev.WorkspaceID, p)
	}
	return nil
}

func (s *Service) liveRoutines(ctx context.Context, workspaceID ids.ID, kind string) ([]Extension, map[ids.ID]Version, map[ids.ID]bool, error) {
	es, err := s.Store.ListExtensions(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, err
	}
	bots := map[ids.ID]bool{}
	versions := map[ids.ID]Version{}
	var live []Extension
	for _, e := range es {
		bots[e.BotID] = true
		if e.State != StateLive || e.LiveVersion == 0 {
			continue
		}
		v, err := s.Store.GetVersion(ctx, workspaceID, e.ID, e.LiveVersion)
		if err != nil {
			continue
		}
		for _, r := range v.Manifest.Routines {
			if r.Trigger.Kind() == kind {
				live = append(live, e)
				versions[e.ID] = v
				break
			}
		}
	}
	return live, versions, bots, nil
}

func (s *Service) queue(ctx context.Context, e Extension, routine, trigger string, data triggerData) error {
	in, _ := json.Marshal(data)
	return s.Store.InsertRoutineRun(ctx, RoutineRun{ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, ExtensionID: e.ID, Routine: routine,
		Trigger: trigger, Input: in, CreatedAt: s.Clock.Now()})
}

func (s *Service) onMessage(ctx context.Context, workspaceID ids.ID, p conversation.MessageCreated) error {
	exts, versions, bots, err := s.liveRoutines(ctx, workspaceID, TriggerMessage)
	if err != nil || len(exts) == 0 {
		return err
	}
	author, _ := ids.Parse(p.AuthorID)
	if bots[author] {
		return nil // a bot's own message never triggers a routine (no loops)
	}
	chID, _ := ids.Parse(p.ChannelID)
	ch, err := s.Channels.ChannelByID(ctx, chID)
	if err != nil {
		return nil
	}
	handle := ""
	if a, err := s.Directory.ActorByID(ctx, author); err == nil {
		handle = "@" + a.Handle
	}
	for _, e := range exts {
		for _, r := range versions[e.ID].Manifest.Routines {
			mt := r.Trigger.Message
			if mt == nil || mt.Channel != ch.Name || (mt.Contains != "" && !strings.Contains(strings.ToLower(p.Body), strings.ToLower(mt.Contains))) {
				continue
			}
			// Only where its bot was put by an approval (a channel renamed
			// to the same name later is not watched; security review).
			if !s.Channels.IsMember(ctx, ch.ID, e.BotID) {
				continue
			}
			if err := s.queue(ctx, e, r.Name, TriggerMessage, triggerData{Kind: TriggerMessage, MessageID: p.MessageID,
				ThreadRootID: p.ThreadRootID, ChannelID: p.ChannelID, Channel: "#" + ch.Name, Author: handle, Text: clip(p.Body, 8000)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) onAction(ctx context.Context, workspaceID ids.ID, p conversation.MessageAction) error {
	authorID, _ := ids.Parse(p.MessageAuthorID)
	author, err := s.Directory.ActorByID(ctx, authorID)
	if err != nil || author.Kind != identity.KindAgent {
		return nil
	}
	actorID, _ := ids.Parse(p.ActorID)
	actor, err := s.Directory.ActorByID(ctx, actorID)
	if err != nil {
		return nil
	}
	es, err := s.Store.ListExtensions(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, e := range es {
		if e.BotID != author.ID {
			continue
		}
		// A click on an extension's message: its action routines.
		if e.State != StateLive || e.LiveVersion == 0 {
			return nil
		}
		v, err := s.Store.GetVersion(ctx, workspaceID, e.ID, e.LiveVersion)
		if err != nil {
			return nil
		}
		for _, r := range v.Manifest.Routines {
			if a := r.Trigger.Action; a == "*" || (a != "" && a == p.ActionID) {
				if err := s.queue(ctx, e, r.Name, TriggerAction, triggerData{Kind: TriggerAction, MessageID: p.MessageID,
					ThreadRootID: p.ThreadRootID, ChannelID: p.ChannelID, ActionID: p.ActionID, Value: p.Value, Label: p.Label,
					Actor: "@" + actor.Handle}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// A click on an agent's message: a reply in its thread, as the person
	// who clicked, mentioning the agent, so it wakes with the answer.
	chID, _ := ids.Parse(p.ChannelID)
	thread, _ := ids.Parse(p.ThreadRootID)
	if thread == ids.Nil {
		thread, _ = ids.Parse(p.MessageID)
	}
	_, err = s.Channels.SendMessage(ctx, actor, conversation.SendInput{ChannelID: chID, ThreadRootID: thread,
		Body: "@" + author.Handle + " ▶ " + strings.ReplaceAll(p.Label, "@", "")})
	return err
}

// CreateWebhook makes a new secret address for a webhook routine and
// returns its path (shown once; the token is kept only hashed). Owner and
// admins.
func (s *Service) CreateWebhook(ctx context.Context, by identity.Actor, id ids.ID, routine string) (string, error) {
	if err := admin(by); err != nil {
		return "", err
	}
	e, err := s.extension(ctx, by, id)
	if err != nil {
		return "", err
	}
	if e.LiveVersion == 0 {
		return "", ErrNoLiveVersion
	}
	v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, e.LiveVersion)
	if err != nil {
		return "", err
	}
	r, ok := v.Manifest.Routine(routine)
	if !ok {
		return "", ErrNoRoutine
	}
	if r.Trigger.Kind() != TriggerWebhook {
		return "", ErrNotWebhook
	}
	sts, err := s.Store.ListRoutineStates(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return "", err
	}
	for _, st := range sts {
		if st.Routine != routine {
			continue
		}
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		token := hex.EncodeToString(b)
		sum := sha256.Sum256([]byte(token))
		var seq int64
		err := s.Tx.InTx(ctx, func(ctx context.Context) error {
			if err := s.Store.SetRoutineWebhook(ctx, e.WorkspaceID, st.ID, sum[:]); err != nil {
				return err
			}
			var err error
			seq, err = s.emit(ctx, by, EventWebhookCreated, "extension", e.ID, ExtensionChanged{ExtensionID: e.ID.String(), Slug: e.Slug, Reason: routine})
			return err
		})
		if err != nil {
			return "", err
		}
		s.notify(seq)
		return fmt.Sprintf("/api/v1/hooks/ext/%s/%s/%s", e.WorkspaceID, st.ID, token), nil
	}
	return "", ErrNoRoutine
}

// ReceiveWebhook queues a webhook routine (no login: the secret address
// is the credential). ctx is the workspace's scope.
func (s *Service) ReceiveWebhook(ctx context.Context, workspaceID, stateID ids.ID, token string, body []byte) error {
	st, err := s.Store.GetRoutineState(ctx, workspaceID, stateID)
	if err != nil || len(st.WebhookTokenHash) == 0 {
		return ErrBadWebhook
	}
	sum := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(sum[:], st.WebhookTokenHash) != 1 {
		return ErrBadWebhook
	}
	e, err := s.Store.GetExtension(ctx, workspaceID, st.ExtensionID)
	if err != nil || e.State != StateLive || e.LiveVersion == 0 {
		return ErrNotLive
	}
	v, err := s.Store.GetVersion(ctx, workspaceID, e.ID, e.LiveVersion)
	if err != nil {
		return ErrNotLive
	}
	if r, ok := v.Manifest.Routine(st.Routine); !ok || r.Trigger.Kind() != TriggerWebhook {
		return ErrBadWebhook
	}
	payload := json.RawMessage(body)
	if !json.Valid(body) {
		payload, _ = json.Marshal(string(body))
	}
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		return s.queue(ctx, e, st.Routine, TriggerWebhook, triggerData{Kind: TriggerWebhook, Payload: payload})
	})
}

// RoutineView is a live routine with its state.
type RoutineView struct {
	Routine
	State *RoutineState
}

// Routines returns the live version's routines with their state, and the
// latest runs. Owner and admins.
func (s *Service) Routines(ctx context.Context, by identity.Actor, id ids.ID) ([]RoutineView, []RoutineRun, error) {
	if err := admin(by); err != nil {
		return nil, nil, err
	}
	e, err := s.extension(ctx, by, id)
	if err != nil {
		return nil, nil, err
	}
	runs, err := s.Store.ListRoutineRuns(ctx, e.WorkspaceID, e.ID, 30)
	if err != nil {
		return nil, nil, err
	}
	if e.LiveVersion == 0 {
		return nil, runs, nil
	}
	v, err := s.Store.GetVersion(ctx, e.WorkspaceID, e.ID, e.LiveVersion)
	if err != nil {
		return nil, nil, err
	}
	sts, err := s.Store.ListRoutineStates(ctx, e.WorkspaceID, e.ID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]RoutineView, 0, len(v.Manifest.Routines))
	for _, r := range v.Manifest.Routines {
		rv := RoutineView{Routine: r}
		for i := range sts {
			if sts[i].Routine == r.Name {
				rv.State = &sts[i]
			}
		}
		out = append(out, rv)
	}
	return out, runs, nil
}
