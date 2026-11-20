// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/miekg/dns"
)

// Defaults for zone apex records that are not set in dns.zones.
const (
	// DefaultZoneTTL is the TTL of the SOA and NS records at a zone apex.
	DefaultZoneTTL = 3600
	// DefaultSOASerial is the SOA serial. OpenGSLB has no zone transfers, so
	// nothing compares serials; a fixed value keeps every overwatch that
	// serves the zone in agreement (see the zone apex ADR in
	// docs/ARCHITECTURE_DECISIONS.md).
	DefaultSOASerial = 1
	// DefaultSOARefresh, DefaultSOARetry and DefaultSOAExpire are the SOA
	// timers. They only matter to secondaries, which OpenGSLB does not have.
	DefaultSOARefresh = 3600
	DefaultSOARetry   = 600
	DefaultSOAExpire  = 604800
	// DefaultSOANegativeTTL is the SOA MINIMUM field: how long resolvers may
	// cache NXDOMAIN and empty answers (RFC 2308). It is short because GSLB
	// names are created at runtime through the API and by agents.
	DefaultSOANegativeTTL = 60
)

// Zone is an authoritative zone with every SOA and NS field resolved.
// All names are canonical: lower case with a trailing dot.
type Zone struct {
	// Name is the zone apex.
	Name string
	// TTL is the TTL of the apex SOA and NS records.
	TTL uint32
	// PrimaryNS is the SOA MNAME.
	PrimaryNS string
	// Contact is the SOA RNAME in DNS name form.
	Contact string
	// Serial, Refresh, Retry, Expire and NegativeTTL are the remaining SOA
	// fields. NegativeTTL is the SOA MINIMUM.
	Serial      uint32
	Refresh     uint32
	Retry       uint32
	Expire      uint32
	NegativeTTL uint32
	// Nameservers is the NS record set at the apex.
	Nameservers []string
	// Implicit is true for a zone that is not listed in dns.zones: a GSLB
	// domain outside every configured zone is served as its own zone.
	Implicit bool
}

// soa returns the zone's SOA record with the given owner name and TTL.
func (z *Zone) soa(owner string, ttl uint32) *dns.SOA {
	return &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   owner,
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		Ns:      z.PrimaryNS,
		Mbox:    z.Contact,
		Serial:  z.Serial,
		Refresh: z.Refresh,
		Retry:   z.Retry,
		Expire:  z.Expire,
		Minttl:  z.NegativeTTL,
	}
}

// negativeSOA returns the SOA record for the authority section of an
// NXDOMAIN or empty NOERROR response. RFC 2308 section 3: its TTL is the
// minimum of the SOA TTL and the SOA MINIMUM field, and it tells resolvers
// how long they may cache the negative answer.
func (z *Zone) negativeSOA() *dns.SOA {
	ttl := z.TTL
	if z.NegativeTTL < ttl {
		ttl = z.NegativeTTL
	}
	return z.soa(z.Name, ttl)
}

// ns returns the zone's NS record set with the given owner name.
func (z *Zone) ns(owner string) []dns.RR {
	rrs := make([]dns.RR, 0, len(z.Nameservers))
	for _, target := range z.Nameservers {
		rrs = append(rrs, &dns.NS{
			Hdr: dns.RR_Header{
				Name:   owner,
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    z.TTL,
			},
			Ns: target,
		})
	}
	return rrs
}

// ZoneTable holds the configured zones and synthesizes implicit ones.
// It is immutable after construction and safe for concurrent use.
type ZoneTable struct {
	zones map[string]*Zone
	// above holds every proper ancestor of a configured zone apex. Such a
	// name exists in the DNS tree (it has a descendant), so a query for it
	// inside an enclosing zone must not be answered with NXDOMAIN.
	above map[string]struct{}
	// nodeID is the node identity that default nameserver names derive from.
	nodeID string
}

