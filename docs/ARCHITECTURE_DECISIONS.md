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

## ADR-018: Anycast Node Discovery (Optional)

**Status**: Proposed
**Date**: 2025-12-20
**Related**: ADR-015 (Agent-Overwatch Architecture)

### Context

ADR-015 established that agents gossip to Overwatch nodes, but it assumes agents are statically configured with all Overwatch addresses:

```yaml
agent:
  gossip:
    overwatch_nodes:
      - "overwatch-1.internal:7946"
      - "overwatch-2.internal:7946"
```

**The scalability problem**: When deploying a new Overwatch node, operators must update the configuration of every agent. For deployments with hundreds of agents across multiple regions, this creates significant operational burden.

**The split-brain risk**: Currently, each Overwatch node operates independently with its own view of registered backends. If agents only connect to a subset of Overwatches:
- Overwatch-1 may know about agents A, B, C
- Overwatch-2 may know about agents D, E, F
- DNS responses differ based on which Overwatch receives the query

ADR-015 mitigates this by requiring agents to gossip to ALL Overwatch nodes. But this requires agents to know about all Overwatches upfront, which doesn't scale.

### Decision

Introduce **optional** anycast-based discovery with overwatch peering. Operators can choose between:

1. **Static Configuration** (existing, default): Agents explicitly list all Overwatch nodes
2. **Anycast Discovery** (new, optional): Agents discover Overwatches via anycast VIP, Overwatches sync state via peering

### Architecture: Anycast Discovery Mode

```
                    ┌─────────────────────────────────────────┐
                    │        Overwatch Peering Mesh           │
                    │   (memberlist gossip between peers)     │
                    │                                         │
                    │  ┌──────────┐      ┌──────────┐        │
                    │  │Overwatch1│◀────▶│Overwatch2│        │
                    │  └────▲─────┘      └─────▲────┘        │
                    │       │                  │              │
                    │       └────────┬─────────┘              │
                    │                │                        │
                    │          ┌─────▼─────┐                  │
                    │          │Overwatch3 │ (newly deployed) │
                    │          └───────────┘                  │
                    └────────────────┬────────────────────────┘
                                     │
                         Anycast VIP │ 10.255.0.1:7946
                    ┌────────────────┴────────────────────────┐
                    │        BGP Anycast Advertisement        │
                    └────────────────┬────────────────────────┘
                                     │
           ┌─────────────────────────┼─────────────────────────┐
           │                         │                         │
      ┌────▼────┐              ┌─────▼────┐              ┌─────▼────┐
      │ Agent A │              │ Agent B  │              │ Agent C  │
      │ (US-E)  │              │ (US-W)   │              │ (EU)     │
      └─────────┘              └──────────┘              └──────────┘
```

**How it works**:

1. **Agent Discovery**: Agent connects to anycast VIP, routes to nearest Overwatch
2. **Join & Learn**: Upon joining, agent receives list of all Overwatch peers
3. **Multi-Connect**: Agent establishes gossip with ALL Overwatches (not just anycast target)
4. **Overwatch Peering**: Overwatches gossip backend registry state between themselves
5. **New Overwatch**: Joins peer mesh, receives state from existing peers, advertises anycast

**Important Limitation**: Overwatches cannot use anycast for peer discovery. Since each Overwatch advertises the anycast VIP, connecting to it would route to themselves. Overwatches must be configured with at least one `bootstrap_peer` (static address). However, once joined, gossip propagates new peers to all existing Overwatches automatically.

| Component | Discovery Method | Manual Config on Add |
|-----------|------------------|----------------------|
| Agents | Anycast VIP | None (zero-touch) |
| Overwatches | Bootstrap peers | New node only (one peer address) |

### Component Specifications

**Agent Discovery Configuration** (optional):

```yaml
agent:
  gossip:
    # Option 1: Static (existing behavior, remains default)
    overwatch_nodes:
      - "overwatch-1.internal:7946"
      - "overwatch-2.internal:7946"

    # Option 2: Anycast discovery (new, optional)
    discovery:
      enabled: true
      anycast_address: "10.255.0.1:7946"
      # Optional fallback if anycast unreachable
      fallback_nodes:
        - "overwatch-1.internal:7946"
```

**Overwatch Peering Configuration** (optional):

```yaml
overwatch:
  peering:
    enabled: true
    # Bootstrap peers (at least one required for new nodes)
    # Existing nodes discover each other via gossip
    bootstrap_peers:
      - "overwatch-1.internal:7947"
      - "overwatch-2.internal:7947"
    # Separate port for peer-to-peer gossip (distinct from agent gossip)
    bind_address: "0.0.0.0:7947"
    # What state to sync between overwatches
    sync:
      backend_registry: true    # Backend health and registration
      latency_data: true        # Passive latency learning data (ADR-017)
      dnssec_keys: true         # DNSSEC key material
```

### Overwatch Peering Protocol

Overwatches form a separate memberlist cluster (port 7947) distinct from the agent gossip cluster (port 7946):

| Cluster | Port | Members | Purpose |
|---------|------|---------|---------|
| Agent Gossip | 7946 | Agents + Overwatches | Health updates, registration |
| Peer Gossip | 7947 | Overwatches only | State synchronization |

**Synchronized State**:

