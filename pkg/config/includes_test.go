// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWithIncludes_BasicMerge(t *testing.T) {
	// Create temp directory structure
	tempDir := t.TempDir()

	// Create regions subdirectory
	regionsDir := filepath.Join(tempDir, "regions")
	if err := os.MkdirAll(regionsDir, 0755); err != nil {
		t.Fatalf("failed to create regions dir: %v", err)
	}

	// Create main config
	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
includes:
  - regions/*.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	// Create region files
	usEastConfig := `
regions:
  - name: us-east-1
    countries: ["US", "CA"]
    servers:
      - address: "10.0.1.10"
        port: 8080
`
	if err := os.WriteFile(filepath.Join(regionsDir, "us-east.yaml"), []byte(usEastConfig), 0640); err != nil {
		t.Fatalf("failed to write us-east config: %v", err)
	}

	euWestConfig := `
regions:
  - name: eu-west-1
    countries: ["GB", "DE", "FR"]
    servers:
      - address: "10.0.2.10"
        port: 8080
`
	if err := os.WriteFile(filepath.Join(regionsDir, "eu-west.yaml"), []byte(euWestConfig), 0640); err != nil {
		t.Fatalf("failed to write eu-west config: %v", err)
	}

	// Load with includes
	cfg, loadedFiles, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Verify regions were merged
	if len(cfg.Regions) != 2 {
		t.Errorf("expected 2 regions, got %d", len(cfg.Regions))
	}

	// Verify both regions are present
	regionNames := make(map[string]bool)
	for _, r := range cfg.Regions {
		regionNames[r.Name] = true
	}
	if !regionNames["us-east-1"] {
		t.Error("us-east-1 region not found")
	}
	if !regionNames["eu-west-1"] {
		t.Error("eu-west-1 region not found")
	}

	// Verify all files were tracked
	if len(loadedFiles) != 3 {
		t.Errorf("expected 3 loaded files, got %d: %v", len(loadedFiles), loadedFiles)
	}
}

func TestLoadWithIncludes_DomainsMerge(t *testing.T) {
	tempDir := t.TempDir()
	domainsDir := filepath.Join(tempDir, "domains")
	if err := os.MkdirAll(domainsDir, 0755); err != nil {
		t.Fatalf("failed to create domains dir: %v", err)
	}

	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
includes:
  - domains/*.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	appDomain := `
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
    ttl: 30
`
	if err := os.WriteFile(filepath.Join(domainsDir, "app.yaml"), []byte(appDomain), 0640); err != nil {
		t.Fatalf("failed to write app domain config: %v", err)
	}

	apiDomain := `
domains:
  - name: api.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
    ttl: 15
`
	if err := os.WriteFile(filepath.Join(domainsDir, "api.yaml"), []byte(apiDomain), 0640); err != nil {
		t.Fatalf("failed to write api domain config: %v", err)
	}

	cfg, _, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	if len(cfg.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(cfg.Domains))
	}
}

func TestLoadWithIncludes_AgentTokensMerge(t *testing.T) {
	tempDir := t.TempDir()

	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
  agent_tokens:
    service1: "token1"
includes:
  - tokens.yaml
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	tokensConfig := `
overwatch:
  agent_tokens:
    service2: "token2"
    service3: "token3"
`
	if err := os.WriteFile(filepath.Join(tempDir, "tokens.yaml"), []byte(tokensConfig), 0640); err != nil {
		t.Fatalf("failed to write tokens config: %v", err)
	}

	cfg, _, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	if len(cfg.Overwatch.AgentTokens) != 3 {
		t.Errorf("expected 3 agent tokens, got %d: %v", len(cfg.Overwatch.AgentTokens), cfg.Overwatch.AgentTokens)
	}
	if cfg.Overwatch.AgentTokens["service1"] != "token1" {
		t.Error("service1 token not preserved")
	}
	if cfg.Overwatch.AgentTokens["service2"] != "token2" {
		t.Error("service2 token not merged")
	}
}

func TestLoadWithIncludes_CircularIncludeDetection(t *testing.T) {
	tempDir := t.TempDir()

	// Create circular includes: a.yaml -> b.yaml -> a.yaml
	aConfig := `
mode: overwatch
includes:
  - b.yaml
`
	aPath := filepath.Join(tempDir, "a.yaml")
	if err := os.WriteFile(aPath, []byte(aConfig), 0640); err != nil {
		t.Fatalf("failed to write a.yaml: %v", err)
	}

	bConfig := `
includes:
  - a.yaml
`
	if err := os.WriteFile(filepath.Join(tempDir, "b.yaml"), []byte(bConfig), 0640); err != nil {
		t.Fatalf("failed to write b.yaml: %v", err)
	}

	_, _, err := LoadWithIncludes(aPath)
	if err == nil {
		t.Fatal("expected circular include error, got nil")
	}

	var circularErr *CircularIncludeError
	if _, ok := err.(*CircularIncludeError); !ok {
		// Check if it's wrapped
		if !strings.Contains(err.Error(), "circular include") {
			t.Errorf("expected CircularIncludeError, got %T: %v", err, err)
		}
	} else {
		_ = circularErr // unused
	}
}

func TestLoadWithIncludes_MaxDepthExceeded(t *testing.T) {
	tempDir := t.TempDir()

	// Create deeply nested includes (depth > MaxIncludeDepth)
	prevFile := ""
	for i := 0; i <= MaxIncludeDepth+2; i++ {
		filename := filepath.Join(tempDir, "level"+string(rune('a'+i))+".yaml")
		var content string
		if prevFile != "" {
			content = "includes:\n  - " + filepath.Base(prevFile)
		} else {
			content = "mode: overwatch"
		}
		if err := os.WriteFile(filename, []byte(content), 0640); err != nil {
			t.Fatalf("failed to write %s: %v", filename, err)
		}
		prevFile = filename
	}

	// Load the deepest file
	_, _, err := LoadWithIncludes(prevFile)
	if err == nil {
		t.Fatal("expected max depth error, got nil")
	}

	if !strings.Contains(err.Error(), "maximum include depth") {
		t.Errorf("expected max depth error, got: %v", err)
	}
}

func TestLoadWithIncludes_DuplicateRegionName(t *testing.T) {
	tempDir := t.TempDir()
	regionsDir := filepath.Join(tempDir, "regions")
	if err := os.MkdirAll(regionsDir, 0755); err != nil {
		t.Fatalf("failed to create regions dir: %v", err)
	}

	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
includes:
  - regions/*.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	// Two files with same region name
	region1 := `
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
`
	if err := os.WriteFile(filepath.Join(regionsDir, "a.yaml"), []byte(region1), 0640); err != nil {
		t.Fatalf("failed to write a.yaml: %v", err)
	}

	region2 := `
regions:
  - name: us-east-1
    servers:
      - address: "10.0.2.10"
        port: 8080
`
	if err := os.WriteFile(filepath.Join(regionsDir, "b.yaml"), []byte(region2), 0640); err != nil {
		t.Fatalf("failed to write b.yaml: %v", err)
	}

	_, _, err := LoadWithIncludes(mainPath)
	if err == nil {
		t.Fatal("expected duplicate region error, got nil")
	}

	if !strings.Contains(err.Error(), "duplicate region name") {
		t.Errorf("expected duplicate region error, got: %v", err)
	}
}

func TestLoadWithIncludes_DuplicateDomainName(t *testing.T) {
	tempDir := t.TempDir()
	domainsDir := filepath.Join(tempDir, "domains")
	if err := os.MkdirAll(domainsDir, 0755); err != nil {
		t.Fatalf("failed to create domains dir: %v", err)
	}

	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
includes:
  - domains/*.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	// Two files with same domain name
	domain1 := `
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
`
	if err := os.WriteFile(filepath.Join(domainsDir, "a.yaml"), []byte(domain1), 0640); err != nil {
		t.Fatalf("failed to write a.yaml: %v", err)
	}

	domain2 := `
domains:
  - name: app.example.com
    routing_algorithm: failover
    regions: [us-east-1]
`
	if err := os.WriteFile(filepath.Join(domainsDir, "b.yaml"), []byte(domain2), 0640); err != nil {
		t.Fatalf("failed to write b.yaml: %v", err)
	}

	_, _, err := LoadWithIncludes(mainPath)
	if err == nil {
		t.Fatal("expected duplicate domain error, got nil")
	}

	if !strings.Contains(err.Error(), "duplicate domain name") {
		t.Errorf("expected duplicate domain error, got: %v", err)
	}
}

func TestLoadWithIncludes_RecursiveGlob(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested directory structure
	teamADir := filepath.Join(tempDir, "domains", "team-a")
	teamBDir := filepath.Join(tempDir, "domains", "team-b")
	if err := os.MkdirAll(teamADir, 0755); err != nil {
		t.Fatalf("failed to create team-a dir: %v", err)
	}
	if err := os.MkdirAll(teamBDir, 0755); err != nil {
		t.Fatalf("failed to create team-b dir: %v", err)
	}

	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
includes:
  - domains/**/*.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	// Team A domain
	teamADomain := `
domains:
  - name: team-a-app.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
`
	if err := os.WriteFile(filepath.Join(teamADir, "app.yaml"), []byte(teamADomain), 0640); err != nil {
		t.Fatalf("failed to write team-a app.yaml: %v", err)
	}

	// Team B domain
	teamBDomain := `
domains:
  - name: team-b-app.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
`
	if err := os.WriteFile(filepath.Join(teamBDir, "app.yaml"), []byte(teamBDomain), 0640); err != nil {
		t.Fatalf("failed to write team-b app.yaml: %v", err)
	}

	cfg, _, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	if len(cfg.Domains) != 2 {
		t.Errorf("expected 2 domains from recursive glob, got %d", len(cfg.Domains))
	}
}

