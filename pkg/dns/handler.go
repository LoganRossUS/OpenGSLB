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

	"github.com/loganrossus/OpenGSLB/pkg/geo"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
	"github.com/miekg/dns"
)

// Handler processes DNS queries.
type Handler struct {
	mu            sync.RWMutex
	registry      *Registry
	zones         *ZoneTable
	health        HealthProvider
	dnssecSigner  DNSSECSigner
	dnssecEnabled bool
	ecsEnabled    bool
	defaultTTL    uint32
	logger        *slog.Logger
}

// NewHandler creates a new DNS handler.
// ADR-015: LeaderChecker is ignored - all Overwatch nodes serve DNS independently.
func NewHandler(cfg HandlerConfig) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	zones := cfg.Zones
	if zones == nil {
		zones = &ZoneTable{}
	}

	return &Handler{
		registry:      cfg.Registry,
		zones:         zones,
		health:        cfg.HealthProvider,
		dnssecSigner:  cfg.DNSSECSigner,
		dnssecEnabled: cfg.DNSSECEnabled,
		ecsEnabled:    cfg.ECSEnabled,
		defaultTTL:    cfg.DefaultTTL,
		logger:        logger,
	}
}

// handlerState is the part of the handler configuration that can change at
// runtime (reload, DNSSEC signer wiring). ServeDNS takes one consistent copy
// per query so that no lock is held while a response is routed and written.
type handlerState struct {
	registry      *Registry
	zones         *ZoneTable
	health        HealthProvider
	dnssecSigner  DNSSECSigner
	dnssecEnabled bool
}

// state returns a consistent copy of the mutable handler configuration.
func (h *Handler) state() handlerState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return handlerState{
		registry:      h.registry,
		zones:         h.zones,
		health:        h.health,
		dnssecSigner:  h.dnssecSigner,
		dnssecEnabled: h.dnssecEnabled,
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
	// q.Name keeps the querier's spelling and is echoed in the question and
	// answer sections (0x20 case randomization depends on that). qname is the
	// canonical lower-case form used for lookups, metrics and logs.
	qname := normalizeDomain(q.Name)
	qtype := dns.TypeToString[q.Qtype]

	// Get client IP for geolocation routing (ECS or source address)
	clientIP := geo.GetClientIP(r, w.RemoteAddr(), h.ecsEnabled)

	h.logger.Debug("DNS query received",
		"name", qname,
		"type", qtype,
		"source", w.RemoteAddr().String(),
		"clientIP", clientIP,
	)

	st := h.state()
	zone, entry := st.resolve(qname)

	switch {
	case q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA:
		h.handleAddressQuery(st, m, q, qname, entry, clientIP)
	case q.Qtype == dns.TypeDNSKEY:
		h.handleDNSKEYQuery(m, qname, entry)
	case (q.Qtype == dns.TypeSOA || q.Qtype == dns.TypeNS) && zone != nil && qname == zone.Name:
		h.handleApexQuery(m, q, zone)
	default:
		h.logger.Debug("unsupported query type", "name", qname, "type", qtype)
		m.SetRcode(m, dns.RcodeNotImplemented)
	}

	// Sign the response if DNSSEC is enabled
	signed := h.signResponse(st, m)

	status := dns.RcodeToString[signed.Rcode]
	h.writeResponse(w, signed, start, status, qname)
}

// resolve finds the zone that is authoritative for qname and the GSLB domain
// registered at qname. Either can be nil. qname must be canonical.
//
// The zone is the configured zone (dns.zones) with the longest matching
// suffix. A name outside every configured zone belongs to the implicit zone
// of the closest registered GSLB domain at or above it, so that domains keep
// resolving when dns.zones does not list their zone or is empty.
func (st handlerState) resolve(qname string) (*Zone, *DomainEntry) {
	entry := st.registry.Lookup(qname)

	zone := st.zones.Find(qname)
	if zone == nil {
		if apex := st.registry.closestDomain(qname); apex != "" {
			zone = st.zones.Implicit(apex)
		}
	}
	return zone, entry
}

// handleApexQuery answers SOA and NS queries at a zone apex.
func (h *Handler) handleApexQuery(m *dns.Msg, q dns.Question, zone *Zone) {
	switch q.Qtype {
	case dns.TypeSOA:
		m.Answer = append(m.Answer, zone.soa(q.Name, zone.TTL))
	case dns.TypeNS:
		m.Answer = append(m.Answer, zone.ns(q.Name)...)
	}
	h.logger.Debug("resolved apex query", "zone", zone.Name, "type", dns.TypeToString[q.Qtype])
}

