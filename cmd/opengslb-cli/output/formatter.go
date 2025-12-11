// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package output provides formatters for CLI output.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// Formatter defines the interface for output formatting.
type Formatter interface {
	// Print outputs the data in the formatter's format.
	Print(data interface{}) error
	// PrintTable outputs tabular data with headers.
	PrintTable(headers []string, rows [][]string)
	// PrintKeyValue outputs key-value pairs.
	PrintKeyValue(pairs []KVPair)
	// PrintMessage outputs a simple message.
	PrintMessage(msg string)
	// PrintError outputs an error message.
	PrintError(err error)
}

// KVPair represents a key-value pair for output.
type KVPair struct {
	Key   string
	Value string
}

// TableFormatter outputs human-readable tables.
type TableFormatter struct {
	Writer io.Writer
}

// NewTableFormatter creates a new TableFormatter.
func NewTableFormatter() *TableFormatter {
	return &TableFormatter{Writer: os.Stdout}
}

// Print outputs data as formatted key-value pairs.
func (f *TableFormatter) Print(data interface{}) error {
	// For generic data, just use fmt.Printf
	fmt.Fprintf(f.Writer, "%+v\n", data)
	return nil
}

// PrintTable outputs tabular data with headers.
func (f *TableFormatter) PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Fprintln(f.Writer, "No data available.")
		return
	}

	w := tabwriter.NewWriter(f.Writer, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Print headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
}

// PrintKeyValue outputs key-value pairs in a formatted way.
func (f *TableFormatter) PrintKeyValue(pairs []KVPair) {
	// Find maximum key length for alignment
	maxKeyLen := 0
	for _, pair := range pairs {
		if len(pair.Key) > maxKeyLen {
			maxKeyLen = len(pair.Key)
		}
	}

	for _, pair := range pairs {
		fmt.Fprintf(f.Writer, "  %-*s  %s\n", maxKeyLen+1, pair.Key+":", pair.Value)
	}
}

// PrintMessage outputs a simple message.
func (f *TableFormatter) PrintMessage(msg string) {
	fmt.Fprintln(f.Writer, msg)
}

// PrintError outputs an error message.
func (f *TableFormatter) PrintError(err error) {
	fmt.Fprintf(f.Writer, "Error: %v\n", err)
}

// JSONFormatter outputs JSON.
type JSONFormatter struct {
	Writer io.Writer
	Pretty bool
}

// NewJSONFormatter creates a new JSONFormatter.
func NewJSONFormatter(pretty bool) *JSONFormatter {
	return &JSONFormatter{Writer: os.Stdout, Pretty: pretty}
}

// Print outputs data as JSON.
func (f *JSONFormatter) Print(data interface{}) error {
	encoder := json.NewEncoder(f.Writer)
	if f.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(data)
}

// PrintTable outputs tabular data as JSON array of objects.
func (f *JSONFormatter) PrintTable(headers []string, rows [][]string) {
	// Convert to array of maps
	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		obj := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				obj[strings.ToLower(strings.ReplaceAll(header, " ", "_"))] = row[i]
			}
		}
		result = append(result, obj)
	}
	f.Print(result)
}

// PrintKeyValue outputs key-value pairs as a JSON object.
func (f *JSONFormatter) PrintKeyValue(pairs []KVPair) {
	obj := make(map[string]string)
	for _, pair := range pairs {
		key := strings.ToLower(strings.ReplaceAll(pair.Key, " ", "_"))
		obj[key] = pair.Value
	}
	f.Print(obj)
}

// PrintMessage outputs a message as JSON.
func (f *JSONFormatter) PrintMessage(msg string) {
	f.Print(map[string]string{"message": msg})
}

// PrintError outputs an error as JSON.
func (f *JSONFormatter) PrintError(err error) {
	f.Print(map[string]string{"error": err.Error()})
}

// GetFormatter returns the appropriate formatter based on the format flag.
func GetFormatter(jsonOutput bool) Formatter {
	if jsonOutput {
		return NewJSONFormatter(true)
	}
	return NewTableFormatter()
}
