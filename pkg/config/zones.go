// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package config

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

// Zone is one entry of dns.zones: a zone this overwatch is authoritative for.
//
// In YAML a zone is either a plain string (the zone name, with every other
// setting left at its default) or a mapping with the fields below. Both forms
// can be mixed in one list:
//
//	zones:
//	  - gslb.example.com
//	  - name: internal.corp
//	    nameservers: [ns1.internal.corp, ns2.internal.corp]
//
// Unset fields are filled in when the DNS server builds its zone table (see
// pkg/dns BuildZones), not here, because the defaults depend on the node
// identity. The configuration therefore reads back exactly as it was written.
type Zone struct {
	// Name is the zone apex, for example "gslb.example.com". A trailing dot is
	// optional and case is ignored.
	Name string `yaml:"name"`

	// TTL is the TTL in seconds of the SOA and NS records at the apex.
	// Default: 3600.
	TTL int `yaml:"ttl,omitempty"`

	// SOA holds the fields of the zone's SOA record.
	SOA ZoneSOA `yaml:"soa,omitempty"`

	// Nameservers is the NS record set at the apex, as fully qualified host
	// names. It should match the delegation in the parent zone.
	// Default: the SOA primary nameserver.
	Nameservers []string `yaml:"nameservers,omitempty"`
}

// ZoneSOA holds the configurable fields of a zone's SOA record. Every field is
// optional. Timers are in seconds, as in a zone file.
type ZoneSOA struct {
	// PrimaryNS is the SOA MNAME, the primary nameserver of the zone.
	// Default: derived from overwatch.identity.node_id (see docs).
	PrimaryNS string `yaml:"primary_ns,omitempty"`

	// Contact is the SOA RNAME, the mailbox of the person responsible for the
	// zone. Accepts "user@example.com" or the DNS form "user.example.com".
	// Default: hostmaster.<zone>.
	Contact string `yaml:"contact,omitempty"`

	// Serial is the SOA serial number. Default: 1.
	Serial uint32 `yaml:"serial,omitempty"`

	// Refresh is the SOA REFRESH timer in seconds. Default: 3600.
	Refresh int `yaml:"refresh,omitempty"`

	// Retry is the SOA RETRY timer in seconds. Default: 600.
	Retry int `yaml:"retry,omitempty"`

	// Expire is the SOA EXPIRE timer in seconds. Default: 604800.
	Expire int `yaml:"expire,omitempty"`

	// NegativeTTL is the SOA MINIMUM field in seconds: how long resolvers may
	// cache NXDOMAIN and empty answers (RFC 2308). Default: 60.
	NegativeTTL int `yaml:"negative_ttl,omitempty"`
}

// zoneYAML has the same fields as Zone without its YAML methods, so the
// mapping form can be decoded and encoded without recursion.
type zoneYAML Zone

// UnmarshalYAML accepts a plain string (the zone name) or a mapping.
func (z *Zone) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*z = Zone{Name: value.Value}
		return nil
	case yaml.MappingNode:
		var raw zoneYAML
		if err := value.Decode(&raw); err != nil {
			return err
		}
		*z = Zone(raw)
		return nil
	default:
		return fmt.Errorf("line %d: a dns.zones entry must be a zone name or a mapping with a name field", value.Line)
	}
}

// MarshalYAML writes a zone that has only a name as a plain string and any
// other zone as a mapping.
func (z Zone) MarshalYAML() (interface{}, error) {
	if z.TTL == 0 && z.SOA == (ZoneSOA{}) && len(z.Nameservers) == 0 {
		return z.Name, nil
	}
	return zoneYAML(z), nil
}

// ZoneNames returns the configured zone names in order, as written.
func (d DNSConfig) ZoneNames() []string {
	if len(d.Zones) == 0 {
		return nil
	}
	names := make([]string, len(d.Zones))
	for i, z := range d.Zones {
		names[i] = z.Name
	}
	return names
}

// MailboxToDNSName converts a zone contact to the DNS name form used in the
// SOA RNAME field. "dns.admin@example.com" becomes "dns\.admin.example.com."
// (dots in the local part are escaped, RFC 1035 section 8). A value without
// "@" is taken to be in DNS name form already and only gets its trailing dot.
func MailboxToDNSName(contact string) (string, error) {
	if contact == "" {
		return "", fmt.Errorf("empty contact")
	}
	name := contact
	if at := strings.LastIndex(contact, "@"); at >= 0 {
		local, domain := contact[:at], contact[at+1:]
		if local == "" || domain == "" {
			return "", fmt.Errorf("%q is not a mailbox of the form user@domain", contact)
		}
		name = strings.ReplaceAll(local, ".", `\.`) + "." + domain
	}
	name = dns.Fqdn(name)
	if !validDNSName(name) {
		return "", fmt.Errorf("%q does not convert to a valid DNS name", contact)
	}
	return name, nil
}

// validDNSName reports whether name is a syntactically valid, non-root DNS
// name (labels of 1 to 63 octets, 255 octets in total).
func validDNSName(name string) bool {
	if name == "" || name == "." {
		return false
	}
	_, ok := dns.IsDomainName(name)
	return ok
}
