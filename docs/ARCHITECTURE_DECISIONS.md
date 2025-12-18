# Architecture Decisions

This document records significant architectural decisions made during OpenGSLB development. Each decision includes context, rationale, and consequences to help future contributors understand why the system is designed the way it is.

> **Note**: ADRs marked with ⚠️ SUPERSEDED have been replaced by newer decisions but are retained for historical context.

---

## ADR-001: Use Go for Implementation
**Status**: Accepted

**Context**: Need to choose a programming language for GSLB implementation.

**Decision**: Use Go (Golang) as the primary language.

**Rationale**:
- Excellent performance for network services
- Strong concurrency support for handling multiple health checks
- Rich standard library for DNS and HTTP operations
- Good ecosystem for building network infrastructure tools
- Easy deployment (single binary)
- Cross-platform compilation (Linux, Windows)

**Consequences**: Team needs Go expertise; benefits from strong typing and performance.

---

## ADR-002: DNS-Based Load Balancing Approach
**Status**: Accepted

**Context**: Need to choose between DNS-based, Anycast, or proxy-based GSLB.

**Decision**: Implement DNS-based GSLB that returns different IP addresses based on routing logic.

**Rationale**:
- DNS-based approach is widely compatible
- Lower operational complexity than Anycast
- More efficient than proxy-based (no single point of failure for data plane)
- Clients cache DNS responses, reducing load on GSLB system
- No network team involvement required (unlike BGP/Anycast)

**Consequences**: 
- TTL affects failover speed
- Clients must respect DNS TTL
- Cannot handle session persistence at DNS level

---

## ⚠️ ADR-003: Health Check Architecture
**Status**: SUPERSEDED by ADR-015

> **This decision has been superseded.** See ADR-015 for the current agent-overwatch architecture.

---

## ADR-004: Configuration via YAML Files
**Status**: Accepted (Amended by ADR-015)

**Context**: Need configuration format for regions, servers, and policies.

**Decision**: Use YAML files for configuration with hot-reload support.

**Rationale**:
- Human-readable and easy to version control
- Well-supported in Go ecosystem
- Can be validated before deployment
- Supports complex nested structures

**Consequences**: 
- Need schema validation
- File watching for hot-reload
- Consider environment variable overrides for secrets

**Amendment (ADR-015)**: YAML defines structural configuration. Runtime overrides stored in embedded KV store. See ADR-015 for details.

---

## ADR-005: Pluggable Routing Algorithms
**Status**: Accepted

**Context**: Different use cases require different routing strategies.

**Decision**: Implement a strategy pattern for routing algorithms with a pluggable interface.

**Supported Algorithms**:
- Round-robin
- Weighted
- Failover (active/standby)
- Geolocation (GeoIP-based)
- Latency-based

**Rationale**:
- Flexibility to add new algorithms without core changes
- Easy to test algorithms in isolation
- Can switch algorithms per domain/service

**Consequences**:
- Need clear interface definition
- Algorithm selection logic required
- Documentation for each algorithm's behavior

---

## ADR-006: Prometheus for Metrics
**Status**: Accepted

**Context**: Need observability into GSLB operations and decisions.

**Decision**: Expose Prometheus metrics for all key operations.

**Rationale**:
- Industry standard for metrics
- Excellent Go client library
- Easy integration with Grafana
- Pull-based model reduces GSLB dependencies

**Consequences**:
- Metrics endpoint secured via IP allowlist
- Configurable bind address to avoid port collisions
- Should implement metric cardinality limits

---

## ⚠️ ADR-007: Separate Control and Data Planes
**Status**: SUPERSEDED by ADR-015

> **This decision has been superseded.** See ADR-015 for the current architecture where Overwatch nodes serve both roles independently.

---

## ADR-008: TTL-Based Failover Strategy
**Status**: Accepted

**Context**: DNS caching affects failover speed.

**Decision**: Use configurable TTLs (default 30-60 seconds) for DNS responses, with health-check-based updates.

**Rationale**:
- Balance between failover speed and DNS query load
- Clients will update within reasonable timeframe
- Health checks can update more frequently than TTL
- Reduces impact of stale DNS caches

