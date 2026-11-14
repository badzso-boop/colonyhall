// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"colonyhall/internal/platform/ids"
)

// Workspace states (round 45). A suspended workspace answers like an
// unknown one on its subdomain.
const (
	WorkspaceActive    = "active"
	WorkspaceSuspended = "suspended"
)

// ErrInvalidSlug is returned for a slug that breaks the rules or is reserved.
var ErrInvalidSlug = errors.New("invalid workspace slug: 3-40 lower-case letters, digits and single hyphens, not starting or ending with a hyphen, not reserved")

// ReservedSlugs are subdomains a workspace may not take. Migration 00051
// keeps a copy of this list; change both together.
var ReservedSlugs = []string{
	"admin", "api", "app", "www", "mail", "smtp", "imap", "pop", "ftp", "ns1", "ns2",
	"static", "assets", "cdn", "status", "docs", "help", "support", "billing", "auth",
	"login", "sso", "id", "operator", "root", "system", "localhost", "test", "dev",
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

// ValidSlug reports whether s may name a workspace's subdomain.
func ValidSlug(s string) bool {
	return slugRe.MatchString(s) && !strings.Contains(s, "--") && !slices.Contains(ReservedSlugs, s)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// SlugFromName derives a slug from a workspace name; when the name gives no
// valid slug, one is made from the id (always valid and unique).
func SlugFromName(name string, id ids.ID) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if ValidSlug(s) {
		return s
	}
	return IDSlug(id)
}

// IDSlug is the fallback slug of a workspace: "ws-" and the first twelve
// hex digits of its id (migration 00051 uses the same rule).
func IDSlug(id ids.ID) string {
	return "ws-" + strings.ReplaceAll(id.String(), "-", "")[:12]
}

// WithDefaults fills an empty slug (IDSlug) and state (active).
func WithDefaults(w Workspace) Workspace {
	if w.Slug == "" {
		w.Slug = IDSlug(w.ID)
	}
	if w.State == "" {
		w.State = WorkspaceActive
	}
	return w
}
