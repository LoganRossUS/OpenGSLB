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

**Consequences**: 
- TTL affects failover speed
- Clients must respect DNS TTL
- Cannot handle session persistence at DNS level

---

## ⚠️ ADR-003: Health Check Architecture
**Status**: SUPERSEDED by ADR-012

> **This decision has been superseded.** The centralized state manager model described below has been replaced by a distributed agent architecture with Raft-elected leadership and gossip-based health propagation. See ADR-012 for the current architecture.

**Original Context**: Need reliable health checking of backend servers across regions.

**Original Decision**: Implement distributed health checkers that report to centralized state manager.

**Original Rationale**:
- Distributed checks provide regional perspective
- Centralized state prevents split-brain scenarios
- Allows for sophisticated health evaluation (consensus-based)

**Why Superseded**: The "centralized state manager" created a single point of failure. ADR-012 introduces Raft consensus among candidate nodes, eliminating the need for a single centralized manager while still preventing split-brain through quorum-based decisions.

---

## ADR-004: Configuration via YAML Files
**Status**: Accepted (Amended by ADR-013)

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

**Amendment (ADR-013)**: YAML remains the source of truth for structural configuration (domains, regions, base settings). Runtime state and dynamic overrides are stored in the embedded KV store. See ADR-013 for details.

---

## ADR-005: Pluggable Routing Algorithms
**Status**: Accepted

**Context**: Different use cases require different routing strategies.

**Decision**: Implement a strategy pattern for routing algorithms with a pluggable interface.

**Rationale**:
- Flexibility to add new algorithms without core changes
- Easy to test algorithms in isolation
- Can switch algorithms per domain/service
- Supports weighted combinations of algorithms

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
- Metrics endpoint needs to be secured
- Need to define useful metrics and labels
- Should implement metric cardinality limits

---

## ⚠️ ADR-007: Separate Control and Data Planes
**Status**: SUPERSEDED by ADR-012

> **This decision has been superseded.** The strict separation of control and data planes has been replaced by a unified candidate node model where the Raft-elected leader handles both DNS queries (data plane) and cluster coordination (control plane). See ADR-012 for the current architecture.

**Original Context**: Need to ensure GSLB system availability even during configuration updates.

**Original Decision**: Separate control plane (configuration, health checks) from data plane (DNS responses).

**Original Rationale**:
- Data plane can continue operating if control plane has issues
- Easier to scale components independently
- Better security isolation
- Clearer system boundaries

**Why Superseded**: While the separation made sense for a single-node design, it created operational complexity for distributed deployments. ADR-012's approach ensures high availability through Raft consensus—if the leader fails, a new leader is elected within seconds and immediately begins serving DNS. The unified model is simpler to deploy and reason about.

---

## ADR-008: TTL-Based Failover Strategy
**Status**: Accepted

**Context**: DNS caching affects failover speed.

**Decision**: Use short TTLs (30-60 seconds) for DNS responses, with health-check-based updates.

**Rationale**:
- Balance between failover speed and DNS query load
- Clients will update within reasonable timeframe
- Health checks can update more frequently than TTL
- Reduces impact of stale DNS caches

**Consequences**:
- Higher DNS query volume
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

When `return_last_healthy: true`, the system will cache and return the last IP address that passed health checks, even after that server becomes unhealthy.

**Rationale**:
- SERVFAIL is the correct DNS response when the server cannot provide an authoritative answer
- Some operators prefer degraded service over no service ("limp mode")
- Making it configurable allows operators to choose based on their requirements
- Default to SERVFAIL as it's more honest and helps surface issues quickly

**Consequences**:
- Must maintain last-known-good state per domain
- Operators must explicitly opt-in to stale responses
- Monitoring should alert when serving stale responses
- Stale responses should be logged for operational visibility

---

## ADR-010: DNS Library Selection
**Status**: Accepted

**Decision**: Use github.com/miekg/dns v1.x

**Rationale**:
- Industry standard (15,000+ importers including CoreDNS/Kubernetes)
- Active maintenance with security updates
- Stable API suitable for our A/AAAA record needs
- v2 (codeberg.org/miekg/dns) exists but has zero production adoption

**Review Date**: Q4 2026 - reassess if v2 reaches maturity

---

## ADR-011: Router Terminology for Server Selection
**Status**: Accepted

**Context**: OpenGSLB is an authoritative DNS server that returns A records pointing to backend servers. It does not route network traffic - clients receive an IP address and connect directly to backends. We needed terminology for the component that selects which server IP to return in DNS responses.

