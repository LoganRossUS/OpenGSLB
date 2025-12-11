// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// TOFU authentication errors.
var (
	// ErrAgentNotFound is returned when an agent is not found in the pinned certificates.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrInvalidToken is returned when the service token is invalid.
	ErrInvalidToken = errors.New("invalid service token")
	// ErrFingerprintMismatch is returned when the certificate fingerprint doesn't match.
	ErrFingerprintMismatch = errors.New("certificate fingerprint mismatch")
	// ErrCertificateExpired is returned when the agent's certificate has expired.
	ErrCertificateExpired = errors.New("certificate expired")
	// ErrCertificateRevoked is returned when the agent's certificate has been revoked.
	ErrCertificateRevoked = errors.New("certificate revoked")
	// ErrInvalidCertificate is returned when the certificate cannot be parsed.
	ErrInvalidCertificate = errors.New("invalid certificate")
	// ErrAuthenticationFailed is returned for generic authentication failures.
	ErrAuthenticationFailed = errors.New("authentication failed")
)

// PinnedCertificate represents a pinned agent certificate in the store.
type PinnedCertificate struct {
	// AgentID is the unique identifier for the agent.
	AgentID string `json:"agent_id"`
	// Fingerprint is the SHA-256 fingerprint of the certificate.
	Fingerprint string `json:"fingerprint"`
	// CertificatePEM is the PEM-encoded certificate.
	CertificatePEM []byte `json:"certificate_pem"`
	// Region is the agent's region.
	Region string `json:"region"`
	// FirstSeen is when this agent was first registered.
	FirstSeen time.Time `json:"first_seen"`
	// LastSeen is when this agent was last authenticated.
	LastSeen time.Time `json:"last_seen"`
	// NotAfter is when the certificate expires.
	NotAfter time.Time `json:"not_after"`
	// Revoked indicates if the certificate has been revoked.
	Revoked bool `json:"revoked"`
	// RevokedAt is when the certificate was revoked (if applicable).
	RevokedAt time.Time `json:"revoked_at,omitempty"`
	// RevokedReason is why the certificate was revoked.
	RevokedReason string `json:"revoked_reason,omitempty"`
}

// AgentAuthConfig configures the agent authentication system.
type AgentAuthConfig struct {
	// ServiceTokens is a list of valid pre-shared tokens for initial registration.
	// Tokens should be SHA-256 hashed for secure comparison.
	ServiceTokens []string

	// Logger for authentication operations.
	Logger *slog.Logger
}

// AgentAuth manages TOFU authentication for agents.
type AgentAuth struct {
	config      *AgentAuthConfig
	store       store.Store
	logger      *slog.Logger
	mu          sync.RWMutex
	tokenHashes [][]byte // Pre-computed SHA-256 hashes of valid tokens
}

