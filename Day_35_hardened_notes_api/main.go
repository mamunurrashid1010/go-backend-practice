// Day 35 — Week 5 close.
//
// No new feature today. The Day 34 wiring is intact:
//   - transactions with atomic audit log (Day 29)
//   - N+1-aware audit JOIN strategies (Day 30)
//   - sqlc-generated repositories for users + notes (Day 31)
//   - Redis cache-aside on GET /notes/{id} with singleflight (Day 32)
//   - Redis sliding-window rate limiter + Idempotency-Key (Day 33)
//   - OpenAPI 3.0 spec + Swagger UI at /docs/ (Day 34)
//
// What changed: auth.RequireUserID extracted; the same helper was
// hand-rolled in notes and audit handlers. README, LICENSE, Makefile,
// tools.go make the repo clonable.
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

	"day35/internal/audit"
	"day35/internal/auth"
	"day35/internal/cache"
	"day35/internal/config"
	"day35/internal/dbtx"
	"day35/internal/idempotency"
	"day35/internal/logging"
	mw "day35/internal/middleware"
	"day35/internal/notes"
	"day35/internal/openapi"
	"day35/internal/ratelimit"
	"day35/internal/respond"
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

	r.Route("/docs", openapi.Mount)

	r.Mount("/auth", authHandler.Router(verifier, mw.RateLimit(authLimiter)))

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
		base.Info("listening",
			slog.String("addr", "http://localhost"+cfg.HTTP.Addr),
			slog.String("docs", "http://localhost"+cfg.HTTP.Addr+"/docs/"),
		)
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