// BuildZones builds the zone table from the configuration. Unset SOA and NS
// fields are derived from overwatch.identity.node_id, or from the host name
// when that is empty.
func BuildZones(cfg *config.Config) (*ZoneTable, error) {
	nodeID := cfg.Overwatch.Identity.NodeID
	if nodeID == "" {
		if hostname, err := os.Hostname(); err == nil {
			nodeID = hostname
		}
	}
	return NewZoneTable(cfg.DNS.Zones, nodeID)
}

// NewZoneTable builds a zone table from zone settings and a node identity.
func NewZoneTable(zones []config.Zone, nodeID string) (*ZoneTable, error) {
	t := &ZoneTable{
		zones:  make(map[string]*Zone, len(zones)),
		above:  make(map[string]struct{}),
		nodeID: nodeID,
	}

	for i, zc := range zones {
		apex := normalizeDomain(zc.Name)
		if apex == "" || apex == "." {
			return nil, fmt.Errorf("dns.zones[%d]: invalid zone name %q", i, zc.Name)
		}
		if _, ok := dns.IsDomainName(apex); !ok {
			return nil, fmt.Errorf("dns.zones[%d]: invalid zone name %q", i, zc.Name)
		}
		if _, dup := t.zones[apex]; dup {
			return nil, fmt.Errorf("dns.zones[%d]: duplicate zone %q", i, zc.Name)
		}

		zone := t.defaultZone(apex)
		zone.Implicit = false

		if zc.TTL > 0 {
			zone.TTL = uint32(zc.TTL)
		}
		if zc.SOA.Contact != "" {
			contact, err := config.MailboxToDNSName(zc.SOA.Contact)
			if err != nil {
				return nil, fmt.Errorf("dns.zones[%d].soa.contact: %w", i, err)
			}
			zone.Contact = strings.ToLower(contact)
		}
		if zc.SOA.Serial > 0 {
			zone.Serial = zc.SOA.Serial
		}
		if zc.SOA.Refresh > 0 {
			zone.Refresh = uint32(zc.SOA.Refresh)
		}
		if zc.SOA.Retry > 0 {
			zone.Retry = uint32(zc.SOA.Retry)
		}
		if zc.SOA.Expire > 0 {
			zone.Expire = uint32(zc.SOA.Expire)
		}
		if zc.SOA.NegativeTTL > 0 {
			zone.NegativeTTL = uint32(zc.SOA.NegativeTTL)
		}

		// The NS set and the SOA primary nameserver default to each other:
		//   - neither set: both are the name derived from the node identity;
		//   - only soa.primary_ns set: it is the whole NS set;
		//   - only nameservers set: the first of them is the primary, so that
		//     overwatches sharing a zone configuration answer the same SOA
		//     whatever their node IDs are.
		var nameservers []string
		for j, ns := range zc.Nameservers {
			name := normalizeDomain(ns)
			if _, ok := dns.IsDomainName(name); !ok || name == "" || name == "." {
				return nil, fmt.Errorf("dns.zones[%d].nameservers[%d]: invalid name %q", i, j, ns)
			}
			nameservers = append(nameservers, name)
		}
		switch {
		case zc.SOA.PrimaryNS != "":
			zone.PrimaryNS = normalizeDomain(zc.SOA.PrimaryNS)
		case len(nameservers) > 0:
			zone.PrimaryNS = nameservers[0]
		}
		if len(nameservers) == 0 {
			nameservers = []string{zone.PrimaryNS}
		}
		zone.Nameservers = nameservers

		t.zones[apex] = zone
	}

	for apex := range t.zones {
		for off, end := dns.NextLabel(apex, 0); !end; off, end = dns.NextLabel(apex, off) {
			t.above[apex[off:]] = struct{}{}
		}
	}

	return t, nil
}

