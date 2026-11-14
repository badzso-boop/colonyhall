// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package lease

import (
	"time"

	"colonyhall/internal/platform/ids"
)

const (
	ConsumerName  = "lease"
	EventAcquired = "lease.acquired"
	EventExtended = "lease.extended"
	EventReleased = "lease.released"
)

// AcquiredEvent is emitted when a lease is acquired.
type AcquiredEvent struct {
	LeaseID   ids.ID    `json:"lease_id"`
	Resource  string    `json:"resource"`
	HolderID  ids.ID    `json:"holder_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Note      string    `json:"note,omitempty"`
}

// ExtendedEvent is emitted when a lease is extended.
type ExtendedEvent struct {
	LeaseID      ids.ID    `json:"lease_id"`
	Resource     string    `json:"resource"`
	HolderID     ids.ID    `json:"holder_id"`
	NewExpiresAt time.Time `json:"new_expires_at"`
}

// ReleasedEvent is emitted when a lease is released.
type ReleasedEvent struct {
	LeaseID    ids.ID `json:"lease_id"`
	Resource   string `json:"resource"`
	HolderID   ids.ID `json:"holder_id"`
	ReleasedBy ids.ID `json:"released_by"`
	Reason     string `json:"reason,omitempty"`
}
