package dbtx

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestRunnerFor_FallsBackToDB_WhenNoTx(t *testing.T) {
	var db *sql.DB
	got := RunnerFor(context.Background(), db)
	// Without a tx in context, RunnerFor must return the supplied db
	// even when it's nil; the type assertion just confirms identity.
	if any(got).(*sql.DB) != db {
		t.Fatalf("RunnerFor without tx should return the db; got %T", got)
	}
}

func TestRunnerFor_PrefersTx_WhenSetOnContext(t *testing.T) {
	tx := &sql.Tx{}
	ctx := WithTx(context.Background(), tx)
	got := RunnerFor(ctx, nil)
	if any(got).(*sql.Tx) != tx {
		t.Fatalf("RunnerFor with tx should return that tx; got %T", got)
	}
}

func TestWithTx_DoesNotLeakAcrossCtx(t *testing.T) {
	tx := &sql.Tx{}
	parent := context.Background()
	WithTx(parent, tx) // ignore — we test that parent is unchanged
	if v := parent.Value(ctxKey{}); v != nil {
		t.Fatalf("WithTx mutated parent ctx; got %v", v)
	}
}

func TestInTx_RunsFn_NoDB(t *testing.T) {
	// We can't BeginTx without a real DB, so this is a guard test only:
	// verify the Transactor type compiles and the InTx signature matches.
	var _ func(context.Context, func(context.Context) error) error = (&Transactor{}).InTx
	_ = errors.New("placeholder so the package import isn't unused")
}
