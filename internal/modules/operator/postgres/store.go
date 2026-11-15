// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres is the Postgres operator store.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/operator"
	"colonyhall/internal/modules/operator/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Store is the Postgres operator store.
type Store struct {
	DB *db.Postgres
}

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func notFound[T any](v T, err error) (T, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return v, db.ErrNotFound
	}
	return v, err
}

func rows(n int64, err error) error {
	if err == nil && n == 0 {
		return db.ErrNotFound
	}
	return err
}

func toOperator(r sqlcgen.Operator) operator.Operator {
	return operator.Operator{ID: r.ID, Handle: r.Handle, DisplayName: r.DisplayName, PasswordHash: r.PasswordHash,
		TOTP: secretbox.Sealed{KeyID: r.TotpKeyID, Nonce: r.TotpNonce, Ciphertext: r.TotpCiphertext}, TOTPLastStep: r.TotpLastStep,
		State: r.State, CreatedAt: r.CreatedAt, ActivatedAt: r.ActivatedAt, LastLoginAt: r.LastLoginAt}
}

// CreateOperator stores an operator.
func (s *Store) CreateOperator(ctx context.Context, o operator.Operator) error {
	err := s.q(ctx).CreateOperator(ctx, sqlcgen.CreateOperatorParams{ID: o.ID, Handle: o.Handle, DisplayName: o.DisplayName,
		PasswordHash: o.PasswordHash, TotpKeyID: o.TOTP.KeyID, TotpNonce: o.TOTP.Nonce, TotpCiphertext: o.TOTP.Ciphertext,
		State: o.State, CreatedAt: o.CreatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// OperatorByHandle finds an operator by handle.
func (s *Store) OperatorByHandle(ctx context.Context, handle string) (operator.Operator, error) {
	r, err := notFound(s.q(ctx).OperatorByHandle(ctx, handle))
	return toOperator(r), err
}

// OperatorByID finds an operator.
func (s *Store) OperatorByID(ctx context.Context, id ids.ID) (operator.Operator, error) {
	r, err := notFound(s.q(ctx).OperatorByID(ctx, id))
	return toOperator(r), err
}

// ListOperators lists every operator by handle.
func (s *Store) ListOperators(ctx context.Context) ([]operator.Operator, error) {
	list, err := s.q(ctx).ListOperators(ctx)
	out := make([]operator.Operator, 0, len(list))
	for _, r := range list {
		out = append(out, toOperator(r))
	}
	return out, err
}

// SetTOTP replaces the secret and the state and resets the last step.
func (s *Store) SetTOTP(ctx context.Context, id ids.ID, t secretbox.Sealed, state string) error {
	return rows(s.q(ctx).SetTOTP(ctx, sqlcgen.SetTOTPParams{ID: id, KeyID: t.KeyID, Nonce: t.Nonce, Ciphertext: t.Ciphertext, State: state}))
}

// ResealTOTP replaces the ciphertext.
func (s *Store) ResealTOTP(ctx context.Context, id ids.ID, t secretbox.Sealed) error {
	return rows(s.q(ctx).ResealTOTP(ctx, sqlcgen.ResealTOTPParams{ID: id, KeyID: t.KeyID, Nonce: t.Nonce, Ciphertext: t.Ciphertext}))
}

// AdvanceTOTPStep records an accepted step after the last one.
func (s *Store) AdvanceTOTPStep(ctx context.Context, id ids.ID, step int64) (bool, error) {
	n, err := s.q(ctx).AdvanceTOTPStep(ctx, sqlcgen.AdvanceTOTPStepParams{ID: id, Step: step})
	return n == 1, err
}

// SetState sets the state and the activation time.
func (s *Store) SetState(ctx context.Context, id ids.ID, state string, activatedAt *time.Time) error {
	return rows(s.q(ctx).SetOperatorState(ctx, sqlcgen.SetOperatorStateParams{ID: id, State: state, ActivatedAt: activatedAt}))
}

// SetLastLogin records a login.
func (s *Store) SetLastLogin(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).SetLastLogin(ctx, sqlcgen.SetLastLoginParams{ID: id, At: &at})
}

// ReplaceBackupCodes replaces the operator's codes.
func (s *Store) ReplaceBackupCodes(ctx context.Context, operatorID ids.ID, codes []operator.BackupCode, at time.Time) error {
	if err := s.q(ctx).DeleteBackupCodes(ctx, operatorID); err != nil {
		return err
	}
	for _, c := range codes {
		if err := s.q(ctx).InsertBackupCode(ctx, sqlcgen.InsertBackupCodeParams{ID: c.ID, OperatorID: operatorID, CodeHash: c.Hash, CreatedAt: at}); err != nil {
			return err
		}
	}
	return nil
}

// UseBackupCode marks an unused code used.
func (s *Store) UseBackupCode(ctx context.Context, operatorID ids.ID, hash []byte, at time.Time) (bool, error) {
	n, err := s.q(ctx).UseBackupCode(ctx, sqlcgen.UseBackupCodeParams{OperatorID: operatorID, CodeHash: hash, At: &at})
	return n == 1, err
}

// UnusedBackupCodes counts the operator's unused codes.
func (s *Store) UnusedBackupCodes(ctx context.Context, operatorID ids.ID) (int, error) {
	n, err := s.q(ctx).UnusedBackupCodes(ctx, operatorID)
	return int(n), err
}

// CreateSession stores a session.
func (s *Store) CreateSession(ctx context.Context, x operator.Session) error {
	return s.q(ctx).CreateSession(ctx, sqlcgen.CreateSessionParams{ID: x.ID, OperatorID: x.OperatorID, TokenHash: x.TokenHash,
		CsrfToken: x.CSRFToken, CreatedAt: x.CreatedAt, LastSeenAt: x.LastSeenAt, ExpiresAt: x.ExpiresAt, RemoteAddr: x.RemoteAddr, UserAgent: x.UserAgent})
}

// SessionByTokenHash finds a session.
func (s *Store) SessionByTokenHash(ctx context.Context, hash []byte) (operator.Session, error) {
	r, err := notFound(s.q(ctx).SessionByTokenHash(ctx, hash))
	return operator.Session{ID: r.ID, OperatorID: r.OperatorID, TokenHash: r.TokenHash, CSRFToken: r.CsrfToken, CreatedAt: r.CreatedAt,
		LastSeenAt: r.LastSeenAt, ExpiresAt: r.ExpiresAt, RemoteAddr: r.RemoteAddr, UserAgent: r.UserAgent, RevokedAt: r.RevokedAt}, err
}

// TouchSession records a use.
func (s *Store) TouchSession(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).TouchSession(ctx, sqlcgen.TouchSessionParams{ID: id, At: at})
}

// RevokeSession ends a session.
func (s *Store) RevokeSession(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).RevokeSession(ctx, sqlcgen.RevokeSessionParams{ID: id, At: &at})
}

