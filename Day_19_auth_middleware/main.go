// Day 19 — auth middleware: verify JWT, inject userID on context.
//
// Run:
//   go mod init day19
//   go get github.com/go-chi/chi/v5
//   go get github.com/jackc/pgx/v5/stdlib
//   go get github.com/jackc/pgx/v5/pgconn
//   go get -tags 'postgres' github.com/golang-migrate/migrate/v4
//   go get github.com/golang-migrate/migrate/v4/database/postgres
//   go get github.com/golang-migrate/migrate/v4/source/file
//   go get github.com/joho/godotenv
//   go get github.com/caarlos0/env/v11
//   go get github.com/go-playground/validator/v10
//   go get golang.org/x/crypto/bcrypt
//   go get github.com/golang-jwt/jwt/v5
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

	"day19/internal/auth"
	"day19/internal/config"
	mw "day19/internal/middleware"
	"day19/internal/respond"
)

func main() {
	if err := godotenv.Load(); err == nil {
		log.Println("loaded .env")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("config: env=%s addr=%s jwt_ttl=%s issuer=%s",
		cfg.Env, cfg.HTTP.Addr, cfg.Auth.JWTAccessTTL, cfg.Auth.JWTIssuer)

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

	// Auth wiring: issuer signs, verifier checks. Both share the secret +
	// issuer name from config.
	userRepo := auth.NewPostgresUserRepository(db)
	tokens := auth.NewTokenIssuer(cfg.Auth.JWTSecret, cfg.Auth.JWTAccessTTL, cfg.Auth.JWTIssuer)
	verifier := auth.NewTokenVerifier(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer)
	authSvc := auth.NewService(userRepo, tokens)
	authHandler := &auth.Handler{Svc: authSvc}

	r := chi.NewRouter()
	r.Use(mw.Recover, mw.RequestID, mw.Logger)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// The verifier is passed into the auth router so the protected group
	// inside can wire RequireAuth.
	r.Mount("/auth", authHandler.Router(verifier))

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
