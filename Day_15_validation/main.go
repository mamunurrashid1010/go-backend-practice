// Day 15 — To-Do API + input validation.
//
// Same wiring as Day 14 (config → DB → migrate → layers → server with
// graceful shutdown). Validation happens inside the todo handlers via the
// internal/validate package, so main.go itself is unchanged in shape.
//
// Run:
//   go mod init day15
//   go get github.com/go-chi/chi/v5
//   go get github.com/jackc/pgx/v5/stdlib
//   go get -tags 'postgres' github.com/golang-migrate/migrate/v4
//   go get github.com/golang-migrate/migrate/v4/database/postgres
//   go get github.com/golang-migrate/migrate/v4/source/file
//   go get github.com/joho/godotenv
//   go get github.com/caarlos0/env/v11
//   go get github.com/go-playground/validator/v10
//   go run .
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"day15/internal/config"
	mw "day15/internal/middleware"
	"day15/internal/respond"
	"day15/internal/todo"
)

func main() {
	if err := godotenv.Load(); err == nil {
		log.Println("loaded .env")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("config: env=%s addr=%s db_pool=%d/%d",
		cfg.Env, cfg.HTTP.Addr, cfg.DB.MaxOpenConns, cfg.DB.MaxIdleConns)

	db, err := sql.Open("pgx", cfg.DB.URL)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DB.ConnMaxIdleTime)

	pingCtx, cancelPing := context.WithTimeout(context.Background(), cfg.DB.PingTimeout)
	defer cancelPing()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("ping: %v", err)
	}

	if err := runMigrations(cfg.DB.URL); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("connected to postgres")

	repo := todo.NewPostgresRepository(db)
	svc := todo.NewService(repo)
	h := &todo.Handler{Svc: svc}

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
		Addr:         cfg.HTTP.Addr,
		Handler:      r,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on http://localhost%s", cfg.HTTP.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
		return
	}
	log.Println("bye")
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
