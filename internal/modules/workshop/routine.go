// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/platform/ids"
)

// Routine triggers (docs/25 13.8).
const (
	TriggerCron    = "cron"
	TriggerMessage = "message"
	TriggerWebhook = "webhook"
	TriggerAction  = "action"
)

// Run states.
const (
	RunPending = "pending"
	RunRunning = "running"
	RunDone    = "done"
	RunFailed  = "failed"
)

// MaxWebhookBody bounds a webhook's payload.
const MaxWebhookBody = 256 * 1024

// MaxRoutines bounds an extension's routines.
const MaxRoutines = 20

// Routine is one routine of a manifest: a trigger and either a tool that
// runs directly (its answer is posted by the extension's bot) or an agent
// the bot asks with a prompt.
type Routine struct {
	Name    string          `json:"name"`
	Trigger RoutineTrigger  `json:"trigger"`
	Tool    string          `json:"tool,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Agent   string          `json:"agent,omitempty"`
	Prompt  string          `json:"prompt,omitempty"`
	// Output is the channel the bot posts in (a message or action trigger
	// answers in the triggering thread when it is empty).
	Output string `json:"output,omitempty"`
}

// RoutineTrigger: exactly one of its fields.
type RoutineTrigger struct {
	Cron     string          `json:"cron,omitempty"`
	Timezone string          `json:"timezone,omitempty"`
	Message  *MessageTrigger `json:"message,omitempty"`
	Webhook  bool            `json:"webhook,omitempty"`
	// Action: a click on the bot's message with this action_id ("*": any).
	Action string `json:"action,omitempty"`
}

// MessageTrigger: a message in a channel, optionally containing a text.
type MessageTrigger struct {
	Channel  string `json:"channel"`
	Contains string `json:"contains,omitempty"`
}

// Kind returns the trigger's kind.
func (t RoutineTrigger) Kind() string {
	switch {
	case t.Cron != "":
		return TriggerCron
	case t.Message != nil:
		return TriggerMessage
	case t.Webhook:
		return TriggerWebhook
	case t.Action != "":
		return TriggerAction
	}
	return ""
}

var (
	channelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)
	handleRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
)

func normChannel(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), "#") }

// normalizeRoutines checks the manifest's routines.
func (m *Manifest) normalizeRoutines() error {
	if len(m.Routines) > MaxRoutines {
		return manifestErr("at most %d routines", MaxRoutines)
	}
	names := map[string]bool{}
	for i := range m.Routines {
		r := &m.Routines[i]
		if !ValidToolName(r.Name) || names[r.Name] {
			return manifestErr("routine name %q: 1–24 letters, digits, _ or -, unique", r.Name)
		}
		names[r.Name] = true
		kinds := 0
		for _, ok := range []bool{r.Trigger.Cron != "", r.Trigger.Message != nil, r.Trigger.Webhook, r.Trigger.Action != ""} {
			if ok {
				kinds++
			}
		}
		if kinds != 1 {
			return manifestErr("routine %q: give exactly one trigger: cron, message, webhook or action", r.Name)
		}
		switch r.Trigger.Kind() {
		case TriggerCron:
			if r.Trigger.Timezone == "" {
				r.Trigger.Timezone = schedule.DefaultTimezone
			}
			if _, err := schedule.Next(r.Trigger.Cron, r.Trigger.Timezone, time.Now()); err != nil {
				return manifestErr("routine %q: %s", r.Name, err.Error())
			}
		case TriggerMessage:
			r.Trigger.Message.Channel = normChannel(r.Trigger.Message.Channel)
			if !channelRe.MatchString(r.Trigger.Message.Channel) || utf8.RuneCountInString(r.Trigger.Message.Contains) > 200 {
				return manifestErr("routine %q: the message trigger needs a channel", r.Name)
			}
		case TriggerAction:
			if r.Trigger.Action != "*" && !regexp.MustCompile(`^[a-z0-9_-]{1,64}$`).MatchString(r.Trigger.Action) {
				return manifestErr("routine %q: action is an action_id or *", r.Name)
			}
		}
		r.Output = normChannel(r.Output)
		if r.Output != "" && !channelRe.MatchString(r.Output) {
			return manifestErr("routine %q: output must be a channel name", r.Name)
		}
		answersInThread := r.Trigger.Kind() == TriggerMessage || r.Trigger.Kind() == TriggerAction
		if r.Output == "" && !answersInThread {
			return manifestErr("routine %q: give the output channel", r.Name)
		}
		switch {
		case r.Tool != "" && r.Agent == "":
			if _, ok := m.Tool(r.Tool); !ok {
				return manifestErr("routine %q: unknown tool %q", r.Name, r.Tool)
			}
			if len(r.Input) > 0 {
				var obj map[string]any
				if json.Unmarshal(r.Input, &obj) != nil {
					return manifestErr("routine %q: input must be a JSON object", r.Name)
				}
			}
			r.Prompt = ""
		case r.Agent != "" && r.Tool == "":
			r.Agent = strings.TrimPrefix(strings.TrimSpace(r.Agent), "@")
			if !handleRe.MatchString(r.Agent) || strings.TrimSpace(r.Prompt) == "" || utf8.RuneCountInString(r.Prompt) > 4000 {
				return manifestErr("routine %q: an agent routine needs the agent's handle and a prompt (max 4000 characters)", r.Name)
			}
			r.Input = nil
		default:
			return manifestErr("routine %q: give either a tool or an agent with a prompt", r.Name)
		}
	}
	return nil
}

// Routine returns a manifest routine by name.
func (m Manifest) Routine(name string) (Routine, bool) {
	for _, r := range m.Routines {
		if r.Name == name {
			return r, true
		}
	}
	return Routine{}, false
}

// RoutineState is a routine's schedule and webhook.
type RoutineState struct {
	ID               ids.ID
	WorkspaceID      ids.ID
	ExtensionID      ids.ID
	Routine          string
	NextRunAt        *time.Time
	LastRunAt        *time.Time
	WebhookTokenHash []byte
}

// RoutineRun is one queued or finished run.
type RoutineRun struct {
	ID          ids.ID
	WorkspaceID ids.ID
	ExtensionID ids.ID
	Routine     string
	Trigger     string
	Input       json.RawMessage
	State       string
	Error       string
	CreatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
}
