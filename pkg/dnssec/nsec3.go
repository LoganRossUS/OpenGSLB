// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dnssec

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// NSEC3Config configures NSEC3 parameters.
type NSEC3Config struct {
	// Iterations is the number of additional hash iterations.
	// Higher values provide more security but cost CPU.
	// Recommended: 10 (RFC 5155 recommends between 10-500)
	Iterations uint16

	// SaltLength is the length of the salt in bytes.
	// Recommended: 8 bytes
	SaltLength int

	// SaltRotationInterval is how often to rotate the salt.
	// Default: 24 hours (not implemented in this version)
	// SaltRotationInterval time.Duration

	// OptOut enables NSEC3 opt-out for unsigned delegations.
	// Default: false (we sign everything)
	OptOut bool
}

// DefaultNSEC3Config returns the default NSEC3 configuration.
func DefaultNSEC3Config() NSEC3Config {
	return NSEC3Config{
		Iterations: 10,
		SaltLength: 8,
		OptOut:     false,
	}
}

// NSEC3Manager manages NSEC3 records for authenticated denial of existence.
// NSEC3 prevents zone enumeration attacks while proving non-existence.
type NSEC3Manager struct {
	config NSEC3Config
	mu     sync.RWMutex
	salts  map[string]string // zone -> salt (hex encoded)
}

// NewNSEC3Manager creates a new NSEC3 manager.
func NewNSEC3Manager(config NSEC3Config) (*NSEC3Manager, error) {
	if config.Iterations == 0 {
		config.Iterations = 10
	}
	if config.SaltLength == 0 {
		config.SaltLength = 8
	}
	if config.SaltLength > 255 {
		return nil, fmt.Errorf("salt length must be <= 255 bytes")
	}

	return &NSEC3Manager{
		config: config,
		salts:  make(map[string]string),
	}, nil
}

// GenerateSalt generates a new salt for a zone.
func (m *NSEC3Manager) GenerateSalt(zone string) (string, error) {
	zone = dns.Fqdn(zone)

	salt := make([]byte, m.config.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	saltHex := strings.ToUpper(hex.EncodeToString(salt))

	m.mu.Lock()
	m.salts[zone] = saltHex
	m.mu.Unlock()

	return saltHex, nil
}

// GetSalt returns the current salt for a zone, generating one if needed.
func (m *NSEC3Manager) GetSalt(zone string) string {
	zone = dns.Fqdn(zone)

	m.mu.RLock()
	salt, exists := m.salts[zone]
	m.mu.RUnlock()

	if exists {
		return salt
	}

	// Generate new salt
	salt, err := m.GenerateSalt(zone)
	if err != nil {
		// Return empty salt on error (still valid, just less secure)
		return ""
	}
	return salt
}

// SetSalt sets a specific salt for a zone (used for key sync).
func (m *NSEC3Manager) SetSalt(zone, salt string) {
	zone = dns.Fqdn(zone)
	m.mu.Lock()
	m.salts[zone] = salt
	m.mu.Unlock()
}

// HashName computes the NSEC3 hash of a DNS name.
func (m *NSEC3Manager) HashName(name, zone string) string {
	zone = dns.Fqdn(zone)
	name = dns.Fqdn(name)
	salt := m.GetSalt(zone)

	return nsec3Hash(name, salt, m.config.Iterations)
}

// nsec3Hash computes the NSEC3 hash of a name.
func nsec3Hash(name, saltHex string, iterations uint16) string {
	// Decode salt from hex
	salt, _ := hex.DecodeString(saltHex)

	// Convert name to wire format (lowercase)
	name = strings.ToLower(name)
	wireFormat := make([]byte, 0, 256)
	for _, label := range dns.SplitDomainName(name) {
		wireFormat = append(wireFormat, byte(len(label)))
		wireFormat = append(wireFormat, []byte(label)...)
	}
	wireFormat = append(wireFormat, 0) // root label

	// Initial hash: H(name || salt)
	h := sha1.New()
	h.Write(wireFormat)
	h.Write(salt)
	digest := h.Sum(nil)

	// Iterate: H(digest || salt)
	for i := uint16(0); i < iterations; i++ {
		h.Reset()
		h.Write(digest)
		h.Write(salt)
		digest = h.Sum(nil)
	}

	// Encode as base32hex (RFC 4648 extended hex alphabet)
	return strings.ToUpper(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(digest))
}

// GenerateNXDOMAIN generates NSEC3 records for proving NXDOMAIN.
// Returns the NSEC3 records that should be added to the authority section.
func (m *NSEC3Manager) GenerateNXDOMAIN(zone, qname string) []dns.RR {
	zone = dns.Fqdn(zone)
	qname = dns.Fqdn(qname)

	// For a proper NXDOMAIN proof, we need:
	// 1. An NSEC3 that covers the hash of qname (closest encloser proof)
	// 2. An NSEC3 that matches the next closer name
	// 3. An NSEC3 for the wildcard at the closest encloser

	// In a dynamic environment (GSLB), we generate synthetic NSEC3 records
	// that prove the specific name doesn't exist.

	qnameHash := m.HashName(qname, zone)
	salt := m.GetSalt(zone)

	// Generate a synthetic NSEC3 that covers the queried name
	// The "previous" hash is just before the queried name's hash
	// The "next" hash is just after
	prevHash := decrementHash(qnameHash)
	nextHash := incrementHash(qnameHash)

	nsec3 := m.createNSEC3(zone, prevHash, nextHash, salt)

	// Also need an NSEC3 for the wildcard
	wildcardName := "*." + closestEncloser(qname, zone)
	wildcardHash := m.HashName(wildcardName, zone)
	wildcardPrevHash := decrementHash(wildcardHash)
	wildcardNextHash := incrementHash(wildcardHash)

	wildcardNSEC3 := m.createNSEC3(zone, wildcardPrevHash, wildcardNextHash, salt)

	return []dns.RR{nsec3, wildcardNSEC3}
}

// GenerateNoData generates NSEC3 records for proving NODATA (name exists but not the type).
func (m *NSEC3Manager) GenerateNoData(zone, qname string, existingTypes []uint16) []dns.RR {
	zone = dns.Fqdn(zone)
	qname = dns.Fqdn(qname)

	qnameHash := m.HashName(qname, zone)
	salt := m.GetSalt(zone)

	// For NODATA, we return an NSEC3 that matches the name
	// but shows the queried type doesn't exist
	nsec3 := m.createNSEC3WithTypes(zone, qnameHash, incrementHash(qnameHash), salt, existingTypes)

	return []dns.RR{nsec3}
}

// createNSEC3 creates an NSEC3 record covering a hash range.
func (m *NSEC3Manager) createNSEC3(zone, ownerHash, nextHash, saltHex string) *dns.NSEC3 {
	return m.createNSEC3WithTypes(zone, ownerHash, nextHash, saltHex, nil)
}

// createNSEC3WithTypes creates an NSEC3 record with specific type bits.
func (m *NSEC3Manager) createNSEC3WithTypes(zone, ownerHash, nextHash, saltHex string, types []uint16) *dns.NSEC3 {
	// Default types (minimal for NXDOMAIN proof)
	if len(types) == 0 {
		types = []uint16{dns.TypeRRSIG, dns.TypeNSEC3}
	}

	// Sort types for canonical ordering
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })

	return &dns.NSEC3{
		Hdr: dns.RR_Header{
			Name:   ownerHash + "." + zone,
			Rrtype: dns.TypeNSEC3,
			Class:  dns.ClassINET,
			Ttl:    300, // Short TTL for dynamic records
		},
		Hash:       1, // SHA-1 (only standardized algorithm)
		Flags:      m.nsec3Flags(),
		Iterations: m.config.Iterations,
		SaltLength: uint8(len(saltHex) / 2),
		Salt:       saltHex,
		HashLength: 20, // SHA-1 produces 20 bytes
		NextDomain: nextHash,
		TypeBitMap: types,
	}
}