**Documented Risks for Low TTLs**:
- TTL < 5s: High query volume, potential resolver issues
- TTL 5-15s: Aggressive, suitable for critical services
- TTL 30-60s: Balanced, recommended for most deployments
- TTL > 60s: Conservative, slower failover

**Consequences**:
- Higher DNS query volume with lower TTLs
- Some clients may cache longer than TTL
- Need monitoring of DNS query rates

---

## ADR-009: Unhealthy Server Response Strategy
**Status**: Accepted

**Context**: When all backend servers for a domain are unhealthy, the GSLB must decide how to respond to DNS queries.

**Decision**: Default to returning SERVFAIL, with a configurable option to return the last known good IP address.

**Configuration**:
```yaml
dns:
  return_last_healthy: false  # Default: return SERVFAIL when all unhealthy
```

**Rationale**:
- SERVFAIL is the correct DNS response when the server cannot provide an authoritative answer
- Some operators prefer degraded service over no service ("limp mode")
- Making it configurable allows operators to choose based on their requirements
- Default to SERVFAIL as it's more honest and helps surface issues quickly

**Consequences**:
- Must maintain last-known-good state per domain
- Operators must explicitly opt-in to stale responses
- Monitoring should alert when serving stale responses

---

## ADR-010: DNS Library Selection
**Status**: Accepted

**Decision**: Use github.com/miekg/dns v1.x

**Rationale**:
- Industry standard (15,000+ importers including CoreDNS/Kubernetes)
- Active maintenance with security updates
- Stable API suitable for our A/AAAA record needs

---

## ADR-011: Router Terminology for Server Selection
**Status**: Accepted

**Context**: OpenGSLB is an authoritative DNS server that returns A records pointing to backend servers. It does not route network traffic.

**Decision**: Use "Router" to describe the server selection component, with clear documentation that this refers to *DNS response routing* (selecting which IP to return), not network traffic routing.

**Important Clarification**: The Router does NOT:
- Handle network traffic
- Proxy requests
- Manage connections to backends

The Router ONLY:
- Receives a pre-filtered list of healthy servers
- Selects one server based on its algorithm
- Returns the selected server for inclusion in the DNS response

---

## ⚠️ ADR-012: Distributed Agent Architecture & HA Control Plane
**Status**: SUPERSEDED by ADR-015

> **This decision has been superseded.** The Raft-based cluster mode has been replaced by the simpler agent-overwatch architecture. See ADR-015.

---

## ⚠️ ADR-013: Hybrid Configuration & KV Store Strategy
**Status**: SUPERSEDED by ADR-015

> **This decision has been superseded.** KV store design revised in ADR-015 for the agent-overwatch model.

---

## ⚠️ ADR-014: Runtime Mode Semantics
**Status**: SUPERSEDED by ADR-015

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
│                                                                          │
│   resolv.conf:                                                          │
│     nameserver 10.0.1.53  ──┐                                           │
│     nameserver 10.0.1.54  ──┼──► Overwatch nodes (any of them)          │
│     nameserver 10.0.1.55  ──┘    Client retries on failure              │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                       OVERWATCH NODES                                    │
│                  (Independent, no coordination)                          │
│                                                                          │
│   Overwatch-1            Overwatch-2            Overwatch-3             │
│   10.0.1.53              10.0.1.54              10.0.1.55               │
│   ┌─────────────┐        ┌─────────────┐        ┌─────────────┐        │
│   │ DNS Server  │        │ DNS Server  │        │ DNS Server  │        │
│   │ Validator   │        │ Validator   │        │ Validator   │        │
│   │ GeoIP DB    │        │ GeoIP DB    │        │ GeoIP DB    │        │
│   │ KV Store    │        │ KV Store    │        │ KV Store    │        │
│   └──────┬──────┘        └──────┬──────┘        └──────┬──────┘        │
│          │                      │                      │                │
│          │         DNSSEC Key Sync (minimal)           │                │
│          └──────────────────────┼──────────────────────┘                │
│                                 │                                        │
│                    All receive gossip from agents                       │
│                    All perform external validation                      │
│                    All serve authoritative DNS                          │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                  ▲
                                  │ Gossip (encrypted, authenticated)
                                  │
