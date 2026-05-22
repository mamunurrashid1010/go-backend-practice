// Package config holds all environment-driven settings for the service.
//
// The contract:
//   - call Load() ONCE, at the top of main()
//   - pass the resulting Config (or sub-structs) into constructors
//   - never call os.Getenv anywhere else
//
// Why a struct, not a bunch of os.Getenv calls?
//
//   1. Typed:    cfg.DB.MaxOpenConns is an int; no strconv.Atoi at every call site.
//   2. Defaulted: every knob has a sensible default in one place.
//   3. Validated: bad config fails at startup, not on the first request.
//   4. Discoverable: one struct = the complete list of knobs the service has.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config — the whole thing.
type Config struct {
	Env  string `env:"APP_ENV" envDefault:"development"`
	HTTP HTTPConfig
	DB   DBConfig
}

// HTTPConfig — the HTTP server's settings.
type HTTPConfig struct {
	Addr         string        `env:"HTTP_ADDR"          envDefault:":8080"`
	ReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT"  envDefault:"10s"`
	WriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
	IdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT"  envDefault:"60s"`
}

// DBConfig — Postgres connection + pool settings.
type DBConfig struct {
	// `required` — no default. Misconfiguration fails at startup.
	URL string `env:"DATABASE_URL,required"`

	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS"     envDefault:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS"     envDefault:"10"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME"  envDefault:"5m"`
	ConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"2m"`
	PingTimeout     time.Duration `env:"DB_PING_TIMEOUT"       envDefault:"5s"`
}

// Load reads env vars into a Config, applies defaults, and validates.
// Returns a typed Config or an error suitable to log.Fatal with.
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

// validate — semantic checks the type system can't enforce.
// Add new rules here as the service grows.
func (c Config) validate() error {
	switch c.Env {
	case "development", "staging", "production":
		// ok
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
	if c.HTTP.ReadTimeout <= 0 {
		return fmt.Errorf("HTTP_READ_TIMEOUT must be > 0, got %s", c.HTTP.ReadTimeout)
	}

	return nil
}

// IsDev — handy boolean for "should we be chatty in logs?"-style decisions.
func (c Config) IsDev() bool { return c.Env == "development" }
