// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package operator

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Store keeps operators, their backup codes, sessions and audit. Every
// call runs in the operator scope (row-level security: the operator
// tables only).
type Store interface {
	// CreateOperator: db.ErrConflict when the handle is taken.
	CreateOperator(ctx context.Context, o Operator) error
	OperatorByHandle(ctx context.Context, handle string) (Operator, error)
	OperatorByID(ctx context.Context, id ids.ID) (Operator, error)
	ListOperators(ctx context.Context) ([]Operator, error)
	// SetTOTP replaces the secret and the state (enrollment, reset) and
	// resets the last step.
	SetTOTP(ctx context.Context, id ids.ID, totp secretbox.Sealed, state string) error
	// ResealTOTP replaces the ciphertext only (key rotation).
	ResealTOTP(ctx context.Context, id ids.ID, totp secretbox.Sealed) error
	// AdvanceTOTPStep records an accepted step; false when it was not
	// after the last one (a replayed code).
	AdvanceTOTPStep(ctx context.Context, id ids.ID, step int64) (bool, error)
	SetState(ctx context.Context, id ids.ID, state string, activatedAt *time.Time) error
	SetLastLogin(ctx context.Context, id ids.ID, at time.Time) error

	// ReplaceBackupCodes deletes the operator's codes and stores these.
	ReplaceBackupCodes(ctx context.Context, operatorID ids.ID, codes []BackupCode, at time.Time) error
	// UseBackupCode marks an unused code used; false when there is none.
	UseBackupCode(ctx context.Context, operatorID ids.ID, hash []byte, at time.Time) (bool, error)
	UnusedBackupCodes(ctx context.Context, operatorID ids.ID) (int, error)

	CreateSession(ctx context.Context, s Session) error
	// SessionByTokenHash returns a session (revoked or not).
	SessionByTokenHash(ctx context.Context, hash []byte) (Session, error)
	TouchSession(ctx context.Context, id ids.ID, at time.Time) error
	RevokeSession(ctx context.Context, id ids.ID, at time.Time) error
	RevokeSessions(ctx context.Context, operatorID ids.ID, at time.Time) error
	// DeleteEndedSessions deletes sessions that ended before before.
	DeleteEndedSessions(ctx context.Context, before time.Time) (int64, error)

	// Plans (round 51b). CreatePlan and UpdatePlan: db.ErrConflict when
	// the name is taken.
	CreatePlan(ctx context.Context, p Plan) error
	PlanByID(ctx context.Context, id ids.ID) (Plan, error)
	ListPlans(ctx context.Context) ([]Plan, error)
	UpdatePlan(ctx context.Context, p Plan) error

	AppendAudit(ctx context.Context, e AuditEntry) error
	// ListAudit returns entries newest first, before the given time (zero:
	// from the newest).
	ListAudit(ctx context.Context, before time.Time, limit int) ([]AuditEntry, error)
}
