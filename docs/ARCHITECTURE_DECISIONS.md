# Architecture Decisions

This document records significant architectural decisions made during OpenGSLB development. Each decision includes context, rationale, and consequences to help future contributors understand why the system is designed the way it is.

> **Note**: ADRs marked with ⚠️ SUPERSEDED have been replaced by newer decisions but are retained for historical context.

---

## ADR-001: Use Go for Implementation

**Status**: Accepted
**Date**: 2024-11-01

### Context

Need to choose a programming language for GSLB implementation.

### Decision

Use Go (Golang) as the primary language.

### Rationale

- Excellent performance for network services
- Strong concurrency support for handling multiple health checks
- Rich standard library for DNS and HTTP operations
- Good ecosystem for building network infrastructure tools
- Easy deployment (single binary)
- Cross-platform compilation (Linux, Windows)

### Consequences

**Positive**:
- Single binary deployment simplifies operations
- Strong typing catches errors at compile time
- Excellent performance for network workloads

**Negative**:
- Team needs Go expertise

---

## ADR-002: DNS-Based Load Balancing Approach

**Status**: Accepted
**Date**: 2024-11-01

### Context

Need to choose between DNS-based, Anycast, or proxy-based GSLB.

### Decision

Implement DNS-based GSLB that returns different IP addresses based on routing logic.

### Rationale

- DNS-based approach is widely compatible
- Lower operational complexity than Anycast
- More efficient than proxy-based (no single point of failure for data plane)
- Clients cache DNS responses, reducing load on GSLB system
- No network team involvement required (unlike BGP/Anycast)

### Consequences

**Positive**:
- Works with any client that supports DNS
- No infrastructure changes required
- Scales naturally through DNS caching

**Negative**:
- TTL affects failover speed
- Clients must respect DNS TTL
- Cannot handle session persistence at DNS level

---

## ⚠️ ADR-003: Health Check Architecture

**Status**: SUPERSEDED by ADR-015
**Date**: 2024-11-01

> **This decision has been superseded.** See ADR-015 for the current agent-overwatch architecture.

---

## ADR-004: Configuration via YAML Files

**Status**: Accepted (Amended by ADR-015)
**Date**: 2024-11-01

### Context

Need configuration format for regions, servers, and policies.

### Decision

Use YAML files for configuration with hot-reload support.

### Rationale

- Human-readable and easy to version control
- Well-supported in Go ecosystem
- Can be validated before deployment
- Supports complex nested structures

### Consequences

**Positive**:
- Easy to read and edit
- Git-friendly for change tracking
- Schema validation catches errors before deployment

**Negative**:
- Need schema validation implementation
- File watching required for hot-reload
- Secrets should use environment variable overrides

**Amendment (ADR-015)**: YAML defines structural configuration. Runtime overrides stored in embedded KV store.

---

## ADR-005: Pluggable Routing Algorithms

**Status**: Accepted
**Date**: 2024-11-01

### Context

Different use cases require different routing strategies.

### Decision

Implement a strategy pattern for routing algorithms with a pluggable interface.

**Supported Algorithms**:
- Round-robin
- Weighted
- Failover (active/standby)
- Geolocation (GeoIP-based)
- Latency-based

### Rationale

- Flexibility to add new algorithms without core changes
- Easy to test algorithms in isolation
- Can switch algorithms per domain/service

### Consequences

**Positive**:
- New algorithms can be added without modifying existing code
- Each algorithm is independently testable
- Per-domain algorithm selection provides flexibility

**Negative**:
- Need clear interface definition
- Algorithm selection logic adds complexity
- Each algorithm requires documentation

---

## ADR-006: Prometheus for Metrics

**Status**: Accepted
**Date**: 2024-11-01

### Context

Need observability into GSLB operations and decisions.

### Decision

Expose Prometheus metrics for all key operations.

### Rationale

- Industry standard for metrics
- Excellent Go client library
- Easy integration with Grafana
- Pull-based model reduces GSLB dependencies

### Consequences

**Positive**:
- Standard tooling works out of the box
- Rich ecosystem of dashboards and alerting
- No push infrastructure required

**Negative**:
- Metrics endpoint needs security (IP allowlist)
- Must implement metric cardinality limits
- Configurable bind address needed to avoid port collisions

---

## ⚠️ ADR-007: Separate Control and Data Planes

