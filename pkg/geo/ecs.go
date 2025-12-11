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
			}
		}
	}

	return &ECSResult{Found: false}
}

// GetClientIP extracts the best available client IP from a DNS query.
// Priority:
// 1. EDNS Client Subnet IP (if enabled and present)
// 2. Source IP from the request
//
// Parameters:
// - msg: the DNS query message
// - sourceAddr: the source address of the DNS query (e.g., from ResponseWriter.RemoteAddr())
// - ecsEnabled: whether to use ECS information
func GetClientIP(msg *dns.Msg, sourceAddr net.Addr, ecsEnabled bool) net.IP {
	// Try ECS first if enabled
	if ecsEnabled {
		ecs := ParseECS(msg)
		if ecs.Found && ecs.IP != nil {
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