func TestLoadWithIncludes_NoMatchingFiles(t *testing.T) {
	tempDir := t.TempDir()

	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
includes:
  - nonexistent/*.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	// Should succeed - no matching files is not an error
	cfg, _, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Should have the default region and domain from main config
	if len(cfg.Regions) != 1 {
		t.Errorf("expected 1 region, got %d", len(cfg.Regions))
	}
}

func TestLoadWithIncludes_WorldWritableFile(t *testing.T) {
	tempDir := t.TempDir()
	includedDir := filepath.Join(tempDir, "includes")
	if err := os.MkdirAll(includedDir, 0755); err != nil {
		t.Fatalf("failed to create includes dir: %v", err)
	}

	mainConfig := `
mode: overwatch
includes:
  - includes/*.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	// Create world-writable include file (security issue)
	worldWritable := `
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
`
	includePath := filepath.Join(includedDir, "insecure.yaml")
	if err := os.WriteFile(includePath, []byte(worldWritable), 0666); err != nil {
		t.Fatalf("failed to write insecure config: %v", err)
	}
	// Make it world-writable
	if err := os.Chmod(includePath, 0666); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}

	_, _, err := LoadWithIncludes(mainPath)
	if err == nil {
		t.Fatal("expected permission error for world-writable file, got nil")
	}

	if !strings.Contains(err.Error(), "world-writable") {
		t.Errorf("expected world-writable error, got: %v", err)
	}
}

func TestLoadWithIncludes_NestedIncludes(t *testing.T) {
	tempDir := t.TempDir()

	// Main config includes base.yaml, which includes regions.yaml
	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
includes:
  - base.yaml
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	baseConfig := `
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
includes:
  - regions.yaml
`
	if err := os.WriteFile(filepath.Join(tempDir, "base.yaml"), []byte(baseConfig), 0640); err != nil {
		t.Fatalf("failed to write base.yaml: %v", err)
	}

	regionsConfig := `
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
`
	if err := os.WriteFile(filepath.Join(tempDir, "regions.yaml"), []byte(regionsConfig), 0640); err != nil {
		t.Fatalf("failed to write regions.yaml: %v", err)
	}

	cfg, loadedFiles, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Verify nested include worked
	if len(cfg.Regions) != 1 {
		t.Errorf("expected 1 region from nested include, got %d", len(cfg.Regions))
	}

	// Verify all files tracked
	if len(loadedFiles) != 3 {
		t.Errorf("expected 3 loaded files, got %d: %v", len(loadedFiles), loadedFiles)
	}
}

func TestLoadWithIncludes_CustomGeoMappingsMerge(t *testing.T) {
	tempDir := t.TempDir()

	mainConfig := `
mode: overwatch
dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
overwatch:
  gossip:
    encryption_key: "dGhpcyBpcyBhIDMyIGJ5dGUga2V5IGZvciB0ZXN0aW5nISE="
  dnssec:
    enabled: true
  geolocation:
    database_path: "/var/lib/opengslb/GeoLite2-Country.mmdb"
    default_region: us-east-1
    custom_mappings:
      - cidr: "10.1.0.0/16"
        region: us-east-1
        comment: "Office network"
includes:
  - geo-mappings.yaml
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
  - name: us-west-2
    servers:
      - address: "10.0.2.10"
        port: 8080
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions: [us-east-1]
`
	mainPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0640); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	geoMappingsConfig := `
overwatch:
  geolocation:
    custom_mappings:
      - cidr: "10.2.0.0/16"
        region: us-west-2
        comment: "Additional office"
`
	if err := os.WriteFile(filepath.Join(tempDir, "geo-mappings.yaml"), []byte(geoMappingsConfig), 0640); err != nil {
		t.Fatalf("failed to write geo-mappings.yaml: %v", err)
	}

	cfg, _, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	if len(cfg.Overwatch.Geolocation.CustomMappings) != 2 {
		t.Errorf("expected 2 custom geo mappings, got %d", len(cfg.Overwatch.Geolocation.CustomMappings))
	}
}

func TestIncludeError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *IncludeError
		expected string
	}{
		{
			name: "with cause",
			err: &IncludeError{
				File:    "/etc/config.yaml",
				Message: "failed to read",
				Cause:   os.ErrNotExist,
			},
			expected: "/etc/config.yaml: failed to read: file does not exist",
		},
		{
			name: "without cause",
			err: &IncludeError{
				File:    "/etc/config.yaml",
				Message: "duplicate region",
			},
			expected: "/etc/config.yaml: duplicate region",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestCircularIncludeError_Error(t *testing.T) {
	err := &CircularIncludeError{
		Path:  []string{"a.yaml", "b.yaml"},
		Cycle: "a.yaml",
	}
	expected := "circular include detected: a.yaml -> b.yaml -> a.yaml"
	if got := err.Error(); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestIncludedFiles_All(t *testing.T) {
	files := &IncludedFiles{
		MainFile: "/etc/config.yaml",
		Includes: []string{"/etc/regions.yaml", "/etc/domains.yaml"},
	}

	all := files.All()
	if len(all) != 3 {
		t.Errorf("expected 3 files, got %d", len(all))
	}
	if all[0] != "/etc/config.yaml" {
		t.Errorf("expected main file first, got %s", all[0])
	}
}
