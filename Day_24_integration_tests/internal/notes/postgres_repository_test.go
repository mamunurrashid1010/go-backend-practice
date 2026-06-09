//go:build integration

// Integration tests for PostgresRepository.
//
// Skipped by default. Run with:
//
//   go test -tags integration ./internal/notes/...
//
// Requires Docker Desktop running. testcontainers-go pulls and starts
// postgres:16-alpine, applies the project's migrations, and runs the tests
// against the real DB. The container is shared across tests in this package
// via TestMain; each test truncates the tables for isolation.

package notes

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Shared state for the package.
var (
	testDB  *sql.DB
	testCtx = context.Background()
)

// TestMain — special function that runs once per package. We use it to
// start Postgres, apply migrations, and tear everything down at the end.
func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Spin up postgres:16-alpine.
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2). // Postgres logs this twice during init.
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("start container: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(context.Background())
		log.Fatalf("dsn: %v", err)
	}

	// Apply migrations from the project root.
	if err := runMigrationsForTest(dsn); err != nil {
		_ = container.Terminate(context.Background())
		log.Fatalf("migrate: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		_ = container.Terminate(context.Background())
		log.Fatalf("sql.Open: %v", err)
	}
	testDB = db

	code := m.Run()

	_ = db.Close()
	_ = container.Terminate(context.Background())
	os.Exit(code)
}

