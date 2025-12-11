// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dnssec

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/miekg/dns"
)

// SignerConfig configures the DNSSEC signer.
type SignerConfig struct {
	// KeyManager provides access to DNSSEC keys.
	KeyManager *KeyManager

	// NSEC3Manager handles authenticated denial of existence.
	NSEC3Manager *NSEC3Manager

	// Logger for signing operations.
	Logger *slog.Logger

	// SignatureTTL is the TTL for RRSIG records.
	// Default: 86400 (24 hours)
	SignatureTTL uint32

	// SignatureInception is how far in the past to set inception time.
	// This allows for clock skew. Default: 1 hour
	SignatureInception time.Duration

	// SignatureExpiration is how far in the future to set expiration.
	// Default: 7 days
	SignatureExpiration time.Duration

	// MetricsCallback is called with signing duration for metrics.
	MetricsCallback func(zone string, duration time.Duration, success bool)
}

// Signer signs DNS responses with DNSSEC.
type Signer struct {
	config SignerConfig
	logger *slog.Logger
}

// NewSigner creates a new DNSSEC signer.
func NewSigner(config SignerConfig) *Signer {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if config.SignatureTTL == 0 {
		config.SignatureTTL = 86400 // 24 hours
	}
	if config.SignatureInception == 0 {
		config.SignatureInception = time.Hour
	}
	if config.SignatureExpiration == 0 {
		config.SignatureExpiration = 7 * 24 * time.Hour
	}

	return &Signer{
		config: config,
		logger: logger,
	}
}

// SignResponse implements the DNSSECSigner interface.
// It accepts interface{} and returns interface{} to avoid import cycles.
func (s *Signer) SignResponse(msg interface{}) (interface{}, error) {
	if msg == nil {
		return msg, nil
	}
	dnsMsg, ok := msg.(*dns.Msg)
	if !ok {
		return msg, fmt.Errorf("expected *dns.Msg, got %T", msg)
	}
	return s.signDNSMessage(dnsMsg)
}

// signDNSMessage signs a DNS response message.
// Returns the signed message and any error encountered.
// If signing fails for any reason, the original message is returned unchanged.
func (s *Signer) signDNSMessage(msg *dns.Msg) (*dns.Msg, error) {
	if msg == nil || len(msg.Question) == 0 {
		return msg, nil
	}

	start := time.Now()
	zone := s.findZoneForName(msg.Question[0].Name)

	keyPair := s.config.KeyManager.GetKey(zone)
	if keyPair == nil || !keyPair.CanSign() {
		s.logger.Debug("no signing key available for zone",
			"zone", zone,
			"name", msg.Question[0].Name,
		)
		return msg, nil
	}

	// Create a copy to avoid modifying the original
	signed := msg.Copy()

	// Sign answer section
	if err := s.signSection(&signed.Answer, keyPair, zone); err != nil {
		s.recordMetrics(zone, time.Since(start), false)
		return msg, fmt.Errorf("failed to sign answer section: %w", err)
	}

	// Sign authority section
	if err := s.signSection(&signed.Ns, keyPair, zone); err != nil {
		s.recordMetrics(zone, time.Since(start), false)
		return msg, fmt.Errorf("failed to sign authority section: %w", err)
	}

	// Add DNSKEY to additional section for DNSKEY queries
	if len(msg.Question) > 0 && msg.Question[0].Qtype == dns.TypeDNSKEY {
		dnskey := keyPair.DNSKEY()
		if dnskey != nil {
			signed.Answer = append(signed.Answer, dnskey)
		}
	}

	// Handle NXDOMAIN with NSEC3
	if msg.Rcode == dns.RcodeNameError && s.config.NSEC3Manager != nil {
		nsec3Records := s.config.NSEC3Manager.GenerateNXDOMAIN(zone, msg.Question[0].Name)
		signed.Ns = append(signed.Ns, nsec3Records...)
		// Sign the NSEC3 records
		if err := s.signSection(&signed.Ns, keyPair, zone); err != nil {
			s.recordMetrics(zone, time.Since(start), false)
			return msg, fmt.Errorf("failed to sign NSEC3 records: %w", err)
		}
	}

	// Set the AD (Authenticated Data) flag
	signed.AuthenticatedData = true

	s.recordMetrics(zone, time.Since(start), true)

	s.logger.Debug("signed DNS response",
		"zone", zone,
		"name", msg.Question[0].Name,
		"answers", len(signed.Answer),
		"duration", time.Since(start),
	)

	return signed, nil
}

