// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    \u2192 https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"context"
	"log/slog"
	"time"

	"github.com/miekg/dns"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
)

// Router selects a server from a pool of servers.
type Router interface {
	Route(ctx context.Context, domain string, servers []ServerInfo) (*ServerInfo, error)
	Algorithm() string
}

// HealthStatusProvider provides health status for servers.
type HealthStatusProvider interface {
	IsHealthy(address string, port int) bool
}

// LeaderChecker determines if this node should serve DNS queries.
// In standalone mode, always returns true.
// In cluster mode, returns true only if this node is the Raft leader.
type LeaderChecker interface {
	IsLeader() bool
}

// standaloneLeaderChecker always returns true (for standalone mode).
type standaloneLeaderChecker struct{}

func (s *standaloneLeaderChecker) IsLeader() bool { return true }

// Handler processes DNS queries.
type Handler struct {
	registry       *Registry
	healthProvider HealthStatusProvider
	leaderChecker  LeaderChecker
	defaultTTL     uint32
	logger         *slog.Logger
	ednsEnabled    bool
	ednsUDPSize    uint16
}

// HandlerConfig contains configuration for the DNS handler.
type HandlerConfig struct {
	Registry       *Registry
	HealthProvider HealthStatusProvider
	LeaderChecker  LeaderChecker // If nil, defaults to standalone (always leader)
	DefaultTTL     uint32
	Logger         *slog.Logger
	EDNSEnabled    *bool  // Pointer to distinguish unset from false; defaults to true
	EDNSUDPSize    uint16 // Advertised UDP buffer size; defaults to 4096
}

// Default EDNS configuration values.
const (
	DefaultEDNSUDPSize = 4096 // RFC 6891 recommends 4096 for EDNS
)

// NewHandler creates a new DNS handler.
func NewHandler(cfg HandlerConfig) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Default EDNS to enabled
	ednsEnabled := true
	if cfg.EDNSEnabled != nil {
		ednsEnabled = *cfg.EDNSEnabled
	}

	// Default UDP size
	ednsUDPSize := cfg.EDNSUDPSize
	if ednsUDPSize == 0 {
		ednsUDPSize = DefaultEDNSUDPSize
	}

	// Default to standalone mode (always leader)
	leaderChecker := cfg.LeaderChecker
	if leaderChecker == nil {
		leaderChecker = &standaloneLeaderChecker{}
	}

	return &Handler{
		registry:       cfg.Registry,
		healthProvider: cfg.HealthProvider,
		leaderChecker:  leaderChecker,
		defaultTTL:     cfg.DefaultTTL,
		logger:         logger,
		ednsEnabled:    ednsEnabled,
		ednsUDPSize:    ednsUDPSize,
	}
}

// SetLeaderChecker updates the leader checker. Thread-safe for use during
// leader transitions.
func (h *Handler) SetLeaderChecker(checker LeaderChecker) {
	h.leaderChecker = checker
}

// ServeDNS implements the dns.Handler interface.
func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	start := time.Now()

	// In cluster mode, only the leader serves DNS queries.
	// Non-leaders return REFUSED to signal clients to try another server.
	if !h.leaderChecker.IsLeader() {
		h.handleNonLeader(w, r)
		return
	}

	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	// Extract EDNS OPT record from request if present
	clientOPT := h.getEDNS(r)

	if len(r.Question) == 0 {
		h.logger.Warn("received DNS query with no questions")
		msg.Rcode = dns.RcodeFormatError
		h.addEDNS(msg, clientOPT)
		if err := w.WriteMsg(msg); err != nil {
			h.logger.Error("failed to write DNS response", "error", err)
		}
		return
	}

	q := r.Question[0]
	domain := q.Name
	queryType := dns.TypeToString[q.Qtype]

	// Log EDNS info if present
	if clientOPT != nil && h.ednsEnabled {
		h.logger.Debug("received DNS query",
			"name", domain,
			"type", queryType,
			"class", dns.ClassToString[q.Qclass],
			"edns_version", clientOPT.Version(),
			"edns_udp_size", clientOPT.UDPSize(),
		)
	} else {
		h.logger.Debug("received DNS query",
			"name", domain,
			"type", queryType,
			"class", dns.ClassToString[q.Qclass],
		)
	}

	switch q.Qtype {
	case dns.TypeA:
		h.handleA(msg, q)
	case dns.TypeAAAA:
		h.handleAAAA(msg, q)
	default:
		h.logger.Debug("unsupported query type",
			"name", domain,
			"type", queryType,
		)
	}

	// Add EDNS OPT record to response if client sent one
	h.addEDNS(msg, clientOPT)

	status := dns.RcodeToString[msg.Rcode]
	duration := time.Since(start).Seconds()
	metrics.RecordDNSQuery(domain, queryType, status)
	metrics.RecordDNSQueryDuration(domain, status, duration)

	if err := w.WriteMsg(msg); err != nil {
		h.logger.Error("failed to write DNS response",
			"error", err,
			"name", domain,
		)
	}
}

// handleNonLeader responds with REFUSED when this node is not the cluster leader.
// Per RFC 2136, REFUSED indicates the server refuses to perform the operation.
// Clients (especially well-behaved resolvers) will retry with another server.
func (h *Handler) handleNonLeader(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetRcode(r, dns.RcodeRefused)

	// Track refused queries for monitoring
	metrics.RecordDNSRefused()

	// Extract query info for logging
	domain := ""
	queryType := ""
	if len(r.Question) > 0 {
		domain = r.Question[0].Name
		queryType = dns.TypeToString[r.Question[0].Qtype]
	}

	h.logger.Debug("refusing query - not leader",
		"name", domain,
		"type", queryType,
	)

	// Still include EDNS if client sent it
	if clientOPT := h.getEDNS(r); clientOPT != nil {
		h.addEDNS(msg, clientOPT)
	}

	if err := w.WriteMsg(msg); err != nil {
		h.logger.Error("failed to write REFUSED response", "error", err)
	}
}

