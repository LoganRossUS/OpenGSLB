// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MaxIncludeDepth is the maximum nesting level for includes to prevent infinite recursion.
const MaxIncludeDepth = 10

// IncludeError represents an error during include processing with file context.
type IncludeError struct {
	File    string
	Message string
	Cause   error
}

func (e *IncludeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.File, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

func (e *IncludeError) Unwrap() error {
	return e.Cause
}

// CircularIncludeError represents a circular include detection.
type CircularIncludeError struct {
	Path  []string
	Cycle string
}

func (e *CircularIncludeError) Error() string {
	return fmt.Sprintf("circular include detected: %s -> %s", strings.Join(e.Path, " -> "), e.Cycle)
}

// includeContext tracks the state of include processing.
type includeContext struct {
	baseDir     string          // Base directory for resolving relative paths
	depth       int             // Current nesting depth
	visited     map[string]bool // Absolute paths of visited files
	visitPath   []string        // Current visit path for cycle detection
	loadedFiles []string        // All files that were loaded (for hot-reload)
}

// newIncludeContext creates a new include context.
func newIncludeContext(baseDir string) *includeContext {
	return &includeContext{
		baseDir:     baseDir,
		depth:       0,
		visited:     make(map[string]bool),
		visitPath:   []string{},
		loadedFiles: []string{},
	}
}

// LoadWithIncludes reads and parses a configuration file, processing any includes.
// Returns the merged configuration and a list of all loaded files (for hot-reload tracking).
func LoadWithIncludes(path string) (*Config, []string, error) {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, nil, &IncludeError{File: path, Message: "failed to resolve path", Cause: err}
	}

	ctx := newIncludeContext(filepath.Dir(absPath))
	cfg, err := ctx.loadWithIncludes(absPath)
	if err != nil {
		return nil, nil, err
	}

	return cfg, ctx.loadedFiles, nil
}

// loadWithIncludes recursively loads a configuration file and its includes.
func (ctx *includeContext) loadWithIncludes(absPath string) (*Config, error) {
	// Check depth limit
	if ctx.depth > MaxIncludeDepth {
		return nil, &IncludeError{
			File:    absPath,
			Message: fmt.Sprintf("maximum include depth (%d) exceeded", MaxIncludeDepth),
		}
	}

	// Check for circular includes
	if ctx.visited[absPath] {
		return nil, &CircularIncludeError{
			Path:  append([]string{}, ctx.visitPath...),
			Cycle: absPath,
		}
	}

	// Mark as visiting
	ctx.visited[absPath] = true
	ctx.visitPath = append(ctx.visitPath, absPath)
	ctx.loadedFiles = append(ctx.loadedFiles, absPath)
	defer func() {
		ctx.visitPath = ctx.visitPath[:len(ctx.visitPath)-1]
	}()

	// Read and parse the file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, &IncludeError{File: absPath, Message: "failed to read file", Cause: err}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, &IncludeError{File: absPath, Message: "failed to parse YAML", Cause: err}
	}

	// Process includes if present
	if len(cfg.Includes) > 0 {
		ctx.depth++
		defer func() { ctx.depth-- }()

		baseDir := filepath.Dir(absPath)
		for _, pattern := range cfg.Includes {
			if err := ctx.processIncludePattern(baseDir, pattern, &cfg); err != nil {
				return nil, err
			}
		}
	}

	return &cfg, nil
}

// processIncludePattern expands a glob pattern and merges matching files.
func (ctx *includeContext) processIncludePattern(baseDir, pattern string, cfg *Config) error {
	// Resolve pattern relative to base directory
	fullPattern := pattern
	if !filepath.IsAbs(pattern) {
		fullPattern = filepath.Join(baseDir, pattern)
	}

	// Expand glob pattern
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return &IncludeError{
			File:    baseDir,
			Message: fmt.Sprintf("invalid glob pattern %q", pattern),
			Cause:   err,
		}
	}

	// Handle ** patterns (recursive glob) manually since filepath.Glob doesn't support it
	if strings.Contains(pattern, "**") {
		matches, err = expandDoubleStarGlob(baseDir, pattern)
		if err != nil {
			return &IncludeError{
				File:    baseDir,
				Message: fmt.Sprintf("failed to expand pattern %q", pattern),
				Cause:   err,
			}
		}
	}

	// Sort matches for deterministic loading order
	sort.Strings(matches)

	// Process each matching file
	for _, match := range matches {
		absMatch, err := filepath.Abs(match)
		if err != nil {
			return &IncludeError{File: match, Message: "failed to resolve path", Cause: err}
		}

		// Skip directories
		info, err := os.Stat(absMatch)
		if err != nil {
			return &IncludeError{File: absMatch, Message: "failed to stat file", Cause: err}
		}
		if info.IsDir() {
			continue
		}

		// Check file permissions (same as main config)
		if err := checkFilePermissions(absMatch); err != nil {
			return &IncludeError{File: absMatch, Message: "permission check failed", Cause: err}
		}

		// Load and merge the included file
		includedCfg, err := ctx.loadWithIncludes(absMatch)
		if err != nil {
			return err
		}

		// Merge into main config
		if err := mergeConfig(cfg, includedCfg, absMatch); err != nil {
			return err
		}
	}

	return nil
}

