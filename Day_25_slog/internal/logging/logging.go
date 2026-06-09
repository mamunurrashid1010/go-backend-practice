// Package logging builds a slog.Logger from config and provides
// context plumbing for the request-scoped logger pattern.
//
// Usage:
//
//	logger := logging.New(cfg.LogLevel, cfg.IsProd())   // in main
//	ctx = logging.With(ctx, logger.With(...))           // in middleware
//	logging.From(ctx).Info("...", attrs...)             // anywhere
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// ctxKey is unexported so no other package can collide with our context key.
type ctxKey struct{}

// New builds the root logger. level is "debug" | "info" | "warn" | "error".
// asJSON true emits one-line JSON (prod); false emits human text (dev).
func New(level string, asJSON bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var h slog.Handler
	if asJSON {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

// With attaches a logger to ctx. Returns a child ctx.
func With(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// From retrieves the logger that was put on ctx by With. If none, returns
// slog.Default so callers never need a nil check.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