| Data | Sync Method | Consistency |
|------|-------------|-------------|
| Backend Registry | Full-state CRDT merge | Eventually consistent |
| Health Status | Last-writer-wins by timestamp | Eventually consistent |
| Latency Data | EWMA merge (weighted average) | Eventually consistent |
| DNSSEC Keys | Existing peer sync (ADR-015) | Strongly consistent |

**Conflict Resolution**: Backend registry uses last-seen-wins with agent authority. If Overwatch-1 and Overwatch-2 have different health status for the same backend:
1. Compare `AgentLastSeen` timestamps
2. More recent timestamp wins
3. Overwatch external validation can override agent claims (per ADR-015 trust hierarchy)

### Discovery Flow

```
┌─────────┐                    ┌────────────┐                    ┌────────────┐
│  Agent  │                    │ Overwatch1 │                    │ Overwatch2 │
└────┬────┘                    └─────┬──────┘                    └─────┬──────┘
     │                               │                                 │
     │ 1. Connect to anycast VIP     │                                 │
     │   (routed to nearest)         │                                 │
     │──────────────────────────────▶│                                 │
     │                               │                                 │
     │ 2. Join memberlist cluster    │                                 │
     │◀─────────────────────────────▶│                                 │
     │                               │                                 │
     │ 3. Receive peer list          │                                 │
     │   [Overwatch1, Overwatch2]    │                                 │
     │◀──────────────────────────────│                                 │
     │                               │                                 │
     │ 4. Connect to all peers       │                                 │
     │─────────────────────────────────────────────────────────────────▶
     │                               │                                 │
     │ 5. Gossip to all Overwatches  │                                 │
     │──────────────────────────────▶│◀────────────────────────────────│
     │                               │                                 │
```

### New Overwatch Deployment Flow

```
┌────────────┐                    ┌────────────┐                    ┌────────────┐
│ Overwatch3 │                    │ Overwatch1 │                    │ Overwatch2 │
│   (new)    │                    │ (existing) │                    │ (existing) │
└─────┬──────┘                    └─────┬──────┘                    └─────┬──────┘
      │                                 │                                 │
      │ 1. Join peer mesh via bootstrap │                                 │
      │────────────────────────────────▶│                                 │
      │                                 │                                 │
      │ 2. Receive full backend registry│                                 │
      │◀────────────────────────────────│                                 │
      │                                 │                                 │
      │ 3. Receive latency data         │                                 │
      │◀────────────────────────────────│                                 │
      │                                 │                                 │
      │ 4. Start advertising anycast    │                                 │
      │ ═══════════════════════════════════════════════════════════════  │
      │                                 │                                 │
      │ 5. Agents discover via anycast  │                                 │
      │◀════════════════════════════════│═════════════════════════════════│
      │                                 │                                 │
```

### Operator Decision Matrix

| Deployment Size | Overwatch Changes | Recommended Mode |
|-----------------|-------------------|------------------|
| Small (1-2 OW, <20 agents) | Rare | Static configuration |
| Medium (2-5 OW, 20-100 agents) | Occasional | Static or Anycast |
| Large (5+ OW, 100+ agents) | Frequent | Anycast discovery |
| Multi-region with dynamic scaling | Common | Anycast discovery |

### Network Requirements (Anycast Mode)

Operators choosing anycast discovery must configure:

1. **Anycast VIP**: A single IP address advertised by all Overwatches
2. **BGP Configuration**: Each Overwatch advertises the anycast prefix
3. **Health-Based Withdrawal**: Overwatch stops advertising if unhealthy

Example BGP setup (operator responsibility):
```
# Each Overwatch runs a BGP daemon (BIRD, FRR, etc.)
# Advertises anycast prefix when healthy
# Withdraws on failure (health check integration)
```

### Combined Anycast: DNS + Discovery

The same anycast VIP can serve both DNS queries and agent discovery on different ports:

| Service | Port | Protocol | Purpose |
|---------|------|----------|---------|
| DNS | 53 | UDP/TCP | Client DNS queries |
| Gossip | 7946 | TCP/UDP | Agent discovery and health updates |
| Peer Sync | 7947 | TCP/UDP | Overwatch-to-Overwatch state sync |

```
                         Anycast VIP: 10.255.0.1
                    ┌────────────────────────────────┐
                    │  :53    - DNS queries          │
                    │  :7946  - Agent gossip         │
                    └───────────────┬────────────────┘
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        │                           │                           │
        ▼                           ▼                           ▼
   ┌─────────┐                 ┌─────────┐                 ┌─────────┐
   │ OW-1    │◀───────────────▶│ OW-2    │◀───────────────▶│ OW-3    │
   │ :7947   │  Peer Gossip    │ :7947   │   Peer Gossip   │ :7947   │
   └─────────┘                 └─────────┘                 └─────────┘
```

**Why combining works**: All routing algorithms (geo, latency, weighted, etc.) make decisions based on the **client's source IP**, which is preserved in DNS queries regardless of anycast routing. The anycast path only determines which Overwatch processes the request, not the routing decision outcome.

**Operator benefit**: Single VIP to manage, single BGP advertisement, unified health-based withdrawal. Clients configure one nameserver address instead of multiple.

### Rationale

- **Optional complexity**: Small deployments keep static config simplicity
- **Scalable operations**: Large deployments avoid config sprawl
- **Consistent state**: Overwatch peering ensures all nodes have complete view
- **Graceful migration**: Can run mixed mode during transition
- **Leverages existing infra**: Uses memberlist (already proven in agent gossip)