// handleA processes A record queries (IPv4).
func (h *Handler) handleA(msg *dns.Msg, q dns.Question) {
	entry := h.registry.Lookup(q.Name)
	if entry == nil {
		h.logger.Debug("domain not found", "name", q.Name)
		msg.Rcode = dns.RcodeNameError
		return
	}

	// Filter to healthy IPv4 servers only
	healthyServers := h.filterHealthyServers(entry.Servers)
	ipv4Servers := filterByAddressFamily(healthyServers, true)

	if len(ipv4Servers) == 0 {
		h.logger.Debug("no healthy IPv4 servers available",
			"domain", entry.Name,
			"total_servers", len(entry.Servers),
			"healthy_servers", len(healthyServers),
		)
		return
	}

	// Use the domain's router to select a server
	ctx := context.Background()
	server, err := entry.Router.Route(ctx, entry.Name, ipv4Servers)
	if err != nil {
		h.logger.Error("routing failed",
			"domain", entry.Name,
			"error", err,
		)
		msg.Rcode = dns.RcodeServerFailure
		return
	}

	ttl := entry.TTL
	if ttl == 0 {
		ttl = h.defaultTTL
	}

	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		A: server.Address,
	}

	msg.Answer = append(msg.Answer, rr)

	metrics.RecordRoutingDecision(entry.Name, entry.Router.Algorithm(), server.Address.String())

	h.logger.Debug("routing decision",
		"domain", entry.Name,
		"type", "A",
		"algorithm", entry.Router.Algorithm(),
		"selected_server", server.Address.String(),
		"region", server.Region,
		"ttl", ttl,
	)
}

// handleAAAA processes AAAA record queries (IPv6).
func (h *Handler) handleAAAA(msg *dns.Msg, q dns.Question) {
	entry := h.registry.Lookup(q.Name)
	if entry == nil {
		h.logger.Debug("domain not found", "name", q.Name)
		msg.Rcode = dns.RcodeNameError
		return
	}

	// Filter to healthy IPv6 servers only
	healthyServers := h.filterHealthyServers(entry.Servers)
	ipv6Servers := filterByAddressFamily(healthyServers, false)

	if len(ipv6Servers) == 0 {
		h.logger.Debug("no healthy IPv6 servers available",
			"domain", entry.Name,
			"total_servers", len(entry.Servers),
			"healthy_servers", len(healthyServers),
		)
		return
	}

	// Use the domain's router to select a server
	ctx := context.Background()
	server, err := entry.Router.Route(ctx, entry.Name, ipv6Servers)
	if err != nil {
		h.logger.Error("routing failed",
			"domain", entry.Name,
			"error", err,
		)
		msg.Rcode = dns.RcodeServerFailure
		return
	}

	ttl := entry.TTL
	if ttl == 0 {
		ttl = h.defaultTTL
	}

	rr := &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		AAAA: server.Address,
	}

	msg.Answer = append(msg.Answer, rr)

	metrics.RecordRoutingDecision(entry.Name, entry.Router.Algorithm(), server.Address.String())

	h.logger.Debug("routing decision",
		"domain", entry.Name,
		"type", "AAAA",
		"algorithm", entry.Router.Algorithm(),
		"selected_server", server.Address.String(),
		"region", server.Region,
		"ttl", ttl,
	)
}

// filterHealthyServers returns only servers that are currently healthy.
func (h *Handler) filterHealthyServers(servers []ServerInfo) []ServerInfo {
	if h.healthProvider == nil {
		return servers
	}

	healthy := make([]ServerInfo, 0, len(servers))
	for _, s := range servers {
		if h.healthProvider.IsHealthy(s.Address.String(), s.Port) {
			healthy = append(healthy, s)
		}
	}
	return healthy
}

// filterByAddressFamily filters servers by IP address family.
func filterByAddressFamily(servers []ServerInfo, ipv4 bool) []ServerInfo {
	filtered := make([]ServerInfo, 0, len(servers))
	for _, s := range servers {
		isIPv4 := s.Address.To4() != nil
		if ipv4 && isIPv4 {
			filtered = append(filtered, s)
		} else if !ipv4 && !isIPv4 {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// getEDNS extracts the OPT pseudo-record from the request's additional section.
// Returns nil if no OPT record is present.
func (h *Handler) getEDNS(r *dns.Msg) *dns.OPT {
	if !h.ednsEnabled {
		return nil
	}
	for _, rr := range r.Extra {
		if opt, ok := rr.(*dns.OPT); ok {
			return opt
		}
	}
	return nil
}

// addEDNS adds an OPT pseudo-record to the response if the client sent one.
// Per RFC 6891, servers should include an OPT record in the response
// when the client included one in the request.
func (h *Handler) addEDNS(msg *dns.Msg, clientOPT *dns.OPT) {
	if clientOPT == nil || !h.ednsEnabled {
		return
	}

	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	opt.SetUDPSize(h.ednsUDPSize)
	// Keep EDNS version 0 (the only version defined)
	opt.SetVersion(0)

	msg.Extra = append(msg.Extra, opt)
}