// runMigrationsForTest finds the migrations dir relative to this file and
// applies it to the test DB.
func runMigrationsForTest(dsn string) error {
	// We're in <project>/internal/notes/. Migrations are at <project>/migrations/.
	migDir, err := filepath.Abs("../../migrations")
	if err != nil {
		return err
	}
	m, err := migrate.New("file://"+filepath.ToSlash(migDir), dsn)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// resetDB clears every table between tests. RESTART IDENTITY makes IDs
// predictable (each test starts fresh at id=1).
func resetDB(t *testing.T) {
	t.Helper()
	const q = `TRUNCATE notes, refresh_tokens, users RESTART IDENTITY CASCADE`
	if _, err := testDB.ExecContext(testCtx, q); err != nil {
		t.Fatalf("resetDB: %v", err)
	}
}

// seedUser inserts a row and returns its id. Notes have a FK on users, so
// every test that creates a note seeds at least one user first.
func seedUser(t *testing.T, email string) int64 {
	t.Helper()
	var id int64
	const q = `INSERT INTO users (email, password_hash) VALUES ($1, 'hash') RETURNING id`
	if err := testDB.QueryRowContext(testCtx, q, email).Scan(&id); err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------
// The actual integration tests.
// ---------------------------------------------------------------------

// TestPGRepo_Create_RoundTrip proves INSERT ... RETURNING populates id +
// server-side timestamps + the user_id we asked for.
func TestPGRepo_Create_RoundTrip(t *testing.T) {
	resetDB(t)
	uid := seedUser(t, "a@b.dev")
	repo := NewPostgresRepository(testDB)

	n, err := repo.Create(testCtx, uid, CreateRequest{Title: "hello", Body: "world"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.ID == 0 {
		t.Errorf("id: want non-zero, got 0")
	}
	if n.UserID != uid {
		t.Errorf("user_id: want %d, got %d", uid, n.UserID)
	}
	if n.Title != "hello" || n.Body != "world" {
		t.Errorf("body fields wrong: %+v", n)
	}
	if n.CreatedAt.IsZero() || n.UpdatedAt.IsZero() {
		t.Errorf("timestamps not populated: created=%v updated=%v", n.CreatedAt, n.UpdatedAt)
	}

	// Read it back.
	got, err := repo.Get(testCtx, uid, n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "hello" {
		t.Errorf("Get title: want hello, got %q", got.Title)
	}
}

// TestPGRepo_Get_NotFound proves sql.ErrNoRows -> ErrNotFound translation.
func TestPGRepo_Get_NotFound(t *testing.T) {
	resetDB(t)
	uid := seedUser(t, "a@b.dev")
	repo := NewPostgresRepository(testDB)

	_, err := repo.Get(testCtx, uid, 999)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestPGRepo_Get_ScopedByUserID — THE IDOR test.
// Note belongs to user A. User B's Get must return ErrNotFound (NOT the note).
// This is the test mocks cannot write.
func TestPGRepo_Get_ScopedByUserID(t *testing.T) {
	resetDB(t)
	aID := seedUser(t, "a@b.dev")
	bID := seedUser(t, "b@b.dev")
	repo := NewPostgresRepository(testDB)

	n, err := repo.Create(testCtx, aID, CreateRequest{Title: "A's secret"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = repo.Get(testCtx, bID, n.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("user B should get ErrNotFound for A's note, got: %v", err)
	}

	// Sanity: A can still see it.
	if _, err := repo.Get(testCtx, aID, n.ID); err != nil {
		t.Fatalf("user A's own note: want no error, got %v", err)
	}
}

// TestPGRepo_Update_WrongUser_NotFound proves UPDATE with the wrong user_id
// returns ErrNotFound (the UPDATE matches zero rows -> RETURNING fires no
// rows -> sql.ErrNoRows -> ErrNotFound).
func TestPGRepo_Update_WrongUser_NotFound(t *testing.T) {
	resetDB(t)
	aID := seedUser(t, "a@b.dev")
	bID := seedUser(t, "b@b.dev")
	repo := NewPostgresRepository(testDB)

	n, _ := repo.Create(testCtx, aID, CreateRequest{Title: "A's"})

	_, err := repo.Update(testCtx, bID, n.ID, UpdateRequest{Title: "hijacked"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	// A's note is unchanged.
	got, _ := repo.Get(testCtx, aID, n.ID)
	if got.Title != "A's" {
		t.Errorf("A's note was tampered: %q", got.Title)
	}
}

// TestPGRepo_Patch_KeepsExistingField proves COALESCE($n, column) actually
// keeps the column when $n is NULL (pointer-field DTO with the field nil).
func TestPGRepo_Patch_KeepsExistingField(t *testing.T) {
	resetDB(t)
	uid := seedUser(t, "a@b.dev")
	repo := NewPostgresRepository(testDB)

	orig, _ := repo.Create(testCtx, uid, CreateRequest{Title: "orig title", Body: "orig body"})

	// PATCH only the body — title nil means COALESCE($3, title) keeps "orig title".
	newBody := "new body"
	got, err := repo.Patch(testCtx, uid, orig.ID, PatchRequest{Body: &newBody})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got.Title != "orig title" {
		t.Errorf("title should be unchanged, got %q", got.Title)
	}
	if got.Body != "new body" {
		t.Errorf("body should be updated, got %q", got.Body)
	}
	if !got.UpdatedAt.After(orig.UpdatedAt) {
		t.Errorf("updated_at should advance: orig=%v got=%v", orig.UpdatedAt, got.UpdatedAt)
	}
}

// TestPGRepo_Delete_RowsAffected proves DELETE with no matching row returns
// ErrNotFound (via RowsAffected == 0).
func TestPGRepo_Delete_RowsAffected(t *testing.T) {
	resetDB(t)
	uid := seedUser(t, "a@b.dev")
	repo := NewPostgresRepository(testDB)

	// Delete a non-existent id.
	if err := repo.Delete(testCtx, uid, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-existent: want ErrNotFound, got %v", err)
	}

	// Create then delete.
	n, _ := repo.Create(testCtx, uid, CreateRequest{Title: "x"})
	if err := repo.Delete(testCtx, uid, n.ID); err != nil {
		t.Fatalf("delete existing: %v", err)
	}

	// Already gone.
	if err := repo.Delete(testCtx, uid, n.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: want ErrNotFound, got %v", err)
	}
}

// TestPGRepo_List_FilterAndLimit proves the dynamic WHERE-clause builder
// and LIMIT actually produce the right SQL.
func TestPGRepo_List_FilterAndLimit(t *testing.T) {
	resetDB(t)
	uid := seedUser(t, "a@b.dev")
	repo := NewPostgresRepository(testDB)

	// Seed 5 notes.
	titles := []string{"learn SQL", "buy milk", "learn Go", "walk dog", "learn Postgres"}
	for _, ti := range titles {
		if _, err := repo.Create(testCtx, uid, CreateRequest{Title: ti}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// No filter — should be 5.
	all, err := repo.List(testCtx, uid, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("no filter: want 5, got %d", len(all))
	}

	// Search "learn" — should be 3.
	withSearch, _ := repo.List(testCtx, uid, ListFilter{Search: "learn"})
	if len(withSearch) != 3 {
		t.Errorf("search=learn: want 3, got %d", len(withSearch))
	}

	// Limit 2 with search learn — should be 2.
	limited, _ := repo.List(testCtx, uid, ListFilter{Search: "learn", Limit: 2})
	if len(limited) != 2 {
		t.Errorf("search+limit: want 2, got %d", len(limited))
	}
}

// TestPGRepo_List_ScopedByUserID proves List doesn't leak other users' notes.
func TestPGRepo_List_ScopedByUserID(t *testing.T) {
	resetDB(t)
	aID := seedUser(t, "a@b.dev")
	bID := seedUser(t, "b@b.dev")
	repo := NewPostgresRepository(testDB)

	_, _ = repo.Create(testCtx, aID, CreateRequest{Title: "A1"})
	_, _ = repo.Create(testCtx, aID, CreateRequest{Title: "A2"})
	_, _ = repo.Create(testCtx, bID, CreateRequest{Title: "B1"})

	aList, _ := repo.List(testCtx, aID, ListFilter{})
	bList, _ := repo.List(testCtx, bID, ListFilter{})

	if len(aList) != 2 {
		t.Errorf("A: want 2, got %d", len(aList))
	}
	if len(bList) != 1 {
		t.Errorf("B: want 1, got %d", len(bList))
	}
	if bList[0].Title != "B1" {
		t.Errorf("B leaked A's note: %q", bList[0].Title)
	}
}
