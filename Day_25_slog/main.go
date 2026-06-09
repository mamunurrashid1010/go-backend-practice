// Day 25 — main.go builds the slog logger and threads it into the middleware.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"day25/internal/auth"
	"day25/internal/config"
	"day25/internal/logging"
	mw "day25/internal/middleware"
	"day25/internal/notes"
	"day25/internal/respond"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		// We don't have a logger yet; use slog.Default to stderr.
		slog.Default().Error("config", slog.Any("err", err))
		os.Exit(1)
	}

	// Build the root logger. JSON in prod/staging, text in dev.
	base := logging.New(cfg.LogLevel, !cfg.IsDev())
	slog.SetDefault(base) // anyone who reaches for slog.Default gets ours

	base.Info("config loaded",
		slog.String("env", cfg.Env),
		slog.String("addr", cfg.HTTP.Addr),
		slog.String("log_level", cfg.LogLevel),
	)

	db, err := sql.Open("pgx", cfg.DB.URL)
	if err != nil {
		base.Error("sql.Open", slog.Any("err", err))
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DB.ConnMaxIdleTime)

	pingCtx, cancelPing := context.WithTimeout(context.Background(), cfg.DB.PingTimeout)
	defer cancelPing()
	if err := db.PingContext(pingCtx); err != nil {
		base.Error("db ping", slog.Any("err", err))
		os.Exit(1)
	}
	if err := runMigrations(cfg.DB.URL); err != nil {
		base.Error("migrate", slog.Any("err", err))
		os.Exit(1)
	}
	base.Info("connected to postgres")

	userRepo := auth.NewPostgresUserRepository(db)
	refreshRepo := auth.NewPostgresRefreshTokenRepository(db)
	tokens := auth.NewTokenIssuer(cfg.Auth.JWTSecret, cfg.Auth.JWTAccessTTL, cfg.Auth.JWTIssuer)
	verifier := auth.NewTokenVerifier(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer)
	authSvc := auth.NewService(userRepo, refreshRepo, tokens, cfg.Auth.RefreshTTL)
	authHandler := &auth.Handler{Svc: authSvc}

	notesRepo := notes.NewPostgresRepository(db)
	notesSvc := notes.NewService(notesRepo)
	notesHandler := &notes.Handler{Svc: notesSvc}

	r := chi.NewRouter()
	r.Use(mw.RequestID)        // rid first
	r.Use(mw.Logger(base))     // logger uses rid from ctx; injects logger
	r.Use(mw.Recover)          // recover uses the request logger

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Mount("/auth", authHandler.Router(verifier))
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(verifier))
		r.Mount("/notes", notesHandler.Router())
	})

	srv := &http.Server{
		Addr: cfg.HTTP.Addr, Handler: r,
		ReadTimeout: cfg.HTTP.ReadTimeout, WriteTimeout: cfg.HTTP.WriteTimeout, IdleTimeout: cfg.HTTP.IdleTimeout,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		base.Info("listening", slog.String("addr", cfg.HTTP.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			base.Error("server", slog.Any("err", err))
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	base.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		base.Error("shutdown", slog.Any("err", err))
	}
	base.Info("bye")
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
	return nil
}
