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
	"strconv"
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
	maxUDPSize    uint16
	defaultTTL    uint32
	logger        *slog.Logger

	returnLastHealthy bool
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
		maxUDPSize:    clampUDPSize(cfg.MaxUDPSize),
		defaultTTL:    cfg.DefaultTTL,
		logger:        logger,

		returnLastHealthy: cfg.ReturnLastHealthy,
	}
}

// clampUDPSize maps a configured maximum UDP response size to the value the
// handler uses: 0 selects the default, anything else is limited to the range
// 512 to 65535.
func clampUDPSize(size int) uint16 {
	switch {
	case size == 0:
		return DefaultMaxUDPSize
	case size < minUDPSize:
		return minUDPSize
	case size > dns.MaxMsgSize:
		return dns.MaxMsgSize
	default:
		return uint16(size)
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
	maxUDPSize    uint16

	returnLastHealthy bool
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
		maxUDPSize:    h.maxUDPSize,

		returnLastHealthy: h.returnLastHealthy,
	}
}

// ServeDNS implements the dns.Handler interface.
// ADR-015: All Overwatch nodes serve DNS independently (no leader check).
func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	start := time.Now()
	st := h.state()

	m := new(dns.Msg)
	m.SetReply(r)
	// The AA bit is set further down, only once the query name is known to
	// be inside a zone this server is authoritative for.

	qname := ""
	// ecsScope is the EDNS Client Subnet SCOPE PREFIX-LENGTH of the response.
	// It stays 0 ("valid for every client") unless the client subnet decided
	// the answer.
	var ecsScope uint8

	queryOPT := r.IsEdns0()
	switch {
	case len(r.Question) == 0:
		h.logger.Warn("DNS query with no questions")
		setError(m, dns.RcodeFormatError)
	case queryOPT != nil && queryOPT.Version() != 0:
		// RFC 6891 section 6.1.3: an EDNS version the server does not
		// implement is answered BADVERS. The OPT record added below states
		// the version it does implement (0).
		h.logger.Debug("unsupported EDNS version", "version", queryOPT.Version())
		setError(m, dns.RcodeBadVers)
	default:
		q := r.Question[0]
		// q.Name keeps the querier's spelling and is echoed in the question
		// and answer sections (0x20 case randomization depends on that).
		// qname is the canonical lower-case form used for lookups, metrics
		// and logs.
		qname = normalizeDomain(q.Name)

		// Client address for geolocation and latency routing: the EDNS
		// Client Subnet when enabled and usable, else the source address.
		clientIP, ecs := h.clientAddress(r, w.RemoteAddr())

		h.logger.Debug("DNS query received",
			"name", qname,
			"type", dns.TypeToString[q.Qtype],
			"source", w.RemoteAddr(),
			"clientIP", clientIP,
		)

		switch {
		case r.Opcode != dns.OpcodeQuery:
			// NOTIFY, UPDATE and the other opcodes are not implemented.
			h.logger.Debug("unsupported opcode", "name", qname, "opcode", dns.OpcodeToString[r.Opcode])
			setError(m, dns.RcodeNotImplemented)
		case q.Qclass != dns.ClassINET:
			// Only class IN is served. A CH or ANY class query must not be
			// answered from IN data.
			h.logger.Debug("query class refused", "name", qname, "class", dns.ClassToString[q.Qclass])
			setError(m, dns.RcodeRefused)
		default:
			if usedClientIP := h.answerQuery(st, m, q, qname, clientIP); usedClientIP && ecs.Usable() {
				// The answer was chosen for this client subnet and is valid
				// for exactly the prefix the resolver sent.
				ecsScope = ecs.SourceNetmask
			}
		}
	}

	// RFC 6891 section 6.1.1: the response has an OPT record if and only if
	// the query had one. It goes in before signing so that a signer can see
	// the DO bit.
	if queryOPT != nil {
		m.Extra = append(m.Extra, h.responseOPT(st, r, queryOPT, ecsScope))
	}

	// Sign the response if DNSSEC is enabled
	signed := h.signResponse(st, m)

	// Fit the response to the transport. Over UDP it must not exceed what
	// the requester can receive, capped at the configured maximum; whatever
	// does not fit is dropped and the TC bit tells the client to retry over
	// TCP. Over TCP the only limit is the 64 KiB of the protocol itself.
	// Truncate keeps the OPT record (RFC 6891 section 7).
	signed.Truncate(responseSizeLimit(w.RemoteAddr(), queryOPT, st.maxUDPSize))

	h.writeResponse(w, signed, start, rcodeString(signed), st.metricDomain(qname))
}

// metricDomain returns the value of the "domain" metric label for a query
// name. The name is chosen by the client, so only names this server has data
// for become label values: a registered domain and a zone apex are their own
// label, every other name is counted under "other". Without this, every
// distinct name a client asks for would add a time series that is never
// removed.
func (st handlerState) metricDomain(qname string) string {
	if qname == "" {
		return ""
	}
	zone, entry := st.resolve(qname)
	if entry != nil || (zone != nil && qname == zone.Name) {
		return qname
	}
	return "other"
}