┌─────────────────────────────────┴───────────────────────────────────────┐
│                            AGENTS                                        │
│                      (on application servers)                            │
│                                                                          │
│   Agent          Agent          Agent          Agent          Agent     │
│   ┌──────┐       ┌──────┐       ┌──────┐       ┌──────┐       ┌──────┐ │
│   │ App  │       │ App  │       │ App  │       │ App  │       │ App  │ │
│   │ App  │       │ App  │       │ App  │       │ App  │       │ App  │ │
│   └──────┘       └──────┘       └──────┘       └──────┘       └──────┘ │
│                                                                          │
│   Agents gossip to ALL Overwatch nodes globally                         │
│   Each agent can register multiple backends                             │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Component Specifications

#### Agent Mode (`--mode=agent`)

| Aspect | Specification |
|--------|---------------|
| **Purpose** | Local health monitoring, predictive failure detection |
| **Deployment** | On application servers, one agent per server |
| **Backends** | Can register multiple backends per agent |
| **Health Checks** | HTTP, HTTPS, TCP (configurable) |
| **Predictive Signals** | CPU, memory, error rate thresholds |
| **Gossip** | Publishes to ALL configured Overwatch nodes globally |
| **DNS** | Does not serve DNS |
| **Heartbeat** | Configurable interval (explicit keepalive) |

#### Overwatch Mode (`--mode=overwatch`)

| Aspect | Specification |
|--------|---------------|
| **Purpose** | DNS authority, external health validation, veto power |
| **Deployment** | Adjacent to or on existing DNS infrastructure |
| **DNS Zones** | Authoritative for configured GSLB zones |
| **Routing** | Round-robin, weighted, failover, geolocation, latency-based |
| **Validation** | External health checks to all backends (configurable interval) |
| **Veto Power** | Overwatch external check always wins over agent claims |
| **Independence** | No coordination with other Overwatch nodes (except DNSSEC keys) |
| **GeoIP** | Local MaxMind database on each node |

### Trust Model

#### Agent Identity

Agents authenticate using a two-factor approach:

1. **Pre-shared service token**: Configured in agent YAML, validated by Overwatch
2. **TOFU certificate pinning**: Agent generates self-signed certificate, presents on first connection with valid token, Overwatch pins certificate for future connections

```yaml
# Agent config
identity:
  service_token: "secret-token-for-myapp"  # Pre-shared with Overwatch
  # Certificate auto-generated on first start
```

```yaml
# Overwatch config  
agent_tokens:
  myapp: "secret-token-for-myapp"
  otherapp: "different-token"
```

#### Gossip Security (MANDATORY)

| Security Feature | Status | Notes |
|-----------------|--------|-------|
| Encryption | **Required** | AES-256 via memberlist |
| Authentication | **Required** | Pre-shared key |
| No opt-out | **Enforced** | Startup fails without encryption key |

```yaml
gossip:
  encryption_key: "base64-encoded-32-byte-key"  # REQUIRED
```

#### Overwatch Independence

- Overwatch nodes do NOT gossip to each other
- Overwatch nodes do NOT share veto decisions
- Each Overwatch validates independently
- Each Overwatch may have slightly different views during convergence
- This is acceptable: DNS is inherently eventually consistent

### Health Authority Hierarchy

From lowest to highest authority:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      AUTHORITY HIERARCHY                                 │
│                      (Higher wins conflicts)                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   4. Human Override (via API)              ◄── HIGHEST AUTHORITY        │
│      └── Explicit veto/enable via API call                              │
│      └── Persists until DELETE or reboot                                │
│                                                                          │
│   3. External Tool Override (via API)                                   │
│      └── CloudWatch, Watcher, custom tooling                            │
│      └── Persists until DELETE or reboot                                │
│                                                                          │
│   2. Overwatch External Validation                                      │
│      └── Overwatch's own health check result                            │
│      └── ALWAYS wins over agent claims                                  │
│                                                                          │
│   1. Agent Health Claim                    ◄── LOWEST AUTHORITY         │
│      └── Agent's local health check + predictive signals                │
│      └── Trusted but verified                                           │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### DNSSEC

