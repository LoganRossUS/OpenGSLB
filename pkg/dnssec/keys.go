// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package dnssec provides DNSSEC signing and key management for OpenGSLB.
//
// DNSSEC is enabled by default per Sprint 5 requirements. Disabling requires
// explicit security acknowledgment. The package supports:
//   - ECDSA P-256 (ECDSAP256SHA256) - default
//   - ECDSA P-384 (ECDSAP384SHA384)
//   - RSA (RSASHA256, RSASHA512) - not recommended for new deployments
//
// Key synchronization between Overwatch nodes uses a "newest key wins" strategy
// with periodic polling. This is the ONLY inter-Overwatch communication.
package dnssec

import (
	"crypto"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Algorithm represents a DNSSEC signing algorithm.
type Algorithm string

const (
	// AlgorithmECDSAP256SHA256 is the default and recommended algorithm.
	AlgorithmECDSAP256SHA256 Algorithm = "ECDSAP256SHA256"
	// AlgorithmECDSAP384SHA384 provides stronger security with larger keys.
	AlgorithmECDSAP384SHA384 Algorithm = "ECDSAP384SHA384"
)

// DefaultAlgorithm is the default DNSSEC signing algorithm.
const DefaultAlgorithm = AlgorithmECDSAP256SHA256

// KeyPair represents a DNSSEC key pair for a zone.
type KeyPair struct {
	// Zone is the DNS zone this key is for (e.g., "gslb.example.com.")
	Zone string `json:"zone"`

	// Algorithm is the signing algorithm used.
	Algorithm Algorithm `json:"algorithm"`

	// Flags indicates key type (256 = ZSK, 257 = KSK).
	Flags uint16 `json:"flags"`

	// KeyTag is the DNSKEY key tag for quick identification.
	KeyTag uint16 `json:"key_tag"`

	// PublicKey is the base64-encoded public key.
	PublicKey string `json:"public_key"`

	// PrivateKey is the base64-encoded private key.
	// Only present for locally generated keys, not synced keys.
	PrivateKey string `json:"private_key,omitempty"`

	// CreatedAt is when the key was generated.
	CreatedAt time.Time `json:"created_at"`

	// NodeID is the identifier of the node that generated this key.
	NodeID string `json:"node_id"`

	// dnskey is the cached DNSKEY record (not serialized).
	dnskey *dns.DNSKEY

	// signer is the cached crypto signer (not serialized).
	signer crypto.Signer
}

// KeyManager manages DNSSEC keys for all zones.
type KeyManager struct {
	mu     sync.RWMutex
	keys   map[string]*KeyPair // zone -> key
	nodeID string
}

// NewKeyManager creates a new DNSSEC key manager.
func NewKeyManager(nodeID string) *KeyManager {
	return &KeyManager{
		keys:   make(map[string]*KeyPair),
		nodeID: nodeID,
	}
}

// GenerateKey generates a new DNSSEC key pair for a zone.
func (km *KeyManager) GenerateKey(zone string, algorithm Algorithm) (*KeyPair, error) {
	if algorithm == "" {
		algorithm = DefaultAlgorithm
	}

	// Normalize zone name (ensure trailing dot)
	zone = dns.Fqdn(zone)

	// Generate the key based on algorithm
	var dnskey *dns.DNSKEY
	var privKey crypto.Signer
	var err error

	switch algorithm {
	case AlgorithmECDSAP256SHA256:
		dnskey, privKey, err = generateECDSAKey(zone, dns.ECDSAP256SHA256)
	case AlgorithmECDSAP384SHA384:
		dnskey, privKey, err = generateECDSAKey(zone, dns.ECDSAP384SHA384)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Create the key pair
	keyPair := &KeyPair{
		Zone:       zone,
		Algorithm:  algorithm,
		Flags:      dnskey.Flags,
		KeyTag:     dnskey.KeyTag(),
		PublicKey:  dnskey.PublicKey,
		PrivateKey: dnskey.PrivateKeyString(privKey),
		CreatedAt:  time.Now().UTC(),
		NodeID:     km.nodeID,
		dnskey:     dnskey,
		signer:     privKey,
	}

	// Store the key
	km.mu.Lock()
	km.keys[zone] = keyPair
	km.mu.Unlock()

	return keyPair, nil
}

// generateECDSAKey generates an ECDSA DNSSEC key pair.
func generateECDSAKey(zone string, alg uint8) (*dns.DNSKEY, crypto.Signer, error) {
	// Create the DNSKEY first with the algorithm
	dnskey := &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   zone,
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		Flags:     257, // KSK (Key Signing Key)
		Protocol:  3,   // DNSSEC
		Algorithm: alg,
	}

	// Use the Generate method to create the key pair
	// For ECDSAP256SHA256, bits is ignored (256 is used)
	// For ECDSAP384SHA384, bits is ignored (384 is used)
	bits := 256
	if alg == dns.ECDSAP384SHA384 {
		bits = 384
	}

	privKey, err := dnskey.Generate(bits)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate DNSSEC key: %w", err)
	}

	// Type assert to crypto.Signer
	signer, ok := privKey.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("generated key does not implement crypto.Signer")
	}

	return dnskey, signer, nil
}

