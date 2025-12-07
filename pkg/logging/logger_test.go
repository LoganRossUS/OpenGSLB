// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger_Levels(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		expectedLevel slog.Level
		wantErr       bool
	}{
		{name: "debug level", level: "debug", expectedLevel: slog.LevelDebug},
		{name: "info level", level: "info", expectedLevel: slog.LevelInfo},
		{name: "warn level", level: "warn", expectedLevel: slog.LevelWarn},
		{name: "warning level", level: "warning", expectedLevel: slog.LevelWarn},
		{name: "error level", level: "error", expectedLevel: slog.LevelError},
		{name: "empty defaults to info", level: "", expectedLevel: slog.LevelInfo},
		{name: "case insensitive", level: "DEBUG", expectedLevel: slog.LevelDebug},
		{name: "invalid level", level: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Level: tt.level, Format: "text"}
			logger, err := NewLogger(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if logger == nil {
				t.Fatal("expected logger, got nil")
			}

			// Verify level by checking if messages are logged
			var buf bytes.Buffer
			logger, _ = NewLoggerWithWriter(cfg, &buf)

			// Log at the expected level - should appear
			switch tt.expectedLevel {
			case slog.LevelDebug:
				logger.Debug("test")
			case slog.LevelInfo:
				logger.Info("test")
			case slog.LevelWarn:
				logger.Warn("test")
			case slog.LevelError:
				logger.Error("test")
			}

			if buf.Len() == 0 {
				t.Error("expected log output at configured level")
			}
		})
	}
}

func TestNewLogger_Formats(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		isJSON  bool
		wantErr bool
	}{
		{name: "json format", format: "json", isJSON: true},
		{name: "text format", format: "text", isJSON: false},
		{name: "empty defaults to json", format: "", isJSON: true},
		{name: "case insensitive", format: "JSON", isJSON: true},
		{name: "invalid format", format: "xml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := Config{Level: "info", Format: tt.format}
			logger, err := NewLoggerWithWriter(cfg, &buf)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Log a message
			logger.Info("test message", "key", "value")
			output := buf.String()

			if tt.isJSON {
				// Verify it's valid JSON
				var m map[string]interface{}
				if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &m); err != nil {
					t.Errorf("expected JSON output, got: %s", output)
				}
				// Verify key fields present
				if m["msg"] != "test message" {
					t.Errorf("expected msg field, got: %v", m)
				}
			} else {
				// Text format should not be valid JSON
				var m map[string]interface{}
				if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &m); err == nil {
					t.Error("expected text output, got JSON")
				}
				// Should contain the message
				if !strings.Contains(output, "test message") {
					t.Errorf("expected output to contain message, got: %s", output)
				}
			}
		})
	}
}

func TestNewLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{Level: "warn", Format: "text"}
	logger, err := NewLoggerWithWriter(cfg, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Debug and Info should not appear
	logger.Debug("debug message")
	logger.Info("info message")
	if buf.Len() > 0 {
		t.Errorf("expected no output for debug/info at warn level, got: %s", buf.String())
	}

	// Warn and Error should appear
	logger.Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Error("expected warn message in output")
	}

	buf.Reset()
	logger.Error("error message")
	if !strings.Contains(buf.String(), "error message") {
		t.Error("expected error message in output")
	}
}