// nsec3Flags returns the NSEC3 flags byte.
func (m *NSEC3Manager) nsec3Flags() uint8 {
	if m.config.OptOut {
		return 1 // Opt-out flag set
	}
	return 0
}

// NSEC3PARAM returns the NSEC3PARAM record for a zone.
// This should be included in zone transfers and at the zone apex.
func (m *NSEC3Manager) NSEC3PARAM(zone string) *dns.NSEC3PARAM {
	zone = dns.Fqdn(zone)
	salt := m.GetSalt(zone)

	return &dns.NSEC3PARAM{
		Hdr: dns.RR_Header{
			Name:   zone,
			Rrtype: dns.TypeNSEC3PARAM,
			Class:  dns.ClassINET,
			Ttl:    0, // NSEC3PARAM has TTL of 0
		},
		Hash:       1, // SHA-1
		Flags:      0, // Flags are 0 in NSEC3PARAM
		Iterations: m.config.Iterations,
		SaltLength: uint8(len(salt) / 2),
		Salt:       salt,
	}
}

// closestEncloser finds the closest encloser of a name within a zone.
func closestEncloser(name, zone string) string {
	name = dns.Fqdn(name)
	zone = dns.Fqdn(zone)

	// The closest encloser is the longest existing ancestor of the name
	// For GSLB, we assume the zone apex always exists
	labels := dns.SplitDomainName(name)
	zoneLabels := len(dns.SplitDomainName(zone))

	if len(labels) <= zoneLabels {
		return zone
	}

	// Return the zone (which is the closest encloser we know exists)
	return zone
}

// incrementHash returns the next hash in lexicographic order.
func incrementHash(hash string) string {
	bytes := []byte(hash)
	for i := len(bytes) - 1; i >= 0; i-- {
		if bytes[i] < 'V' { // 'V' is the highest base32hex character
			bytes[i]++
			return string(bytes)
		}
		bytes[i] = '0' // Reset to lowest and carry
	}
	// Overflow - wrap around (shouldn't happen in practice)
	return hash
}

// decrementHash returns the previous hash in lexicographic order.
func decrementHash(hash string) string {
	bytes := []byte(hash)
	for i := len(bytes) - 1; i >= 0; i-- {
		if bytes[i] > '0' { // '0' is the lowest base32hex character
			bytes[i]--
			return string(bytes)
		}
		bytes[i] = 'V' // Reset to highest and borrow
	}
	// Underflow - wrap around (shouldn't happen in practice)
	return hash
}

// GetConfig returns the current NSEC3 configuration.
func (m *NSEC3Manager) GetConfig() NSEC3Config {
	return m.config
}
