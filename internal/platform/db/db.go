// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package db holds the transaction port and its Postgres and in-memory
// implementations. Stores never begin transactions themselves: a service
// wraps a use case in TxRunner.InTx, and every store call made with the
// returned context joins that transaction.
package db

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxRunner runs fn inside one transaction. If fn returns an error the
// transaction is rolled back (Postgres) and the error is returned.
type TxRunner interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// ErrNotFound is returned by stores when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned by stores when a unique constraint is violated.
var ErrConflict = errors.New("conflict")

// Querier is the subset of pgx used by stores; both *pgxpool.Pool and pgx.Tx satisfy it.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// Postgres is a TxRunner backed by a pgx pool.
type Postgres struct {
	Pool *pgxpool.Pool
}

// Open connects to Postgres and verifies the connection.
func Open(ctx context.Context, url string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	cfg.PrepareConn = applyScope
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &Postgres{Pool: pool}, nil
}

// ErrScopeChanged is returned when a nested InTx asks for another scope
// than the transaction it would join: the connection keeps the outer
// scope, so joining would silently run the inner work with it (a
// db.WithWorkspace inside a system-mode transaction would not narrow).
var ErrScopeChanged = errors.New("db: nested transaction with a different scope")

// txScopeKey holds the scope a transaction was opened with.
type txScopeKey struct{}

// joinScope checks that ctx asks for the scope of the transaction it joins.
func joinScope(ctx context.Context) error {
	if have, ok := ctx.Value(txScopeKey{}).(Scope); ok && have != ScopeOf(ctx) {
		return fmt.Errorf("%w: the transaction has %+v, the call asks for %+v", ErrScopeChanged, have, ScopeOf(ctx))
	}
	return nil
}

// InTx runs fn in a transaction, or joins the transaction already in ctx.
// Joining needs the same scope (ErrScopeChanged otherwise).
func (p *Postgres) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		if err := joinScope(ctx); err != nil {
			return err
		}
		return fn(ctx)
	}
	ctx = context.WithValue(ctx, txScopeKey{}, ScopeOf(ctx))
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

// Q returns the transaction in ctx, or the pool when there is none.
func (p *Postgres) Q(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return p.Pool
}

// IsUniqueViolation reports whether err is a Postgres unique-constraint error.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Memory is a TxRunner for the in-memory reference stores. It serialises
// transactions with one lock; it does not roll back partial writes, so
// in-memory stores must validate before they write.
type Memory struct {
	mu sync.Mutex
}

type memTxKey struct{}

// InTx runs fn under the global lock, joining an outer InTx if present.
// Like Postgres, joining needs the same scope (ErrScopeChanged otherwise).
func (m *Memory) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx.Value(memTxKey{}) != nil {
		if err := joinScope(ctx); err != nil {
			return err
		}
		return fn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(context.WithValue(context.WithValue(ctx, txScopeKey{}, ScopeOf(ctx)), memTxKey{}, true))
}
