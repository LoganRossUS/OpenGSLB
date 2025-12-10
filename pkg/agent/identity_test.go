// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewIdentity_GeneratesCredentials(t *testing.T) {
	// Create temp directory for credentials
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	id, err := NewIdentity(IdentityConfig{
		ServiceToken: "test-token-12345678",
		Region:       "us-east",
		CertPath:     certPath,
		KeyPath:      keyPath,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	// Verify identity fields
	if id.ServiceToken != "test-token-12345678" {
		t.Errorf("expected service token 'test-token-12345678', got %q", id.ServiceToken)
	}
	if id.Region != "us-east" {
		t.Errorf("expected region 'us-east', got %q", id.Region)
	}
	if id.Fingerprint == "" {
		t.Error("fingerprint should not be empty")
	}
	if id.AgentID == "" {
		t.Error("agent ID should not be empty")
	}
	if len(id.Certificate) == 0 {
		t.Error("certificate should not be empty")
	}
	if len(id.PrivateKey) == 0 {
		t.Error("private key should not be empty")
	}

	// Verify files were created
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("certificate file was not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("private key file was not created")
	}
}

func TestNewIdentity_LoadsExistingCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create first identity
	id1, err := NewIdentity(IdentityConfig{
		ServiceToken: "test-token-12345678",
		Region:       "us-east",
		CertPath:     certPath,
		KeyPath:      keyPath,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("first NewIdentity failed: %v", err)
	}

	// Create second identity with same paths - should load existing
	id2, err := NewIdentity(IdentityConfig{
		ServiceToken: "test-token-12345678",
		Region:       "us-east",
		CertPath:     certPath,
		KeyPath:      keyPath,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("second NewIdentity failed: %v", err)
	}

	// Fingerprints should match (same certificate loaded)
	if id1.Fingerprint != id2.Fingerprint {
		t.Errorf("fingerprints should match: %s != %s", id1.Fingerprint, id2.Fingerprint)
	}
	if id1.AgentID != id2.AgentID {
		t.Errorf("agent IDs should match: %s != %s", id1.AgentID, id2.AgentID)
	}
}

func TestIdentity_AgentID_IncludesRegion(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	id, err := NewIdentity(IdentityConfig{
		ServiceToken: "test-token-12345678",
		Region:       "eu-west",
		CertPath:     certPath,
		KeyPath:      keyPath,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	// Agent ID should contain region
	if len(id.AgentID) < 7 { // "eu-west" + "-" + at least some fingerprint
		t.Errorf("agent ID too short: %s", id.AgentID)
	}
	if id.AgentID[:7] != "eu-west" {
		t.Errorf("agent ID should start with region, got %s", id.AgentID)
	}
}

func TestIdentity_NeedsRotation(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	id, err := NewIdentity(IdentityConfig{
		ServiceToken: "test-token-12345678",
		Region:       "us-east",
		CertPath:     certPath,
		KeyPath:      keyPath,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	// Certificate is valid for 1 year, so:
	// - Should not need rotation with 30-day threshold
	if id.NeedsRotation(30 * 24 * time.Hour) {
		t.Error("fresh certificate should not need rotation with 30-day threshold")
	}

	// - Should need rotation with 400-day threshold (longer than validity)
	if !id.NeedsRotation(400 * 24 * time.Hour) {
		t.Error("certificate should need rotation with 400-day threshold")
	}
}

func TestIdentity_RotateCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	id, err := NewIdentity(IdentityConfig{
		ServiceToken: "test-token-12345678",
		Region:       "us-east",
		CertPath:     certPath,
		KeyPath:      keyPath,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	oldFingerprint := id.Fingerprint
	oldAgentID := id.AgentID

	// Rotate certificate
	if err := id.RotateCertificate(certPath, keyPath); err != nil {
		t.Fatalf("RotateCertificate failed: %v", err)
	}

	// Fingerprint should change (new certificate)
	if id.Fingerprint == oldFingerprint {
		t.Error("fingerprint should change after rotation")
	}

	// Agent ID should also change
	if id.AgentID == oldAgentID {
		t.Error("agent ID should change after rotation")
	}

	// Service token and region should remain the same
	if id.ServiceToken != "test-token-12345678" {
		t.Errorf("service token changed: %s", id.ServiceToken)
	}
	if id.Region != "us-east" {
		t.Errorf("region changed: %s", id.Region)
	}
}

func TestDefaultIdentityPaths(t *testing.T) {
	certPath, keyPath := DefaultIdentityPaths()
	if certPath != "/var/lib/opengslb/agent.crt" {
		t.Errorf("unexpected default cert path: %s", certPath)
	}
	if keyPath != "/var/lib/opengslb/agent.key" {
		t.Errorf("unexpected default key path: %s", keyPath)
	}
}

func TestIdentity_GetMethods(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "agent.crt")
	keyPath := filepath.Join(tmpDir, "agent.key")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	id, err := NewIdentity(IdentityConfig{
		ServiceToken: "test-token-12345678",
		Region:       "ap-south",
		CertPath:     certPath,
		KeyPath:      keyPath,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("NewIdentity failed: %v", err)
	}

	// Test getter methods
	if len(id.GetCertificatePEM()) == 0 {
		t.Error("GetCertificatePEM returned empty")
	}
	if id.GetFingerprint() != id.Fingerprint {
		t.Error("GetFingerprint mismatch")
	}
	if id.GetAgentID() != id.AgentID {
		t.Error("GetAgentID mismatch")
	}
}
