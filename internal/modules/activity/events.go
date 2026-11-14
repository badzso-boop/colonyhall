// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package activity

// EventCreated tells the recipient (alone) that their feed grew, for the
// badge; EventRead that items were marked read (their other tabs). Ids and
// counts only.
const (
	EventCreated = "activity.created"
	EventRead    = "activity.read"
)

// Created is the payload of EventCreated.
type Created struct {
	ItemID string `json:"item_id"`
	Kind   Kind   `json:"kind"`
}

// Read is the payload of EventRead.
type Read struct {
	Marked int `json:"marked"`
}