// clientAddress returns the address that routing decisions are made for and
// the EDNS Client Subnet of the query. The ECS result is non-nil only when
// ECS support is enabled.
func (h *Handler) clientAddress(r *dns.Msg, source net.Addr) (net.IP, *geo.ECSResult) {
	if !h.ecsEnabled {
		return geo.GetClientIP(r, source, false), nil
	}
	ecs := geo.ParseECS(r)
	if ecs.Usable() {
		return ecs.IP, ecs
	}
	return geo.GetClientIP(r, source, false), ecs
}

// responseOPT builds the OPT record of a response to a query with EDNS.
func (h *Handler) responseOPT(st handlerState, r *dns.Msg, queryOPT *dns.OPT, ecsScope uint8) *dns.OPT {
	opt := &dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT}}
	// The size this server is willing to receive and send over UDP.
	opt.SetUDPSize(st.maxUDPSize)
	// RFC 3225 section 3: the DO bit of the query is copied to the response.
	if queryOPT.Do() {
		opt.SetDo()
	}
	// RFC 7871 section 7.2.1: a server with ECS enabled that receives an ECS
	// option includes one in its response, with FAMILY, SOURCE PREFIX-LENGTH
	// and ADDRESS as in the query. A server with ECS disabled must not.
	if h.ecsEnabled {
		if ecs := geo.ParseECS(r); ecs.Found {
			opt.Option = append(opt.Option, geo.ECSResponseOption(ecs.Option, ecsScope))
		}
	}
	return opt
}

// responseSizeLimit returns the largest response, in bytes, that may be sent
// to the requester.
func responseSizeLimit(remote net.Addr, queryOPT *dns.OPT, maxUDPSize uint16) int {
	if remote != nil && remote.Network() == "tcp" {
		return dns.MaxMsgSize
	}
	// Without EDNS a requester accepts 512 bytes (RFC 1035 section 4.2.1).
	// With EDNS it states its size; values below 512 count as 512 (RFC 6891
	// section 6.2.5).
	limit := minUDPSize
	if queryOPT != nil && int(queryOPT.UDPSize()) > limit {
		limit = int(queryOPT.UDPSize())
	}
	if limit > int(maxUDPSize) {
		limit = int(maxUDPSize)
	}
	return limit
}

// rcodeString returns the name of the response code for metrics and logs.
func rcodeString(m *dns.Msg) string {
	// Rcode 16 is BADVERS in a response with an OPT record (RFC 6891) and
	// BADSIG in a TSIG record. This server only ever produces the former.
	if m.Rcode == dns.RcodeBadVers {
		return "BADVERS"
	}
	if s, ok := dns.RcodeToString[m.Rcode]; ok {
		return s
	}
	return "RCODE" + strconv.Itoa(m.Rcode)
}

// answerQuery fills in the response to a class IN query. It reports whether
// the client address decided the answer (see routing.ClientAware).
func (h *Handler) answerQuery(st handlerState, m *dns.Msg, q dns.Question, qname string, clientIP net.IP) (usedClientIP bool) {
	qtype := dns.TypeToString[q.Qtype]

	zone, entry := st.resolve(qname)
	if zone == nil {
		// The name is outside every zone this server is authoritative for.
		// An authoritative-only server refuses such a query. Answering
		// NXDOMAIN would be a false statement about someone else's name.
		h.logger.Debug("query outside served zones refused", "name", qname, "type", qtype)
		setError(m, dns.RcodeRefused)
		return false
	}

	if q.Qtype == dns.TypeAXFR || q.Qtype == dns.TypeIXFR {
		// Zone transfers are not implemented. Refuse them: an empty NOERROR
		// could be mistaken by a secondary for an empty zone.
		h.logger.Debug("zone transfer refused", "name", qname, "type", qtype)
		setError(m, dns.RcodeRefused)
		return false
	}

	// Everything below is a statement about data in one of our zones.
	m.Authoritative = true

	// The apex of a zone exists even when no GSLB domain is registered at it.
	apex := qname == zone.Name

	switch {
	case entry == nil && !apex && !st.hasNameBelow(qname):
		// Nothing exists at or below the name. The zone SOA in the authority
		// section lets resolvers cache the NXDOMAIN (RFC 2308 section 2.1).
		h.logger.Debug("domain not found", "name", qname)
		m.Rcode = dns.RcodeNameError // NXDOMAIN
		m.Ns = []dns.RR{zone.negativeSOA()}
	case (q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA) && entry != nil:
		return h.handleAddressQuery(st, m, q, qname, zone, entry, clientIP)
	case (q.Qtype == dns.TypeSOA || q.Qtype == dns.TypeNS) && apex:
		h.handleApexQuery(m, q, zone)
	case q.Qtype == dns.TypeDNSKEY && st.dnssecEnabled && st.dnssecSigner != nil:
		// DNSKEY records are added by the signer during signResponse.
		// Leave the answer empty and successful for it to fill in.
		h.logger.Debug("resolved DNSKEY query", "domain", qname)
	default:
		// The name exists but the server has no data of this type. That is
		// not an error: NOERROR with an empty answer (RFC 2308 "NODATA").
		// This covers every type OpenGSLB does not serve (TXT, MX, CNAME,
		// HTTPS, ANY, DS, ...), SOA and NS below an apex, address queries at
		// an apex that is not itself a GSLB domain, and every query for an
		// empty non-terminal (a name with a GSLB domain beneath it).
		h.logger.Debug("no data for query type", "name", qname, "type", qtype)
		setNoData(m, zone)
	}
	return false
}