// RevokeSessions ends every session of the operator.
func (s *Store) RevokeSessions(ctx context.Context, operatorID ids.ID, at time.Time) error {
	return s.q(ctx).RevokeSessions(ctx, sqlcgen.RevokeSessionsParams{OperatorID: operatorID, At: &at})
}

// DeleteEndedSessions deletes sessions that ended before before.
func (s *Store) DeleteEndedSessions(ctx context.Context, before time.Time) (int64, error) {
	return s.q(ctx).DeleteEndedSessions(ctx, sqlcgen.DeleteEndedSessionsParams{Before: before, IdleBefore: before.Add(-operator.SessionIdle)})
}

func optID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}

// AppendAudit appends an entry.
func (s *Store) AppendAudit(ctx context.Context, e operator.AuditEntry) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return err
	}
	return s.q(ctx).AppendAudit(ctx, sqlcgen.AppendAuditParams{ID: e.ID, OperatorID: optID(e.OperatorID), Action: e.Action,
		WorkspaceID: optID(e.WorkspaceID), Payload: payload, RemoteAddr: e.RemoteAddr, CreatedAt: e.CreatedAt})
}

// ListAudit returns entries newest first, before the given time.
func (s *Store) ListAudit(ctx context.Context, before time.Time, limit int) ([]operator.AuditEntry, error) {
	var b *time.Time
	if !before.IsZero() {
		b = &before
	}
	list, err := s.q(ctx).ListAudit(ctx, sqlcgen.ListAuditParams{Before: b, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]operator.AuditEntry, 0, len(list))
	for _, r := range list {
		e := operator.AuditEntry{ID: r.ID, Action: r.Action, RemoteAddr: r.RemoteAddr, CreatedAt: r.CreatedAt}
		if r.OperatorID != nil {
			e.OperatorID = *r.OperatorID
		}
		if r.WorkspaceID != nil {
			e.WorkspaceID = *r.WorkspaceID
		}
		if err := json.Unmarshal(r.Payload, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// Customers implements operator.Overview through colony_operator_overview(),
// in the caller's (operator) scope.
func (s *Store) Customers(ctx context.Context, only ids.ID, onlineSince time.Time) ([]operator.Customer, error) {
	rows, err := s.q(ctx).Overview(ctx, sqlcgen.OverviewParams{OnlyWs: uuid.UUID(only), OnlineSince: onlineSince})
	if err != nil {
		return nil, err
	}
	out := make([]operator.Customer, 0, len(rows))
	for _, r := range rows {
		c := operator.Customer{ID: ids.ID(r.ID), Name: r.Name, Slug: r.Slug, State: r.State, SuspendedAt: optTime(r.SuspendedAt),
			SuspendedReason: r.SuspendedReason, CreatedAt: r.CreatedAt, HasOwner: r.HasOwner, Users: int(r.Users),
			Agents: int(r.Agents), Hosts: int(r.Hosts), HostsOnline: int(r.HostsOnline), LastActivity: optTime(r.LastActivity),
			Storage: operator.Storage{OwnerQuotaBytes: r.QuotaBytes, FilesBytes: r.FilesBytes, DBBytes: r.DbBytes, PlanStorageBytes: r.PlanStorageBytes},
			PlanID:  r.PlanID, BreakGlassOpen: r.BreakGlassOpen}
		if r.InviteCreatedAt.Year() > 1 {
			c.Invite = &operator.InviteState{CreatedAt: r.InviteCreatedAt, ExpiresAt: r.InviteExpiresAt, Used: r.InviteUsed, Revoked: r.InviteRevoked}
		}
		out = append(out, c)
	}
	return out, nil
}

// optTime: the query's 0001-01-01 stands for a missing time.
func optTime(t time.Time) *time.Time {
	if t.Year() <= 1 {
		return nil
	}
	return &t
}

// CreatePlan stores a plan; db.ErrConflict when the name is taken.
func (s *Store) CreatePlan(ctx context.Context, p operator.Plan) error {
	err := s.q(ctx).CreatePlan(ctx, sqlcgen.CreatePlanParams{ID: p.ID, Name: p.Name, StorageBytes: p.StorageBytes,
		CpuCores: int32(p.CPUCores), MemoryBytes: p.MemoryBytes, ArchivedAt: p.ArchivedAt, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// PlanByID reads a plan.
func (s *Store) PlanByID(ctx context.Context, id ids.ID) (operator.Plan, error) {
	r, err := notFound(s.q(ctx).PlanByID(ctx, id))
	return planOf(r), err
}

// ListPlans lists every plan: the current ones first, by storage.
func (s *Store) ListPlans(ctx context.Context) ([]operator.Plan, error) {
	rows, err := s.q(ctx).ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]operator.Plan, len(rows))
	for i, r := range rows {
		out[i] = planOf(r)
	}
	return out, nil
}

// UpdatePlan writes a plan; db.ErrConflict when the name is taken.
func (s *Store) UpdatePlan(ctx context.Context, p operator.Plan) error {
	err := s.q(ctx).UpdatePlan(ctx, sqlcgen.UpdatePlanParams{ID: p.ID, Name: p.Name, StorageBytes: p.StorageBytes,
		CpuCores: int32(p.CPUCores), MemoryBytes: p.MemoryBytes, ArchivedAt: p.ArchivedAt, UpdatedAt: p.UpdatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

func planOf(r sqlcgen.Plan) operator.Plan {
	return operator.Plan{ID: r.ID, Name: r.Name, StorageBytes: r.StorageBytes, CPUCores: int(r.CpuCores), MemoryBytes: r.MemoryBytes,
		ArchivedAt: r.ArchivedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
