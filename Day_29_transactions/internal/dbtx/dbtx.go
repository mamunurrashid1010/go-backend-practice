// Package dbtx — Postgres transaction plumbing.
//
// The point of the pattern: callers wrap a unit of work in InTx(...).
// Inside the callback, the context carries a *sql.Tx; repository code
// that uses RunnerFor(ctx, r.db) silently switches to that tx for the
// duration. The repo never has to take "is this in a tx?" as a parameter.
package dbtx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DBTX is the subset of *sql.DB / *sql.Tx that the repositories use.
// Both types satisfy it without any conversion.
type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type ctxKey struct{}

// WithTx puts a *sql.Tx onto ctx for RunnerFor to pick up.
func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// RunnerFor returns the *sql.Tx attached to ctx if there is one, else
// the supplied *sql.DB. Repos call this on every query.
func RunnerFor(ctx context.Context, db *sql.DB) DBTX {
	if tx, ok := ctx.Value(ctxKey{}).(*sql.Tx); ok {
		return tx
	}
	return db
}

// Transactor owns BeginTx + Commit/Rollback semantics for the app.
// One per *sql.DB; constructed in main.go and handed to services that
// need atomic multi-statement operations.
type Transactor struct {
	db *sql.DB
}

func New(db *sql.DB) *Transactor { return &Transactor{db: db} }

// InTx runs fn inside a transaction with the database's default
// isolation level (Read Committed for Postgres). The tx is attached to
// the ctx passed to fn; repos resolve it via RunnerFor.
func (t *Transactor) InTx(ctx context.Context, fn func(context.Context) error) error {
	return t.InTxOpts(ctx, nil, fn)
}

// InTxOpts is the same as InTx but lets you pick an isolation level or
// read-only mode. Pass &sql.TxOptions{Isolation: sql.LevelSerializable}
// for operations that need it.
func (t *Transactor) InTxOpts(ctx context.Context, opts *sql.TxOptions, fn func(context.Context) error) error {
	tx, err := t.db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(WithTx(ctx, tx)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
		}
		return err
	}

	return tx.Commit()
}