**Status**: SUPERSEDED by ADR-015
**Date**: 2024-11-01

> **This decision has been superseded.** See ADR-015 for the current architecture where Overwatch nodes serve both roles independently.

---

## ADR-008: TTL-Based Failover Strategy

**Status**: Accepted
**Date**: 2024-11-01

### Context

DNS caching affects failover speed.

### Decision

Use configurable TTLs (default 30-60 seconds) for DNS responses, with health-check-based updates.

### Rationale

- Balance between failover speed and DNS query load
- Clients will update within reasonable timeframe
- Health checks can update more frequently than TTL
- Reduces impact of stale DNS caches

**TTL Guidelines**:

| TTL | Use Case | Trade-off |
|-----|----------|-----------|
| < 5s | Not recommended | High query volume, resolver issues |
| 5-15s | Critical services | Aggressive, fast failover |
| 30-60s | Most deployments | Balanced, recommended |
| > 60s | Stable services | Conservative, slower failover |

### Consequences

**Positive**:
- Configurable per deployment needs
- Health checks provide faster-than-TTL updates
- Reasonable failover times for most use cases

**Negative**:
- Higher DNS query volume with lower TTLs
- Some clients cache longer than TTL
- Need monitoring of DNS query rates

---

## ADR-009: Unhealthy Server Response Strategy

**Status**: Accepted
**Date**: 2024-11-01

### Context

When all backend servers for a domain are unhealthy, the GSLB must decide how to respond to DNS queries.

### Decision

Default to returning SERVFAIL, with a configurable option to return the last known good IP address.

```yaml
dns:
  return_last_healthy: false  # Default: return SERVFAIL when all unhealthy
```

### Rationale

- SERVFAIL is the correct DNS response when the server cannot provide an authoritative answer
- Some operators prefer degraded service over no service ("limp mode")
- Making it configurable allows operators to choose based on their requirements
- Default to SERVFAIL as it's more honest and helps surface issues quickly

### Consequences

**Positive**:
- Honest failure signaling by default
- Configurable for "limp mode" when needed
- Clear operational semantics

**Negative**:
- Must maintain last-known-good state per domain
- Operators must explicitly opt-in to stale responses
- Monitoring should alert when serving stale responses

---

## ADR-010: DNS Library Selection

**Status**: Accepted
**Date**: 2024-12-01

### Context

Need a DNS library for protocol handling.

### Decision

Use `github.com/miekg/dns` v1.x.

### Rationale

- Industry standard (15,000+ importers including CoreDNS/Kubernetes)
- Active maintenance with security updates
- Stable API suitable for our A/AAAA record needs

### Consequences

**Positive**:
- Battle-tested in production at scale
- Comprehensive DNS protocol support
- Active community and maintenance

**Negative**:
- External dependency (mitigated by stability and reputation)

---

## ADR-011: Router Terminology for Server Selection

**Status**: Accepted
**Date**: 2024-12-01

### Context

OpenGSLB is an authoritative DNS server that returns A records pointing to backend servers. It does not route network traffic.

### Decision

Use "Router" to describe the server selection component, with clear documentation that this refers to *DNS response routing* (selecting which IP to return), not network traffic routing.

### Rationale

**The Router does NOT**:
- Handle network traffic
- Proxy requests
- Manage connections to backends

**The Router ONLY**:
- Receives a pre-filtered list of healthy servers
- Selects one server based on its algorithm
- Returns the selected server for inclusion in the DNS response

### Consequences

**Positive**:
- Clear terminology within the codebase
- Consistent with industry GSLB terminology

**Negative**:
- May confuse users expecting network routing
- Requires clear documentation

---

## ⚠️ ADR-012: Distributed Agent Architecture & HA Control Plane

**Status**: SUPERSEDED by ADR-015
**Date**: 2025-04-01

> **This decision has been superseded.** The Raft-based cluster mode has been replaced by the simpler agent-overwatch architecture. See ADR-015.

---

## ⚠️ ADR-013: Hybrid Configuration & KV Store Strategy

**Status**: SUPERSEDED by ADR-015
**Date**: 2025-04-01

> **This decision has been superseded.** KV store design revised in ADR-015 for the agent-overwatch model.

---

## ⚠️ ADR-014: Runtime Mode Semantics

