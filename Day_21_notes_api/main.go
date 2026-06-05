// Day 21 — Notes API mini-project (Week 3 closer).
//
// Wiring:
//   1. config + DB + migrate
//   2. auth wiring (Day 17–20)
//   3. notes wiring (Day 21)
//   4. /auth/* public + protected /me; /notes/* fully protected
//
// Run:
//   go mod init day21 + go get the deps from README; go run .
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

	"day21/internal/auth"
	"day21/internal/config"
	mw "day21/internal/middleware"
	"day21/internal/notes"
	"day21/internal/respond"
)

func main() {
	if err := godotenv.Load(); err == nil {
		log.Println("loaded .env")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("config: env=%s addr=%s access_ttl=%s refresh_ttl=%s",
		cfg.Env, cfg.HTTP.Addr, cfg.Auth.JWTAccessTTL, cfg.Auth.RefreshTTL)

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

	// ----- Auth wiring (Day 17–20) -----------------------------------
	userRepo := auth.NewPostgresUserRepository(db)
	refreshRepo := auth.NewPostgresRefreshTokenRepository(db)
	tokens := auth.NewTokenIssuer(cfg.Auth.JWTSecret, cfg.Auth.JWTAccessTTL, cfg.Auth.JWTIssuer)
	verifier := auth.NewTokenVerifier(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer)
	authSvc := auth.NewService(userRepo, refreshRepo, tokens, cfg.Auth.RefreshTTL)
	authHandler := &auth.Handler{Svc: authSvc}

	// ----- Notes wiring (Day 21) -------------------------------------
	notesRepo := notes.NewPostgresRepository(db)
	notesSvc := notes.NewService(notesRepo)
	notesHandler := &notes.Handler{Svc: notesSvc}

	// ----- Router ----------------------------------------------------
	r := chi.NewRouter()
	r.Use(mw.Recover, mw.RequestID, mw.Logger)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// /auth/* handles its own public/protected split internally.
	r.Mount("/auth", authHandler.Router(verifier))

	// /notes/* is fully protected — wrap the mount in a RequireAuth group.
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(verifier))
		r.Mount("/notes", notesHandler.Router())
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		respond.NotFound(w, "route not found: "+r.Method+" "+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		respond.MethodNotAllowed(w, "")
	})

	// ----- Server with graceful shutdown -----------------------------
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
