// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTableFormatter_PrintTable(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		rows     [][]string
		contains []string
	}{
		{
			name:    "basic table",
			headers: []string{"NAME", "STATUS"},
			rows: [][]string{
				{"server1", "healthy"},
				{"server2", "unhealthy"},
			},
			contains: []string{"NAME", "STATUS", "server1", "healthy", "server2", "unhealthy"},
		},
		{
			name:     "empty table",
			headers:  []string{"NAME", "STATUS"},
			rows:     [][]string{},
			contains: []string{"No data available"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			f := &TableFormatter{Writer: buf}
			f.PrintTable(tt.headers, tt.rows)

			output := buf.String()
			for _, s := range tt.contains {
				if !strings.Contains(output, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, output)
				}
			}
		})
	}
}

func TestTableFormatter_PrintKeyValue(t *testing.T) {
	buf := &bytes.Buffer{}
	f := &TableFormatter{Writer: buf}

	pairs := []KVPair{
		{Key: "Mode", Value: "overwatch"},
		{Key: "Uptime", Value: "1h 30m"},
	}

	f.PrintKeyValue(pairs)

	output := buf.String()
	if !strings.Contains(output, "Mode:") {
		t.Errorf("expected output to contain 'Mode:', got:\n%s", output)
	}
	if !strings.Contains(output, "overwatch") {
		t.Errorf("expected output to contain 'overwatch', got:\n%s", output)
	}
}

func TestTableFormatter_PrintMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	f := &TableFormatter{Writer: buf}

	f.PrintMessage("Test message")

	if !strings.Contains(buf.String(), "Test message") {
		t.Errorf("expected output to contain 'Test message', got: %s", buf.String())
	}
}

func TestJSONFormatter_PrintTable(t *testing.T) {
	buf := &bytes.Buffer{}
	f := &JSONFormatter{Writer: buf, Pretty: false}

	headers := []string{"NAME", "STATUS"}
	rows := [][]string{
		{"server1", "healthy"},
		{"server2", "unhealthy"},
	}

	f.PrintTable(headers, rows)

	var result []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}

	if result[0]["name"] != "server1" {
		t.Errorf("expected 'server1', got %q", result[0]["name"])
	}
	if result[0]["status"] != "healthy" {
		t.Errorf("expected 'healthy', got %q", result[0]["status"])
	}
}

func TestJSONFormatter_PrintKeyValue(t *testing.T) {
	buf := &bytes.Buffer{}
	f := &JSONFormatter{Writer: buf, Pretty: false}

	pairs := []KVPair{
		{Key: "Mode", Value: "overwatch"},
		{Key: "Up Time", Value: "1h 30m"},
	}

	f.PrintKeyValue(pairs)

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["mode"] != "overwatch" {
		t.Errorf("expected 'overwatch', got %q", result["mode"])
	}
	// Key with space should be converted to underscore
	if result["up_time"] != "1h 30m" {
		t.Errorf("expected '1h 30m', got %q", result["up_time"])
	}
}

func TestJSONFormatter_PrintMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	f := &JSONFormatter{Writer: buf, Pretty: false}

	f.PrintMessage("Test message")

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["message"] != "Test message" {
		t.Errorf("expected 'Test message', got %q", result["message"])
	}
}

func TestJSONFormatter_PrintPretty(t *testing.T) {
	buf := &bytes.Buffer{}
	f := &JSONFormatter{Writer: buf, Pretty: true}

	f.Print(map[string]string{"key": "value"})

	// Pretty output should contain newlines and indentation
	if !strings.Contains(buf.String(), "\n") {
		t.Error("expected pretty output to contain newlines")
	}
}

func TestGetFormatter(t *testing.T) {
	// JSON output
	f := GetFormatter(true)
	if _, ok := f.(*JSONFormatter); !ok {
		t.Error("expected JSONFormatter for jsonOutput=true")
	}

	// Table output
	f = GetFormatter(false)
	if _, ok := f.(*TableFormatter); !ok {
		t.Error("expected TableFormatter for jsonOutput=false")
	}
}