**Status**: SUPERSEDED by ADR-015
**Date**: 2025-04-01

> **This decision has been superseded.** Runtime modes redefined in ADR-015 (agent/overwatch instead of standalone/cluster).

---

## ADR-015: Agent-Overwatch Architecture

**Status**: Accepted
**Date**: 2025-12-10
**Supersedes**: ADR-003, ADR-007, ADR-012, ADR-013, ADR-014

### Context

Previous iterations of OpenGSLB explored Raft consensus, VRRP for VIP failover, and anycast-based architectures. These approaches introduced operational complexity:

- **Raft consensus**: Required odd-numbered node clusters, added latency for leader election, didn't solve the VIP problem
- **VRRP/Anycast**: Required network team involvement (BGP configuration) for each deployment
- **Cluster mode**: Created coordination overhead without proportional benefit

The fundamental insight: **DNS clients already have built-in redundancy**. When configured with multiple nameservers, clients automatically retry on failure. This eliminates the need for complex VIP failover mechanisms.

### Decision

OpenGSLB adopts a simplified two-component architecture:

1. **Agent**: Runs on application servers, monitors local health, gossips state to Overwatch nodes
2. **Overwatch**: Runs adjacent to or on DNS infrastructure, validates health claims, serves authoritative DNS

**Key Simplifications**:
- No Raft consensus (removed)
- No VRRP (removed)
- No VIP management (removed)
- No cluster coordination (removed)
- Overwatch nodes are fully independent

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            CLIENTS                                       │
│   resolv.conf:                                                          │
│     nameserver 10.0.1.53  ──┐                                           │
│     nameserver 10.0.1.54  ──┼──► Overwatch nodes (any of them)          │
│     nameserver 10.0.1.55  ──┘    Client retries on failure              │
└─────────────────────────────────────────────────────────────────────────┘
                                  │
┌─────────────────────────────────┴───────────────────────────────────────┐
│                       OVERWATCH NODES                                    │
│                  (Independent, no coordination)                          │
│                                                                          │
│   Overwatch-1            Overwatch-2            Overwatch-3             │
│   ┌─────────────┐        ┌─────────────┐        ┌─────────────┐        │
│   │ DNS Server  │        │ DNS Server  │        │ DNS Server  │        │
│   │ Validator   │        │ Validator   │        │ Validator   │        │
│   │ GeoIP DB    │        │ GeoIP DB    │        │ GeoIP DB    │        │
│   │ KV Store    │        │ KV Store    │        │ KV Store    │        │
│   └─────────────┘        └─────────────┘        └─────────────┘        │
│          │                      │                      │                │
│          └──────────────────────┼──────────────────────┘                │
│                    DNSSEC Key Sync (minimal)                            │
└─────────────────────────────────────────────────────────────────────────┘
                                  ▲
                                  │ Gossip (encrypted, authenticated)
┌─────────────────────────────────┴───────────────────────────────────────┐
│                            AGENTS                                        │
│                      (on application servers)                            │
│                                                                          │
│   Agent          Agent          Agent          Agent          Agent     │
│   ┌──────┐       ┌──────┐       ┌──────┐       ┌──────┐       ┌──────┐ │
│   │ App  │       │ App  │       │ App  │       │ App  │       │ App  │ │
│   └──────┘       └──────┘       └──────┘       └──────┘       └──────┘ │
│   Agents gossip to ALL Overwatch nodes globally                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Component Specifications

**Agent Mode** (`--mode=agent`):

| Aspect | Specification |
|--------|---------------|
| Purpose | Local health monitoring, predictive failure detection |
| Deployment | On application servers, one agent per server |
| Backends | Can register multiple backends per agent |
| Health Checks | HTTP, HTTPS, TCP (configurable) |
| Predictive Signals | CPU, memory, error rate thresholds |
| Gossip | Publishes to ALL configured Overwatch nodes globally |
| DNS | Does not serve DNS |
| Heartbeat | Configurable interval (explicit keepalive) |

**Overwatch Mode** (`--mode=overwatch`):

| Aspect | Specification |
|--------|---------------|
| Purpose | DNS authority, external health validation, veto power |
| Deployment | Adjacent to or on existing DNS infrastructure |
| DNS Zones | Authoritative for configured GSLB zones |
| Routing | Round-robin, weighted, failover, geolocation, latency-based |
| Validation | External health checks to all backends (configurable interval) |
| Veto Power | Overwatch external check always wins over agent claims |
| Independence | No coordination with other Overwatch nodes (except DNSSEC keys) |
| GeoIP | Local MaxMind database on each node |

