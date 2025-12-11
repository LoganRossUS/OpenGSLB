// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// Handler processes DNS queries.
type Handler struct {
	mu         sync.RWMutex
	registry   *Registry
	health     HealthProvider
	defaultTTL uint32
	logger     *slog.Logger
}

// NewHandler creates a new DNS handler.
// ADR-015: LeaderChecker is ignored - all Overwatch nodes serve DNS independently.
func NewHandler(cfg HandlerConfig) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Handler{
		registry:   cfg.Registry,
		health:     cfg.HealthProvider,
		defaultTTL: cfg.DefaultTTL,
		logger:     logger,
	}
}

// ServeDNS implements the dns.Handler interface.
// ADR-015: All Overwatch nodes serve DNS independently (no leader check).
func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	start := time.Now()

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	if len(r.Question) == 0 {
		h.logger.Warn("DNS query with no questions")
		m.SetRcode(m, dns.RcodeFormatError)
		h.writeResponse(w, m, start, "FORMERR", "")
		return
	}

	q := r.Question[0]
	qname := q.Name
	qtype := dns.TypeToString[q.Qtype]

	h.logger.Debug("DNS query received",
		"name", qname,
		"type", qtype,
		"source", w.RemoteAddr().String(),
	)

	switch q.Qtype {
	case dns.TypeA:
		h.handleAQuery(m, qname, q)
	case dns.TypeAAAA:
		h.handleAAAAQuery(m, qname, q)
	default:
		h.logger.Debug("unsupported query type", "name", qname, "type", qtype)
		m.SetRcode(m, dns.RcodeNotImplemented)
	}

	status := dns.RcodeToString[m.Rcode]
	h.writeResponse(w, m, start, status, qname)
}

// handleAQuery processes A record queries (IPv4).
func (h *Handler) handleAQuery(m *dns.Msg, qname string, q dns.Question) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entry := h.registry.Lookup(qname)
	if entry == nil {
		h.logger.Debug("domain not found", "name", qname)
		m.SetRcode(m, dns.RcodeNameError) // NXDOMAIN
		return
	}

	servers := h.getHealthyIPv4Servers(entry)
	if len(servers) == 0 {
		h.logger.Debug("no healthy IPv4 servers", "domain", qname)
		m.SetRcode(m, dns.RcodeServerFailure)
		return
	}

	pool := routing.NewSimpleServerPool(servers)
	selected, err := entry.Router.Route(context.Background(), pool)
	if err != nil {
		h.logger.Error("routing failed", "domain", qname, "error", err)
		m.SetRcode(m, dns.RcodeServerFailure)
		return
	}

	h.addARecord(m, q, selected, entry.TTL)
	metrics.RecordRoutingDecision(qname, entry.Router.Algorithm(), selected.Address)

	h.logger.Debug("resolved A query",
		"domain", qname,
		"selected", selected.Address,
		"algorithm", entry.Router.Algorithm(),
	)
}

// handleAAAAQuery processes AAAA record queries (IPv6).
func (h *Handler) handleAAAAQuery(m *dns.Msg, qname string, q dns.Question) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entry := h.registry.Lookup(qname)
	if entry == nil {
		h.logger.Debug("domain not found", "name", qname)
		m.SetRcode(m, dns.RcodeNameError) // NXDOMAIN
		return
	}

	servers := h.getHealthyIPv6Servers(entry)
	if len(servers) == 0 {
		h.logger.Debug("no healthy IPv6 servers", "domain", qname)
		m.SetRcode(m, dns.RcodeServerFailure)
		return
	}

	pool := routing.NewSimpleServerPool(servers)
	selected, err := entry.Router.Route(context.Background(), pool)
	if err != nil {
		h.logger.Error("routing failed", "domain", qname, "error", err)
		m.SetRcode(m, dns.RcodeServerFailure)
		return
	}

	h.addAAAARecord(m, q, selected, entry.TTL)
	metrics.RecordRoutingDecision(qname, entry.Router.Algorithm(), selected.Address)

	h.logger.Debug("resolved AAAA query",
		"domain", qname,
		"selected", selected.Address,
		"algorithm", entry.Router.Algorithm(),
	)
}

// getHealthyIPv4Servers returns healthy IPv4 servers from the entry.
func (h *Handler) getHealthyIPv4Servers(entry *DomainEntry) []*routing.Server {
	var servers []*routing.Server

	for _, server := range entry.Servers {
		// Check if IPv4
		if server.Address.To4() == nil {
			continue
		}

		// Check health
		if h.health != nil && !h.health.IsHealthy(server.Address.String(), server.Port) {
			continue
		}

		servers = append(servers, &routing.Server{
			Address: server.Address.String(),
			Port:    server.Port,
			Weight:  server.Weight,
			Region:  server.Region,
		})
	}

	return servers
}

// getHealthyIPv6Servers returns healthy IPv6 servers from the entry.
func (h *Handler) getHealthyIPv6Servers(entry *DomainEntry) []*routing.Server {
	var servers []*routing.Server

	for _, server := range entry.Servers {
		// Check if IPv6 (not IPv4)
		if server.Address.To4() != nil {
			continue
		}

		// Check health
		if h.health != nil && !h.health.IsHealthy(server.Address.String(), server.Port) {
			continue
		}

		servers = append(servers, &routing.Server{
			Address: server.Address.String(),
			Port:    server.Port,
			Weight:  server.Weight,
			Region:  server.Region,
		})
	}

	return servers
}

// addARecord adds an A record to the response.
func (h *Handler) addARecord(m *dns.Msg, q dns.Question, server *routing.Server, ttl uint32) {
	if ttl == 0 {
		ttl = h.defaultTTL
	}

	ip := net.ParseIP(server.Address)
	if ip == nil {
		h.logger.Error("invalid IP address", "address", server.Address)
		return
	}

	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		A: ip.To4(),
	}

	m.Answer = append(m.Answer, rr)
}

// addAAAARecord adds an AAAA record to the response.
func (h *Handler) addAAAARecord(m *dns.Msg, q dns.Question, server *routing.Server, ttl uint32) {
	if ttl == 0 {
		ttl = h.defaultTTL
	}

	ip := net.ParseIP(server.Address)
	if ip == nil {
		h.logger.Error("invalid IP address", "address", server.Address)
		return
	}

	rr := &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		AAAA: ip,
	}

	m.Answer = append(m.Answer, rr)
}

// writeResponse writes the DNS response and records metrics.
func (h *Handler) writeResponse(w dns.ResponseWriter, m *dns.Msg, start time.Time, status string, domain string) {
	duration := time.Since(start)

	if err := w.WriteMsg(m); err != nil {
		h.logger.Error("failed to write DNS response",
			"error", err,
			"domain", domain,
		)
	}

	qtype := ""
	if len(m.Question) > 0 {
		qtype = dns.TypeToString[m.Question[0].Qtype]
	}

	metrics.RecordDNSQuery(domain, qtype, status)
	metrics.RecordDNSQueryDuration(domain, status, duration.Seconds())

	h.logger.Debug("DNS response sent",
		"domain", domain,
		"status", status,
		"duration", duration,
	)
}

// UpdateRegistry updates the handler's registry.
func (h *Handler) UpdateRegistry(registry *Registry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.registry = registry
	h.logger.Info("DNS handler registry updated")
}