**Decision**: Use "Router" to describe the server selection component, with clear documentation that this refers to *DNS response routing* (selecting which IP to return), not network traffic routing.

**Terminology**:
- **Router**: The component that selects which backend server IP to include in a DNS response
- **Route()**: The method that performs server selection from a pool of candidates
- **Routing Algorithm**: The strategy used for selection (round-robin, weighted, geolocation, etc.)

**Rationale**:
- "Routing" is commonly used in load balancing contexts to describe request distribution decisions
- The term aligns with the project documentation and Sprint 2 planning materials
- Alternative terms considered:
  - `Selector` / `ServerSelector` - more literal but less common in load balancing literature
  - `Balancer` / `LoadBalancer` - implies traffic handling, not just DNS
  - `Picker` - too informal for the codebase style
- The interface is already defined in `pkg/dns/handler.go` and changing it would require modifications to merged code

**Interface Definition** (in `pkg/dns/handler.go`):
```go
// Router selects a server from a pool of servers.
// This interface will be implemented by routing algorithms in pkg/routing.
type Router interface {
    Route(ctx context.Context, servers []ServerInfo) (*ServerInfo, error)
}
```

**Important Clarification**: The Router does NOT:
- Handle network traffic
- Proxy requests
- Manage connections to backends

The Router ONLY:
- Receives a pre-filtered list of healthy servers from the DNS handler
- Selects one server based on its algorithm
- Returns the selected server for inclusion in the DNS response

**Consequences**:
- Documentation and code comments should clarify "routing" means server selection
- New contributors should understand this is DNS-level decision making
- The `pkg/routing/` package contains selection algorithms, not network routing logic

---

## ADR-012: Distributed Agent Architecture & High-Availability Control Plane
**Status**: Accepted  
**Date**: 2025-04-08  
**Supersedes**: ADR-003, ADR-007

**Context**: OpenGSLB started as a centralized, single-binary authoritative DNS server with health checking and routing. As the project evolved toward multi-region, multi-cloud, and enterprise-grade deployments, the limitations of a single active control plane became clear:

- A single node answering queries becomes both data plane and control plane → single point of failure
- No safe way to handle an unhealthy leader without dropping queries
- Scaling beyond a few regions introduces polling storms and latency
- Health checks from a single vantage point miss regional network issues

