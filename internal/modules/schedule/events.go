// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package schedule

// Event types (audit log; admins see them live).
const (
	EventCreated  = "schedule.created"
	EventCanceled = "schedule.canceled"
	EventFired    = "schedule.fired"
)

// Event is the payload of schedule events.
type Event struct {
	ScheduleID string `json:"schedule_id"`
	ChannelID  string `json:"channel_id,omitempty"`
	Cron       string `json:"cron,omitempty"`
	NextRunAt  string `json:"next_run_at,omitempty"`
	MessageID  string `json:"message_id,omitempty"`
	Late       bool   `json:"late,omitempty"`
	Error      string `json:"error,omitempty"`
}
