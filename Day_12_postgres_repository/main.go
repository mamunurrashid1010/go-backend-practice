// Day 12 — Postgres-backed To-Do API.
//
// The handler / service / errors / todo files are UNCHANGED from Day 11.
// The only Day-12 change to the API is this file (and a new
// postgres_repository.go alongside it).
//
// Startup sequence:
//  1. Load .env (optional, dev convenience)
//  2. Open the *sql.DB pool with the pgx driver
//  3. Run migrations from ./migrations
//  4. Build repo → service → handler
//  5. Start the HTTP server
//
// Run:
//
//	go mod init day12
//	go get github.com/go-chi/chi/v5
//	go get github.com/jackc/pgx/v5/stdlib
//	go get -tags 'postgres' github.com/golang-migrate/migrate/v4
//	go get github.com/golang-migrate/migrate/v4/database/postgres
//	go get github.com/golang-migrate/migrate/v4/source/file
//	go get github.com/joho/godotenv
//	go run .
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	mw "day12/internal/middleware"
	"day12/internal/respond"
	"day12/internal/todo"
)

func main() {
	// ----------------------------------------------------------
	// 1. Config — DATABASE_URL from .env or the shell.
	// ----------------------------------------------------------
	if err := godotenv.Load(); err == nil {
		log.Println("loaded .env")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set (try copying .env.example to .env)")
	}

	// ----------------------------------------------------------
	// 2. Open the pool — ONCE, here in main. Hand it to the repo below.
	// ----------------------------------------------------------
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("ping: %v", err)
	}

	// ----------------------------------------------------------
	// 3. Migrate. If migrations fail, refuse to start.
	// ----------------------------------------------------------
	if err := runMigrations(dsn); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("connected to postgres")

	// ----------------------------------------------------------
	// 4. THE LINE. Day 11 was:
	//        repo := todo.NewInMemoryRepository()
	// ----------------------------------------------------------
	repo := todo.NewPostgresRepository(db)
	svc := todo.NewService(repo)
	h := &todo.Handler{Svc: svc}

	// ----------------------------------------------------------
	// 5. HTTP router + middleware stack.
	// ----------------------------------------------------------
	r := chi.NewRouter()
	r.Use(mw.Recover, mw.RequestID, mw.Logger)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Mount("/todos", h.Router())

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		respond.NotFound(w, "route not found: "+r.Method+" "+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		respond.MethodNotAllowed(w, "")
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("listening on http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}

// runMigrations applies any pending migrations and is a no-op when the DB
// is already up to date.
func runMigrations(dsn string) error {
	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = m.Close()
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	v, _, vErr := m.Version()
	if errors.Is(vErr, migrate.ErrNilVersion) {
		log.Println("migrations: no migrations applied")
		return nil
	}
	if vErr == nil {
		log.Printf("migrations up: %d", v)
	}
	return nil
}