// signSection signs all RRsets in a section.
func (s *Signer) signSection(section *[]dns.RR, keyPair *KeyPair, zone string) error {
	if len(*section) == 0 {
		return nil
	}

	// Group RRs by type and name to form RRsets
	rrsets := s.groupRRsets(*section)

	var signedSection []dns.RR

	for _, rrset := range rrsets {
		if len(rrset) == 0 {
			continue
		}

		// Skip already-signed RRsets (RRSIG records)
		if rrset[0].Header().Rrtype == dns.TypeRRSIG {
			signedSection = append(signedSection, rrset...)
			continue
		}

		// Add the original records
		signedSection = append(signedSection, rrset...)

		// Create and add the signature
		rrsig, err := s.signRRset(rrset, keyPair, zone)
		if err != nil {
			return fmt.Errorf("failed to sign RRset: %w", err)
		}

		signedSection = append(signedSection, rrsig)
	}

	*section = signedSection
	return nil
}

// groupRRsets groups RRs by name and type to form RRsets.
func (s *Signer) groupRRsets(rrs []dns.RR) [][]dns.RR {
	groups := make(map[string][]dns.RR)

	for _, rr := range rrs {
		key := fmt.Sprintf("%s:%d", rr.Header().Name, rr.Header().Rrtype)
		groups[key] = append(groups[key], rr)
	}

	result := make([][]dns.RR, 0, len(groups))
	for _, rrset := range groups {
		result = append(result, rrset)
	}

	return result
}

// signRRset creates an RRSIG for an RRset.
func (s *Signer) signRRset(rrset []dns.RR, keyPair *KeyPair, zone string) (*dns.RRSIG, error) {
	if len(rrset) == 0 {
		return nil, fmt.Errorf("cannot sign empty RRset")
	}

	dnskey := keyPair.DNSKEY()
	if dnskey == nil {
		return nil, fmt.Errorf("no DNSKEY available")
	}

	signer := keyPair.Signer()
	if signer == nil {
		return nil, fmt.Errorf("no signer available (key may be synced without private key)")
	}

	now := time.Now().UTC()

	rrsig := &dns.RRSIG{
		Hdr: dns.RR_Header{
			Name:   rrset[0].Header().Name,
			Rrtype: dns.TypeRRSIG,
			Class:  dns.ClassINET,
			Ttl:    s.config.SignatureTTL,
		},
		TypeCovered: rrset[0].Header().Rrtype,
		Algorithm:   dnskey.Algorithm,
		Labels:      uint8(dns.CountLabel(rrset[0].Header().Name)),
		OrigTtl:     rrset[0].Header().Ttl,
		Expiration:  uint32(now.Add(s.config.SignatureExpiration).Unix()),
		Inception:   uint32(now.Add(-s.config.SignatureInception).Unix()),
		KeyTag:      dnskey.KeyTag(),
		SignerName:  zone,
	}

	// Sign the RRset
	if err := rrsig.Sign(signer, rrset); err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}

	return rrsig, nil
}

// findZoneForName finds the appropriate zone for a DNS name.
// Returns the zone or the name itself if no zone matches.
func (s *Signer) findZoneForName(name string) string {
	name = dns.Fqdn(name)

	// Get all managed zones
	zones := s.config.KeyManager.GetZones()

	// Find the longest matching zone suffix
	var bestMatch string
	for _, zone := range zones {
		if dns.IsSubDomain(zone, name) {
			if len(zone) > len(bestMatch) {
				bestMatch = zone
			}
		}
	}

	if bestMatch != "" {
		return bestMatch
	}

	// Fall back to the name's zone (parent domain)
	labels := dns.SplitDomainName(name)
	if len(labels) >= 2 {
		return dns.Fqdn(labels[len(labels)-2] + "." + labels[len(labels)-1])
	}

	return name
}

// recordMetrics records signing metrics if a callback is configured.
func (s *Signer) recordMetrics(zone string, duration time.Duration, success bool) {
	if s.config.MetricsCallback != nil {
		s.config.MetricsCallback(zone, duration, success)
	}
}

// AddDNSKEYToResponse adds the DNSKEY record to a response.
// This should be called for DNSKEY queries.
func (s *Signer) AddDNSKEYToResponse(msg *dns.Msg, zone string) {
	keyPair := s.config.KeyManager.GetKey(zone)
	if keyPair == nil {
		return
	}

	dnskey := keyPair.DNSKEY()
	if dnskey != nil {
		msg.Answer = append(msg.Answer, dnskey)
	}
}

// GetDNSKEY returns the DNSKEY record for a zone.
func (s *Signer) GetDNSKEY(zone string) *dns.DNSKEY {
	keyPair := s.config.KeyManager.GetKey(zone)
	if keyPair == nil {
		return nil
	}
	return keyPair.DNSKEY()
}

// GetDSRecord returns the DS record for a zone.
func (s *Signer) GetDSRecord(zone string) *dns.DS {
	keyPair := s.config.KeyManager.GetKey(zone)
	if keyPair == nil {
		return nil
	}
	return keyPair.DSRecord()
}
