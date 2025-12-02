// Package logging provides configurable structured logging for OpenGSLB.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config holds logging configuration.
type Config struct {
	Level  string
	Format string
}

// NewLogger creates a configured slog.Logger based on the provided config.
// Supported levels: debug, info, warn, error (case-insensitive)
// Supported formats: text, json (case-insensitive, defaults to json)
func NewLogger(cfg Config) (*slog.Logger, error) {
	return NewLoggerWithWriter(cfg, os.Stdout)
}

// NewLoggerWithWriter creates a configured slog.Logger writing to the specified writer.
// This is useful for testing.
func NewLoggerWithWriter(cfg Config, w io.Writer) (*slog.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "text":
		handler = slog.NewTextHandler(w, opts)
	case "json", "":
		handler = slog.NewJSONHandler(w, opts)
	default:
		return nil, fmt.Errorf("unsupported log format: %q (supported: text, json)", cfg.Format)
	}

	return slog.New(handler), nil
}

// parseLevel converts a string level to slog.Level.
func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level: %q (supported: debug, info, warn, error)", level)
	}
}