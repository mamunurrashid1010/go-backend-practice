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
	JWTIssuer    string        `env:"JWT_ISSUER" envDefault:"day24-auth"`
	RefreshTTL   time.Duration `env:"REFRESH_TTL" envDefault:"720h"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if len(cfg.Auth.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 chars")
	}
	if cfg.Auth.RefreshTTL <= cfg.Auth.JWTAccessTTL {
		return Config{}, fmt.Errorf("REFRESH_TTL must be greater than JWT_ACCESS_TTL")
	}
	return cfg, nil
}
