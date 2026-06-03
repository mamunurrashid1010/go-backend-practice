// Package config — Day 20 adds REFRESH_TTL.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env  string `env:"APP_ENV" envDefault:"development"`
	HTTP HTTPConfig
	DB   DBConfig
	Auth AuthConfig
}

type HTTPConfig struct {
	Addr            string        `env:"HTTP_ADDR"             envDefault:":8080"`
	ReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT"     envDefault:"10s"`
	WriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT"    envDefault:"10s"`
	IdleTimeout     time.Duration `env:"HTTP_IDLE_TIMEOUT"     envDefault:"60s"`
	ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

type DBConfig struct {
	URL             string        `env:"DATABASE_URL,required"`
	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS"     envDefault:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS"     envDefault:"10"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME"  envDefault:"5m"`
	ConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"2m"`
	PingTimeout     time.Duration `env:"DB_PING_TIMEOUT"       envDefault:"5s"`
}

type AuthConfig struct {
	JWTSecret    string        `env:"JWT_SECRET,required"`
	JWTAccessTTL time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	JWTIssuer    string        `env:"JWT_ISSUER"     envDefault:"day20-auth"`
	RefreshTTL   time.Duration `env:"REFRESH_TTL"    envDefault:"720h"` // 30 days
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
	if c.DB.MaxOpenConns < 1 {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be >= 1, got %d", c.DB.MaxOpenConns)
	}
	if c.DB.MaxIdleConns < 0 || c.DB.MaxIdleConns > c.DB.MaxOpenConns {
		return fmt.Errorf("DB_MAX_IDLE_CONNS (%d) must be between 0 and DB_MAX_OPEN_CONNS (%d)",
			c.DB.MaxIdleConns, c.DB.MaxOpenConns)
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		return fmt.Errorf("HTTP_SHUTDOWN_TIMEOUT must be > 0")
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 chars (got %d)", len(c.Auth.JWTSecret))
	}
	if c.Auth.JWTAccessTTL <= 0 {
		return fmt.Errorf("JWT_ACCESS_TTL must be > 0")
	}
	if c.Auth.RefreshTTL <= 0 {
		return fmt.Errorf("REFRESH_TTL must be > 0")
	}
	if c.Auth.RefreshTTL <= c.Auth.JWTAccessTTL {
		return fmt.Errorf("REFRESH_TTL (%s) must be greater than JWT_ACCESS_TTL (%s)",
			c.Auth.RefreshTTL, c.Auth.JWTAccessTTL)
	}
	return nil
}

func (c Config) IsDev() bool { return c.Env == "development" }