#### Requirements

| Aspect | Decision |
|--------|----------|
| Default State | **Enabled** |
| Opt-out | Requires explicit acknowledgment |
| Algorithm | NSEC3 (prevents zone enumeration) |
| Key Rotation | Automated |

#### Key Management

- Each Overwatch can generate keys
- Overwatches poll each other's API for key sync (only inter-Overwatch communication)
- Newest key wins (by timestamp)
- Keys stored in local KV store

```yaml
dnssec:
  enabled: true  # Default
  
  # OR to disable:
  enabled: false
  security_acknowledgment: "I understand that disabling DNSSEC allows DNS spoofing attacks"
  
  # Key sync
  key_sync:
    peers:
      - "https://overwatch-2.internal:9090"
      - "https://overwatch-3.internal:9090"
    poll_interval: "1h"
```

#### DS Record Notification

- Exposed via API: `GET /api/v1/dnssec/ds`
- Webhook notification on key rotation (configurable)
- CLI command: `opengslb dnssec ds-record`

### External Override API

External tools (CloudWatch, Watcher, etc.) can override health state:

```bash
# Mark backend unhealthy
curl -X PUT http://overwatch:9090/api/v1/overrides/myapp/10.0.1.10 \
  -d '{"healthy": false, "reason": "high latency detected by CloudWatch"}'

# Clear override
curl -X DELETE http://overwatch:9090/api/v1/overrides/myapp/10.0.1.10
```

**Override Behavior**:
- Persists until explicit DELETE or Overwatch reboot
- Higher authority than Overwatch's own validation
- Lower authority than human API calls (same mechanism, different audit trail)

**Security**: IP allowlist only. Authentication delegated to external reverse proxy.

### DNS Integration

Overwatch integrates with existing DNS infrastructure via conditional forwarding:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    DNS INTEGRATION PATTERNS                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   Pattern 1: Overwatch on DNS Server                                    │
│   ┌─────────────────────────┐                                           │
│   │ DNS Server (BIND, etc)  │                                           │
│   │ forwards gslb.* to      │──► localhost:5353 (Overwatch)             │
│   └─────────────────────────┘                                           │
│                                                                          │
│   Pattern 2: Overwatch Adjacent                                         │
│   ┌─────────────────────────┐      ┌─────────────────────┐              │
│   │ DNS Server              │      │ Overwatch           │              │
│   │ forwards gslb.* to      │──────► 10.0.1.53:53        │              │
│   └─────────────────────────┘      └─────────────────────┘              │
│                                                                          │
│   Pattern 3: Direct Client Resolution                                   │
│   ┌─────────────────────────┐                                           │
│   │ Client resolv.conf      │                                           │
│   │ nameserver 10.0.1.53    │──► Overwatch (authoritative)              │
│   │ nameserver 10.0.1.54    │──► Overwatch (redundancy)                 │
│   └─────────────────────────┘                                           │
│                                                                          │
│   Pattern 4: Public DNS (NS Delegation)                                 │
│   ┌─────────────────────────┐                                           │
│   │ Parent Zone             │                                           │
│   │ gslb.example.com NS     │──► overwatch.example.com                  │
│   └─────────────────────────┘                                           │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### KV Store

Each Overwatch has an independent embedded KV store (bbolt):

| Data Type | Storage | Mutable |
|-----------|---------|---------|
| Domain/region config | YAML | No (reload required) |
| Health state | KV | Yes (continuous) |
| Agent registrations | KV | Yes (via gossip) |
| Weight overrides | KV | Yes (via API) |
| DNSSEC keys | KV | Yes (via sync) |
| Pinned agent certs | KV | Yes (TOFU) |

### Platform Support

| Platform | Support Level |
|----------|---------------|
| Linux (amd64, arm64) | Full |
| Windows (amd64) | Full (including Domain Controllers) |
| macOS | Development only |

### Failure Scenarios

