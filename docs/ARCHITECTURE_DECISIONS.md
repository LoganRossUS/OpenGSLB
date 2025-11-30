# Architecture Decisions

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

## ADR-003: Health Check Architecture
**Status**: Accepted

**Context**: Need reliable health checking of backend servers across regions.

**Decision**: Implement distributed health checkers that report to centralized state manager.

**Rationale**:
- Distributed checks provide regional perspective
- Centralized state prevents split-brain scenarios
- Allows for sophisticated health evaluation (consensus-based)

**Consequences**:
- Requires state synchronization mechanism
- More complex than simple health checks
- Better accuracy and reliability

---

## ADR-004: Configuration via YAML Files
**Status**: Accepted

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

## ADR-007: Separate Control and Data Planes
**Status**: Accepted

**Context**: Need to ensure GSLB system availability even during configuration updates.

**Decision**: Separate control plane (configuration, health checks) from data plane (DNS responses).

**Rationale**:
- Data plane can continue operating if control plane has issues
- Easier to scale components independently
- Better security isolation
- Clearer system boundaries

**Consequences**:
- More complex deployment
- Need state synchronization between planes
- Better overall reliability

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
