// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	data map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		data: make(map[string][]byte),
	}
}

func (m *mockStore) Get(ctx context.Context, key string) ([]byte, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return nil, store.ErrKeyNotFound
}

func (m *mockStore) Set(ctx context.Context, key string, value []byte) error {
	m.data[key] = value
	return nil
}

func (m *mockStore) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockStore) List(ctx context.Context, prefix string) ([]store.KVPair, error) {
	var pairs []store.KVPair
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			pairs = append(pairs, store.KVPair{Key: k, Value: v})
		}
	}
	return pairs, nil
}

func (m *mockStore) Watch(ctx context.Context, prefix string) (<-chan store.WatchEvent, error) {
	ch := make(chan store.WatchEvent)
	close(ch)
	return ch, nil
}

func (m *mockStore) Close() error {
	return nil
}

// generateTestCertificate creates a test certificate and private key.
func generateTestCertificate(region string, validFor time.Duration) ([]byte, *ecdsa.PrivateKey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "opengslb-agent-" + region,
			Organization: []string{"OpenGSLB Agent"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(validFor),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM, privateKey, nil
}

func TestNewAgentAuth(t *testing.T) {
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"token1", "token2"},
	}

	auth := NewAgentAuth(cfg, newMockStore())
	if auth == nil {
		t.Fatal("NewAgentAuth returned nil")
	}

	if len(auth.tokenHashes) != 2 {
		t.Errorf("expected 2 token hashes, got %d", len(auth.tokenHashes))
	}
}

func TestAgentAuth_AuthenticateAgent_NewAgent(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token-123"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	// Generate a test certificate
	certPEM, _, err := generateTestCertificate("us-west-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test certificate: %v", err)
	}

	// Authenticate with valid token (TOFU)
	err = auth.AuthenticateAgent(ctx, "agent-001", certPEM, "valid-token-123")
	if err != nil {
		t.Errorf("AuthenticateAgent should succeed for new agent with valid token: %v", err)
	}

	// Verify certificate was pinned
	pinned, err := auth.GetPinnedCertificate(ctx, "agent-001")
	if err != nil {
		t.Errorf("GetPinnedCertificate failed: %v", err)
	}
	if pinned == nil {
		t.Fatal("pinned certificate should not be nil")
	}
	if pinned.AgentID != "agent-001" {
		t.Errorf("expected agent ID 'agent-001', got %q", pinned.AgentID)
	}
	if pinned.Region != "us-west-1" {
		t.Errorf("expected region 'us-west-1', got %q", pinned.Region)
	}
}

func TestAgentAuth_AuthenticateAgent_InvalidToken(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token-123"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	certPEM, _, err := generateTestCertificate("us-east-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test certificate: %v", err)
	}

	// Authenticate with invalid token
	err = auth.AuthenticateAgent(ctx, "agent-002", certPEM, "wrong-token")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAgentAuth_AuthenticateAgent_KnownAgent(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	certPEM, _, err := generateTestCertificate("eu-west-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test certificate: %v", err)
	}

	// First authentication (TOFU)
	err = auth.AuthenticateAgent(ctx, "agent-003", certPEM, "valid-token")
	if err != nil {
		t.Fatalf("first AuthenticateAgent failed: %v", err)
	}

	// Second authentication (should succeed without token)
	err = auth.AuthenticateAgent(ctx, "agent-003", certPEM, "")
	if err != nil {
		t.Errorf("second AuthenticateAgent should succeed: %v", err)
	}
}

func TestAgentAuth_AuthenticateAgent_FingerprintMismatch(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	// Generate first certificate
	certPEM1, _, err := generateTestCertificate("ap-south-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate first test certificate: %v", err)
	}

	// First authentication (TOFU)
	err = auth.AuthenticateAgent(ctx, "agent-004", certPEM1, "valid-token")
	if err != nil {
		t.Fatalf("first AuthenticateAgent failed: %v", err)
	}

	// Generate different certificate
	certPEM2, _, err := generateTestCertificate("ap-south-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate second test certificate: %v", err)
	}

	// Try to authenticate with different certificate
	err = auth.AuthenticateAgent(ctx, "agent-004", certPEM2, "")
	if err != ErrFingerprintMismatch {
		t.Errorf("expected ErrFingerprintMismatch, got %v", err)
	}
}

