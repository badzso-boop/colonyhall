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
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &Postgres{Pool: pool}, nil
}

// InTx runs fn in a transaction, or joins the transaction already in ctx.
func (p *Postgres) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
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
func (m *Memory) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx.Value(memTxKey{}) != nil {
		return fn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(context.WithValue(ctx, memTxKey{}, true))
}