**The Vision**: OpenGSLB should be **predictive from the inside** (agents know they're about to fail based on local signals) while also being **reactive from the outside** (overwatch validates agent self-assessment and can veto). This dual-perspective approach is what differentiates OpenGSLB from other GSLB solutions.

**Decision**: OpenGSLB adopts a fully distributed agent architecture with Raft-elected overwatch leadership.

### Core Principles

1. **One binary, two runtime modes**
   - `--mode=standalone` → Single-node operation (current behavior, default for simplicity)
   - `--mode=cluster` → Distributed mode with Raft consensus and gossip

2. **In cluster mode, all nodes are peers that can become overwatch**
   - No static role assignment
   - Raft election determines current overwatch (leader)
   - Any node can serve DNS queries, but only the leader's responses are authoritative for routing decisions

3. **Anycast VIP advertised by all cluster nodes**
   - Same VIP (e.g., `10.99.99.1`) bound on all nodes
   - Only the Raft-elected leader answers DNS queries on the anycast VIP
   - Non-leaders drop packets or return `REFUSED`
   - Eliminates "unhealthy leader still answering" problem

4. **Predictive + Reactive Health Model**
   - **Predictive (Agent-side)**: Nodes monitor local CPU, memory, error rates, custom hooks → signal "bleed me" before failure
   - **Reactive (Overwatch-side)**: Leader performs cross-DC probes, validates agent claims, can override/veto
   - Gossip protocol (hashicorp/memberlist) propagates health events

5. **Raft consensus for cluster state**
   - Leader election among cluster nodes
   - Replicated KV store for runtime state (see ADR-013)
   - Survives node, rack, AZ, region, and cloud provider outages

### Deployment Modes

| Mode | Use Case | Nodes | HA |
|------|----------|-------|-----|
| `standalone` | Development, simple deployments, testing | 1 | No |
| `cluster` | Production, multi-region, enterprise | 3-7 (odd) | Yes |

### Failover Scenarios

| Scenario | Behavior | Recovery Time |
|----------|----------|---------------|
| Leader node crashes | Raft detects heartbeat loss → new leader elected → DNS listener activated | ≤2s |
| Entire AZ fails | Nodes in that AZ disappear → remaining nodes re-elect leader | 1-3s |
| Leader unhealthy but reachable | Raft demotes (no heartbeat) → new leader takes over | ≤2s |
| Network partition (split-brain) | Minority side loses quorum → stops answering DNS (safe) | Immediate |
| Agent predicts failure (CPU 98%) | Agent signals "bleed me" → leader reduces weight over 30s | Graceful |

### Enabled Features

| Feature | Enabled By | Status |
|---------|-----------|--------|
| Zero-downtime control plane | Raft leader election | Planned (Sprint 4) |
| Predictive health (local signals) | Agent → gossip → overwatch | Planned (Sprint 4) |
| External veto (cross-DC validation) | Overwatch probes + override logic | Planned (Sprint 4) |
| Dynamic service auto-registration | Agent → gossip → KV store | Planned |
| Embedded KV store | Raft-replicated (see ADR-013) | Planned |
| Multi-DC federation | Raft + WAN gossip | Future |
| Web UI dashboard | Leader serves /ui | Future |

### Implementation Phases

| Phase | Work | Effort |
|-------|------|--------|
| 1 | Add `--mode` flag, refactor for mode-aware startup | 1 day |
| 2 | Integrate `hashicorp/raft` + `raft-boltdb` in `pkg/cluster` | 3-4 days |
| 3 | Leader guard in `ServeDNS` | ½ day |
| 4 | Bind anycast VIP on all cluster nodes at startup | ½ day |
| 5 | Gossip health events via memberlist | 4-5 days |
| 6 | Predictive health signals (CPU, memory, error rate) | 2 days |
| 7 | External veto/override logic | 2 days |

**Total**: ~3 engineer-weeks for production-ready HA

### Consequences

**Positive**:
- True zero-downtime control plane
- No manual intervention required for failover
- Single binary remains the deployment artifact
- Natural path to Consul-rivaling features
- Enables safe anycast deployment across multiple clouds
- Predictive + reactive model catches failures before and as they happen

**Negative (Mitigated)**:
- Higher memory in cluster mode (~80MB vs ~30MB) → acceptable for control plane
- Requires odd number of nodes for Raft quorum → standard practice
- More complex debugging (Raft logs) → offset by reliability gains
- Standalone mode preserves simple deployment option

---

## ADR-013: Hybrid Configuration & KV Store Strategy
**Status**: Accepted  
**Date**: 2025-04-08  
**Amends**: ADR-004

**Context**: With the distributed architecture (ADR-012), we need to decide how configuration and runtime state are stored and synchronized across nodes. Options range from "YAML only" to "KV store for everything" (Consul-style).

**Decision**: Adopt a hybrid model where YAML defines structural configuration and the embedded KV store holds runtime state and dynamic overrides.

### Data Partitioning

| Data Type | Storage | Mutable At Runtime | Example |
|-----------|---------|-------------------|---------|
| Domain definitions | YAML | No (requires reload) | `domains: [{name: app.example.com}]` |
| Region definitions | YAML | No (requires reload) | `regions: [{name: us-east-1}]` |
| Static server definitions | YAML | No (requires reload) | Servers listed in config file |
| DNS settings | YAML | No (requires reload) | `dns: {listen_address: ":53"}` |
| Health check state | KV | Yes (continuous) | `health/us-east-1/10.0.1.10` |
| Dynamic server registration | KV | Yes (agents register) | `services/web/10.0.2.50:8080` |
| Weight overrides | KV | Yes (API/CLI) | `overrides/us-east-1/10.0.1.10/weight` |
| Leader state | KV | Yes (Raft-managed) | `cluster/leader` |
| Agent metadata | KV | Yes (gossip) | `agents/node-1/cpu_percent` |

### Precedence Rules

When both YAML and KV store have relevant data:

1. **KV overrides win** for: weights, enabled/disabled status, dynamic registrations
2. **YAML defines structure**: domains, regions, base configuration
3. **Runtime state is KV-only**: health, leader election, agent metrics

### Configuration Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        Startup Flow                              │
├─────────────────────────────────────────────────────────────────┤
│  YAML Files ──→ Config Loader ──→ Validation ──→ In-Memory     │
│                                                      │          │
│                                         ┌────────────┘          │
│                                         ▼                       │
│                              ┌──────────────────┐               │
│                              │   KV Store       │               │
│                              │  (Raft-backed)   │               │
│                              └────────┬─────────┘               │
│                                       │                         │
│  ┌────────────────────────────────────┼─────────────────────┐   │
│  │                Runtime Flow        │                     │   │
│  ├────────────────────────────────────┼─────────────────────┤   │
│  │  Agent Health ──→ Gossip ──→ KV    │                     │   │
│  │  API Overrides ──────────────→ KV  │                     │   │
│  │  Dynamic Registration ───────→ KV  │                     │   │
│  │                                    ▼                     │   │
│  │                           DNS Handler                    │   │
│  │                    (merges YAML + KV state)              │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### SIGHUP Behavior (Hot Reload)

1. Re-read all YAML files (including includes)
2. Validate new configuration
3. If valid: update in-memory config, preserve KV state
4. If invalid: reject, log error, continue with old config
5. KV overrides remain in effect (not cleared by reload)

### KV Store Implementation

- **Engine**: bbolt (embedded, single-file) for standalone; Raft-replicated for cluster mode
- **No external dependencies**: aligns with self-hosted philosophy
- **Backup**: single file copy (standalone) or Raft snapshot (cluster)

### Example: Weight Override via API

```bash
# YAML defines base weight of 100
# Operator wants to drain server for maintenance

$ curl -X PUT http://localhost:9090/api/v1/overrides/us-east-1/10.0.1.10 \
    -d '{"weight": 0, "reason": "maintenance window"}'

# Server immediately excluded from rotation
# YAML unchanged, override persists until removed

$ curl -X DELETE http://localhost:9090/api/v1/overrides/us-east-1/10.0.1.10

# Server returns to YAML-defined weight
```

### Consequences

**Positive**:
- YAML remains human-readable, version-controllable, auditable
- Operators keep familiar workflow (edit file, SIGHUP)
- Dynamic operations possible without file changes
- Clear mental model: "YAML = what should exist, KV = what's happening now"
- No external dependencies (embedded KV)

**Negative (Mitigated)**:
- Two sources of data requires clear precedence rules → documented above
- Operators must understand override behavior → API returns effective config with source attribution
- Backup strategy differs by mode → documented in runbooks

---

## ADR-014: Runtime Mode Semantics
**Status**: Accepted  
**Date**: 2025-04-08  
**Related**: ADR-012, ADR-013

**Context**: ADR-012 introduced distributed agent architecture with Raft consensus. We need to define exact behavior differences between deployment modes, ensuring backward compatibility for users who want simple single-node deployments while enabling full clustering capabilities for enterprise users.

**Decision**: OpenGSLB supports two runtime modes selected via `--mode` flag, with clear behavioral differences.

### Mode Definitions

| Aspect | `--mode=standalone` (default) | `--mode=cluster` |
|--------|-------------------------------|------------------|
| **Target Use Case** | Development, simple deployments, single-DC | Production, multi-region, enterprise HA |
| **Nodes** | 1 | 3-7 (odd number for quorum) |
| **High Availability** | None (single point of failure) | Yes (Raft consensus) |
| **DNS Listener** | Always active | Active only on Raft leader |
| **Health Checks** | Local only (overwatch perspective) | Local + gossip from peers |
| **Predictive Health** | Disabled (no peers to notify) | Enabled (agents signal "bleed me") |
| **KV Store** | bbolt (local file) | Raft-replicated bbolt |
| **Configuration** | YAML only | YAML + KV overrides + dynamic registration |
| **API Overrides** | Supported (local effect only) | Supported (replicated to cluster) |
| **Anycast VIP** | Optional (single node) | Recommended (all nodes advertise) |

### Startup Behavior

**Standalone Mode** (`--mode=standalone` or default):
```
1. Load YAML configuration
2. Validate configuration
3. Initialize local bbolt KV store
4. Start health checkers (local perspective only)
5. Start DNS listener (always active)
6. Start metrics/API server
7. Ready to serve
```

**Cluster Mode** (`--mode=cluster`):
```
1. Load YAML configuration
2. Validate configuration
3. Initialize Raft subsystem
   - If bootstrap node: initialize new cluster
   - If joining: connect to existing cluster via --join flag
4. Wait for Raft leader election (blocks until quorum)
5. Initialize Raft-replicated KV store
6. Start health checkers
7. Start gossip protocol (memberlist)
8. If elected leader:
   - Activate DNS listener
   - Begin external health validation
9. If follower:
   - DNS listener bound but returns REFUSED
   - Participate in gossip
   - Ready to become leader if elected
10. Start metrics/API server
11. Ready to serve (leader) or ready to failover (follower)
```

### Configuration

```yaml
# Standalone mode (default) - minimal config
dns:
  listen_address: ":53"
  
# Cluster mode - additional settings
cluster:
  mode: cluster  # Can also be set via --mode flag
  node_name: node-1  # Unique identifier, defaults to hostname
  bind_address: "10.0.1.10:7946"  # Gossip/Raft bind address
  advertise_address: "10.0.1.10:7946"  # Address other nodes use to reach this node
  
  # Bootstrap configuration (first node only)
  bootstrap: true  # Set on exactly one node during initial cluster formation
  
  # Join configuration (subsequent nodes)
  join:
    - "10.0.1.11:7946"
    - "10.0.1.12:7946"
  
  # Raft settings
  raft:
    data_dir: "/var/lib/opengslb/raft"
    heartbeat_timeout: "1s"
    election_timeout: "1s"
    snapshot_interval: "120s"
    snapshot_threshold: 8192
  
  # Gossip settings
  gossip:
    encryption_key: ""  # Optional: 32-byte base64-encoded key
    
  # Anycast VIP (all cluster nodes should advertise this)
  anycast_vip: "10.99.99.1"
```

### Command Line Flags

```bash
# Standalone mode (default)
opengslb --config /etc/opengslb/config.yaml

# Explicit standalone
opengslb --mode=standalone --config /etc/opengslb/config.yaml

# Cluster mode - bootstrap first node
opengslb --mode=cluster --bootstrap --config /etc/opengslb/config.yaml

# Cluster mode - join existing cluster
opengslb --mode=cluster --join=10.0.1.10:7946,10.0.1.11:7946 --config /etc/opengslb/config.yaml
```

### Feature Availability by Mode

| Feature | Standalone | Cluster |
|---------|------------|---------|
| DNS serving (A/AAAA records) | ✅ | ✅ (leader only) |
| Round-robin routing | ✅ | ✅ |
| Weighted routing | ✅ | ✅ |
| Failover routing | ✅ | ✅ |
| Geolocation routing | ✅ | ✅ |
| Latency-based routing | ✅ | ✅ |
| HTTP/TCP health checks | ✅ | ✅ |
| Hot reload (SIGHUP) | ✅ | ✅ |
| Prometheus metrics | ✅ | ✅ |
| Health status API | ✅ | ✅ |
| Local KV store | ✅ | ✅ (Raft-replicated) |
| API weight overrides | ✅ (local) | ✅ (replicated) |
| Predictive health (agent signals) | ❌ | ✅ |
| External health veto | ❌ | ✅ |
| Dynamic service registration | ❌ | ✅ |
| Automatic failover | ❌ | ✅ |
| Leader election | ❌ | ✅ |
| Gossip protocol | ❌ | ✅ |
| Multi-node coordination | ❌ | ✅ |

### Migration Path

**Standalone → Cluster:**
1. Deploy two additional nodes in cluster mode with `--join` pointing to existing infrastructure
2. Reconfigure original node with `--mode=cluster --bootstrap`
3. Restart original node (brief downtime)
4. Other nodes join, Raft elects leader
5. Original node may or may not become leader
6. Anycast VIP ensures seamless client experience

**Cluster → Standalone:**
1. Drain traffic from cluster
2. Stop all nodes except one
3. Restart remaining node with `--mode=standalone`
4. Local bbolt retains last-known state (health resets)

### Consequences

**Positive**:
- Backward compatible—existing single-node deployments work unchanged
- Clear upgrade path to HA
- Mode-specific optimizations possible
- Users choose complexity level appropriate to their needs

**Negative (Mitigated)**:
- Two code paths to maintain → shared core, mode-specific initialization
- Documentation must cover both modes → clear separation in docs
- Testing matrix increases → CI covers both modes

---

## Document History

| Date | ADR | Change |
|------|-----|--------|
| 2024-11 | 001-008 | Initial architecture decisions |
| 2024-12 | 009-011 | Sprint 2/3 decisions |
| 2025-04 | 012 | Distributed agent architecture (supersedes 003, 007) |
| 2025-04 | 013 | Hybrid configuration & KV store strategy (amends 004) |
| 2025-04 | 014 | Runtime mode semantics (standalone vs cluster) |