// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package operator

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Customer is what the operator sees of a workspace: account data and
// numbers, never content (no channel, message, file or person names).
type Customer struct {
	ID              ids.ID
	Name            string
	Slug            string
	State           string
	SuspendedAt     *time.Time
	SuspendedReason string
	CreatedAt       time.Time
	HasOwner        bool
	// Invite is the newest owner invite, when there is one.
	Invite       *InviteState
	Users        int
	Agents       int
	Hosts        int
	HostsOnline  int
	LastActivity *time.Time
	Storage      Storage
}

// InviteState is an invite without its token.
type InviteState struct {
	CreatedAt time.Time
	ExpiresAt time.Time
	Used      bool
	Revoked   bool
}

// Storage is the workspace storage of round 54.
type Storage struct {
	UsedBytes  int64
	QuotaBytes int64
	FilesBytes int64
	DBBytes    int64
	// State: ok, warning_80, warning_95, full.
	State string
}

// InviteLink is a fresh invite: its token is shown once.
type InviteLink struct {
	Token     string
	ExpiresAt time.Time
}

// Customers is the operator's view of the workspaces, implemented by the
// composition root over the modules' APIs (each workspace read in its own
// narrowed scope). The operator scope itself never sees workspace rows.
type Customers interface {
	List(ctx context.Context) ([]Customer, error)
	// Get: db.ErrNotFound for an unknown workspace.
	Get(ctx context.Context, id ids.ID) (Customer, error)
	Create(ctx context.Context, name, slug string) (Customer, error)
	// Invite revokes the open invite and makes a new one; an error when
	// the workspace has an owner.
	Invite(ctx context.Context, id, operatorID ids.ID) (InviteLink, error)
	SetSuspended(ctx context.Context, id ids.ID, suspend bool, reason string) (Customer, error)
}
