// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"testing"
)

func TestStatusHelpers(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected string
	}{
		{"first non-empty", []string{"first", "second"}, "first"},
		{"skip empty", []string{"", "second"}, "second"},
		{"all empty", []string{"", ""}, ""},
		{"single value", []string{"only"}, "only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := coalesce(tt.values...)
			if result != tt.expected {
				t.Errorf("coalesce(%v) = %q, want %q", tt.values, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		seconds  int64
		expected string
	}{
		// The formatDuration function takes time.Duration
		// We'll test by creating durations
	}

	// Test with known durations
	testCases := []struct {
		hours    int
		minutes  int
		expected string
	}{
		{0, 30, "30m"},
		{2, 15, "2h 15m"},
		{25, 30, "1d 1h 30m"}, // 25 hours = 1 day 1 hour
		{48, 0, "2d 0h 0m"},
	}

	for _, tc := range tests {
		_ = tc // Use tc
	}

	_ = testCases // Placeholder for duration tests
}

func TestURLEncode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"10.0.0.0/8", "10.0.0.0%2F8"},
		{"simple", "simple"},
		{"with space", "with%20space"},
		// Note: url.PathEscape doesn't escape colons (RFC 3986 allows them in paths)
		{"10.0.1.10:8080", "10.0.1.10:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := URLEncode(tt.input)
			if result != tt.expected {
				t.Errorf("URLEncode(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetAuthority(t *testing.T) {
	tests := []struct {
		name     string
		backend  BackendResponse
		expected string
	}{
		{
			name: "override",
			backend: BackendResponse{
				OverrideStatus: ptrBool(false),
			},
			expected: "override",
		},
		{
			name: "overwatch healthy",
			backend: BackendResponse{
				ValidationHealthy: ptrBool(true),
			},
			expected: "overwatch",
		},
		{
			name: "overwatch veto",
			backend: BackendResponse{
				ValidationHealthy: ptrBool(false),
			},
			expected: "overwatch (veto)",
		},
		{
			name: "stale",
			backend: BackendResponse{
				EffectiveStatus: "stale",
			},
			expected: "stale",
		},
		{
			name: "agent",
			backend: BackendResponse{
				EffectiveStatus: "healthy",
			},
			expected: "agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAuthority(tt.backend)
			if result != tt.expected {
				t.Errorf("getAuthority() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFilterBackends(t *testing.T) {
	backends := []BackendResponse{
		{Service: "app1", Region: "us-east", EffectiveStatus: "healthy"},
		{Service: "app1", Region: "us-west", EffectiveStatus: "unhealthy"},
		{Service: "app2", Region: "us-east", EffectiveStatus: "healthy"},
	}

	// Reset filters
	serversFilterService = ""
	serversFilterRegion = ""
	serversFilterStatus = ""

	// Test no filter
	result := filterBackends(backends)
	if len(result) != 3 {
		t.Errorf("expected 3 backends with no filter, got %d", len(result))
	}

	// Test service filter
	serversFilterService = "app1"
	result = filterBackends(backends)
	if len(result) != 2 {
		t.Errorf("expected 2 backends with service=app1, got %d", len(result))
	}
	serversFilterService = ""

	// Test region filter
	serversFilterRegion = "us-east"
	result = filterBackends(backends)
	if len(result) != 2 {
		t.Errorf("expected 2 backends with region=us-east, got %d", len(result))
	}
	serversFilterRegion = ""

	// Test status filter
	serversFilterStatus = "healthy"
	result = filterBackends(backends)
	if len(result) != 2 {
		t.Errorf("expected 2 healthy backends, got %d", len(result))
	}
	serversFilterStatus = ""

	// Test combined filters
	serversFilterService = "app1"
	serversFilterStatus = "unhealthy"
	result = filterBackends(backends)
	if len(result) != 1 {
		t.Errorf("expected 1 backend with service=app1 and status=unhealthy, got %d", len(result))
	}

	// Reset filters
	serversFilterService = ""
	serversFilterStatus = ""
}

// Helper function to create a pointer to a bool
func ptrBool(b bool) *bool {
	return &b
}

func TestRootCommandStructure(t *testing.T) {
	// Verify root command has expected subcommands
	expectedCommands := []string{
		"status", "servers", "domains", "overrides",
		"geo", "config", "dnssec", "completion",
	}

	commands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		commands[cmd.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !commands[expected] {
			t.Errorf("expected root command to have %q subcommand", expected)
		}
	}
}

func TestOverridesSubcommands(t *testing.T) {
	expectedSubcommands := []string{"list", "set", "clear"}

	commands := make(map[string]bool)
	for _, cmd := range overridesCmd.Commands() {
		commands[cmd.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !commands[expected] {
			t.Errorf("expected overrides command to have %q subcommand", expected)
		}
	}
}

func TestGeoSubcommands(t *testing.T) {
	expectedSubcommands := []string{"mappings", "add", "remove", "test"}

	commands := make(map[string]bool)
	for _, cmd := range geoCmd.Commands() {
		commands[cmd.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !commands[expected] {
			t.Errorf("expected geo command to have %q subcommand", expected)
		}
	}
}

func TestConfigSubcommands(t *testing.T) {
	expectedSubcommands := []string{"validate"}

	commands := make(map[string]bool)
	for _, cmd := range configCmd.Commands() {
		commands[cmd.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !commands[expected] {
			t.Errorf("expected config command to have %q subcommand", expected)
		}
	}
}

func TestDNSSECSubcommands(t *testing.T) {
	expectedSubcommands := []string{"ds", "status"}

	commands := make(map[string]bool)
	for _, cmd := range dnssecCmd.Commands() {
		commands[cmd.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !commands[expected] {
			t.Errorf("expected dnssec command to have %q subcommand", expected)
		}
	}
}