### Trust Model

**Agent Identity**: Two-factor authentication:
1. Pre-shared service token (configured in YAML)
2. TOFU certificate pinning (agent generates cert, Overwatch pins on first valid connection)

**Gossip Security** (MANDATORY):

| Feature | Status | Notes |
|---------|--------|-------|
| Encryption | Required | AES-256 via memberlist |
| Authentication | Required | Pre-shared key |
| Opt-out | Not allowed | Startup fails without encryption key |

**Health Authority Hierarchy** (highest to lowest):
1. Human Override (via API)
2. External Tool Override (via API)
3. Overwatch External Validation
4. Agent Health Claim

### Rationale

- DNS clients already have built-in redundancy via multiple nameservers
- Eliminates operational complexity of Raft/VRRP/VIP management
- Each Overwatch is independently deployable
- No network team involvement required
- Security-first with mandatory encryption

### Consequences

**Positive**:
- Dramatically simpler architecture (no Raft, no VRRP, no VIPs)
- Leverages existing DNS client redundancy
- Each Overwatch is independently deployable
- Works on Linux and Windows (including Domain Controllers)
- Cloud-agnostic

**Negative** (Mitigated):
- Overwatches may have slightly different views → acceptable for DNS (eventually consistent)
- DNSSEC key sync requires minimal Overwatch communication → simple API polling
- Client-side failover adds ~2s on Overwatch failure → standard DNS behavior

---

## ADR-016: Unified Server Registration and Service-to-Domain Mapping

**Status**: Accepted
**Date**: 2025-12-18
**Breaking Change**: Requires OpenGSLB 1.1.0+

### Context

Prior to v1.1.0, OpenGSLB had two parallel and disconnected tracking systems for backend servers:

1. **Static Servers (Config-based)**: Defined in `regions[].servers[]`, used by DNS registry
2. **Agent-Registered Servers (Dynamic)**: Registered via gossip, tracked in Backend Registry, never used for DNS responses

This created fundamental problems:

**Problem 1**: No service-to-domain mapping for static servers. All servers in a region were included in all domains using that region.

**Problem 2**: Agent-registered servers were stored but never used for DNS responses.

**Problem 3**: Two separate health tracking systems existed in parallel.

### Decision

Unify server registration with three registration methods feeding a single source of truth:

1. **Static Configuration** (YAML file)
2. **Agent Self-Registration** (gossip heartbeat)
3. **API Registration** (HTTP POST)

All methods register servers into a unified Backend Registry that feeds the DNS Registry.

### Architecture

**Before v1.1.0** (Two Parallel Worlds):
```
Config File → DNS Registry → DNS Handler (static only)
Agent Gossip → Backend Registry → API (never used for DNS)
```

**After v1.1.0** (Unified):
```
Config File  ─┐
Agent Gossip ─┼─► Backend Registry ─► DNS Registry ─► DNS Handler
API POST     ─┘        │
                       └─► Validator (external validation)
```

### Implementation

**Required `service` field on static servers**:

```yaml
# Before (v1.0.x) - NO LONGER VALID
regions:
  - name: us-east
    servers:
      - address: 10.0.1.10
        port: 8080

# After (v1.1.0) - REQUIRED
regions:
  - name: us-east
    servers:
      - address: 10.0.1.10
        port: 8080
        weight: 100
        service: webapp.example.com  # REQUIRED
```

