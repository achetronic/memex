// Package logger provides a slog-based logger initializer.
// It supports two output formats: "console" (human-readable) and "json"
// (structured, suitable for production and log aggregators).
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New creates and returns a *slog.Logger configured according to the given
// format and level strings. Format must be "console" or "json". Level must
// be one of "debug", "info", "warn", "error". Unknown values fall back to
// "info" and "console" respectively.
func New(format, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
