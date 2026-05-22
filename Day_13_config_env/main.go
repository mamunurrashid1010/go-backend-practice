// Day 13 — config moved into its own package.
//
// Compare to Day 12's main.go:
//   - hard-coded ":8080", "10s", 25, 10 → cfg.HTTP.Addr / cfg.DB.MaxOpenConns / ...
//   - one os.Getenv call → one config.Load() call
//   - bad config fails at startup with a clear error, before the server starts
//
// Run:
//   go mod init day13
//   go get github.com/go-chi/chi/v5
//   go get github.com/jackc/pgx/v5/stdlib
//   go get -tags 'postgres' github.com/golang-migrate/migrate/v4
//   go get github.com/golang-migrate/migrate/v4/database/postgres
//   go get github.com/golang-migrate/migrate/v4/source/file
//   go get github.com/joho/godotenv
//   go get github.com/caarlos0/env/v11
//   go run .
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"day13/internal/config"
	mw "day13/internal/middleware"
	"day13/internal/respond"
	"day13/internal/todo"
)

func main() {
	// ----------------------------------------------------------
	// 1. Load .env into the process environment.
	// ----------------------------------------------------------
	if err := godotenv.Load(); err == nil {
		log.Println("loaded .env")
	}

	// ----------------------------------------------------------
	// 2. Read + validate config. Anything broken? Fail here, loudly.
	// ----------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("config: env=%s addr=%s db_pool=%d/%d",
		cfg.Env, cfg.HTTP.Addr, cfg.DB.MaxOpenConns, cfg.DB.MaxIdleConns)

	// ----------------------------------------------------------
	// 3. Open the DB pool using values from cfg, not magic numbers.
	// ----------------------------------------------------------
	db, err := sql.Open("pgx", cfg.DB.URL)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DB.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(context.Background(), cfg.DB.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("ping: %v", err)
	}

	// ----------------------------------------------------------
	// 4. Migrations.
	// ----------------------------------------------------------
	if err := runMigrations(cfg.DB.URL); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("connected to postgres")

	// ----------------------------------------------------------
	// 5. Build the layers.
	// ----------------------------------------------------------
	repo := todo.NewPostgresRepository(db)
	svc := todo.NewService(repo)
	h := &todo.Handler{Svc: svc}

	// ----------------------------------------------------------
	// 6. HTTP router.
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

	// ----------------------------------------------------------
	// 7. Start the server using HTTP settings from cfg.
	// ----------------------------------------------------------
	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      r,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	log.Printf("listening on http://localhost%s", cfg.HTTP.Addr)
	log.Fatal(srv.ListenAndServe())
}

func runMigrations(dsn string) error {
	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
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