### Consequences

**Positive**:
- Zero agent config changes when adding Overwatches (anycast mode)
- Consistent backend registry across all Overwatches
- New Overwatches immediately operational after peer sync
- Backward compatible (static mode unchanged)

**Negative** (Mitigated):
- Anycast requires BGP configuration → operator choice, documented requirements
- Additional network port (7947) for peer gossip → configurable, optional
- Eventual consistency window during sync → acceptable for DNS (per ADR-015)
- More complex failure modes → comprehensive monitoring and documentation

### Testing Considerations

> ⚠️ **BLOCKER**: This ADR remains in Proposed status until a valid testing strategy is established.

**The Challenge**: Anycast requires BGP peering with network infrastructure. There is no router to peer with in unit tests or standard integration tests.

| Component | Testability | Notes |
|-----------|-------------|-------|
| Overwatch Peering | ✅ Testable | Memberlist gossip between overwatches, no network dependency |
| Agent Discovery Logic | ✅ Testable | Code path for anycast vs static, mockable |
| Backend Registry Sync | ✅ Testable | CRDT merge logic, pure functions |
| BGP Advertisement | ❌ Not testable | Requires real network infrastructure |
| Anycast Routing | ❌ Not testable | Requires BGP-capable routers |
| Health-Based Withdrawal | ❌ Not testable | Requires BGP daemon integration |

**Potential Testing Approaches** (to be evaluated):

1. **Containerized BGP Lab**: Use GoBGP or FRR in containers to simulate a minimal anycast environment
   - Pro: Realistic BGP behavior
   - Con: Complex setup, slow tests, flaky in CI

2. **Split Testing Strategy**:
   - Unit/integration tests for peering and discovery logic (testable parts)
   - Manual acceptance tests for anycast behavior (documented runbook)
   - Con: No automated coverage of anycast path

3. **Mock Network Layer**: Abstract BGP health signaling behind an interface
   - Pro: Enables unit testing of advertisement logic
   - Con: Doesn't validate actual BGP behavior

4. **Dedicated Test Environment**: Physical or cloud-based lab with real routers
   - Pro: Full end-to-end validation
   - Con: Cost, maintenance, not suitable for CI

**Decision Required**: Before implementation, establish which testing approach(es) will be used and document in a testing plan.

### Migration Path

**Phase 1**: Enable overwatch peering (no agent changes)
```yaml
# All overwatches
overwatch:
  peering:
    enabled: true
    bootstrap_peers: ["overwatch-1:7947", "overwatch-2:7947"]
```

**Phase 2**: Configure anycast infrastructure (network team)

**Phase 3**: Update agents to use discovery (gradual rollout)
```yaml
agent:
  gossip:
    discovery:
      enabled: true
      anycast_address: "10.255.0.1:7946"
      fallback_nodes: ["overwatch-1:7946"]  # Keep during transition
```

**Phase 4**: Remove static overwatch_nodes from agent configs

---

## ADR-019: Toolchain and Supply Chain Policy

**Status**: Accepted
**Date**: 2026-11-20


### Context

An audit of v1.1.10 found three different Go versions in use: `go.mod` said 1.23.0, CI and the release workflow built with 1.24, and the Dockerfile built with `golang:1.23-alpine`. `govulncheck` reported 39 reachable vulnerabilities, all in the Go standard library, and the CI job that ran it ended in `|| true`, so it could not fail. The runtime image was `alpine:latest`, the linter version was `latest`, and nothing scanned the published image or proposed dependency updates.

For an authoritative DNS server the standard library is most of the attack surface (net, net/http, crypto/tls), so the toolchain version is a security property of a release, not a developer convenience.

### Decision

1. **`go.mod` is the single source of the Go version.** The `go` line names an exact patch release (1.26.11 at the time of writing). Workflows use `go-version-file: go.mod`. The Dockerfile builder image tag is the only second copy and is bumped in the same commit.
2. **No separate `toolchain` line.** `go mod tidy` removes a toolchain line that equals the go line. A lower go line with a higher toolchain line was rejected: with `GOTOOLCHAIN=local` it lets an older toolchain build the module without complaint.
3. **`govulncheck ./...` is a gate.** It runs on every pull request, on pushes to `main`, and weekly on `main`. It fails only when the code calls a vulnerable symbol. A standard library finding is fixed by raising the go line and the builder image.
4. **The container image is scanned before it is published.** Trivy fails the image workflow on HIGH and CRITICAL vulnerabilities, including ones with no fix available. A finding is accepted only by listing it in `.trivyignore` with a reason, so the acceptance is reviewed and recorded.
5. **Build inputs are pinned.** The runtime base image is an exact Alpine patch release, golangci-lint is an exact version, and third-party actions without a moving major tag are pinned by commit. Dependabot proposes the updates.
6. **Dependabot targets `develop`** for Go modules, GitHub Actions and Docker, because pull requests to `main` are accepted only from `develop`. Go minor and major updates of the builder image are excluded and done by hand together with `go.mod`.
7. **CodeQL** analyzes the Go code on pull requests, on pushes to `main` and `develop`, and weekly.

### Rationale