// Find returns the configured zone that contains name: the zone whose apex is
// the longest suffix of name on a label boundary. It returns nil when name is
// outside every configured zone. name must be canonical.
func (t *ZoneTable) Find(name string) *Zone {
	if t == nil || len(t.zones) == 0 {
		return nil
	}
	for off, end := 0, false; !end; off, end = dns.NextLabel(name, off) {
		if zone, ok := t.zones[name[off:]]; ok {
			return zone
		}
	}
	return nil
}

// Implicit returns the zone served for a GSLB domain that lies outside every
// configured zone. The domain name is the apex and every SOA and NS field has
// its default value. apex must be canonical.
func (t *ZoneTable) Implicit(apex string) *Zone {
	if t == nil {
		t = &ZoneTable{}
	}
	return t.defaultZone(apex)
}

// hasApexBelow reports whether a configured zone apex lies below name.
func (t *ZoneTable) hasApexBelow(name string) bool {
	if t == nil {
		return false
	}
	_, ok := t.above[name]
	return ok
}

// Zones returns a copy of the configured zones sorted by name.
func (t *ZoneTable) Zones() []Zone {
	if t == nil {
		return nil
	}
	out := make([]Zone, 0, len(t.zones))
	for _, zone := range t.zones {
		z := *zone
		z.Nameservers = append([]string(nil), zone.Nameservers...)
		out = append(out, z)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// defaultZone returns a zone for apex with every field at its default.
func (t *ZoneTable) defaultZone(apex string) *Zone {
	primary := defaultPrimaryNS(t.nodeID, apex)
	return &Zone{
		Name:        apex,
		TTL:         DefaultZoneTTL,
		PrimaryNS:   primary,
		Contact:     defaultContact(apex),
		Serial:      DefaultSOASerial,
		Refresh:     DefaultSOARefresh,
		Retry:       DefaultSOARetry,
		Expire:      DefaultSOAExpire,
		NegativeTTL: DefaultSOANegativeTTL,
		Nameservers: []string{primary},
		Implicit:    true,
	}
}

// defaultPrimaryNS derives the default SOA MNAME (and NS target) of a zone
// from the node identity:
//
//   - a node ID that is a host name with at least two labels (for example
//     "overwatch-1.dc1.example.net") is used as is;
//   - any other node ID is reduced to one DNS label and placed under the
//     zone apex ("overwatch-1" in zone "gslb.example.com." gives
//     "overwatch-1.gslb.example.com.");
//   - with no usable node ID the label "ns" is used.
func defaultPrimaryNS(nodeID, apex string) string {
	id := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(nodeID), "."))
	if strings.Contains(id, ".") && isHostname(id) {
		return id + "."
	}
	label := hostLabel(id)
	if label == "" {
		label = "ns"
	}
	if name := label + "." + apex; isDomainName(name) {
		return name
	}
	if name := "ns." + apex; isDomainName(name) {
		return name
	}
	// The apex is too long to take another label. It is a valid name itself.
	return apex
}

// defaultContact returns the default SOA RNAME: hostmaster.<apex>.
func defaultContact(apex string) string {
	if name := "hostmaster." + apex; isDomainName(name) {
		return name
	}
	// The apex is too long to take another label.
	return "hostmaster.invalid."
}

// isDomainName reports whether name is a syntactically valid DNS name.
func isDomainName(name string) bool {
	_, ok := dns.IsDomainName(name)
	return ok
}

// isHostname reports whether name consists only of letter-digit-hyphen labels
// and is a valid DNS name.
func isHostname(name string) bool {
	if !isDomainName(name) {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" || label != hostLabel(label) {
			return false
		}
	}
	return true
}

// hostLabel reduces s to a single valid host name label: lower-case letters,
// digits and hyphens, no leading or trailing hyphen, at most 63 octets.
// Every other character becomes a hyphen.
func hostLabel(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		default:
			b.WriteByte('-')
		}
	}
	label := strings.Trim(b.String(), "-")
	if len(label) > 63 {
		label = strings.TrimRight(label[:63], "-")
	}
	return label
}
