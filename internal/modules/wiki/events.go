// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package wiki

import "colonyhall/internal/platform/ids"

// Event types (audit log).
const (
	EventCreated  = "wiki.created"
	EventAppended = "wiki.appended"
	EventEdited   = "wiki.edited"
	EventDeleted  = "wiki.deleted"
	EventRestored = "wiki.restored"
)

// Event is the payload of wiki audit events.
type Event struct {
	PageID ids.ID `json:"page_id"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Rev    int    `json:"rev"`
	Kind   string `json:"kind"`
}