// setError turns m into an error response that makes no statement about zone
// data: the given rcode, the AA bit clear and no records.
func setError(m *dns.Msg, rcode int) {
	m.Rcode = rcode
	m.Authoritative = false
	m.Answer = nil
	m.Ns = nil
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

// hasNameBelow reports whether a GSLB domain or a configured zone apex lies
// beneath qname. Such a name is an empty non-terminal: it exists, so the
// answer for it is an empty NOERROR and never NXDOMAIN. NXDOMAIN states that
// nothing exists at or below the name (RFC 8020), and a resolver that caches
// it may stop resolving the names beneath it.
func (st handlerState) hasNameBelow(qname string) bool {
	return st.registry.hasDescendant(qname) || st.zones.hasApexBelow(qname)
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
//
// Three outcomes are possible for a registered name:
//   - no server of the queried address family is configured: the name exists
//     but has no data of this type, which is NOERROR with an empty answer;
//   - servers of the family are configured and at least one is usable: one of
//     them is answered;
//   - servers of the family are configured and none is usable: SERVFAIL, or
//     with dns.return_last_healthy an answer chosen among all of them.
//
// It reports whether the client address decided the answer.
func (h *Handler) handleAddressQuery(st handlerState, m *dns.Msg, q dns.Question, qname string, zone *Zone, entry *DomainEntry, clientIP net.IP) (usedClientIP bool) {
	wantIPv4 := q.Qtype == dns.TypeA
	if !hasServersOfFamily(entry, wantIPv4) {
		h.logger.Debug("no servers of the queried address family", "domain", qname, "type", dns.TypeToString[q.Qtype])
		setNoData(m, zone)
		return false
	}

	servers := healthyServers(st.health, entry, wantIPv4)
	lastHealthy := false
	if len(servers) == 0 {
		if !st.returnLastHealthy {
			h.logger.Debug("no healthy servers", "domain", qname, "type", dns.TypeToString[q.Qtype])
			setError(m, dns.RcodeServerFailure)
			return false
		}
		// dns.return_last_healthy: every server of this family is unhealthy.
		// Answer as if all of them were healthy, so the routing algorithm
		// picks among the configured servers. A possibly dead address is
		// preferred over no address at all.
		servers = healthyServers(nil, entry, wantIPv4)
		lastHealthy = true
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
		setError(m, dns.RcodeServerFailure)
		return false
	}

	if wantIPv4 {
		h.addARecord(m, q, selected, entry.TTL)
	} else {
		h.addAAAARecord(m, q, selected, entry.TTL)
	}
	metrics.RecordRoutingDecision(qname, entry.Router.Algorithm(), selected.Address)
	if lastHealthy {
		metrics.RecordLastHealthyAnswer(qname, dns.TypeToString[q.Qtype])
	}

	h.logger.Debug("resolved address query",
		"domain", qname,
		"type", dns.TypeToString[q.Qtype],
		"selected", selected.Address,
		"algorithm", entry.Router.Algorithm(),
		"last_healthy", lastHealthy,
	)

	aware, ok := entry.Router.(routing.ClientAware)
	return ok && aware.UsesClientIP() && clientIP != nil
}

// setNoData turns m into an empty NOERROR response: the name exists but has
// no data of the queried type. The zone SOA in the authority section lets
// resolvers cache the answer (RFC 2308 section 2.2).
func setNoData(m *dns.Msg, zone *Zone) {
	m.Rcode = dns.RcodeSuccess
	m.Answer = nil
	m.Ns = []dns.RR{zone.negativeSOA()}
}

// hasServersOfFamily reports whether the entry has at least one configured
// server of the wanted address family, healthy or not.
func hasServersOfFamily(entry *DomainEntry, wantIPv4 bool) bool {
	for _, server := range entry.Servers {
		if (server.Address.To4() != nil) == wantIPv4 {
			return true
		}
	}
	return false
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

// SetMaxUDPSize sets the cap on UDP responses, which is also the UDP payload
// size advertised in EDNS responses. 0 selects DefaultMaxUDPSize.
func (h *Handler) SetMaxUDPSize(size int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.maxUDPSize = clampUDPSize(size)
}

// SetReturnLastHealthy sets dns.return_last_healthy (see HandlerConfig).
func (h *Handler) SetReturnLastHealthy(enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.returnLastHealthy = enabled
}

// Zones returns the configured zones with every SOA and NS default resolved.
func (h *Handler) Zones() []Zone {
	return h.state().zones.Zones()
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
