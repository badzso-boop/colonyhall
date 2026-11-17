// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Service holds the secret use cases. Callers join its transaction (a
// connector and its secret are created together).
type Service struct {
	Store  Store
	Keys   *secretbox.Keyring // nil: secrets cannot be stored (ErrNoKeys)
	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock
}

// Available reports whether the server can store secrets.
func (s *Service) Available() bool { return s != nil && s.Keys != nil }

func (s *Service) admin(by identity.Actor) error {
	if by.Kind != identity.KindHuman || !by.IsAdmin() || by.Halted {
		return ErrForbidden
	}
	return nil
}

// Create seals and stores a new secret. Only the owner and the admins.
func (s *Service) Create(ctx context.Context, by identity.Actor, kind, name, value string) (Secret, error) {
	if err := s.admin(by); err != nil {
		return Secret{}, err
	}
	if !s.Available() {
		return Secret{}, ErrNoKeys
	}
	if !validKind(kind) {
		return Secret{}, ErrBadKind
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return Secret{}, ErrBadName
	}
	if err := checkValue(value); err != nil {
		return Secret{}, err
	}
	sec := Secret{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, Kind: kind, Name: name, Hint: hint(value),
		CreatedBy: by.ID, CreatedAt: s.Clock.Now()}
	sealed, err := s.Keys.Seal([]byte(value), aad(sec.WorkspaceID, sec.ID, kind))
	if err != nil {
		return Secret{}, err
	}
	sec.Sealed = sealed
	err = s.inTx(ctx, by, EventCreated, sec, func(ctx context.Context) error { return s.Store.Insert(ctx, sec) })
	return sec, err
}

// Replace stores a new value for a secret (rotation by the person).
func (s *Service) Replace(ctx context.Context, by identity.Actor, id ids.ID, value string) (Secret, error) {
	if err := s.admin(by); err != nil {
		return Secret{}, err
	}
	if !s.Available() {
		return Secret{}, ErrNoKeys
	}
	if err := checkValue(value); err != nil {
		return Secret{}, err
	}
	var sec Secret
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		sec, err = s.get(ctx, by.WorkspaceID, id)
		if err != nil {
			return err
		}
		sealed, err := s.Keys.Seal([]byte(value), aad(sec.WorkspaceID, sec.ID, sec.Kind))
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		sec.Sealed, sec.Hint, sec.RotatedAt = sealed, hint(value), &now
		return s.inTx(ctx, by, EventReplaced, sec, func(ctx context.Context) error {
			return s.Store.Replace(ctx, sec.WorkspaceID, sec.ID, sealed, sec.Hint, now)
		})
	})
	return sec, err
}

// Delete removes a secret (soft delete; the row stays without use).
func (s *Service) Delete(ctx context.Context, by identity.Actor, id ids.ID) error {
	if err := s.admin(by); err != nil {
		return err
	}
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		sec, err := s.get(ctx, by.WorkspaceID, id)
		if err != nil {
			return err
		}
		return s.inTx(ctx, by, EventDeleted, sec, func(ctx context.Context) error {
			return s.Store.Delete(ctx, sec.WorkspaceID, sec.ID, s.Clock.Now())
		})
	})
}

// Info returns a secret without its value (name, kind, hint).
func (s *Service) Info(ctx context.Context, workspaceID, id ids.ID) (Secret, error) {
	sec, err := s.get(ctx, workspaceID, id)
	sec.Sealed = secretbox.Sealed{}
	return sec, err
}

// Reveal opens a secret for the server's own outbound request. It is not
// reachable from any endpoint or tool; the caller must never log, store or
// return the value.
func (s *Service) Reveal(ctx context.Context, workspaceID, id ids.ID) (string, error) {
	if !s.Available() {
		return "", ErrNoKeys
	}
	sec, err := s.get(ctx, workspaceID, id)
	if err != nil {
		return "", err
	}
	v, err := s.Keys.Open(sec.Sealed, aad(sec.WorkspaceID, sec.ID, sec.Kind))
	if err != nil {
		return "", err
	}
	return string(v), nil
}

// Rotate re-seals every workspace secret that is not sealed with the
// active key (the rotate-secrets command; ctx must be in system scope).
func (s *Service) Rotate(ctx context.Context) (int, error) {
	if !s.Available() {
		return 0, ErrNoKeys
	}
	list, err := s.Store.ListForRotation(ctx, s.Keys.Active())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, sec := range list {
		v, err := s.Keys.Open(sec.Sealed, aad(sec.WorkspaceID, sec.ID, sec.Kind))
		if err != nil {
			return n, fmt.Errorf("secret %s: %w", sec.ID, err)
		}
		sealed, err := s.Keys.Seal(v, aad(sec.WorkspaceID, sec.ID, sec.Kind))
		if err != nil {
			return n, err
		}
		if err := s.Store.Reseal(ctx, sec.WorkspaceID, sec.ID, sealed); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Service) get(ctx context.Context, workspaceID, id ids.ID) (Secret, error) {
	sec, err := s.Store.Get(ctx, workspaceID, id)
	if errors.Is(err, db.ErrNotFound) {
		return Secret{}, ErrNotFound
	}
	return sec, err
}

func (s *Service) inTx(ctx context.Context, by identity.Actor, typ string, sec Secret, fn func(ctx context.Context) error) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), sec.WorkspaceID, typ, by.ID, ids.Nil, "secret", sec.ID,
			Changed{SecretID: sec.ID.String(), Kind: sec.Kind, Name: sec.Name}, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err == nil && s.Bus != nil {
		s.Bus.Notify(seq)
	}
	return err
}
