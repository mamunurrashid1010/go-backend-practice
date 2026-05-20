// Day 9 — Go talks to Postgres via database/sql + pgx driver.
//
// Standalone demo (no HTTP yet). Walks through:
//  1. sql.Open + pool tuning + PingContext
//  2. QueryContext + rows.Next + Scan + defer rows.Close
//  3. QueryRowContext with INSERT ... RETURNING id
//  4. QueryRowContext for one row + sql.ErrNoRows
//  5. ExecContext for UPDATE/DELETE + RowsAffected
//
// Prereq: Day 8's Postgres must be running (docker compose up -d).
//
// Run:
//
//	go mod init day09
//	go get github.com/jackc/pgx/v5/stdlib
//	go run .
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	// "Underscore import" — only registers the "pgx" driver name with
	// database/sql via its init() function. We never call pgx directly.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Todo mirrors the columns in the todos table created on Day 8.
type Todo struct {
	ID        int64
	Title     string
	Done      bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func main() {
	// The DSN. In real code this comes from os.Getenv("DATABASE_URL") — Day 13.
	const dsn = "postgres://app:app@localhost:5433/appdb?sslmode=disable"

	// ------------------------------------------------------------------
	// 1. Open the pool. Does NOT actually connect — just validates args.
	// ------------------------------------------------------------------
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close() // ONCE, on shutdown. Never per query.

	// ------------------------------------------------------------------
	// 2. Pool tuning. Defaults are fine; explicit is nicer.
	// ------------------------------------------------------------------
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// ------------------------------------------------------------------
	// 3. Ping with a short timeout so a wrong DSN fails fast.
	// ------------------------------------------------------------------
	ctx := context.Background()
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("PingContext: %v", err)
	}
	log.Println("connected to postgres")

	// ------------------------------------------------------------------
	// 4. List all todos (Query → rows → Scan → defer Close → Err check).
	// ------------------------------------------------------------------
	log.Println("--- list before ---")
	if err := listTodos(ctx, db); err != nil {
		log.Fatalf("list: %v", err)
	}

	// ------------------------------------------------------------------
	// 5. Insert one with RETURNING. Postgres doesn't support LastInsertId(),
	//    so we use QueryRowContext + Scan to read back the new ID.
	// ------------------------------------------------------------------
	newID, err := createTodo(ctx, db, "row from Day 9", false)
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	log.Printf("created todo id=%d", newID)

	// ------------------------------------------------------------------
	// 6. Get one (QueryRowContext + Scan). Handle sql.ErrNoRows.
	// ------------------------------------------------------------------
	t, err := getTodo(ctx, db, newID)
	if err != nil {
		log.Fatalf("get: %v", err)
	}
	log.Printf("fetched: id=%d title=%q done=%t created_at=%s",
		t.ID, t.Title, t.Done, t.CreatedAt.Format(time.RFC3339))

	// ------------------------------------------------------------------
	// 7. Demonstrate sql.ErrNoRows — fetch a missing ID.
	// ------------------------------------------------------------------
	_, err = getTodo(ctx, db, 999_999)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		log.Println("get(999_999): not found (as expected)")
	case err != nil:
		log.Fatalf("get(999_999): %v", err)
	}

	// ------------------------------------------------------------------
	// 8. Update with ExecContext + RowsAffected to detect "not found".
	// ------------------------------------------------------------------
	if err := markDone(ctx, db, newID); err != nil {
		log.Fatalf("markDone: %v", err)
	}
	log.Printf("marked id=%d done", newID)

	// ------------------------------------------------------------------
	// 9. Delete + final list.
	// ------------------------------------------------------------------
	if err := deleteTodo(ctx, db, newID); err != nil {
		log.Fatalf("delete: %v", err)
	}
	log.Printf("deleted id=%d", newID)

	log.Println("--- list after ---")
	if err := listTodos(ctx, db); err != nil {
		log.Fatalf("list: %v", err)
	}
}

// listTodos prints all rows. Demonstrates the canonical Query → Next → Scan
// → defer Close → rows.Err() pattern.
func listTodos(ctx context.Context, db *sql.DB) error {
	const q = `
		SELECT id, title, done, created_at, updated_at
		FROM   todos
		ORDER  BY id
	`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return fmt.Errorf("QueryContext: %w", err)
	}
	defer rows.Close() // critical — without this, the connection leaks.

	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return fmt.Errorf("Scan: %w", err)
		}
		fmt.Printf("  [%3d] %-40s done=%t\n", t.ID, truncate(t.Title, 40), t.Done)
	}
	// rows.Next() returning false can mean "done" OR "an error happened".
	// rows.Err() distinguishes the two.
	return rows.Err()
}

// createTodo inserts and returns the new ID. INSERT ... RETURNING is
// Postgres-specific and the idiomatic way to get the new key from Go.
func createTodo(ctx context.Context, db *sql.DB, title string, done bool) (int64, error) {
	const q = `INSERT INTO todos (title, done) VALUES ($1, $2) RETURNING id`
	var id int64
	if err := db.QueryRowContext(ctx, q, title, done).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert: %w", err)
	}
	return id, nil
}

// getTodo returns one row by id. Intentionally returns sql.ErrNoRows unwrapped
// — callers check it with errors.Is, which also works through wrapping.
func getTodo(ctx context.Context, db *sql.DB, id int64) (Todo, error) {
	const q = `
		SELECT id, title, done, created_at, updated_at
		FROM   todos
		WHERE  id = $1
	`
	var t Todo
	err := db.QueryRowContext(ctx, q, id).
		Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Todo{}, err
	}
	return t, nil
}

// markDone updates one row. RowsAffected catches the "wrong id" case so the
// caller can return a 404 — UPDATE with no matching row is NOT an error
// in SQL, so the driver returns no error either.
func markDone(ctx context.Context, db *sql.DB, id int64) error {
	const q = `UPDATE todos SET done = true, updated_at = now() WHERE id = $1`
	res, err := db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("no todo with id=%d", id)
	}
	return nil
}

// deleteTodo: same shape as markDone.
func deleteTodo(ctx context.Context, db *sql.DB, id int64) error {
	const q = `DELETE FROM todos WHERE id = $1`
	res, err := db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no todo with id=%d", id)
	}
	return nil
}

// tiny helper for pretty printing.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
