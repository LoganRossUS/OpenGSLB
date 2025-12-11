// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Identity manages agent authentication credentials.
// ADR-015: Uses TOFU (Trust-On-First-Use) with pre-shared service tokens.
type Identity struct {
	// ServiceToken is the pre-shared authentication token
	ServiceToken string

	// Region is the geographic region this agent belongs to
	Region string

	// Certificate is the agent's X.509 certificate (PEM encoded)
	Certificate []byte

	// PrivateKey is the agent's private key (PEM encoded)
	PrivateKey []byte

	// Fingerprint is the SHA-256 fingerprint of the certificate
	Fingerprint string

	// AgentID is a unique identifier derived from the certificate
	AgentID string

	logger *slog.Logger
}

// IdentityConfig configures agent identity management.
type IdentityConfig struct {
	// ServiceToken is the pre-shared token for Overwatch authentication
	ServiceToken string

	// Region is the geographic region this agent belongs to
	Region string

	// CertPath is where to store/load the certificate
	// Defaults to /var/lib/opengslb/agent.crt
	CertPath string

	// KeyPath is where to store/load the private key
	// Defaults to /var/lib/opengslb/agent.key
	KeyPath string

	// Logger for identity operations
	Logger *slog.Logger
}

// DefaultIdentityPaths returns the default certificate and key paths.
func DefaultIdentityPaths() (certPath, keyPath string) {
	return "/var/lib/opengslb/agent.crt", "/var/lib/opengslb/agent.key"
}

// NewIdentity creates or loads agent identity credentials.
// If certificates don't exist, they are generated automatically (TOFU).
func NewIdentity(cfg IdentityConfig) (*Identity, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	certPath := cfg.CertPath
	keyPath := cfg.KeyPath
	if certPath == "" || keyPath == "" {
		certPath, keyPath = DefaultIdentityPaths()
	}

	id := &Identity{
		ServiceToken: cfg.ServiceToken,
		Region:       cfg.Region,
		logger:       logger,
	}

	// Try to load existing credentials
	if err := id.loadCredentials(certPath, keyPath); err == nil {
		logger.Info("loaded existing agent identity",
			"fingerprint", id.Fingerprint,
			"agent_id", id.AgentID,
		)
		return id, nil
	}

	// Generate new credentials
	logger.Info("generating new agent identity")
	if err := id.generateCredentials(certPath, keyPath); err != nil {
		return nil, fmt.Errorf("failed to generate credentials: %w", err)
	}

	logger.Info("generated new agent identity",
		"fingerprint", id.Fingerprint,
		"agent_id", id.AgentID,
		"cert_path", certPath,
	)

	return id, nil
}

// loadCredentials attempts to load existing certificate and key files.
func (id *Identity) loadCredentials(certPath, keyPath string) error {
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %w", err)
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	// Parse certificate to extract fingerprint
	block, _ := pem.Decode(cert)
	if block == nil {
		return fmt.Errorf("failed to decode certificate PEM")
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check if certificate is still valid
	now := time.Now()
	if now.Before(x509Cert.NotBefore) || now.After(x509Cert.NotAfter) {
		return fmt.Errorf("certificate expired or not yet valid")
	}

	id.Certificate = cert
	id.PrivateKey = key
	id.Fingerprint = calculateFingerprint(x509Cert)
	id.AgentID = generateAgentID(id.Fingerprint, id.Region)

	return nil
}

// generateCredentials creates new certificate and private key.
func (id *Identity) generateCredentials(certPath, keyPath string) error {
	// Generate ECDSA private key (P-256 for good security/performance balance)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("opengslb-agent-%s", id.Region),
			Organization: []string{"OpenGSLB Agent"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(365 * 24 * time.Hour), // 1 year validity
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write certificate file
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Write private key file with restricted permissions
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		// Clean up certificate file on failure
		os.Remove(certPath)
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Parse the certificate we just created to get fingerprint
	x509Cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("failed to parse generated certificate: %w", err)
	}

	id.Certificate = certPEM
	id.PrivateKey = keyPEM
	id.Fingerprint = calculateFingerprint(x509Cert)
	id.AgentID = generateAgentID(id.Fingerprint, id.Region)

	return nil
}

// GetCertificatePEM returns the PEM-encoded certificate.
func (id *Identity) GetCertificatePEM() []byte {
	return id.Certificate
}

// GetFingerprint returns the SHA-256 fingerprint of the certificate.
func (id *Identity) GetFingerprint() string {
	return id.Fingerprint
}

// GetAgentID returns the unique agent identifier.
func (id *Identity) GetAgentID() string {
	return id.AgentID
}

// calculateFingerprint computes the SHA-256 fingerprint of a certificate.
func calculateFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// generateAgentID creates a unique agent identifier from fingerprint and region.
func generateAgentID(fingerprint, region string) string {
	// Use first 12 chars of fingerprint + region for readable ID
	shortFingerprint := fingerprint
	if len(shortFingerprint) > 12 {
		shortFingerprint = shortFingerprint[:12]
	}
	if region != "" {
		return fmt.Sprintf("%s-%s", region, shortFingerprint)
	}
	return shortFingerprint
}

// RotateCertificate generates new credentials, typically called before expiry.
func (id *Identity) RotateCertificate(certPath, keyPath string) error {
	id.logger.Info("rotating agent certificate",
		"old_fingerprint", id.Fingerprint,
	)

	// Backup old credentials
	oldCert := id.Certificate
	oldKey := id.PrivateKey
	oldFingerprint := id.Fingerprint

	// Generate new credentials
	if err := id.generateCredentials(certPath, keyPath); err != nil {
		// Restore old credentials on failure
		id.Certificate = oldCert
		id.PrivateKey = oldKey
		id.Fingerprint = oldFingerprint
		return fmt.Errorf("failed to rotate certificate: %w", err)
	}

	id.logger.Info("certificate rotated successfully",
		"old_fingerprint", oldFingerprint,
		"new_fingerprint", id.Fingerprint,
	)

	return nil
}

// NeedsRotation checks if the certificate should be rotated.
// Returns true if certificate expires within the given threshold.
func (id *Identity) NeedsRotation(threshold time.Duration) bool {
	block, _ := pem.Decode(id.Certificate)
	if block == nil {
		return true // Can't parse, should regenerate
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true // Can't parse, should regenerate
	}

	return time.Now().Add(threshold).After(cert.NotAfter)
}
