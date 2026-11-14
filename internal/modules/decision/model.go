// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package decision

import (
	"errors"
	"fmt"
	"time"

	"colonyhall/internal/platform/ids"
)

// Validation and domain errors.
var (
	ErrTextEmpty          = errors.New("decision text cannot be empty")
	ErrTextTooLong        = errors.New("decision text must be at most 300 characters")
	ErrRationaleEmpty     = errors.New("decision rationale cannot be empty")
	ErrRationaleTooLong   = errors.New("decision rationale must be at most 500 characters")
	ErrSupersededNotFound = errors.New("superseded decision not found")
	ErrForbidden          = errors.New("forbidden")
)

// Decision represents a recorded decision in a workspace.
type Decision struct {
	ID           ids.ID
	Number       int
	WorkspaceID  ids.ID
	AuthorID     ids.ID
	ChannelID    ids.ID
	ThreadRootID *ids.ID
	Text         string
	Rationale    string
	Supersedes   *int
	SupersededBy *int
	CreatedAt    time.Time
}

// FormattedNumber returns the reference string (e.g. "D-12").
func (d Decision) FormattedNumber() string {
	return fmt.Sprintf("D-%d", d.Number)
}