func TestAgentAuth_RevokeCertificate(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	certPEM, _, err := generateTestCertificate("us-west-2", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test certificate: %v", err)
	}

	// Register agent
	err = auth.AuthenticateAgent(ctx, "agent-005", certPEM, "valid-token")
	if err != nil {
		t.Fatalf("AuthenticateAgent failed: %v", err)
	}

	// Revoke certificate
	err = auth.RevokeCertificate(ctx, "agent-005", "security concern")
	if err != nil {
		t.Errorf("RevokeCertificate failed: %v", err)
	}

	// Verify revoked
	pinned, err := auth.GetPinnedCertificate(ctx, "agent-005")
	if err != nil {
		t.Fatalf("GetPinnedCertificate failed: %v", err)
	}
	if !pinned.Revoked {
		t.Error("certificate should be revoked")
	}
	if pinned.RevokedReason != "security concern" {
		t.Errorf("expected reason 'security concern', got %q", pinned.RevokedReason)
	}

	// Try to authenticate with revoked certificate
	err = auth.AuthenticateAgent(ctx, "agent-005", certPEM, "")
	if err != ErrCertificateRevoked {
		t.Errorf("expected ErrCertificateRevoked, got %v", err)
	}
}

func TestAgentAuth_UpdateCertificate(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	// Generate initial certificate
	certPEM1, _, err := generateTestCertificate("ca-central-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate first test certificate: %v", err)
	}

	// Register agent
	err = auth.AuthenticateAgent(ctx, "agent-006", certPEM1, "valid-token")
	if err != nil {
		t.Fatalf("AuthenticateAgent failed: %v", err)
	}

	// Get old fingerprint
	pinned, err := auth.GetPinnedCertificate(ctx, "agent-006")
	if err != nil {
		t.Fatalf("GetPinnedCertificate failed: %v", err)
	}
	oldFingerprint := pinned.Fingerprint

	// Generate new certificate
	certPEM2, _, err := generateTestCertificate("ca-central-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate second test certificate: %v", err)
	}

	// Update certificate (rotation)
	err = auth.UpdateCertificate(ctx, "agent-006", certPEM2, oldFingerprint)
	if err != nil {
		t.Errorf("UpdateCertificate failed: %v", err)
	}

	// Verify fingerprint changed
	pinned, err = auth.GetPinnedCertificate(ctx, "agent-006")
	if err != nil {
		t.Fatalf("GetPinnedCertificate failed: %v", err)
	}
	if pinned.Fingerprint == oldFingerprint {
		t.Error("fingerprint should have changed after update")
	}

	// Try to update with wrong old fingerprint
	certPEM3, _, _ := generateTestCertificate("ca-central-1", 365*24*time.Hour)
	err = auth.UpdateCertificate(ctx, "agent-006", certPEM3, oldFingerprint)
	if err != ErrFingerprintMismatch {
		t.Errorf("expected ErrFingerprintMismatch for wrong old fingerprint, got %v", err)
	}
}

