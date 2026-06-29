// Day 33 — distributed rate limit + idempotency keys.
//
// New from Day 32:
//   - ratelimit.Limiter is an interface; ratelimit.NewMemory and
//     ratelimit.NewRedis both implement it
//   - main.go picks the backend from RATE_LIMIT_BACKEND
//   - idempotency.Middleware wraps mutating routes under /notes
//
// Everything else (notes cache, sqlc, transactions, audit, CORS,
// slog) is the Day 32 wiring.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/redis/go-redis/v9"

	"day33/internal/audit"
	"day33/internal/auth"
	"day33/internal/cache"
	"day33/internal/config"
	"day33/internal/dbtx"
	"day33/internal/idempotency"
	"day33/internal/logging"
	mw "day33/internal/middleware"
	"day33/internal/notes"
	"day33/internal/ratelimit"
	"day33/internal/respond"
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
	base.Info("config",
		slog.String("env", cfg.Env),
		slog.String("addr", cfg.HTTP.Addr),
		slog.String("rate_limit_backend", cfg.RateLimit.Backend),
	)

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

	rdb, err := openRedis(cfg)
	if err != nil {
		base.Error("open redis", slog.Any("err", err))
		return
	}
	defer rdb.Close()
	base.Info("connected to redis")

	notesCache := cache.New(rdb, cfg.Redis.Jitter)
	tx := dbtx.New(db)

	userRepo := auth.NewPostgresUserRepository(db)
	rtRepo := auth.NewPostgresRefreshTokenRepository(db)
	auditRepo := audit.NewPostgresRepository(db)
	notesRepo := notes.NewPostgresRepository(db)

	issuer := auth.NewTokenIssuer(cfg.Auth.JWTSecret, cfg.Auth.JWTAccessTTL, cfg.Auth.JWTIssuer)
	verifier := auth.NewTokenVerifier(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer)
	authSvc := auth.NewService(userRepo, rtRepo, issuer, cfg.Auth.RefreshTTL)
	notesSvc := notes.NewService(notesRepo, auditRepo, tx, notesCache, cfg.Redis.NotesTTL)

	authHandler := &auth.Handler{Svc: authSvc}
	notesHandler := &notes.Handler{Svc: notesSvc}
	auditHandler := &audit.Handler{Repo: auditRepo}

	globalLimiter, authLimiter, stopLimiters := buildLimiters(cfg, rdb)
	defer stopLimiters()

	idemStore := idempotency.NewStore(rdb, cfg.Idempotency.TTL, cfg.Idempotency.LeaseTTL)

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

	// Idempotency lives under auth so the key can be scoped per-user.
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(verifier))
		r.Use(idempotency.Middleware(idemStore))
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

// buildLimiters picks the rate-limit backend per RATE_LIMIT_BACKEND.
// Returns (global, auth, stop) — stop() is a no-op for Redis and
// closes the cleanup goroutines for in-process.
func buildLimiters(cfg config.Config, rdb *redis.Client) (ratelimit.Limiter, ratelimit.Limiter, func()) {
	switch cfg.RateLimit.Backend {
	case "redis":
		g := ratelimit.NewRedis(rdb, "rl:global", cfg.RateLimit.Window, cfg.RateLimit.GlobalMaxPerMinute)
		a := ratelimit.NewRedis(rdb, "rl:auth", cfg.RateLimit.Window, cfg.RateLimit.AuthMaxPerMinute)
		return g, a, func() {}
	default:
		g := ratelimit.NewMemory(cfg.RateLimit.GlobalRPS, cfg.RateLimit.GlobalBurst, cfg.RateLimit.TTL)
		a := ratelimit.NewMemory(cfg.RateLimit.AuthRPS, cfg.RateLimit.AuthBurst, cfg.RateLimit.TTL)
		return g, a, func() { g.Stop(); a.Stop() }
	}
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

func openRedis(cfg config.Config) (*redis.Client, error) {
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	rdb := redis.NewClient(opt)
	pingCtx, cancel := context.WithTimeout(context.Background(), cfg.Redis.PingTimeout)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return rdb, nil
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
