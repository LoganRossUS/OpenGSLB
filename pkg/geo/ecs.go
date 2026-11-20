// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package geo

import (
	"net"

	"github.com/miekg/dns"
)

// ECSResult contains the result of parsing EDNS Client Subnet from a DNS message.
type ECSResult struct {
	// IP is the client subnet IP address
	IP net.IP

	// SourceNetmask is the source prefix length
	SourceNetmask uint8

	// Found indicates whether ECS was present in the query
	Found bool

	// Option is the ECS option exactly as received, for echoing in the
	// response (see ECSResponseOption). Nil when Found is false.
	Option *dns.EDNS0_SUBNET
}

// Usable reports whether the option carries a client subnet that may be used
// to tailor the response. A SOURCE PREFIX-LENGTH of 0 means the sender does
// not want any client address used (RFC 7871 section 7.1.2); the server then
// falls back to the address the query came from.
func (r *ECSResult) Usable() bool {
	return r != nil && r.Found && r.IP != nil && r.SourceNetmask > 0
}

// ParseECS extracts the EDNS Client Subnet option from a DNS message.
// Returns the client IP if ECS is present, or nil if not.
//
// EDNS Client Subnet (ECS) is defined in RFC 7871. It allows recursive
// resolvers to send the client's subnet to authoritative servers for
// geo-aware responses.
//
// Example: When a user in Tokyo queries Google DNS (8.8.8.8), Google
// may include an ECS option with the user's subnet (e.g., 203.0.113.0/24)
// so the authoritative server can return the Tokyo datacenter IP.
func ParseECS(msg *dns.Msg) *ECSResult {
	if msg == nil {
		return &ECSResult{Found: false}
	}

	// Look for OPT record in Additional section
	for _, rr := range msg.Extra {
		opt, ok := rr.(*dns.OPT)
		if !ok {
			continue
		}

		// Search for EDNS0_SUBNET option
		for _, o := range opt.Option {
			subnet, ok := o.(*dns.EDNS0_SUBNET)
			if !ok {
				continue
			}

			// Found ECS option
			return &ECSResult{
				IP:            subnet.Address,
				SourceNetmask: subnet.SourceNetmask,
				Found:         true,
				Option:        subnet,
			}
		}
	}

	return &ECSResult{Found: false}
}

// GetClientIP extracts the best available client IP from a DNS query.
// Priority:
//  1. EDNS Client Subnet IP (if enabled, present and with a source prefix
//     length greater than 0)
//  2. Source IP from the request
//
// Parameters:
// - msg: the DNS query message
// - sourceAddr: the source address of the DNS query (e.g., from ResponseWriter.RemoteAddr())
// - ecsEnabled: whether to use ECS information
func GetClientIP(msg *dns.Msg, sourceAddr net.Addr, ecsEnabled bool) net.IP {
	// Try ECS first if enabled
	if ecsEnabled {
		if ecs := ParseECS(msg); ecs.Usable() {
			return ecs.IP
		}
	}

	// Fall back to source address
	if sourceAddr == nil {
		return nil
	}

	switch addr := sourceAddr.(type) {
	case *net.UDPAddr:
		return addr.IP
	case *net.TCPAddr:
		return addr.IP
	default:
		// Try to parse as host:port string
		host, _, err := net.SplitHostPort(sourceAddr.String())
		if err != nil {
			return nil
		}
		return net.ParseIP(host)
	}
}

// ECSResponseOption returns the ECS option for a response to a query that
// carried the given option. FAMILY, SOURCE PREFIX-LENGTH and ADDRESS are
// copied from the query, as RFC 7871 section 7.2.1 requires, and SCOPE
// PREFIX-LENGTH is set to scope: the prefix length the answer is valid for,
// 0 when it is valid for every client.
func ECSResponseOption(query *dns.EDNS0_SUBNET, scope uint8) *dns.EDNS0_SUBNET {
	if query == nil {
		return nil
	}
	return &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        query.Family,
		SourceNetmask: query.SourceNetmask,
		SourceScope:   scope,
		Address:       query.Address,
	}
}

// AddECSResponse adds an EDNS Client Subnet response option to a DNS message.
// This echoes back the client's subnet with scope information.
//
// Parameters:
// - msg: the DNS response message
// - clientIP: the client's IP address
// - sourceNetmask: the source prefix length from the query
// - scopeNetmask: the scope prefix length (how specific our answer is)
func AddECSResponse(msg *dns.Msg, clientIP net.IP, sourceNetmask, scopeNetmask uint8) {
	if msg == nil || clientIP == nil {
		return
	}

	// Determine address family
	family := uint16(1) // IPv4
	if clientIP.To4() == nil {
		family = 2 // IPv6
	}

	// Create ECS option
	ecs := &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        family,
		SourceNetmask: sourceNetmask,
		SourceScope:   scopeNetmask,
		Address:       clientIP,
	}

	// Find or create OPT record
	var opt *dns.OPT
	for _, rr := range msg.Extra {
		if o, ok := rr.(*dns.OPT); ok {
			opt = o
			break
		}
	}

	if opt == nil {
		opt = &dns.OPT{
			Hdr: dns.RR_Header{
				Name:   ".",
				Rrtype: dns.TypeOPT,
			},
		}
		msg.Extra = append(msg.Extra, opt)
	}

	// Add ECS option
	opt.Option = append(opt.Option, ecs)
}