| Scenario | Behavior | Recovery |
|----------|----------|----------|
| Overwatch node dies | Clients retry to next nameserver | Automatic (~2s client timeout) |
| Agent dies with app | Overwatch detects missing heartbeat, external check confirms | Automatic (configurable interval) |
| Agent lies about health | Overwatch external validation catches, vetoes | Automatic |
| Network partition | Each Overwatch operates with its view | Eventual consistency on heal |
| All backends unhealthy | SERVFAIL (default) or last-known-good | Configurable |

### Migration from Sprint 4

**This is a breaking change.** Sprint 4's cluster mode (Raft-based) is removed.

| Sprint 4 Component | ADR-015 Equivalent |
|--------------------|-------------------|
| `--mode=standalone` | `--mode=overwatch` (single node) |
| `--mode=cluster` | Removed (use multiple independent Overwatches) |
| Raft consensus | Removed |
| Leader election | Removed |
| Agent concept | `--mode=agent` (refined) |
| Overwatch concept | `--mode=overwatch` (expanded) |
| Overlord concept | Removed (merged into Overwatch) |

### Configuration Examples

#### Agent Configuration

```yaml
mode: agent

identity:
  service_token: "pre-shared-secret-token"
  region: us-east

backends:
  - service: myapp
    address: 10.0.2.100
    port: 8080
    health_check:
      type: http
      path: /health
      interval: 5s
      timeout: 2s
      
  - service: otherapp
    address: 10.0.2.100
    port: 9090
    health_check:
      type: tcp
      interval: 10s
      timeout: 3s

predictive:
  enabled: true
  cpu_threshold: 85
  memory_threshold: 90
  error_rate_threshold: 5

gossip:
  encryption_key: "base64-encoded-32-byte-key"
  overwatch_nodes:
    - overwatch-us-east-1.internal:7946
    - overwatch-us-east-2.internal:7946
    - overwatch-eu-west-1.internal:7946
    - overwatch-ap-south-1.internal:7946

heartbeat:
  interval: 10s

metrics:
  enabled: true
  address: "/var/run/opengslb/metrics.sock"
```

#### Overwatch Configuration

```yaml
mode: overwatch

identity:
  node_id: overwatch-us-east-1
  region: us-east

dns:
  listen_address: "0.0.0.0:53"
  zones:
    - gslb.example.com
    - gslb.internal.corp
  default_ttl: 30

dnssec:
  enabled: true
  key_sync:
    peers:
      - "https://overwatch-us-east-2.internal:9090"
      - "https://overwatch-eu-west-1.internal:9090"
    poll_interval: "1h"

agent_tokens:
  myapp: "pre-shared-secret-token"
  otherapp: "different-token"

gossip:
  bind_address: "0.0.0.0:7946"
  encryption_key: "base64-encoded-32-byte-key"

validation:
  enabled: true
  check_interval: 30s
  check_timeout: 5s

routing:
  default_algorithm: weighted

geolocation:
  database_path: "/var/lib/opengslb/GeoLite2-Country.mmdb"

api:
  address: "0.0.0.0:9090"
  allowed_networks:
    - 10.0.0.0/8
    - 192.168.0.0/16

metrics:
  enabled: true
  address: "0.0.0.0:9091"
  allowed_networks:
    - 10.0.0.0/8
```

### Consequences

**Positive**:
- Dramatically simpler architecture (no Raft, no VRRP, no VIPs)
- No network team involvement required
- Leverages existing DNS client redundancy
- Each Overwatch is independently deployable
- Security-first with mandatory encryption
- Works on Linux and Windows (including Domain Controllers)
- Cloud-agnostic (works anywhere)

**Negative (Mitigated)**:
- Overwatches may have slightly different views → acceptable for DNS
- DNSSEC key sync requires minimal Overwatch communication → simple API polling
- Client-side failover adds ~2s on Overwatch failure → standard DNS behavior

### Security Summary

| Feature | Status | Opt-Out |
|---------|--------|---------|
| Gossip encryption | Mandatory | No |
| Gossip authentication | Mandatory | No |
| Agent TOFU certificates | Mandatory | No |
| DNSSEC | Default enabled | Explicit acknowledgment required |
| API IP allowlist | Recommended | Configurable |
| DNS rate limiting | Default enabled | Configurable |

---

