package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env         string `env:"APP_ENV" envDefault:"development"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	HTTP        HTTPConfig
	DB          DBConfig
	Redis       RedisConfig
	Auth        AuthConfig
	RateLimit   RateLimitConfig
	Idempotency IdempotencyConfig
	CORS        CORSConfig
}
type HTTPConfig struct {
	Addr            string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
	IdleTimeout     time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
	ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"10s"`
}
type DBConfig struct {
	URL             string        `env:"DATABASE_URL,required"`
	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"25"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"5m"`
	ConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"2m"`
	PingTimeout     time.Duration `env:"DB_PING_TIMEOUT" envDefault:"5s"`
}
type RedisConfig struct {
	URL         string        `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`
	PingTimeout time.Duration `env:"REDIS_PING_TIMEOUT" envDefault:"3s"`
	NotesTTL    time.Duration `env:"REDIS_NOTES_TTL" envDefault:"5m"`
	Jitter      float64       `env:"REDIS_JITTER" envDefault:"0.1"`
}
type AuthConfig struct {
	JWTSecret    string        `env:"JWT_SECRET,required"`
	JWTAccessTTL time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	JWTIssuer    string        `env:"JWT_ISSUER" envDefault:"day34-auth"`
	RefreshTTL   time.Duration `env:"REFRESH_TTL" envDefault:"720h"`
}
type RateLimitConfig struct {
	Backend            string        `env:"RATE_LIMIT_BACKEND" envDefault:"redis"`
	GlobalRPS          float64       `env:"RATE_LIMIT_GLOBAL_RPS" envDefault:"10"`
	GlobalBurst        int           `env:"RATE_LIMIT_GLOBAL_BURST" envDefault:"30"`
	AuthRPS            float64       `env:"RATE_LIMIT_AUTH_RPS" envDefault:"1"`
	AuthBurst          int           `env:"RATE_LIMIT_AUTH_BURST" envDefault:"5"`
	TTL                time.Duration `env:"RATE_LIMIT_TTL" envDefault:"5m"`
	GlobalMaxPerMinute int           `env:"RATE_LIMIT_GLOBAL_MAX_PER_MINUTE" envDefault:"60"`
	AuthMaxPerMinute   int           `env:"RATE_LIMIT_AUTH_MAX_PER_MINUTE" envDefault:"10"`
	Window             time.Duration `env:"RATE_LIMIT_WINDOW" envDefault:"1m"`
}
type IdempotencyConfig struct {
	TTL      time.Duration `env:"IDEMPOTENCY_TTL" envDefault:"24h"`
	LeaseTTL time.Duration `env:"IDEMPOTENCY_LEASE_TTL" envDefault:"60s"`
}
type CORSConfig struct {
	AllowedOrigins   []string      `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
	AllowedMethods   []string      `env:"CORS_ALLOWED_METHODS" envSeparator:"," envDefault:"GET,POST,PUT,PATCH,DELETE,OPTIONS"`
	AllowedHeaders   []string      `env:"CORS_ALLOWED_HEADERS" envSeparator:"," envDefault:"Authorization,Content-Type,X-Request-ID,Idempotency-Key"`
	ExposedHeaders   []string      `env:"CORS_EXPOSED_HEADERS" envSeparator:"," envDefault:"X-Request-ID,X-RateLimit-Limit,X-RateLimit-Remaining,X-RateLimit-Reset,Retry-After,Idempotent-Replayed"`
	MaxAge           time.Duration `env:"CORS_MAX_AGE" envDefault:"5m"`
	AllowCredentials bool          `env:"CORS_ALLOW_CREDENTIALS" envDefault:"true"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	switch c.Env {
	case "development", "staging", "production":
	default:
		return fmt.Errorf("APP_ENV must be development|staging|production, got %q", c.Env)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be debug|info|warn|error, got %q", c.LogLevel)
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 chars")
	}
	if c.Auth.RefreshTTL <= c.Auth.JWTAccessTTL {
		return fmt.Errorf("REFRESH_TTL must be greater than JWT_ACCESS_TTL")
	}
	if c.DB.MaxIdleConns > c.DB.MaxOpenConns {
		return fmt.Errorf("DB_MAX_IDLE_CONNS (%d) must not exceed DB_MAX_OPEN_CONNS (%d)", c.DB.MaxIdleConns, c.DB.MaxOpenConns)
	}
	if c.Redis.Jitter < 0 || c.Redis.Jitter > 1 {
		return fmt.Errorf("REDIS_JITTER must be in [0, 1], got %f", c.Redis.Jitter)
	}
	if c.Redis.NotesTTL <= 0 {
		return fmt.Errorf("REDIS_NOTES_TTL must be > 0")
	}
	switch c.RateLimit.Backend {
	case "memory", "redis":
	default:
		return fmt.Errorf("RATE_LIMIT_BACKEND must be memory|redis, got %q", c.RateLimit.Backend)
	}
	if c.RateLimit.GlobalRPS <= 0 || c.RateLimit.GlobalBurst <= 0 {
		return fmt.Errorf("rate limit global rps and burst must be > 0")
	}
	if c.RateLimit.AuthRPS <= 0 || c.RateLimit.AuthBurst <= 0 {
		return fmt.Errorf("rate limit auth rps and burst must be > 0")
	}
	if c.RateLimit.GlobalMaxPerMinute <= 0 || c.RateLimit.AuthMaxPerMinute <= 0 {
		return fmt.Errorf("rate limit max-per-minute must be > 0")
	}
	if c.RateLimit.Window <= 0 {
		return fmt.Errorf("RATE_LIMIT_WINDOW must be > 0")
	}
	if c.Idempotency.TTL <= 0 || c.Idempotency.LeaseTTL <= 0 {
		return fmt.Errorf("idempotency TTLs must be > 0")
	}
	if c.Idempotency.LeaseTTL > c.Idempotency.TTL {
		return fmt.Errorf("IDEMPOTENCY_LEASE_TTL must be <= IDEMPOTENCY_TTL")
	}
	for _, o := range c.CORS.AllowedOrigins {
		if o == "*" && c.CORS.AllowCredentials {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS=* is not allowed with CORS_ALLOW_CREDENTIALS=true")
		}
	}
	return nil
}

func (c Config) IsDev() bool  { return c.Env == "development" }
func (c Config) IsProd() bool { return c.Env == "production" }