- One version in one place removes the drift that caused the finding.
- An exact patch on the go line means nobody can build a release with an unpatched standard library by accident, including downstream builders.
- A scheduled scan is needed because advisories are published against code that has not changed.
- Strict image scanning with a reviewed ignore file was preferred over ignoring unfixed vulnerabilities, which hides findings without a decision.

### Consequences

**Positive**:
- `govulncheck` reports no vulnerabilities at the time of the change, and a new reachable one fails CI within a week.
- Releases are reproducible with respect to toolchain, linter and base image.

**Negative**:
- A new Go patch release that fixes a reachable standard library vulnerability turns the weekly run and every open pull request red until `go.mod` and the Dockerfile are bumped.
- An unfixable HIGH or CRITICAL finding in the base image blocks image publication until it is fixed upstream or recorded in `.trivyignore`.
- Contributors need Go 1.26.11 or later (older Go from 1.21 downloads it automatically) and golangci-lint v2 built with Go 1.26 or later.
- Building the Dockerfile fails if `go.mod` is bumped without the builder image, because the golang image sets `GOTOOLCHAIN=local`. This is intended: it keeps the two in step.

**Limits**:
- The scheduled scans run on the default branch (`main`) only.
- The image scan runs when the image is built, on pushes to `main` and on tags. Images that are already published are not rescanned.
- Dependabot security updates always target the default branch and are not affected by decision 6.

---

## ADR-020: Validation and Resource Bounds for Gossip Input

**Status**: Accepted
**Date**: 2026-11-20
**Related**: ADR-015 (Agent-Overwatch Architecture), ADR-017 (Passive Latency Learning)


### Context

ADR-015 states that the Overwatch does not trust agents: it validates their health claims. That principle was not applied to the gossip messages themselves. Everything a cluster member sends reaches `GossipHandler` after one `json.Unmarshal`, and the cluster is every host that holds the gossip encryption key.

Fuzz tests of the gossip decoder (Phase 1, task 1B.4) found three ways in which one member could harm an Overwatch with well-formed messages:

| Input | Effect |
|-------|--------|
| `heartbeat` with backend weights near 5e18 | The weighted router's weight sum overflowed and `rand.Intn` panicked on the next DNS query for the domain. miekg/dns does not recover handler panics, so the process stopped. |
| Any message with a new `type` string | The type became a label of `opengslb_gossip_messages_received_total`. Series were never removed. |
| `latency_report` with many new subnets on a full table | One full table scan per new subnet under the table write lock, on the only goroutine that processes gossip. Tens of seconds for one message at the default limit. |

### Decision

1. **Numeric fields of backends received over gossip are range-checked before they reach the backend registry, the DNS registry or a router.** Port must be 1-65535 and weight 0-1000000 (`overwatch.MaxGossipWeight`). A backend outside the ranges is skipped and logged at WARN. The rest of the message is processed. JSON numbers that do not fit an `int` are pinned to the `int` limits first, so the result does not depend on the CPU architecture.
2. **Strings chosen by a gossip peer are not used as metric label values unless they come from a fixed set.** Unknown message types are counted as `type="unknown"`.
3. **Eviction from the learned latency table frees a batch.** One pass removes the oldest 1% of `MaxEntries` (at least one subnet), so the scan that finds the oldest entries is paid once per batch and not once per new subnet.

### Alternatives Considered

| Alternative | Reason not chosen |
|-------------|-------------------|
| Fix only the weighted router (saturating sum) | Leaves nonsensical ports and weights in the registry, the store and the API. The check belongs at the trust boundary. The router hardening is still worth doing and is listed as follow-up. |
| Weight limit of 1000 (the documented range) | The agent config validator accepts any non-negative weight today. A limit of 1000 could reject backends of configs that work. |
| Drop the whole heartbeat when one backend is invalid | One bad entry would take the agent's other backends to stale. |
| Keep an age-ordered index (heap or list) in the latency table | Constant-time eviction, but more state to keep consistent on every update. Batching bounds the cost with a small change. |
| Drop unknown message types from the metric | Operators would lose the signal that unknown messages arrive, for example during a mixed-version rollout. |

### Consequences

**Positive**:
- A member of the gossip cluster can no longer crash an Overwatch, grow its metric series without bound through the message type, or stall gossip processing with one message.
- The fuzz targets in `pkg/overwatch` assert these bounds on every input, so a new message field that skips validation fails the test suite.

**Negative**:
- An agent configured with a backend weight above 1000000 still starts, and the Overwatch now ignores that backend with a WARN. The agent-side validator does not have the same upper bound yet.
- After an insert into a full latency table the table holds up to 1% fewer subnets than the limit until it fills again. Data for the oldest subnets is discarded slightly earlier than before.

**Not addressed here** (follow-up): messages are not bound to an authenticated agent identity (agent authentication over gossip is not wired in), any member can update or deregister a backend it does not own, and there is no limit on the number of backends per agent or of backend entries per subnet in the latency table.

---

## ADR-021: Authoritative DNS Response Semantics and Zone Apex Records

**Status**: Accepted
**Date**: 2026-11-20
**Related**: ADR-015 (Agent-Overwatch Architecture)

### Context