// handleAddressQuery processes A (IPv4) and AAAA (IPv6) queries.
func (h *Handler) handleAddressQuery(st handlerState, m *dns.Msg, q dns.Question, qname string, entry *DomainEntry, clientIP net.IP) {
	if entry == nil {
		h.logger.Debug("domain not found", "name", qname)
		m.SetRcode(m, dns.RcodeNameError) // NXDOMAIN
		return
	}

	wantIPv4 := q.Qtype == dns.TypeA
	servers := healthyServers(st.health, entry, wantIPv4)
	if len(servers) == 0 {
		h.logger.Debug("no healthy servers", "domain", qname, "type", dns.TypeToString[q.Qtype])
		m.SetRcode(m, dns.RcodeServerFailure)
		return
	}

	// Create context with client IP for geolocation routing
	// and domain name for learned latency routing (ADR-017)
	ctx := context.Background()
	if clientIP != nil {
		ctx = routing.WithClientIP(ctx, clientIP)
	}
	// Use entry.Name which is normalized, strip trailing dot for service name format
	domainName := entry.Name
	if len(domainName) > 0 && domainName[len(domainName)-1] == '.' {
		domainName = domainName[:len(domainName)-1]
	}
	ctx = routing.WithDomain(ctx, domainName)

	pool := routing.NewSimpleServerPool(servers)
	selected, err := entry.Router.Route(ctx, pool)
	if err != nil {
		h.logger.Error("routing failed", "domain", qname, "error", err)
		m.SetRcode(m, dns.RcodeServerFailure)
		return
	}

	if wantIPv4 {
		h.addARecord(m, q, selected, entry.TTL)
	} else {
		h.addAAAARecord(m, q, selected, entry.TTL)
	}
	metrics.RecordRoutingDecision(qname, entry.Router.Algorithm(), selected.Address)

	h.logger.Debug("resolved address query",
		"domain", qname,
		"type", dns.TypeToString[q.Qtype],
		"selected", selected.Address,
		"algorithm", entry.Router.Algorithm(),
	)
}

// healthyServers returns the servers of the entry that belong to the wanted
// address family (IPv4 when wantIPv4 is true, IPv6 otherwise) and are healthy
// according to the provider. A nil provider treats every server as healthy.
func healthyServers(health HealthProvider, entry *DomainEntry, wantIPv4 bool) []*routing.Server {
	var servers []*routing.Server

	for _, server := range entry.Servers {
		if (server.Address.To4() != nil) != wantIPv4 {
			continue
		}

		if health != nil && !health.IsHealthy(server.Address.String(), server.Port) {
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

// getHealthyIPv4Servers returns healthy IPv4 servers from the entry.
func (h *Handler) getHealthyIPv4Servers(entry *DomainEntry) []*routing.Server {
	return healthyServers(h.state().health, entry, true)
}

// getHealthyIPv6Servers returns healthy IPv6 servers from the entry.
func (h *Handler) getHealthyIPv6Servers(entry *DomainEntry) []*routing.Server {
	return healthyServers(h.state().health, entry, false)
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

// SetZones replaces the handler's zone table (configuration reload).
func (h *Handler) SetZones(zones *ZoneTable) {
	if zones == nil {
		zones = &ZoneTable{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.zones = zones
	h.logger.Info("DNS handler zones updated", "zones", len(zones.zones))
}

// Zones returns the configured zones with every SOA and NS default resolved.
func (h *Handler) Zones() []Zone {
	return h.state().zones.Zones()
}

// handleDNSKEYQuery processes DNSKEY record queries.
func (h *Handler) handleDNSKEYQuery(m *dns.Msg, qname string, entry *DomainEntry) {
	// Check if the domain exists in our registry
	if entry == nil {
		h.logger.Debug("domain not found for DNSKEY query", "name", qname)
		m.SetRcode(m, dns.RcodeNameError) // NXDOMAIN
		return
	}

	// DNSKEY records are added by the signer during signResponse
	// Just mark the query as successful - the signer will add the key
	h.logger.Debug("resolved DNSKEY query", "domain", qname)
}

// signResponse signs the DNS response if DNSSEC is enabled.
// Returns the original message if DNSSEC is disabled or signing fails.
func (h *Handler) signResponse(st handlerState, m *dns.Msg) *dns.Msg {
	if !st.dnssecEnabled || st.dnssecSigner == nil {
		return m
	}

	// Call the signer - it handles all DNSSEC signing
	signed, err := st.dnssecSigner.SignResponse(m)
	if err != nil {
		h.logger.Warn("failed to sign DNS response",
			"error", err,
		)
		return m
	}

	// Type assert back to *dns.Msg
	if signedMsg, ok := signed.(*dns.Msg); ok {
		return signedMsg
	}

	return m
}

// SetDNSSECSigner sets the DNSSEC signer for signing responses.
func (h *Handler) SetDNSSECSigner(signer DNSSECSigner, enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dnssecSigner = signer
	h.dnssecEnabled = enabled
	h.logger.Info("DNSSEC signer configured", "enabled", enabled)
}