**Server CRUD API**:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/services/{service}/servers` | GET | List servers |
| `/api/v1/services/{service}/servers` | POST | Add server |
| `/api/v1/services/{service}/servers/{addr}:{port}` | PATCH | Update weight |
| `/api/v1/services/{service}/servers/{addr}:{port}` | DELETE | Remove server |

### Rationale

- Explicit service-to-domain mapping prevents misconfiguration
- Unified architecture eliminates parallel tracking systems
- API-driven operations enable dynamic server management
- All registration methods feed the same validation pipeline

### Consequences

**Positive**:
- Single source of truth for all servers
- Explicit service mapping prevents errors
- Agent-registered servers appear in DNS responses
- Full CRUD via API

**Negative** (Mitigated):
- Breaking change requires `service` field → clear error messages and migration guide
- Config verbosity increases → necessary for correctness

---

## ADR-017: Passive Latency Learning via OS TCP Statistics

**Status**: Accepted
**Date**: 2025-12-19
**Related**: ADR-015 (Agent-Overwatch Architecture)

### Context

Latency-based routing requires knowing network latency between clients and backends. At DNS resolution time, we only know the source IP (often a resolver) and optionally EDNS Client Subnet.

**How competitors solve this**:

| Vendor | Approach | Limitation |
|--------|----------|------------|
| F5 GTM | Active LDNS probing | LDNS ≠ client location; probes blocked |
| AWS Route53 | Pre-computed database | Only works for AWS regions |
| Cloudflare | Edge PoP measurement | Requires 330+ global PoPs |
| Citrix NetScaler | LDNS probing chain | Same LDNS limitations |

**Critical finding**: No GSLB product measures actual client-to-backend latency. All use proxies.

**The opportunity**: OpenGSLB agents run on application servers. The OS already tracks TCP RTT for congestion control. We can read this data to learn actual client latencies.

### Decision

Implement passive latency learning using OS-native TCP statistics only.

**Linux**: Netlink INET_DIAG (`tcp_info.tcpi_rtt`) - requires CAP_NET_ADMIN
**Windows**: GetPerTcpConnectionEStats API - requires Administrator

**Approaches rejected**:

| Approach | Reason |
|----------|--------|
| Application SDK | Requires code changes; doesn't work for COTS |
| eBPF | CAP_BPF allows kernel code execution; catastrophic if compromised |
| Packet capture | CAP_NET_RAW required; CPU overhead |
| Network TAP | Infrastructure dependency |

### Data Flow

```
1. Client connects to application (normal traffic)
2. OS tracks RTT for congestion control (always happens)
3. Agent polls OS for connection RTT (every 10s)
4. Agent aggregates by subnet: 203.0.113.0/24 → 45ms average
5. Agent gossips aggregated data to Overwatch (every 30s)
6. Overwatch uses learned data for routing decisions
```

### Privacy Protection

Individual client IPs never leave the agent. All data aggregated to subnets:

| Protocol | Aggregation | Addresses per Bucket |
|----------|-------------|---------------------|
| IPv4 | /24 | 256 |
| IPv6 | /48 | ~2^80 |

### Configuration

```yaml
# Agent configuration
latency_learning:
  enabled: true
  poll_interval: 10s
  ipv4_prefix: 24
  ipv6_prefix: 48
  min_connection_age: 5s
  max_subnets: 100000
  subnet_ttl: 168h
  min_samples: 5
  report_interval: 30s
  ewma_alpha: 0.3
```

### Security Analysis

| Platform | Capability | Risk Level |
|----------|------------|------------|
| Linux | CAP_NET_ADMIN | Low (read-only diagnostics) |
| Windows | Administrator | Low (standard for services) |

**CAP_NET_ADMIN does NOT allow**:
- Kernel code execution (unlike eBPF)
- Packet content capture (unlike CAP_NET_RAW)
- Memory access or privilege escalation

### Rationale

- Unique capability: No other GSLB learns from actual client connections
- Zero application changes: Works with any software
- Minimal overhead: Polling existing kernel structures every 10s
- Safe privileges: Read-only network diagnostics

### Consequences

**Positive**:
- Real client latency data (unique in market)
- Works with commercial off-the-shelf software
- Minimal CPU impact
- Graceful degradation to geo-routing if collection fails
- Cross-platform (Linux and Windows)

**Negative** (Mitigated):
- Requires elevated privileges → read-only, well-understood scope
- Cold start period → falls back to geo-inference
- macOS/BSD not supported → Linux and Windows cover enterprise deployments

---

## Document History

| Date | ADR | Change |
|------|-----|--------|
| 2024-11 | 001-008 | Initial architecture decisions |
| 2024-12 | 009-011 | Sprint 2/3 decisions |
| 2025-04 | 012-014 | Distributed architecture (Raft-based) |
| 2025-12-10 | 015 | Agent-Overwatch architecture (supersedes 003, 007, 012-014) |
| 2025-12-18 | 016 | Unified server registration |
| 2025-12-19 | 017 | Passive latency learning |