An audit of v1.1.10 found that the DNS server answered in ways resolvers treat as a broken or lying nameserver: name matching was case sensitive, an AAAA query for an IPv4-only name returned SERVFAIL, every type other than A, AAAA and DNSKEY returned NOTIMP (including SOA and NS at the apex), NXDOMAIN had no SOA to cache it with, names in other people's zones got an authoritative NXDOMAIN, EDNS was ignored, and `dns.return_last_healthy` was parsed but never read.

### Decision

1. **Names are matched case-insensitively** and the response echoes the question name, and the owner names of the answer, in the case the querier used (0x20 randomization).
2. **Response codes follow what the server knows about the name**:

   | Query | Response |
   |-------|----------|
   | Name with a usable server of the queried family | NOERROR, AA, answer |
   | Name exists, no server of that family configured, or a type the server has no data for | NOERROR, AA, empty answer, zone SOA in authority |
   | Servers of that family configured, none usable | SERVFAIL (an answer from all configured servers when `dns.return_last_healthy` is true) |
   | Name does not exist inside a served zone | NXDOMAIN, AA, zone SOA in authority |
   | Name with nothing of its own but a domain beneath it (empty non-terminal) | NOERROR, AA, empty answer, SOA (RFC 8020) |
   | Name outside every served zone | REFUSED, AA clear |
   | AXFR, IXFR | REFUSED |

   The SOA in the authority section has TTL min(SOA TTL, SOA MINIMUM) (RFC 2308).
3. **Each zone has an apex SOA and NS set.** An entry of `dns.zones` is either a zone name (unchanged) or a mapping with `name`, `ttl`, `soa` (`primary_ns`, `contact`, `serial`, `refresh`, `retry`, `expire`, `negative_ttl`) and `nameservers`. Unset fields are defaulted when the zone table is built: the primary nameserver is the first configured nameserver, otherwise a name derived from `overwatch.identity.node_id`; the contact is `hostmaster.<zone>`; the NS set is the primary nameserver. A GSLB domain outside every listed zone, which is every domain when `dns.zones` is empty, is served as its own implicit zone, so existing configurations keep resolving.
4. **The SOA serial is fixed** (1 unless configured). OpenGSLB zones are not transferred, and answers change with health, not with a zone version, so a changing serial would promise something the server does not do.
5. **EDNS(0)**: a response has an OPT record exactly when the query had one. UDP responses are limited to the requester's advertised size, at least 512 bytes and at most `dns.max_udp_size` (default 1232, range 512 to 4096), and are truncated with TC when they do not fit. With ECS enabled the ECS option is echoed; the scope prefix equals the source prefix only when the routing decision used the client subnet.
6. **The `domain` label of the query metrics is bounded**: registered domains and zone apexes are label values, every other name is `other`.

### Consequences

**Positive**: resolvers can cache negative answers, dual-stack clients no longer wait on SERVFAIL, and the server no longer makes authoritative statements about names it does not serve. `test/conformance` pins the table above over UDP and TCP.

**Negative (behavior changes operators will see)**: monitoring that expects NXDOMAIN for a name outside the served zones now sees REFUSED; checks that expect NOTIMP or SERVFAIL for the cases above see NOERROR; dashboards that group by the `domain` label see `other` for names that are not registered domains; UDP responses above 1232 bytes are truncated unless `dns.max_udp_size` is raised.

---

## ADR-022: One Model for Servers and Domain Backends in the REST API

**Status**: Accepted
**Date**: 2026-11-20
**Related**: ADR-016 (Unified Server Registration)

### Context

`POST /api/v1/domains/{name}/backends` and `POST /api/v1/servers` created two different kinds of object. A domain backend went straight into the DNS registry: it was never health checked, was absent from `/servers`, could not be updated, and its DELETE route was unreachable. A server created through the API was treated as an agent backend by the stale check and removed after `remove_after`. The Overlord portal creates servers through the backends endpoint and updates them under `/servers`, so its writes mostly failed (docs/api-alignment.md, M3 to M8 and M15). In the published image the default data directory could not be created, and every write then answered 500 `not implemented` (M16).

### Decision

1. **One object.** Both endpoints create a server in the overwatch backend registry with source `api`. Both listings return the same `BackendServer`. The domain is in `service`; `name` and `metadata.service` remain as deprecated aliases. Backends stored by 1.1.10 are migrated to servers at startup.
2. **Per-server health check.** `health_check` is stored and applied by the validator. `interval` and `timeout` are duration strings (`"30s"`), as in the configuration file. A bare number is refused with 400, because the old decoder read it as nanoseconds and no client can have meant that.
3. **`routing_algorithm` is the canonical domain field**, with the name and values of the configuration file. `routing_policy` is accepted on input as a deprecated alias and still returned with the same value.
4. **Errors say what happened**: 404 unknown, 409 duplicate or owned by the configuration file or an agent, 400 invalid, 503 when the node has no persistent store.
5. **Startup and the store.** A store that cannot be created is logged at WARN (`RUNNING WITHOUT PERSISTENCE`) and the node serves the configuration file read-only. A store that exists and cannot be opened stops the start, because its domains and servers would silently be missing from DNS. The container image creates `/var/lib/opengslb` for its runtime user (UID 1000) and declares it a volume.

### Consequences

**Positive**: what a client creates is health checked, listed, updatable and deletable through either path, and a deleted server leaves DNS with the call.

