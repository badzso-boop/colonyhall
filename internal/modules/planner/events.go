// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package planner

// Event types. Payloads carry ids and states only: the text is the owner's.
const (
	EventTaskCreated       = "planner.task_created"
	EventTaskUpdated       = "planner.task_updated"
	EventTaskStatusChanged = "planner.task_status_changed"
	EventTaskMoved         = "planner.task_moved"
	EventTaskDeleted       = "planner.task_deleted"
	EventCalendarsChanged  = "planner.calendars_changed"
)

// TaskEvent is the payload of every task event.
type TaskEvent struct {
	TaskID    string `json:"task_id"`
	OwnerID   string `json:"owner_id"`
	OldStatus string `json:"old_status,omitempty"`
	NewStatus string `json:"new_status,omitempty"`
}

// CalendarsEvent is the payload of EventCalendarsChanged.
type CalendarsEvent struct {
	OwnerID    string `json:"owner_id"`
	CalendarID string `json:"calendar_id,omitempty"`
}
