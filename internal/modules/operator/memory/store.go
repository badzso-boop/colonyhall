// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory operator store (tests, COLONY_STORE=memory).
package memory

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

type backupCode struct {
	operatorID ids.ID
	hash       string
	usedAt     *time.Time
}

// Store is the in-memory operator store.
type Store struct {
	mu        sync.RWMutex
	operators map[ids.ID]operator.Operator
	codes     []backupCode
	sessions  map[ids.ID]operator.Session
	audit     []operator.AuditEntry
	plans     map[ids.ID]operator.Plan
}

// New returns an empty store.
func New() *Store {
	return &Store{operators: map[ids.ID]operator.Operator{}, sessions: map[ids.ID]operator.Session{}, plans: map[ids.ID]operator.Plan{}}
}

// CreateOperator stores an operator.
func (s *Store) CreateOperator(_ context.Context, o operator.Operator) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.operators {
		if x.Handle == o.Handle {
			return db.ErrConflict
		}
	}
	s.operators[o.ID] = o
	return nil
}

// OperatorByHandle finds an operator by handle.
func (s *Store) OperatorByHandle(_ context.Context, handle string) (operator.Operator, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, x := range s.operators {
		if x.Handle == handle {
			return x, nil
		}
	}
	return operator.Operator{}, db.ErrNotFound
}

// OperatorByID finds an operator.
func (s *Store) OperatorByID(_ context.Context, id ids.ID) (operator.Operator, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.operators[id]
	if !ok {
		return operator.Operator{}, db.ErrNotFound
	}
	return o, nil
}

// ListOperators lists every operator by handle.
func (s *Store) ListOperators(_ context.Context) ([]operator.Operator, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := slices.Collect(maps.Values(s.operators))
	slices.SortFunc(out, func(a, b operator.Operator) int {
		if a.Handle < b.Handle {
			return -1
		}
		if a.Handle > b.Handle {
			return 1
		}
		return 0
	})
	return out, nil
}

func (s *Store) update(id ids.ID, f func(*operator.Operator)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.operators[id]
	if !ok {
		return db.ErrNotFound
	}
	f(&o)
	s.operators[id] = o
	return nil
}

// SetTOTP replaces the secret and the state and resets the last step.
func (s *Store) SetTOTP(_ context.Context, id ids.ID, totp secretbox.Sealed, state string) error {
	return s.update(id, func(o *operator.Operator) { o.TOTP, o.State, o.TOTPLastStep = totp, state, 0 })
}

// ResealTOTP replaces the ciphertext.
func (s *Store) ResealTOTP(_ context.Context, id ids.ID, totp secretbox.Sealed) error {
	return s.update(id, func(o *operator.Operator) { o.TOTP = totp })
}

// AdvanceTOTPStep records an accepted step after the last one.
func (s *Store) AdvanceTOTPStep(_ context.Context, id ids.ID, step int64) (bool, error) {
	ok := false
	err := s.update(id, func(o *operator.Operator) {
		if step > o.TOTPLastStep {
			o.TOTPLastStep, ok = step, true
		}
	})
	return ok, err
}

// SetState sets the state and the activation time.
func (s *Store) SetState(_ context.Context, id ids.ID, state string, activatedAt *time.Time) error {
	return s.update(id, func(o *operator.Operator) { o.State, o.ActivatedAt = state, activatedAt })
}

// SetLastLogin records a login.
func (s *Store) SetLastLogin(_ context.Context, id ids.ID, at time.Time) error {
	return s.update(id, func(o *operator.Operator) { o.LastLoginAt = &at })
}

// ReplaceBackupCodes replaces the operator's codes.
func (s *Store) ReplaceBackupCodes(_ context.Context, operatorID ids.ID, codes []operator.BackupCode, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes = slices.DeleteFunc(s.codes, func(c backupCode) bool { return c.operatorID == operatorID })
	for _, c := range codes {
		s.codes = append(s.codes, backupCode{operatorID: operatorID, hash: string(c.Hash)})
	}
	return nil
}

// UseBackupCode marks an unused code used.
func (s *Store) UseBackupCode(_ context.Context, operatorID ids.ID, hash []byte, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, c := range s.codes {
		if c.operatorID == operatorID && c.hash == string(hash) && c.usedAt == nil {
			s.codes[k].usedAt = &at
			return true, nil
		}
	}
	return false, nil
}