**Negative (breaking for API clients)**: a client that sent `interval` or `timeout` as a number now gets 400; unknown health-check fields are refused instead of ignored; a domain without backends lists an empty array instead of 404; writes on a node without a store answer 503 instead of 500. A node whose existing store file is unreadable no longer starts.

---

## ADR-023: Configuration Reload Merges All State and Reports What It Cannot Apply

**Status**: Accepted
**Date**: 2026-11-20

### Context

On SIGHUP an overwatch rebuilt its DNS registry from the configuration file alone, so everything created through the REST API answered NXDOMAIN until the next restart. In agent mode a reload logged success and changed nothing. Settings that are only read at startup were silently ignored by a reload.

### Decision

1. **Startup and reload build DNS state with the same function**, which merges the configuration file, the domains in the store and the servers in the backend registry. Static servers come from the configuration file only; servers removed from the file are removed from the registry, at reload and at the next start.
2. **Runtime writes to the DNS registry go through one synchronizer** that replays the changes made while a reload was building the replacement, so a change that races with a reload is not lost. A reload that cannot read the store fails and keeps the state it is serving.
3. **Agent reload is real**: `agent.backends` is applied (added, removed, updated) and the overwatch nodes are told.
4. **A reload reports the changed settings it did not apply.** Reloadable settings are listed explicitly (`regions`, `domains`, `dns.default_ttl`, `dns.zones`, `dns.max_udp_size`, `dns.return_last_healthy`, `agent.backends`); any other changed setting is logged at WARN as needing a restart. A new setting needs a restart until it is added to that list.

### Consequences

**Positive**: a reload never drops API-created or agent-registered state, and never reports success for something it did not do.

**Negative**: a reload after editing a restart-only setting now logs a warning where it used to log success.

---

## ADR-024: DNSSEC Is Not Active; the Acknowledgment Requirement Is Removed

**Status**: Accepted
**Date**: 2026-11-20
**Related**: ADR-015 (which describes DNSSEC as enabled by default)

### Context

`pkg/dnssec` contains a key manager, a signer, NSEC3 and key synchronization, but nothing in `cmd/opengslb` creates them and `Handler.SetDNSSECSigner` is never called. With `overwatch.dnssec.enabled: true` responses carried no signatures and the DNSSEC API endpoints were not served. Configuration validation nevertheless required a security acknowledgment sentence to set `enabled: false`, and the documentation described DNSSEC as complete and on by default. (The actual default of `enabled` has always been false; a configuration without the key failed validation because of the acknowledgment.)

### Decision

1. With `dnssec.enabled: true` the overwatch logs one WARN at startup that signing is not active and responses are unsigned.
2. The acknowledgment is no longer required. `security_acknowledgment` is still parsed, so existing files load, and is ignored.
3. README, docs, the reference configuration and the website describe DNSSEC as in development and not active.
4. The default of `enabled` is unchanged (false). Wiring the signer is a later phase and will revisit the default.

### Consequences

Operators are no longer told their zones are signed when they are not. A configuration that omits the `dnssec` section now validates. Nobody should publish DS records for a zone served by this release.

---

## ADR-025: GTM Parity Features (Phase 2): Configuration Schema and API Shape

**Status**: Proposed (awaiting approval; nothing here is implemented yet)
**Date**: 2026-11-20
**Related**: ADR-015, ADR-016, ADR-021 (DNS response semantics), ADR-022 (one server model), ADR-023 (reload)

### Context

The target user is moving wide IPs off F5 BIG-IP DNS (GTM). Eight things they rely on there are missing: more than one answer per response, CNAME pool members, wildcard names, persistence, a last-resort answer, protection against the GSLB itself being isolated, manual resume after a failover, and a way to import an existing configuration.

This note fixes the configuration schema and the API shape for all of them before any code is written. Every feature is per domain (the isolation guard is per node), optional, and off by default: a configuration or API client that does not use the new keys behaves exactly as today.

### Decision

#### Configuration schema

```yaml
overwatch:
  # 6. Isolation guard (node level). Off by default.
  isolation_guard:
    enabled: false
    window: 30s         # how far back transitions to unhealthy are counted
    threshold: 0.8      # freeze when this share of the backends that were healthy
                        # at the start of the window turned unhealthy within it
    min_backends: 3     # the guard is inactive with fewer tracked backends
    max_freeze: 0s      # 0: stay frozen until checks recover or an operator thaws

regions:
  - name: us-east
    servers:
      - address: 10.0.1.10          # an IP member, as today
        port: 443
        service: app.gslb.example.com
      # 2. A CNAME member: a host name instead of an address.
      - cname: app.cdn.example.net
        service: www.gslb.example.com
        weight: 100
        health_target: "198.51.100.7:443"   # optional; without it the member is always healthy

domains:
  - name: app.gslb.example.com      # 3. or a wildcard: "*.apps.gslb.example.com"
    routing_algorithm: failover
    regions: [us-east, eu-west]
    ttl: 30

    max_answers: 1                  # 1. 1 to 16. Default 1.

    persistence:                    # 4. Off unless enabled.
      enabled: false
      ipv4_prefix: 24               # 8 to 32. Default 24.
      ipv6_prefix: 56               # 16 to 128. Default 56.

    fallback:                       # 5. At most one of the two keys.
      region: dr-site               # servers of this domain in that region are last-resort only
      addresses: ["192.0.2.10", "2001:db8::10"]   # static, never health checked

    failback: automatic             # 7. automatic (default) or manual. Failover algorithm only.
```

