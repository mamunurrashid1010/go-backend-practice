// Day 31 — sqlc.
//
// New from Day 30:
//   - sqlc.yaml + queries/*.sql describe the canonical user + note queries
//   - internal/db/ holds the generated (committed) Go code
//   - notes + auth repositories are thin adapters over *db.Queries
//
// Everything else (transactions, audit JOIN strategies, cursor List,
// auth + refresh + CORS + rate limit + slog) is the Day 30 wiring.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"day31/internal/audit"
	"day31/internal/auth"
	"day31/internal/config"
	"day31/internal/dbtx"
	"day31/internal/logging"
	mw "day31/internal/middleware"
	"day31/internal/notes"
	"day31/internal/ratelimit"
	"day31/internal/respond"
)

func main() {
	if err := godotenv.Load(); err == nil {
		slog.Default().Info("loaded .env")
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Default().Error("config", slog.Any("err", err))
		return
	}

	base := logging.New(cfg.LogLevel, !cfg.IsDev())
	slog.SetDefault(base)
	base.Info("config", slog.String("env", cfg.Env), slog.String("addr", cfg.HTTP.Addr))

	db, err := openDB(cfg)
	if err != nil {
		base.Error("open db", slog.Any("err", err))
		return
	}
	defer db.Close()

	if err := runMigrations(cfg.DB.URL, base); err != nil {
		base.Error("migrate", slog.Any("err", err))
		return
	}
	base.Info("connected to postgres")

	tx := dbtx.New(db)

	userRepo := auth.NewPostgresUserRepository(db)
	rtRepo := auth.NewPostgresRefreshTokenRepository(db)
	auditRepo := audit.NewPostgresRepository(db)
	notesRepo := notes.NewPostgresRepository(db)

	issuer := auth.NewTokenIssuer(cfg.Auth.JWTSecret, cfg.Auth.JWTAccessTTL, cfg.Auth.JWTIssuer)
	verifier := auth.NewTokenVerifier(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer)
	authSvc := auth.NewService(userRepo, rtRepo, issuer, cfg.Auth.RefreshTTL)
	notesSvc := notes.NewService(notesRepo, auditRepo, tx)

	authHandler := &auth.Handler{Svc: authSvc}
	notesHandler := &notes.Handler{Svc: notesSvc}
	auditHandler := &audit.Handler{Repo: auditRepo}

	globalLimiter := ratelimit.New(cfg.RateLimit.GlobalRPS, cfg.RateLimit.GlobalBurst, cfg.RateLimit.TTL)
	authLimiter := ratelimit.New(cfg.RateLimit.AuthRPS, cfg.RateLimit.AuthBurst, cfg.RateLimit.TTL)
	defer globalLimiter.Stop()
	defer authLimiter.Stop()

	corsMW := mw.CORS(mw.CORSConfig{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowedMethods:   cfg.CORS.AllowedMethods,
		AllowedHeaders:   cfg.CORS.AllowedHeaders,
		ExposedHeaders:   cfg.CORS.ExposedHeaders,
		MaxAge:           cfg.CORS.MaxAge,
		AllowCredentials: cfg.CORS.AllowCredentials,
	})

	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.Logger(base))
	r.Use(corsMW)
	r.Use(mw.RateLimit(globalLimiter))
	r.Use(mw.Recover)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Mount("/auth", authHandler.Router(verifier, mw.RateLimit(authLimiter)))

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(verifier))
		r.Mount("/notes", notesHandler.Router())
		r.Mount("/audit", auditHandler.Router())
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		respond.NotFound(w, "route not found: "+r.Method+" "+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		respond.MethodNotAllowed(w, "")
	})

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		base.Info("listening", slog.String("addr", "http://localhost"+cfg.HTTP.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			base.Error("serve", slog.Any("err", err))
			stop()
		}
	}()

	<-ctx.Done()
	base.Info("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		base.Error("shutdown", slog.Any("err", err))
		return
	}
	base.Info("shutdown clean")
}

func openDB(cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DB.URL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DB.ConnMaxIdleTime)
	pingCtx, cancel := context.WithTimeout(context.Background(), cfg.DB.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runMigrations(dsn string, log *slog.Logger) error {
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
		log.Info("migrations: none applied")
		return nil
	}
	if vErr == nil {
		log.Info("migrations up", slog.Int("version", int(v)))
	}
	return nil
}