func TestAgentAuth_DeletePinnedCertificate(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	certPEM, _, err := generateTestCertificate("eu-central-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test certificate: %v", err)
	}

	// Register agent
	err = auth.AuthenticateAgent(ctx, "agent-007", certPEM, "valid-token")
	if err != nil {
		t.Fatalf("AuthenticateAgent failed: %v", err)
	}

	// Delete pinned certificate
	err = auth.DeletePinnedCertificate(ctx, "agent-007")
	if err != nil {
		t.Errorf("DeletePinnedCertificate failed: %v", err)
	}

	// Verify deleted
	_, err = auth.GetPinnedCertificate(ctx, "agent-007")
	if err != ErrAgentNotFound {
		t.Errorf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestAgentAuth_ListPinnedCertificates(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	// Register multiple agents
	for i, region := range []string{"us-west-1", "us-east-1", "eu-west-1"} {
		certPEM, _, err := generateTestCertificate(region, 365*24*time.Hour)
		if err != nil {
			t.Fatalf("failed to generate test certificate %d: %v", i, err)
		}
		agentID := "agent-" + region
		if err := auth.AuthenticateAgent(ctx, agentID, certPEM, "valid-token"); err != nil {
			t.Fatalf("AuthenticateAgent failed for %s: %v", agentID, err)
		}
	}

	// List all
	list, err := auth.ListPinnedCertificates(ctx)
	if err != nil {
		t.Fatalf("ListPinnedCertificates failed: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 pinned certificates, got %d", len(list))
	}
}

func TestAgentAuth_GetExpiringCertificates(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	// Register agent with short-lived certificate
	shortCertPEM, _, err := generateTestCertificate("short-lived", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate short-lived certificate: %v", err)
	}
	if err := auth.AuthenticateAgent(ctx, "agent-short", shortCertPEM, "valid-token"); err != nil {
		t.Fatalf("AuthenticateAgent failed: %v", err)
	}

	// Register agent with long-lived certificate
	longCertPEM, _, err := generateTestCertificate("long-lived", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate long-lived certificate: %v", err)
	}
	if err := auth.AuthenticateAgent(ctx, "agent-long", longCertPEM, "valid-token"); err != nil {
		t.Fatalf("AuthenticateAgent failed: %v", err)
	}

	// Get expiring within 30 days
	expiring, err := auth.GetExpiringCertificates(ctx, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("GetExpiringCertificates failed: %v", err)
	}
	if len(expiring) != 1 {
		t.Errorf("expected 1 expiring certificate, got %d", len(expiring))
	}
	if len(expiring) > 0 && expiring[0].AgentID != "agent-short" {
		t.Errorf("expected agent-short to be expiring, got %s", expiring[0].AgentID)
	}
}

func TestAgentAuth_ExpiredCertificate(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{"valid-token"},
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	// Generate an expired certificate (this is tricky - we can't easily create one)
	// Instead, test with a certificate that's not yet valid
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	future := time.Now().Add(24 * time.Hour)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "opengslb-agent-future",
		},
		NotBefore: future,
		NotAfter:  future.Add(365 * 24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Try to authenticate with not-yet-valid certificate
	err = auth.AuthenticateAgent(ctx, "agent-future", certPEM, "valid-token")
	if err != ErrCertificateExpired {
		t.Errorf("expected ErrCertificateExpired for not-yet-valid certificate, got %v", err)
	}
}

func TestAgentAuth_NoTokensConfigured(t *testing.T) {
	st := newMockStore()
	cfg := &AgentAuthConfig{
		ServiceTokens: []string{}, // No tokens
	}
	auth := NewAgentAuth(cfg, st)
	ctx := context.Background()

	certPEM, _, err := generateTestCertificate("us-west-1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test certificate: %v", err)
	}

	// Should fail without any tokens configured
	err = auth.AuthenticateAgent(ctx, "agent-no-tokens", certPEM, "any-token")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken when no tokens configured, got %v", err)
	}
}

func TestExtractRegionFromCert(t *testing.T) {
	testCases := []struct {
		cn       string
		expected string
	}{
		{"opengslb-agent-us-west-1", "us-west-1"},
		{"opengslb-agent-eu-central-1", "eu-central-1"},
		{"opengslb-agent-", ""},
		{"other-agent-us-west-1", ""},
		{"", ""},
	}

	for _, tc := range testCases {
		privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

		template := x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName: tc.cn,
			},
			NotBefore: time.Now(),
			NotAfter:  time.Now().Add(time.Hour),
		}

		certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
		cert, _ := x509.ParseCertificate(certDER)

		result := extractRegionFromCert(cert)
		if result != tc.expected {
			t.Errorf("extractRegionFromCert(%q) = %q, expected %q", tc.cn, result, tc.expected)
		}
	}
}