// expandDoubleStarGlob expands patterns containing ** for recursive directory matching.
func expandDoubleStarGlob(baseDir, pattern string) ([]string, error) {
	var matches []string

	// Split pattern into parts
	parts := strings.Split(pattern, string(filepath.Separator))

	// Find the ** index
	doubleStarIdx := -1
	for i, part := range parts {
		if part == "**" {
			doubleStarIdx = i
			break
		}
	}

	if doubleStarIdx == -1 {
		// No **, use standard glob
		fullPattern := pattern
		if !filepath.IsAbs(pattern) {
			fullPattern = filepath.Join(baseDir, pattern)
		}
		return filepath.Glob(fullPattern)
	}

	// Build prefix path (before **)
	prefixParts := parts[:doubleStarIdx]
	prefix := filepath.Join(baseDir, filepath.Join(prefixParts...))

	// Build suffix pattern (after **)
	suffixParts := parts[doubleStarIdx+1:]
	suffix := filepath.Join(suffixParts...)

	// Walk the directory tree
	err := filepath.Walk(prefix, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Try to match the suffix pattern in this directory
			testPattern := filepath.Join(path, suffix)
			dirMatches, err := filepath.Glob(testPattern)
			if err != nil {
				return err
			}
			matches = append(matches, dirMatches...)
		}
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return matches, nil
}

// checkFilePermissions verifies the included file has appropriate permissions.
// On Unix systems, warn if file is world-writable.
func checkFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// Check if file is world-writable (security concern)
	mode := info.Mode()
	if mode&0002 != 0 {
		return fmt.Errorf("file is world-writable, which is a security risk")
	}

	return nil
}

// mergeConfig merges an included configuration into the main configuration.
// Arrays are concatenated, maps are merged (later values override).
func mergeConfig(main, included *Config, sourceFile string) error {
	// Merge regions (concatenate arrays)
	if len(included.Regions) > 0 {
		// Check for duplicate region names
		existingRegions := make(map[string]bool)
		for _, r := range main.Regions {
			existingRegions[r.Name] = true
		}
		for _, r := range included.Regions {
			if existingRegions[r.Name] {
				return &IncludeError{
					File:    sourceFile,
					Message: fmt.Sprintf("duplicate region name %q", r.Name),
				}
			}
		}
		main.Regions = append(main.Regions, included.Regions...)
	}

	// Merge domains (concatenate arrays)
	if len(included.Domains) > 0 {
		// Check for duplicate domain names
		existingDomains := make(map[string]bool)
		for _, d := range main.Domains {
			existingDomains[d.Name] = true
		}
		for _, d := range included.Domains {
			if existingDomains[d.Name] {
				return &IncludeError{
					File:    sourceFile,
					Message: fmt.Sprintf("duplicate domain name %q", d.Name),
				}
			}
		}
		main.Domains = append(main.Domains, included.Domains...)
	}

	// Merge agent tokens (merge maps, later values override)
	if len(included.Overwatch.AgentTokens) > 0 {
		if main.Overwatch.AgentTokens == nil {
			main.Overwatch.AgentTokens = make(map[string]string)
		}
		for k, v := range included.Overwatch.AgentTokens {
			main.Overwatch.AgentTokens[k] = v
		}
	}

	// Merge agent backends (concatenate arrays)
	if len(included.Agent.Backends) > 0 {
		main.Agent.Backends = append(main.Agent.Backends, included.Agent.Backends...)
	}

	// Merge custom geo mappings (concatenate arrays)
	if len(included.Overwatch.Geolocation.CustomMappings) > 0 {
		main.Overwatch.Geolocation.CustomMappings = append(
			main.Overwatch.Geolocation.CustomMappings,
			included.Overwatch.Geolocation.CustomMappings...,
		)
	}

	// Note: Scalar fields (dns, logging, metrics, api, etc.) are NOT merged from includes.
	// These should only be set in the main configuration file.

	return nil
}

// IncludedFiles is a helper type for tracking loaded files for hot-reload.
type IncludedFiles struct {
	MainFile string
	Includes []string
}

// All returns all files including the main file.
func (f *IncludedFiles) All() []string {
	all := make([]string, 0, len(f.Includes)+1)
	all = append(all, f.MainFile)
	all = append(all, f.Includes...)
	return all
}