Validation rejects: `max_answers` outside 1 to 16; both `fallback.region` and `fallback.addresses`; a `fallback.region` that is also in `regions` or does not exist; `failback: manual` with an algorithm other than `failover`; a server with both or neither of `address` and `cname`; a CNAME member of a domain that is a zone apex; a wildcard that is not the leftmost label, or a wildcard domain outside every zone of `dns.zones`.

#### Behavior

1. **Multiple answers** (F5: pool "Maximum Answers Returned"). Routers gain an optional `Rank` method that returns the candidates in order; the handler takes the first `max_answers` healthy ones of the queried address family. A router without `Rank` keeps returning one server.

   | Algorithm | Order |
   |-----------|-------|
   | round-robin | rotation starting at the next server |
   | weighted | weighted random draw without replacement |
   | failover | the servers of the active tier only, in configuration order |
   | geolocation | the servers of the matched region only |
   | latency, learned_latency | ascending latency |

   A **tier** is the set of servers that share a priority. A server of the configuration file gets the position of its region in the domain's `regions` list as its priority; an API server uses its `priority` field. With `max_answers: 1` every algorithm answers as it does today.

   Resolvers and clients choose among the returned records themselves. With latency routing and `max_answers` above 1 the slower servers in the answer will receive traffic; the documentation says so.

2. **CNAME members** (F5: CNAME pools). When the selected member is a CNAME member, the answer is one CNAME record with the queried name as owner, for every query type, and `max_answers` does not apply. A domain may mix address and CNAME members. A CNAME cannot exist at a zone apex, and a domain outside every zone of `dns.zones` is the apex of its own implicit zone (ADR-021), so **a domain with CNAME members must lie inside a zone listed in `dns.zones`**. The server ID is `{service}:{cname}:{port}`, with port 0 when there is none.

3. **Wildcards** (F5: wildcard wide IPs). Matching follows RFC 4592: an exact name wins; otherwise the wildcard directly below the closest existing ancestor of the query name is used. `*.apps.example.com` therefore does not match `a.b.apps.example.com` when `b.apps.example.com` exists. The answer's owner name is the queried name. Metrics use the wildcard name as the `domain` label, so the label stays bounded.

4. **Persistence** (F5: wide IP persistence). The client key is the ECS subnet when the query carries one and ECS is enabled, otherwise the source address, cut to the configured prefix. The server is chosen by rendezvous hashing (highest random weight) over `(domain, client key, server)`, weighted by server weight. Removing a server moves only that server's clients, and every overwatch node computes the same choice from the same healthy set without shared state. The algorithm first narrows the candidates (failover: the active tier; geolocation: the matched region; the others: all healthy servers) and persistence then orders them, replacing the algorithm's own rotation, random draw or latency order. With `max_answers` above 1 the highest N scores are returned.

   This differs from F5, which stores a persistence record per client with a TTL and synchronizes it. The hash needs no state and no synchronization; in exchange a client can move when the healthy set changes, and two nodes that briefly disagree about health can answer differently.

5. **Fallback** (F5: wide IP "Last Resort Pool" and pool "Fallback IP"). Order of evaluation, per address family: healthy primary servers, then the fallback, then `dns.return_last_healthy`, then SERVFAIL. Servers of the domain in `fallback.region` are never primaries. A region fallback answers from its healthy servers; a static fallback always answers. New metric `opengslb_dns_fallback_answers_total{domain,type,source}` with `source` `region` or `static`.

6. **Isolation guard** (no direct F5 equivalent). The guard watches the effective status of all backends on this node. When the share that turned unhealthy within `window` reaches `threshold`, the node concludes that it, not the backends, is cut off: it freezes the health state DNS answers use at the last state before the window, logs at WARN, and sets `opengslb_overwatch_isolation_frozen` to 1 (`opengslb_overwatch_isolation_freezes_total` counts freezes). Manual overrides still apply while frozen. It thaws when the share of frozen backends failing their checks falls below half the threshold, when `max_freeze` passes, or when an operator thaws it through the API.

7. **Manual resume** (F5: pool "Manual Resume"). With `failback: manual`, once the failover algorithm has left a tier it does not return to it when that tier recovers. It stays on the tier it moved to until an operator resumes; if that tier fails too, it moves to the next healthy tier and stays there. The hold is kept per node, stored in the node's data directory so it survives a restart, and reported by `opengslb_failover_held{domain}`. Nodes do not share it: resume is sent to every node (Overlord does this).

8. **Import** (`opengslb-cli import f5 <file>`). Reads a `bigip_gtm.conf` or `tmsh list gtm` output and writes OpenGSLB YAML for wide IPs, pools and members to stdout, and a list of everything it did not translate, with the line it came from, to stderr. It maps only what has an exact counterpart (round-robin, ratio, global-availability, static fallback IP, maximum answers, CNAME pools, wildcard names, manual resume, HTTP, HTTPS and TCP monitors) and never guesses: a wide IP with several pools under a pool-level method other than global-availability, topology records, iRules and other monitors are reported, not approximated. The exit status is non-zero when anything was left untranslated, unless `--allow-partial` is given.

#### API shape (diff against `api/openapi.yaml`)