// NewAgentAuth creates a new agent authentication manager.
func NewAgentAuth(cfg *AgentAuthConfig, st store.Store) *AgentAuth {
	if cfg == nil {
		cfg = &AgentAuthConfig{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	auth := &AgentAuth{
		config: cfg,
		store:  st,
		logger: logger,
	}

	// Pre-compute token hashes for constant-time comparison
	for _, token := range cfg.ServiceTokens {
		hash := sha256.Sum256([]byte(token))
		auth.tokenHashes = append(auth.tokenHashes, hash[:])
	}

	return auth
}

// AuthenticateAgent authenticates an agent using TOFU.
// On first connection with a valid service token, the certificate is pinned.
// On subsequent connections, the certificate fingerprint must match.
func (a *AgentAuth) AuthenticateAgent(ctx context.Context, agentID string, certPEM []byte, serviceToken string) error {
	// Parse and validate the certificate
	cert, err := parseCertificatePEM(certPEM)
	if err != nil {
		a.logger.Warn("failed to parse agent certificate",
			"agent_id", agentID,
			"error", err,
		)
		return ErrInvalidCertificate
	}

	// Check certificate validity period
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		a.logger.Warn("agent certificate not valid",
			"agent_id", agentID,
			"not_before", cert.NotBefore,
			"not_after", cert.NotAfter,
		)
		return ErrCertificateExpired
	}

	fingerprint := calculateFingerprint(cert)

	// Check if agent is already pinned
	pinned, err := a.GetPinnedCertificate(ctx, agentID)
	if err != nil && !errors.Is(err, ErrAgentNotFound) {
		return fmt.Errorf("failed to check pinned certificate: %w", err)
	}

	if pinned != nil {
		// Agent is known - verify certificate matches pinned one
		return a.verifyKnownAgent(ctx, pinned, fingerprint)
	}

	// New agent - TOFU registration with service token
	return a.registerNewAgent(ctx, agentID, certPEM, fingerprint, serviceToken, cert)
}

// verifyKnownAgent verifies a known agent's certificate matches the pinned one.
func (a *AgentAuth) verifyKnownAgent(ctx context.Context, pinned *PinnedCertificate, fingerprint string) error {
	// Check if certificate is revoked
	if pinned.Revoked {
		a.logger.Warn("agent certificate is revoked",
			"agent_id", pinned.AgentID,
			"revoked_at", pinned.RevokedAt,
			"reason", pinned.RevokedReason,
		)
		return ErrCertificateRevoked
	}

	// Verify fingerprint matches (constant-time comparison)
	if subtle.ConstantTimeCompare([]byte(fingerprint), []byte(pinned.Fingerprint)) != 1 {
		a.logger.Warn("certificate fingerprint mismatch",
			"agent_id", pinned.AgentID,
			"expected", pinned.Fingerprint[:16]+"...",
			"got", fingerprint[:16]+"...",
		)
		return ErrFingerprintMismatch
	}

	// Update last seen
	pinned.LastSeen = time.Now()
	if err := a.savePinnedCertificate(ctx, pinned); err != nil {
		a.logger.Warn("failed to update last seen", "agent_id", pinned.AgentID, "error", err)
	}

	a.logger.Debug("agent authenticated successfully",
		"agent_id", pinned.AgentID,
		"fingerprint", fingerprint[:16]+"...",
	)

	return nil
}

// registerNewAgent registers a new agent with TOFU.
func (a *AgentAuth) registerNewAgent(ctx context.Context, agentID string, certPEM []byte, fingerprint, serviceToken string, cert *x509.Certificate) error {
	// Validate service token
	if !a.validateServiceToken(serviceToken) {
		a.logger.Warn("invalid service token for new agent",
			"agent_id", agentID,
		)
		return ErrInvalidToken
	}

	// Extract region from certificate (CommonName format: opengslb-agent-{region})
	region := extractRegionFromCert(cert)

	// Create pinned certificate entry
	now := time.Now()
	pinned := &PinnedCertificate{
		AgentID:        agentID,
		Fingerprint:    fingerprint,
		CertificatePEM: certPEM,
		Region:         region,
		FirstSeen:      now,
		LastSeen:       now,
		NotAfter:       cert.NotAfter,
		Revoked:        false,
	}

	// Save to store
	if err := a.savePinnedCertificate(ctx, pinned); err != nil {
		return fmt.Errorf("failed to save pinned certificate: %w", err)
	}

	a.logger.Info("new agent registered via TOFU",
		"agent_id", agentID,
		"fingerprint", fingerprint[:16]+"...",
		"region", region,
		"expires", cert.NotAfter,
	)

	RecordAgentRegistration(agentID, region)
	return nil
}

// validateServiceToken checks if the provided token is valid.
// Uses constant-time comparison to prevent timing attacks.
func (a *AgentAuth) validateServiceToken(token string) bool {
	if len(a.tokenHashes) == 0 {
		// No tokens configured - reject all new registrations
		return false
	}

	tokenHash := sha256.Sum256([]byte(token))
	for _, validHash := range a.tokenHashes {
		if subtle.ConstantTimeCompare(tokenHash[:], validHash) == 1 {
			return true
		}
	}
	return false
}

// GetPinnedCertificate retrieves a pinned certificate by agent ID.
func (a *AgentAuth) GetPinnedCertificate(ctx context.Context, agentID string) (*PinnedCertificate, error) {
	if a.store == nil {
		return nil, ErrAgentNotFound
	}

	key := store.PrefixPinnedCerts + agentID
	data, err := a.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to get pinned certificate: %w", err)
	}

	var pinned PinnedCertificate
	if err := json.Unmarshal(data, &pinned); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pinned certificate: %w", err)
	}

	return &pinned, nil
}

// savePinnedCertificate saves a pinned certificate to the store.
func (a *AgentAuth) savePinnedCertificate(ctx context.Context, pinned *PinnedCertificate) error {
	if a.store == nil {
		return nil
	}

	data, err := json.Marshal(pinned)
	if err != nil {
		return fmt.Errorf("failed to marshal pinned certificate: %w", err)
	}

	key := store.PrefixPinnedCerts + pinned.AgentID
	return a.store.Set(ctx, key, data)
}

