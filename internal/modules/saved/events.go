// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package saved

// Events. They carry ids only (never the note) and reach the owner alone.
const (
	// EventChanged: an item was saved, changed or removed (other tabs).
	EventChanged = "saved.changed"
	// EventReminded: a reminder was due and posted (the activity view).
	EventReminded = "saved.reminded"
)

// Changed is the payload of EventChanged.
type Changed struct {
	ItemID    string `json:"item_id"`
	MessageID string `json:"message_id,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

// Reminded is the payload of EventReminded.
type Reminded struct {
	ItemID          string `json:"item_id"`
	MessageID       string `json:"message_id,omitempty"`
	ChannelID       string `json:"channel_id,omitempty"`
	ReminderMessage string `json:"reminder_message_id"`
	ReminderChannel string `json:"reminder_channel_id"`
}