## ADR-016: Unified Server Registration and Service-to-Domain Mapping
**Status**: Accepted
**Date**: 2025-12-18
**Version**: Introduces breaking changes → OpenGSLB 1.1.0

### Context

Prior to v1.1.0, OpenGSLB had **two parallel and disconnected tracking systems** for backend servers:

1. **Static Servers (Config-based)**: Defined in `regions[].servers[]`, used by DNS registry, validated by Health Manager
2. **Agent-Registered Servers (Dynamic)**: Registered via gossip with `backend.service` field, tracked in Backend Registry, never used for DNS responses

This created fundamental architectural problems:

**Problem 1: No Service-to-Domain Mapping for Static Servers**
```yaml
regions:
  - name: us-east
    servers:
      - address: 10.0.1.10:8080  # webapp server
      - address: 10.0.1.11:9000  # api server
      - address: 10.0.1.12:5432  # database server

domains:
  - name: webapp.example.com
    regions:
      - us-east  # ← Includes ALL 3 servers (webapp, api, database)!
```

**Result**: DNS queries for `webapp.example.com` could return the database IP because all servers in a region were included in all domains using that region.

**Problem 2: Agent-Registered Servers Not Used for DNS**

Agents correctly specified `backend.service = domain.name` but this data was:
- Stored in Backend Registry
- Used for API queries (`/api/v1/domains/{name}/backends`)
- **Never consulted by DNS handler**

DNS responses were built exclusively from static config, ignoring agent registrations entirely.

**Problem 3: Two Separate Health Tracking Systems**

```go
// DNS Handler used Health Manager (static servers only)
handler := dns.NewHandler(dns.HandlerConfig{
    HealthProvider: a.healthManager,  // ← Static servers only!
})

// Backend Registry tracked agent health (never used for DNS)
backendRegistry.IsHealthy(address, port)  // ← Method exists, never called!
```

**Problem 4: No API for Server Management**

- No way to add/remove servers dynamically via API
- Required config file reload for changes
- Couldn't build dynamic server pools

### Decision

**Unify server registration** with three registration methods feeding a single source of truth:

1. **Static Configuration** (YAML file)
2. **Agent Self-Registration** (gossip heartbeat)
3. **API Registration** (HTTP POST)

All three methods register servers into a **unified Backend Registry** that feeds the **DNS Registry** for query responses.

### Architecture Overview

#### Before v1.1.0 (Two Parallel Worlds)

```
┌─────────────────────────────────────────────────────────────────────┐
│                    STATIC WORLD                                      │
│                                                                      │
│   Config File (regions.servers)                                     │
│         ↓                                                            │
│   DNS Registry ← built at startup, never updated                    │
│         ↓                                                            │
│   DNS Handler ← answers queries                                     │
│         ↓                                                            │
│   Health Manager ← checks health of static servers                  │
│                                                                      │
│   ❌ No service field on servers                                     │
│   ❌ All servers in region go to all domains                         │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                    DYNAMIC WORLD                                     │
│                                                                      │
│   Agent Heartbeat (backend.service = domain name)                   │
│         ↓                                                            │
│   Backend Registry ← stores by "service:address:port"               │
│         ↓                                                            │
│   Validator ← checks health externally                              │
│         ↓                                                            │
│   API Handlers ← for /api/v1/domains/{name}/backends                │
│                                                                      │
│   ❌ NOT used for DNS responses!                                     │
│   ❌ Parallel health tracking                                        │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

#### After v1.1.0 (Unified Architecture)

```
┌─────────────────────────────────────────────────────────────────────┐
│                    UNIFIED REGISTRATION                              │
│                                                                      │
│   ┌─────────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│   │ Static Config   │  │ Agent Heartbeat  │  │ API POST         │  │
│   │ (YAML)          │  │ (Gossip)         │  │ (HTTP)           │  │
│   └────────┬────────┘  └────────┬─────────┘  └────────┬─────────┘  │
│            │                    │                      │            │
│            └────────────────────┼──────────────────────┘            │
│                                 ↓                                   │
│                      BACKEND REGISTRY                               │
│                   (Single Source of Truth)                          │
│              - Tracks ALL servers                                   │
│              - Each server has service field                        │
│              - Indexed by "service:address:port"                    │
│                                 ↓                                   │
│                   ┌─────────────┴─────────────┐                     │
│                   ↓                           ↓                     │
│            DNS REGISTRY                   VALIDATOR                 │
│         (Dynamic Updates)            (External Validation)          │
│                   ↓                                                 │
│            DNS HANDLER                                              │
│         (Answers Queries)                                           │
│                                                                     │
│   ✅ All servers mapped to specific domains                         │
│   ✅ DNS responses include agent-registered servers                 │
│   ✅ Unified health tracking                                        │
│   ✅ Full CRUD via API                                              │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Implementation Details