```diff
 components.schemas.Domain, DomainCreateRequest, DomainUpdateRequest:
+  max_answers:   integer, 1..16, default 1
+  persistence:   { enabled: boolean, ipv4_prefix: integer, ipv6_prefix: integer }
+  fallback:      { region: string } or { addresses: [string] }
+  failback:      "automatic" | "manual"
   name:          now also accepts a wildcard ("*.apps.gslb.example.com")

 components.schemas.BackendServer, ServerCreateRequest, DomainBackendCreateRequest, ServerUpdateRequest:
+  cname:         string  (host name; a member has either address or cname)
+  health_target: string  ("host:port"; CNAME members only)
   address:       no longer required when cname is set
+  BackendServer.tier:     integer  (the tier the server is in; read-only)
+  BackendServer.fallback: boolean  (true for a server of the domain's fallback region; read-only)

+paths./api/v1/domains/{name}/failover:
+  get:   200 DomainFailoverResponse { mode, active_tier, preferred_tier, held, held_since }
+         404 unknown domain; 409 the domain does not use the failover algorithm
+paths./api/v1/domains/{name}/failover/resume:
+  post:  200 DomainFailoverResponse (hold cleared); 404; 409 as above or nothing is held

+paths./api/v1/overwatch/isolation:
+  get:   200 IsolationResponse { enabled, frozen, frozen_since, unhealthy_share, threshold, window, backends }
+paths./api/v1/overwatch/isolation/thaw:
+  post:  200 IsolationResponse; 409 not frozen

 components.schemas.StatsResponse:
+  isolation_frozen: boolean
```

No existing field changes meaning and none is removed. Unknown values are refused with 400, as for the existing fields (ADR-022).

CLI: `opengslb-cli failover status <domain>`, `opengslb-cli failover resume <domain>`, `opengslb-cli isolation status`, `opengslb-cli isolation thaw`, `opengslb-cli import f5 <file>`.

#### Delivery order

Each feature is delivered end to end before the next starts: spec, Go configuration and API, Go behavior, Overlord client, Overlord UI, contract test, conformance test case and documentation section naming the F5 equivalent. Order: 1 multiple answers (it introduces `Rank` and tiers, which 4, 5 and 7 build on), 5 fallback, 7 manual resume, 4 persistence, 3 wildcards, 2 CNAME members, 6 isolation guard, 8 import tool.

### Alternatives Considered

| Alternative | Reason not chosen |
|-------------|-------------------|
| A separate `pools` section, as in F5 | Regions already are the pools. A second grouping would have to be mapped onto regions everywhere (health checks, geolocation, the API). Tiers by region order give the same failover behavior. |
| Stateful persistence records with a TTL, as in F5 | Needs state shared between overwatch nodes, which ADR-015 rules out. |
| Ring-based consistent hashing | Needs virtual nodes to balance and a rebuild on every health change. Rendezvous hashing is a few lines, has no structure to maintain and honors weights directly. |
| Allow CNAME members for any domain | A CNAME at a zone apex breaks the zone (RFC 1034 section 3.6.2), and implicit zones make every stand-alone domain an apex. |
| Share the manual-resume hold between nodes over gossip | Overwatch nodes do not gossip with each other today; adding that is a larger change than this phase. Recorded as a follow-up. |
| Isolation guard that probes an external reference address | Adds a dependency on a third party's availability and a setting most operators would get wrong. The share of simultaneous failures needs no external input. |

### Consequences

**Positive**: wide IPs that use these GTM features can be moved without changing how clients are answered. Everything is off by default, so an upgrade changes nothing until a key is set.

**Negative**:
- Routers gain a second method and the handler a multi-record path; `max_answers: 1` must stay byte-for-byte what it is today, which the conformance suite will pin.
- Tiers give a meaning to region order (and to the API `priority` field) that only the failover algorithm had before.
- The isolation guard can keep answering with backends that are really down. That is its purpose, and the reason it is off by default, has `max_freeze` and can be thawed by hand.
- The manual-resume hold is per node, so nodes can serve different tiers until an operator resumes on all of them.

### Open questions for the owner

1. **Isolation guard `max_freeze` default**: unlimited (as written), or a finite default such as 10 minutes so that a real total outage cannot be masked indefinitely?
2. **Persistence prefix defaults**: /24 and /56 (as written, matching common ECS practice), or F5's /32 and /128?
3. **`max_answers` upper limit**: 16 (as written) keeps the largest answer well inside the 1232 byte UDP default. F5 allows up to 500.
4. **CNAME members only inside `dns.zones`**: acceptable, or should a stand-alone domain with CNAME members be allowed with its apex records dropped?
5. **Import tool exit status**: non-zero on a partial translation unless `--allow-partial` (as written), or always zero with the report on stderr?

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
| 2025-12-20 | 018 | Anycast node discovery (optional) |
| 2026-11-20 | 019 | Toolchain and supply chain policy |
| 2026-11-20 | 020 | Validation and resource bounds for gossip input |
| 2026-11-20 | 021 | Authoritative DNS response semantics and zone apex records |
| 2026-11-20 | 022 | One model for servers and domain backends in the REST API |
| 2026-11-20 | 023 | Configuration reload merges all state |
| 2026-11-20 | 024 | DNSSEC is not active; acknowledgment requirement removed |
| 2026-11-20 | 025 | GTM parity features (Phase 2): configuration schema and API shape (proposed) |
