// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"io"
	"log"
	"log/slog"

	"github.com/hashicorp/go-hclog"
)

// HCLogAdapter adapts slog.Logger to hashicorp's hclog.Logger interface.
type HCLogAdapter struct {
	logger *slog.Logger
	name   string
}

// NewHCLogAdapter creates a new HCLogAdapter.
func NewHCLogAdapter(logger *slog.Logger) hclog.Logger {
	return &HCLogAdapter{logger: logger}
}

// Log implements hclog.Logger.
func (h *HCLogAdapter) Log(level hclog.Level, msg string, args ...interface{}) {
	switch level {
	case hclog.Trace, hclog.Debug:
		h.logger.Debug(msg, convertArgs(args)...)
	case hclog.Info:
		h.logger.Info(msg, convertArgs(args)...)
	case hclog.Warn:
		h.logger.Warn(msg, convertArgs(args)...)
	case hclog.Error:
		h.logger.Error(msg, convertArgs(args)...)
	}
}

// Trace implements hclog.Logger.
func (h *HCLogAdapter) Trace(msg string, args ...interface{}) {
	h.logger.Debug(msg, convertArgs(args)...)
}

// Debug implements hclog.Logger.
func (h *HCLogAdapter) Debug(msg string, args ...interface{}) {
	h.logger.Debug(msg, convertArgs(args)...)
}

// Info implements hclog.Logger.
func (h *HCLogAdapter) Info(msg string, args ...interface{}) {
	h.logger.Info(msg, convertArgs(args)...)
}

// Warn implements hclog.Logger.
func (h *HCLogAdapter) Warn(msg string, args ...interface{}) {
	h.logger.Warn(msg, convertArgs(args)...)
}

// Error implements hclog.Logger.
func (h *HCLogAdapter) Error(msg string, args ...interface{}) {
	h.logger.Error(msg, convertArgs(args)...)
}

// IsTrace implements hclog.Logger.
func (h *HCLogAdapter) IsTrace() bool { return true }

// IsDebug implements hclog.Logger.
func (h *HCLogAdapter) IsDebug() bool { return true }

// IsInfo implements hclog.Logger.
func (h *HCLogAdapter) IsInfo() bool { return true }

// IsWarn implements hclog.Logger.
func (h *HCLogAdapter) IsWarn() bool { return true }

// IsError implements hclog.Logger.
func (h *HCLogAdapter) IsError() bool { return true }

// ImpliedArgs implements hclog.Logger.
func (h *HCLogAdapter) ImpliedArgs() []interface{} { return nil }

// With implements hclog.Logger.
func (h *HCLogAdapter) With(args ...interface{}) hclog.Logger {
	return &HCLogAdapter{
		logger: h.logger.With(convertArgs(args)...),
		name:   h.name,
	}
}

// Name implements hclog.Logger.
func (h *HCLogAdapter) Name() string { return h.name }

// Named implements hclog.Logger.
func (h *HCLogAdapter) Named(name string) hclog.Logger {
	newName := name
	if h.name != "" {
		newName = h.name + "." + name
	}
	return &HCLogAdapter{
		logger: h.logger.With("name", newName),
		name:   newName,
	}
}

// ResetNamed implements hclog.Logger.
func (h *HCLogAdapter) ResetNamed(name string) hclog.Logger {
	return &HCLogAdapter{
		logger: h.logger.With("name", name),
		name:   name,
	}
}

// SetLevel implements hclog.Logger.
func (h *HCLogAdapter) SetLevel(level hclog.Level) {}

// GetLevel implements hclog.Logger.
func (h *HCLogAdapter) GetLevel() hclog.Level { return hclog.Debug }

// StandardLogger implements hclog.Logger.
func (h *HCLogAdapter) StandardLogger(opts *hclog.StandardLoggerOptions) *log.Logger {
	return log.New(io.Discard, "", 0)
}

// StandardWriter implements hclog.Logger.
func (h *HCLogAdapter) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	return io.Discard
}

// convertArgs converts hclog key-value pairs to slog format.
func convertArgs(args []interface{}) []any {
	if len(args) == 0 {
		return nil
	}
	result := make([]any, len(args))
	copy(result, args)
	return result
}