#### 1. Required `service` Field on Static Servers

**Before:**
```yaml
regions:
  - name: us-east
    servers:
      - address: 10.0.1.10
        port: 8080
        weight: 100
```

**After (v1.1.0):**
```yaml
regions:
  - name: us-east
    servers:
      - address: 10.0.1.10
        port: 8080
        weight: 100
        service: webapp.example.com  # ← REQUIRED
```

**Rationale**:
- Explicit is better than implicit
- Prevents accidental misconfiguration
- Matches agent backend model
- Since no production deployments exist, breaking change is acceptable

#### 2. Dynamic DNS Registry Updates

**DNS Registry** gains new methods:
- `RegisterServer(service, address, port, weight, region)` - Add/update server
- `DeregisterServer(service, address, port)` - Remove server
- `UpdateServerWeight(service, address, port, weight)` - Adjust weight

**Trigger points**:
- **Agent heartbeat received** → Register server in DNS registry
- **Agent goes stale** → Deregister server from DNS registry
- **API POST /servers** → Register server in DNS registry
- **API DELETE /servers/{id}** → Deregister server from DNS registry
- **Config reload** → Re-register static servers

#### 3. Unified Backend Registry

Backend Registry becomes the **single source of truth** for all servers:

```go
type Backend struct {
    Service         string          // Domain name (e.g., "webapp.example.com")
    Address         string          // IP address
    Port            int             // Port number
    Weight          int             // Load balancing weight
    Region          string          // Geographic region

    // Health tracking
    AgentHealthy    bool            // Agent's local health claim
    AgentLastSeen   time.Time       // Last heartbeat from agent
    ValidationHealthy bool          // Overwatch external validation result
    EffectiveStatus BackendStatus   // Computed status (validation wins)

    // Registration source
    Source          RegistrationSource  // static, agent, api
    AgentID         string              // Empty if static/API

    // Metadata
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type RegistrationSource string
const (
    SourceStatic RegistrationSource = "static"  // Config file
    SourceAgent  RegistrationSource = "agent"   // Agent gossip
    SourceAPI    RegistrationSource = "api"     // API call
)
```

**Key behaviors**:
- Static servers: `Source=SourceStatic`, `AgentID=""`, validation-only health
- Agent servers: `Source=SourceAgent`, `AgentID="agent-123"`, predictive health
- API servers: `Source=SourceAPI`, `AgentID=""`, validation-only health

#### 4. Unified External Validation

Validator validates **ALL** backends in registry, regardless of source:

```go
// Validator checks all backends
for _, backend := range registry.GetAllBackends() {
    result := validator.Check(backend.Address, backend.Port, healthCheckConfig)
    registry.UpdateValidationResult(backend, result)
}
```

**Authority hierarchy** (unchanged from ADR-015):
1. Human override (API) - highest
2. External tool override (API)
3. **Overwatch external validation** - always wins over agent claims
4. Agent health claim - lowest

#### 5. Server CRUD API

**New endpoints**:

```bash
# List all servers for a service
GET /api/v1/services/{service}/servers

# Add server (API registration)
POST /api/v1/services/{service}/servers
{
  "address": "10.0.1.50",
  "port": 8080,
  "weight": 100,
  "region": "us-east"
}

# Update server weight
PATCH /api/v1/services/{service}/servers/{address}:{port}
{
  "weight": 150
}

# Remove server
DELETE /api/v1/services/{service}/servers/{address}:{port}

# Get server details
GET /api/v1/services/{service}/servers/{address}:{port}
```

