package dbtx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type ctxKey struct{}

func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

func RunnerFor(ctx context.Context, db *sql.DB) DBTX {
	if tx, ok := ctx.Value(ctxKey{}).(*sql.Tx); ok {
		return tx
	}
	return db
}

type Transactor struct {
	db *sql.DB
}

func New(db *sql.DB) *Transactor { return &Transactor{db: db} }

func (t *Transactor) InTx(ctx context.Context, fn func(context.Context) error) error {
	return t.InTxOpts(ctx, nil, fn)
}

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