// RevokeCertificate revokes an agent's pinned certificate.
func (a *AgentAuth) RevokeCertificate(ctx context.Context, agentID, reason string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	pinned, err := a.GetPinnedCertificate(ctx, agentID)
	if err != nil {
		return err
	}

	pinned.Revoked = true
	pinned.RevokedAt = time.Now()
	pinned.RevokedReason = reason

	if err := a.savePinnedCertificate(ctx, pinned); err != nil {
		return fmt.Errorf("failed to save revoked certificate: %w", err)
	}

	a.logger.Info("agent certificate revoked",
		"agent_id", agentID,
		"reason", reason,
	)

	RecordAgentRevocation(agentID)
	return nil
}

// DeletePinnedCertificate removes an agent's pinned certificate entirely.
func (a *AgentAuth) DeletePinnedCertificate(ctx context.Context, agentID string) error {
	if a.store == nil {
		return nil
	}

	key := store.PrefixPinnedCerts + agentID
	if err := a.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete pinned certificate: %w", err)
	}

	a.logger.Info("agent pinned certificate deleted", "agent_id", agentID)
	return nil
}

// UpdateCertificate updates an agent's pinned certificate (for rotation).
// The old fingerprint must be provided and must match to prevent unauthorized updates.
func (a *AgentAuth) UpdateCertificate(ctx context.Context, agentID string, newCertPEM []byte, oldFingerprint string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get existing pinned certificate
	pinned, err := a.GetPinnedCertificate(ctx, agentID)
	if err != nil {
		return err
	}

	// Verify old fingerprint matches
	if subtle.ConstantTimeCompare([]byte(oldFingerprint), []byte(pinned.Fingerprint)) != 1 {
		a.logger.Warn("certificate update rejected - old fingerprint mismatch",
			"agent_id", agentID,
		)
		return ErrFingerprintMismatch
	}

	// Parse new certificate
	newCert, err := parseCertificatePEM(newCertPEM)
	if err != nil {
		return ErrInvalidCertificate
	}

	// Update the pinned certificate
	newFingerprint := calculateFingerprint(newCert)
	pinned.CertificatePEM = newCertPEM
	pinned.Fingerprint = newFingerprint
	pinned.NotAfter = newCert.NotAfter
	pinned.LastSeen = time.Now()

	if err := a.savePinnedCertificate(ctx, pinned); err != nil {
		return fmt.Errorf("failed to save updated certificate: %w", err)
	}

	a.logger.Info("agent certificate updated",
		"agent_id", agentID,
		"old_fingerprint", oldFingerprint[:16]+"...",
		"new_fingerprint", newFingerprint[:16]+"...",
		"new_expiry", newCert.NotAfter,
	)

	return nil
}

// ListPinnedCertificates returns all pinned certificates.
func (a *AgentAuth) ListPinnedCertificates(ctx context.Context) ([]*PinnedCertificate, error) {
	if a.store == nil {
		return nil, nil
	}

	pairs, err := a.store.List(ctx, store.PrefixPinnedCerts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pinned certificates: %w", err)
	}

	var result []*PinnedCertificate
	for _, pair := range pairs {
		var pinned PinnedCertificate
		if err := json.Unmarshal(pair.Value, &pinned); err != nil {
			a.logger.Warn("failed to unmarshal pinned certificate", "key", pair.Key, "error", err)
			continue
		}
		result = append(result, &pinned)
	}

	return result, nil
}

// GetExpiringCertificates returns certificates expiring within the given threshold.
func (a *AgentAuth) GetExpiringCertificates(ctx context.Context, threshold time.Duration) ([]*PinnedCertificate, error) {
	all, err := a.ListPinnedCertificates(ctx)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(threshold)
	var expiring []*PinnedCertificate
	for _, pinned := range all {
		if !pinned.Revoked && pinned.NotAfter.Before(deadline) {
			expiring = append(expiring, pinned)
		}
	}

	return expiring, nil
}

// parseCertificatePEM parses a PEM-encoded certificate.
func parseCertificatePEM(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, ErrInvalidCertificate
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// calculateFingerprint computes the SHA-256 fingerprint of a certificate.
func calculateFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// extractRegionFromCert extracts the region from certificate CommonName.
// Expected format: opengslb-agent-{region}
func extractRegionFromCert(cert *x509.Certificate) string {
	cn := cert.Subject.CommonName
	const prefix = "opengslb-agent-"
	if len(cn) > len(prefix) && cn[:len(prefix)] == prefix {
		return cn[len(prefix):]
	}
	return ""
}
