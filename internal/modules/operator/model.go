// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package operator is the platform operator (round 51,
// docs/24-hosztolt-ajanlat.md): the people who run a multi-tenant server.
// An operator is not an actor of any workspace: own tables, own sessions,
// own address (admin.<domain>), password + TOTP with backup codes. Every
// call runs in the operator database scope (opscope), which sees the
// operator tables and no workspace's rows; what the operator learns about
// customers comes through the Customers port, without content.
package operator

import (
	"errors"
	"regexp"
	"time"

	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Operator states.
const (
	// StatePending: created, the first TOTP code not confirmed yet.
	StatePending  = "pending"
	StateActive   = "active"
	StateDisabled = "disabled"
)

// Operator is a platform operator.
type Operator struct {
	ID           ids.ID
	Handle       string
	DisplayName  string
	PasswordHash string
	// TOTP is the encrypted TOTP secret.
	TOTP         secretbox.Sealed
	TOTPLastStep int64
	State        string
	CreatedAt    time.Time
	ActivatedAt  *time.Time
	LastLoginAt  *time.Time
}

// Session is an operator's browser session. Only the hash of the cookie
// value is stored.
type Session struct {
	ID         ids.ID
	OperatorID ids.ID
	TokenHash  []byte
	CSRFToken  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	RemoteAddr string
	UserAgent  string
	RevokedAt  *time.Time
}

// Session limits: short, because an operator session reaches every
// customer's account data.
const (
	SessionTTL  = 8 * time.Hour
	SessionIdle = 30 * time.Minute
	// touchEvery throttles the last-seen writes.
	touchEvery = time.Minute
)

// Live reports whether the session is usable at now.
func (s Session) Live(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt) && now.Before(s.LastSeenAt.Add(SessionIdle))
}

// AuditEntry is one operator action. Payloads never carry secrets.
type AuditEntry struct {
	ID ids.ID
	// OperatorID is ids.Nil for a command run on the server.
	OperatorID  ids.ID
	Action      string
	WorkspaceID ids.ID
	Payload     map[string]any
	RemoteAddr  string
	CreatedAt   time.Time
}

// Audit actions.
const (
	ActionCreated            = "operator.created"
	ActionActivated          = "operator.activated"
	ActionTOTPReset          = "operator.totp_reset"
	ActionDisabled           = "operator.disabled"
	ActionLogin              = "operator.login"
	ActionLoginFailed        = "operator.login_failed"
	ActionLogout             = "operator.logout"
	ActionBackupCodeUsed     = "operator.backup_code_used"
	ActionBackupCodesRenewed = "operator.backup_codes_renewed"
	ActionSecretsRotated     = "operator.secrets_rotated"
	ActionWorkspaceCreated   = "workspace.created"
	ActionInviteIssued       = "workspace.invite_issued"
	ActionWorkspaceSuspended = "workspace.suspended"
	ActionWorkspaceRestored  = "workspace.restored"
)

// BackupCode is a stored backup code: only its hash.
type BackupCode struct {
	ID   ids.ID
	Hash []byte
}

// BackupCodeCount is how many backup codes an operator gets.
const BackupCodeCount = 10

// Errors.
var (
	// ErrBadCredentials: wrong handle, password or code; one answer for all.
	ErrBadCredentials = errors.New("wrong handle, password or code")
	ErrUnauthorized   = errors.New("operator login required")
	ErrInvalidHandle  = errors.New("an operator handle is 2–32 lower-case letters, digits, '-' or '_'")
	ErrExists         = errors.New("an operator with this handle already exists")
	ErrNotPending     = errors.New("the operator has no TOTP enrollment waiting for its first code")
	ErrBadCode        = errors.New("the code is not valid; check the authenticator app's time")
	ErrNoKeys         = errors.New("operator accounts need COLONY_SECRET_KEYS")
	ErrNotMultitenant = errors.New("the operator exists only in multi-tenant mode (COLONY_MULTITENANT=1)")
)

var handleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// ValidHandle reports whether h is a valid operator handle.
func ValidHandle(h string) bool { return handleRe.MatchString(h) }

// totpAAD binds an operator's TOTP secret to its row.
func totpAAD(id ids.ID) []byte { return secretbox.AAD("operator", id.String(), "", "totp") }