// GetKey returns the DNSSEC key for a zone.
func (km *KeyManager) GetKey(zone string) *KeyPair {
	zone = dns.Fqdn(zone)
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.keys[zone]
}

// SetKey stores a key pair (used for synced keys).
func (km *KeyManager) SetKey(key *KeyPair) error {
	if key == nil {
		return fmt.Errorf("key cannot be nil")
	}

	zone := dns.Fqdn(key.Zone)
	key.Zone = zone

	// Rebuild the DNSKEY from stored data if needed
	if key.dnskey == nil {
		if err := key.rebuildDNSKEY(); err != nil {
			return fmt.Errorf("failed to rebuild DNSKEY: %w", err)
		}
	}

	km.mu.Lock()
	km.keys[zone] = key
	km.mu.Unlock()

	return nil
}

// GetAllKeys returns all managed keys.
func (km *KeyManager) GetAllKeys() []*KeyPair {
	km.mu.RLock()
	defer km.mu.RUnlock()

	keys := make([]*KeyPair, 0, len(km.keys))
	for _, key := range km.keys {
		keys = append(keys, key)
	}
	return keys
}

// GetZones returns all zones with managed keys.
func (km *KeyManager) GetZones() []string {
	km.mu.RLock()
	defer km.mu.RUnlock()

	zones := make([]string, 0, len(km.keys))
	for zone := range km.keys {
		zones = append(zones, zone)
	}
	return zones
}

// RemoveKey removes a key for a zone.
func (km *KeyManager) RemoveKey(zone string) {
	zone = dns.Fqdn(zone)
	km.mu.Lock()
	delete(km.keys, zone)
	km.mu.Unlock()
}

// DNSKEY returns the dns.DNSKEY record for this key pair.
func (kp *KeyPair) DNSKEY() *dns.DNSKEY {
	if kp.dnskey != nil {
		return kp.dnskey
	}

	// Rebuild from stored data
	if err := kp.rebuildDNSKEY(); err != nil {
		return nil
	}
	return kp.dnskey
}

// Signer returns the crypto.Signer for this key pair.
// Returns nil if this key was synced and doesn't have the private key.
func (kp *KeyPair) Signer() crypto.Signer {
	return kp.signer
}

// rebuildDNSKEY rebuilds the DNSKEY record from stored data.
func (kp *KeyPair) rebuildDNSKEY() error {
	var alg uint8
	switch kp.Algorithm {
	case AlgorithmECDSAP256SHA256:
		alg = dns.ECDSAP256SHA256
	case AlgorithmECDSAP384SHA384:
		alg = dns.ECDSAP384SHA384
	default:
		return fmt.Errorf("unsupported algorithm: %s", kp.Algorithm)
	}

	kp.dnskey = &dns.DNSKEY{
		Hdr: dns.RR_Header{
			Name:   kp.Zone,
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		Flags:     kp.Flags,
		Protocol:  3,
		Algorithm: alg,
		PublicKey: kp.PublicKey,
	}

	// Note: Private key reconstruction is not supported for synced keys.
	// Synced keys can only be used for verification, not signing.
	// The signer field remains nil for synced keys.

	return nil
}

// DSRecord returns the DS record for registering with the parent zone.
func (kp *KeyPair) DSRecord() *dns.DS {
	dnskey := kp.DNSKEY()
	if dnskey == nil {
		return nil
	}

	// Use SHA-256 for the DS record digest
	return dnskey.ToDS(dns.SHA256)
}

// DSRecordString returns the DS record as a string for easy publication.
func (kp *KeyPair) DSRecordString() string {
	ds := kp.DSRecord()
	if ds == nil {
		return ""
	}
	return ds.String()
}

// MarshalJSON implements json.Marshaler.
func (kp *KeyPair) MarshalJSON() ([]byte, error) {
	type Alias KeyPair
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(kp),
	})
}

// UnmarshalJSON implements json.Unmarshaler.
func (kp *KeyPair) UnmarshalJSON(data []byte) error {
	type Alias KeyPair
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(kp),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Rebuild DNSKEY after unmarshal
	return kp.rebuildDNSKEY()
}

// CanSign returns true if this key can be used for signing.
// Keys synced from other nodes may only have the public key.
func (kp *KeyPair) CanSign() bool {
	return kp.signer != nil
}

// Age returns how long ago this key was created.
func (kp *KeyPair) Age() time.Duration {
	return time.Since(kp.CreatedAt)
}

// IsNewerThan returns true if this key is newer than the other key.
func (kp *KeyPair) IsNewerThan(other *KeyPair) bool {
	if other == nil {
		return true
	}
	return kp.CreatedAt.After(other.CreatedAt)
}
