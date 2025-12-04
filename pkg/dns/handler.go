package dns

import (
	"context"
	"log/slog"

	"github.com/miekg/dns"
)

// Router selects a server from a pool of servers.
// This interface will be implemented by routing algorithms in pkg/routing.
// Note: "Router" refers to DNS response routing (selecting which IP to return),
// not network traffic routing. See ADR-011 for terminology rationale.
type Router interface {
	Route(ctx context.Context, domain string, servers []ServerInfo) (*ServerInfo, error)
	Algorithm() string
}

// HealthStatusProvider provides health status for servers.
// This interface will be implemented by the health manager in pkg/health.
type HealthStatusProvider interface {
	IsHealthy(address string, port int) bool
}

// Handler processes DNS queries.
type Handler struct {
	registry       *Registry
	router         Router
	healthProvider HealthStatusProvider
	defaultTTL     uint32
	logger         *slog.Logger
}

// HandlerConfig contains configuration for the DNS handler.
type HandlerConfig struct {
	Registry       *Registry
	Router         Router
	HealthProvider HealthStatusProvider
	DefaultTTL     uint32
	Logger         *slog.Logger
}

// NewHandler creates a new DNS handler.
func NewHandler(cfg HandlerConfig) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Handler{
		registry:       cfg.Registry,
		router:         cfg.Router,
		healthProvider: cfg.HealthProvider,
		defaultTTL:     cfg.DefaultTTL,
		logger:         logger,
	}
}

// ServeDNS implements the dns.Handler interface.
// This is the main entry point for processing DNS queries.
func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	if len(r.Question) == 0 {
		h.logger.Warn("received DNS query with no questions")
		msg.Rcode = dns.RcodeFormatError
		if err := w.WriteMsg(msg); err != nil {
			h.logger.Error("failed to write DNS response", "error", err)
		}
		return
	}

	q := r.Question[0]
	h.logger.Debug("received DNS query",
		"name", q.Name,
		"type", dns.TypeToString[q.Qtype],
		"class", dns.ClassToString[q.Qclass],
	)

	switch q.Qtype {
	case dns.TypeA:
		h.handleA(msg, q)
	case dns.TypeAAAA:
		h.handleAAAA(msg, q)
	default:
		// We only handle A and AAAA records
		// Return empty response with NOERROR for other types
		h.logger.Debug("unsupported query type",
			"name", q.Name,
			"type", dns.TypeToString[q.Qtype],
		)
	}

	if err := w.WriteMsg(msg); err != nil {
		h.logger.Error("failed to write DNS response",
			"error", err,
			"name", q.Name,
		)
	}
}

// handleA processes A record queries (IPv4).
func (h *Handler) handleA(msg *dns.Msg, q dns.Question) {
	entry := h.registry.Lookup(q.Name)
	if entry == nil {
		h.logger.Debug("domain not found", "name", q.Name)
		msg.Rcode = dns.RcodeNameError // NXDOMAIN
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
		// Return empty answer (NOERROR with no records) - domain exists but no IPv4
		// This is correct behavior per RFC when the domain exists but has no records of requested type
		return
	}

	// Use router to select a server
	ctx := context.Background()
	server, err := h.router.Route(ctx, entry.Name, ipv4Servers)
	if err != nil {
		h.logger.Error("routing failed",
			"domain", entry.Name,
			"error", err,
		)
		msg.Rcode = dns.RcodeServerFailure
		return
	}

	// Determine TTL - use domain-specific TTL if set, otherwise default
	ttl := entry.TTL
	if ttl == 0 {
		ttl = h.defaultTTL
	}

	// Build A record response
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

	h.logger.Debug("routing decision",
		"domain", entry.Name,
		"type", "A",
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
		msg.Rcode = dns.RcodeNameError // NXDOMAIN
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
		// Return empty answer (NOERROR with no records) - domain exists but no IPv6
		return
	}

	// Use router to select a server
	ctx := context.Background()
	server, err := h.router.Route(ctx, entry.Name, ipv6Servers)
	if err != nil {
		h.logger.Error("routing failed",
			"domain", entry.Name,
			"error", err,
		)
		msg.Rcode = dns.RcodeServerFailure
		return
	}

	// Determine TTL - use domain-specific TTL if set, otherwise default
	ttl := entry.TTL
	if ttl == 0 {
		ttl = h.defaultTTL
	}

	// Build AAAA record response
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

	h.logger.Debug("routing decision",
		"domain", entry.Name,
		"type", "AAAA",
		"selected_server", server.Address.String(),
		"region", server.Region,
		"ttl", ttl,
	)
}

// filterHealthyServers returns only servers that are currently healthy.
func (h *Handler) filterHealthyServers(servers []ServerInfo) []ServerInfo {
	if h.healthProvider == nil {
		// No health provider configured - assume all servers healthy
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
// If ipv4 is true, returns only IPv4 addresses; otherwise returns only IPv6.
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
