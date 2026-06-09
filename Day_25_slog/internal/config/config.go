// Package config — Day 25 adds LOG_LEVEL.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env      string `env:"APP_ENV" envDefault:"development"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	HTTP     HTTPConfig
	DB       DBConfig
	Auth     AuthConfig
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
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"10"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"5m"`
	ConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"2m"`
	PingTimeout     time.Duration `env:"DB_PING_TIMEOUT" envDefault:"5s"`
}

type AuthConfig struct {
	JWTSecret    string        `env:"JWT_SECRET,required"`
	JWTAccessTTL time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	JWTIssuer    string        `env:"JWT_ISSUER" envDefault:"day25-auth"`
	RefreshTTL   time.Duration `env:"REFRESH_TTL" envDefault:"720h"`
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
	return nil
}

func (c Config) IsDev() bool  { return c.Env == "development" }
func (c Config) IsProd() bool { return c.Env == "production" }
