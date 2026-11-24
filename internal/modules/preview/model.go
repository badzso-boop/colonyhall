// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package preview

import (
	"errors"
	"path"
	"regexp"
	"strings"
	"time"

	"colonyhall/internal/platform/ids"
)

// Preview states.
const (
	StateRequested = "requested"
	StateRunning   = "running"
	StateStopped   = "stopped"
	StateFailed    = "failed"
)

// Limits and defaults.
const (
	DefaultTTL = 4 * time.Hour
	MaxTTL     = 24 * time.Hour
	MaxNameLen = 50
)

// Errors.
var (
	ErrForbidden      = errors.New("not allowed")
	ErrNotFound       = errors.New("preview not found")
	ErrNotAllowed     = errors.New("preview is not enabled for this agent")
	ErrBadName        = errors.New("invalid preview name (must be alphanumeric/dashes, 1-50 chars)")
	ErrBadPath        = errors.New("invalid compose path (must be a clean relative .yaml or .yml path)")
	ErrBadTTL         = errors.New("invalid ttl (must be between 1 minute and 24 hours)")
	ErrAlreadyRunning = errors.New("a preview with this name is already active")
	// ErrOffline: the preview exists but is not running (or has no address
	// yet); the proxy answers 502.
	ErrOffline = errors.New("the preview is not running")
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,49}$`)

// ValidateName checks that a preview name is valid for a docker compose project.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > MaxNameLen || !nameRegex.MatchString(name) {
		return ErrBadName
	}
	return nil
}

// ValidateComposePath checks that a compose file path is a safe relative path within the agent's work directory.
func ValidateComposePath(p string) error {
	p = strings.TrimSpace(p)
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return ErrBadPath
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ErrBadPath
	}
	ext := path.Ext(clean)
	if ext != ".yaml" && ext != ".yml" {
		return ErrBadPath
	}
	return nil
}

// Preview represents an agent's application running via docker compose.
type Preview struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	AgentID      ids.ID
	HostID       ids.ID
	ChannelID    ids.ID
	ThreadRootID *ids.ID
	Name         string
	ComposePath  string
	State        string
	URL          string
	Error        string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
