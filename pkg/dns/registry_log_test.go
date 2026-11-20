// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// captureDefaultLogger replaces the process-wide slog logger with one that
// writes records at or above level to the returned buffer, until the test
// ends. Tests that use it must not run in parallel.
func captureDefaultLogger(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

// TestRegistry_LookupMissIsNotLoggedAtInfo covers finding 1B #8: a lookup of
// a name that is not registered happens on every query for an unknown name
// and must not write to the log at INFO.
func TestRegistry_LookupMissIsNotLoggedAtInfo(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&DomainEntry{Name: "app.gslb.example.com"})

	buf := captureDefaultLogger(t, slog.LevelInfo)
	for i := 0; i < 3; i++ {
		if entry := registry.Lookup("missing.gslb.example.com."); entry != nil {
			t.Fatalf("lookup of an unregistered name returned %v", entry)
		}
	}
	if entry := registry.Lookup("app.gslb.example.com."); entry == nil {
		t.Fatal("lookup of a registered name returned nil")
	}
	if buf.Len() != 0 {
		t.Errorf("Registry.Lookup logged at INFO or above on the query path:\n%s", buf.String())
	}
}

// TestRegistry_LookupMissIsLoggedAtDebug checks that the miss is still
// visible when an operator turns on debug logging.
func TestRegistry_LookupMissIsLoggedAtDebug(t *testing.T) {
	registry := NewRegistry()

	buf := captureDefaultLogger(t, slog.LevelDebug)
	registry.Lookup("missing.gslb.example.com.")

	out := buf.String()
	if !strings.Contains(out, "level=DEBUG") || !strings.Contains(out, "missing.gslb.example.com.") {
		t.Errorf("want a DEBUG record naming the missing domain, got:\n%s", out)
	}
	if strings.Contains(out, "level=INFO") {
		t.Errorf("lookup miss still logged at INFO:\n%s", out)
	}
}