// UnusedBackupCodes counts the operator's unused codes.
func (s *Store) UnusedBackupCodes(_ context.Context, operatorID ids.ID) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, c := range s.codes {
		if c.operatorID == operatorID && c.usedAt == nil {
			n++
		}
	}
	return n, nil
}

// CreateSession stores a session.
func (s *Store) CreateSession(_ context.Context, x operator.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[x.ID] = x
	return nil
}

// SessionByTokenHash finds a session.
func (s *Store) SessionByTokenHash(_ context.Context, hash []byte) (operator.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, x := range s.sessions {
		if string(x.TokenHash) == string(hash) {
			return x, nil
		}
	}
	return operator.Session{}, db.ErrNotFound
}

func (s *Store) updateSession(id ids.ID, f func(*operator.Session)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, ok := s.sessions[id]
	if !ok {
		return db.ErrNotFound
	}
	f(&x)
	s.sessions[id] = x
	return nil
}

// TouchSession records a use.
func (s *Store) TouchSession(_ context.Context, id ids.ID, at time.Time) error {
	return s.updateSession(id, func(x *operator.Session) { x.LastSeenAt = at })
}

// RevokeSession ends a session.
func (s *Store) RevokeSession(_ context.Context, id ids.ID, at time.Time) error {
	return s.updateSession(id, func(x *operator.Session) {
		if x.RevokedAt == nil {
			x.RevokedAt = &at
		}
	})
}

// RevokeSessions ends every session of the operator.
func (s *Store) RevokeSessions(_ context.Context, operatorID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, x := range s.sessions {
		if x.OperatorID == operatorID && x.RevokedAt == nil {
			x.RevokedAt = &at
			s.sessions[id] = x
		}
	}
	return nil
}

// DeleteEndedSessions deletes sessions that ended before before.
func (s *Store) DeleteEndedSessions(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for id, x := range s.sessions {
		if x.ExpiresAt.Before(before) || (x.RevokedAt != nil && x.RevokedAt.Before(before)) ||
			x.LastSeenAt.Add(operator.SessionIdle).Before(before) {
			delete(s.sessions, id)
			n++
		}
	}
	return n, nil
}

// AppendAudit appends an entry.
func (s *Store) AppendAudit(_ context.Context, e operator.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = append(s.audit, e)
	return nil
}

// ListAudit returns entries newest first, before the given time.
func (s *Store) ListAudit(_ context.Context, before time.Time, limit int) ([]operator.AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []operator.AuditEntry
	for i := len(s.audit) - 1; i >= 0 && len(out) < limit; i-- {
		if e := s.audit[i]; before.IsZero() || e.CreatedAt.Before(before) {
			out = append(out, e)
		}
	}
	return out, nil
}

// CreatePlan stores a plan; db.ErrConflict when the name is taken.
func (s *Store) CreatePlan(_ context.Context, p operator.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.plans {
		if x.Name == p.Name {
			return db.ErrConflict
		}
	}
	s.plans[p.ID] = p
	return nil
}

// PlanByID reads a plan.
func (s *Store) PlanByID(_ context.Context, id ids.ID) (operator.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	if !ok {
		return operator.Plan{}, db.ErrNotFound
	}
	return p, nil
}

// ListPlans lists every plan: the current ones first, by storage, by name.
func (s *Store) ListPlans(_ context.Context) ([]operator.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := slices.Collect(maps.Values(s.plans))
	slices.SortFunc(out, func(a, b operator.Plan) int {
		if (a.ArchivedAt != nil) != (b.ArchivedAt != nil) {
			if a.ArchivedAt == nil {
				return -1
			}
			return 1
		}
		if a.StorageBytes != b.StorageBytes {
			if a.StorageBytes < b.StorageBytes {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// UpdatePlan writes a plan; db.ErrConflict when the name is taken.
func (s *Store) UpdatePlan(_ context.Context, p operator.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[p.ID]; !ok {
		return db.ErrNotFound
	}
	for _, x := range s.plans {
		if x.Name == p.Name && x.ID != p.ID {
			return db.ErrConflict
		}
	}
	s.plans[p.ID] = p
	return nil
}
