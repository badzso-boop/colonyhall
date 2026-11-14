// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package sidebar manages personal sidebar sections and custom channel ordering for humans.
package sidebar

import (
	"errors"
	"time"

	"colonyhall/internal/platform/ids"
)

// Section is a custom group in a user's sidebar.
type Section struct {
	ID          ids.ID     `json:"id"`
	WorkspaceID ids.ID     `json:"workspace_id"`
	OwnerID     ids.ID     `json:"owner_id"`
	Name        string     `json:"name"`
	Position    int        `json:"position"`
	Collapsed   bool       `json:"collapsed"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	Items       []Item     `json:"items"`
}

// Item is a channel or DM placed inside a section.
type Item struct {
	SectionID ids.ID `json:"section_id"`
	ChannelID ids.ID `json:"channel_id"`
	Position  int    `json:"position"`
}

// Errors.
var (
	ErrForbidden       = errors.New("forbidden: sidebar sections are for human users only")
	ErrSectionNotFound = errors.New("sidebar section not found")
	ErrInvalidName     = errors.New("section name must be 1-40 characters")
)
