// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package secrets keeps a workspace's encrypted secrets (round 56a,
// docs/25-onfejlesztes-terv.md 13.2; the table of docs/24 2.3 brought
// forward): connector tokens now, the hosted guard's AI and repository keys
// with 51c. A value is sealed with secretbox (COLONY_SECRET_KEYS), bound to
// its row and workspace, and never shown again whole: only its hint (the
// last 4 characters). Only the server reads it back, to put it on an
// outbound request; it never goes to the chat, a log, an event or a model.
package secrets

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Kinds.
const (
	KindConnectorToken  = "connector_token"
	KindConnectorHeader = "connector_header"
	KindConnectorBasic  = "connector_basic"
)

// MaxValue bounds a secret's length.
const MaxValue = 8192

// Errors.
var (
	ErrForbidden = errors.New("only the owner and the admins manage secrets")
	// ErrNoKeys: the server has no COLONY_SECRET_KEYS.
	ErrNoKeys   = errors.New("the server has no encryption key (COLONY_SECRET_KEYS); ask the server's owner to set it")
	ErrBadValue = errors.New("the secret is empty or longer than 8192 characters")
	ErrBadName  = errors.New("the name must be 1–80 characters")
	ErrBadKind  = errors.New("unknown secret kind")
	ErrNotFound = errors.New("secret not found")
)

// Secret is one stored secret. Sealed never leaves the server.
type Secret struct {
	ID          ids.ID
	WorkspaceID ids.ID
	Kind        string
	Name        string
	Provider    string
	Sealed      secretbox.Sealed
	Hint        string
	CreatedBy   ids.ID
	CreatedAt   time.Time
	RotatedAt   *time.Time
	DeletedAt   *time.Time
}

func validKind(k string) bool {
	switch k {
	case KindConnectorToken, KindConnectorHeader, KindConnectorBasic:
		return true
	}
	return false
}

// hint returns the last 4 characters of a value long enough to keep the
// rest secret; shorter values get no hint.
func hint(v string) string {
	r := []rune(v)
	if len(r) < 12 {
		return ""
	}
	return string(r[len(r)-4:])
}

func checkValue(v string) error {
	if strings.TrimSpace(v) == "" || utf8.RuneCountInString(v) > MaxValue {
		return ErrBadValue
	}
	return nil
}

func aad(workspaceID, id ids.ID, kind string) []byte {
	return secretbox.AAD("workspace_secret", id.String(), workspaceID.String(), kind)
}