**Response includes registration source**:
```json
{
  "service": "webapp.example.com",
  "address": "10.0.1.10",
  "port": 8080,
  "weight": 100,
  "region": "us-east",
  "source": "agent",
  "agent_id": "agent-us-east-1",
  "healthy": true,
  "effective_status": "healthy",
  "agent_last_seen": "2025-12-18T10:30:00Z",
  "validation_result": "healthy",
  "last_validated": "2025-12-18T10:29:45Z"
}
```

### Configuration Changes

#### Static Server Configuration (BREAKING CHANGE)

**Old (v1.0.x) - NO LONGER VALID**:
```yaml
regions:
  - name: us-east
    servers:
      - address: 10.0.1.10
        port: 8080
```

**New (v1.1.0) - REQUIRED**:
```yaml
regions:
  - name: us-east
    servers:
      - address: 10.0.1.10
        port: 8080
        weight: 100
        service: webapp.example.com  # ← REQUIRED
```

#### Agent Configuration (UNCHANGED)

Agent backends already have the `service` field:
```yaml
agent:
  backends:
    - service: webapp.example.com  # ✓ Already correct
      address: 127.0.0.1
      port: 8080
```

#### Domain Configuration (UNCHANGED)

Domains still reference regions:
```yaml
domains:
  - name: webapp.example.com
    regions:
      - us-east
    routing_algorithm: weighted
```

**But now**: Only servers where `service == "webapp.example.com"` are included in the domain's server pool.

### Migration Guide (v1.0.x → v1.1.0)

**Step 1**: Add `service` field to all static servers in config:
```yaml
# Before
servers:
  - address: 10.0.1.10
    port: 8080
    weight: 100

# After
servers:
  - address: 10.0.1.10
    port: 8080
    weight: 100
    service: webapp.example.com  # Add this
```

**Step 2**: Validate configuration:
```bash
opengslb validate --config /etc/opengslb/overwatch.yaml
```

**Step 3**: Upgrade binary

**Step 4**: Restart service

**Validation**: Configuration without `service` field will **fail validation** with clear error message.

### Consequences

#### Positive

✅ **Unified architecture**: Single source of truth for all servers
✅ **Service-to-domain mapping**: Explicit, prevents misconfiguration
✅ **Dynamic DNS**: Agent-registered servers immediately available in DNS
✅ **API-driven ops**: Full CRUD for server management
✅ **Unified health tracking**: One validation system for all servers
✅ **Better observability**: Registration source visible in metrics/API
✅ **Predictable behavior**: Clear data flow from registration → validation → DNS

#### Negative (Mitigated)

⚠️ **Breaking change**: Requires `service` field on all static servers
→ **Mitigation**: Clear error messages, validation tool, migration guide

⚠️ **More coupling**: DNS registry now updated by multiple sources
→ **Mitigation**: Thread-safe registry with clear ownership, audit logging

⚠️ **Config verbosity**: `service` field adds lines to config
→ **Mitigation**: Necessary explicitness, prevents errors

### Implementation Checklist

- [x] Update `Server` struct with required `service` field
- [ ] Add dynamic registration methods to DNS Registry
- [ ] Wire agent heartbeat to DNS Registry updates
- [ ] Extend Backend Registry with `Source` field
- [ ] Add Server CRUD API endpoints
- [ ] Update all demo configurations
- [ ] Update documentation (config reference, deployment guides)
- [ ] Update version to 1.1.0
- [ ] Add config validation with clear error messages
- [ ] Test all three registration methods

### Related ADRs

- **ADR-015**: Agent-Overwatch Architecture - Established agent registration via gossip
- **ADR-004**: Configuration via YAML Files - Extended with service field requirement

---

## Document History

| Date | ADR | Change |
|------|-----|--------|
| 2024-11 | 001-008 | Initial architecture decisions |
| 2024-12 | 009-011 | Sprint 2/3 decisions |
| 2025-04 | 012-014 | Distributed architecture (Raft-based) |
| 2025-12 | 015 | **Agent-Overwatch architecture** (supersedes 003, 007, 012, 013, 014) |
| 2025-12 | 016 | **Unified Server Registration** (v1.1.0 breaking changes) |
