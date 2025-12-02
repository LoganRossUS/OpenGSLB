# OpenGSLB Future Features Roadmap

This document captures planned features that have been identified but deferred to future sprints. Each feature includes context on why it was deferred and any architectural considerations.

---

## Configuration & Operations

### Hot Reload (SIGHUP)

**Priority:** High  
**Target:** Sprint 4 (Monitoring & Operations) or Sprint 5 (Production Readiness)  
**Identified:** 2025-12-02 (Sprint 2 manual testing)

**Description:**  
Allow operators to reload configuration without restarting the application, similar to `nginx -s reload`.

**User Story:**  
As an operator, I want to update the configuration and apply changes without downtime so that I can add/remove servers and domains without disrupting service.

**Implementation Notes:**
- Add SIGHUP signal handler in `main.go`
- Add `Reload()` method to `Application` struct
- Re-read and validate config before applying
- Gracefully transition components:
  - Health manager: Add new servers, remove old ones, preserve state for unchanged
  - DNS registry: Update domain entries atomically
  - Router: No state changes needed (stateless selection)
- Handle validation failures (reject bad config, keep running with old)

**Open Questions:**
- Should health status be preserved for unchanged servers?
- How to report reload success/failure to operator? (log, exit code, status endpoint?)
- Should we support `--reload` CLI flag in addition to SIGHUP?

**Why Deferred:**  
Current architecture supports this without refactoring. It's an operational feature that isn't needed until production deployment.

---

### Configuration Includes

**Priority:** Medium  
**Target:** Sprint 3 or later  
**Identified:** 2025-12-02 (Sprint 2 manual testing)

**Description:**  
Support including external configuration files to avoid one large monolithic config file.

**User Story:**  
As an operator managing many regions and domains, I want to split configuration into multiple files so that I can organize and manage them independently.

**Proposed Syntax:**
```yaml
# /etc/opengslb/config.yaml
dns:
  listen_address: ":53"
  default_ttl: 60

includes:
  - /etc/opengslb/regions/*.yaml
  - /etc/opengslb/domains/*.yaml

logging:
  level: info
```

```yaml
# /etc/opengslb/regions/us-east.yaml
regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 80
    health_check:
      type: http
      interval: 30s
      path: /health
```

**Implementation Notes:**
- Add `Includes []string` field to root config
- Modify `config.Load()` to:
  1. Parse main config
  2. Expand glob patterns in `includes`
  3. Parse each included file
  4. Merge into main config (append to slices)
  5. Validate merged result
- Apply same permission checks to included files
- Detect circular includes
- Improve error messages to include file:line context

**Open Questions:**
- Merge semantics: append only, or allow override?
- Should included files support their own `includes`? (nested)
- Glob pattern support: just `*` or full glob syntax?

**Why Deferred:**  
Purely additive feature that doesn't affect core architecture. Current flat config works fine for development and small deployments.

---

## Health Checking

### TCP Health Checks

**Priority:** Medium  
**Target:** Sprint 2 (stretch) or Sprint 3  
**Identified:** Sprint 2 planning

**Description:**  
Support TCP connection health checks for services that don't expose HTTP endpoints.

**User Story:**  
As an operator, I want to health check TCP services (databases, custom protocols) so that I can use OpenGSLB for non-HTTP backends.

**Implementation Notes:**
- Add `TCPChecker` implementing `health.Checker` interface
- Config: `type: tcp` in health_check section
- Check: successful TCP connect within timeout = healthy
- No request/response validation (connect-only)

**Why Deferred:**  
HTTP covers most use cases. TCP is a straightforward addition when needed.

---

### Health Check Consensus

**Priority:** Low  
**Target:** Phase 3 or later  
**Identified:** ADR-003, Sprint planning

**Description:**  
Distributed health checkers that require consensus before marking a server unhealthy.

**User Story:**  
As an operator with multi-region deployment, I want health decisions to be based on checks from multiple vantage points so that regional network issues don't cause false positives.

**Implementation Notes:**
- Requires distributed deployment (multiple OpenGSLB instances)
- Gossip protocol for sharing health check results (hashicorp/memberlist)
- Configurable consensus threshold (e.g., 2/3 checkers agree)
- Significant complexity increase

**Why Deferred:**  
Requires distributed architecture. Single-node deployment must be solid first.

---

## Routing Algorithms

### Weighted Routing

**Priority:** High  
**Target:** Sprint 3  
**Identified:** Sprint 2 planning, ADR-005

**Description:**  
Route traffic based on server weights for proportional distribution.

**User Story:**  
As an operator, I want to send more traffic to higher-capacity servers so that I can optimize resource utilization.

**Implementation Notes:**
- New `WeightedRouter` implementing `dns.Router` interface
- Use server `Weight` field from config (already exists)
- Algorithm: weighted random selection or weighted round-robin

---

### Geolocation Routing

**Priority:** Medium  
**Target:** Phase 3  
**Identified:** Project planning, ADR-005

**Description:**  
Route clients to nearest region based on IP geolocation.

**User Story:**  
As an operator, I want clients to be routed to the nearest datacenter so that latency is minimized.

**Implementation Notes:**
- GeoIP database integration (MaxMind GeoIP2)
- Map client IP to region
- Fall back to round-robin if geo lookup fails
- Database update mechanism

**Open Questions:**
- MaxMind vs alternative GeoIP provider?
- License implications for GeoIP database?

---

### Latency-Based Routing

**Priority:** Medium  
**Target:** Phase 3  
**Identified:** Project planning

**Description:**  
Route to server with lowest measured latency.

**User Story:**  
As an operator, I want traffic routed to the fastest responding server so that users get optimal performance.

**Implementation Notes:**
- Health checks already measure latency
- Expose latency data to router
- Selection: lowest latency or weighted by inverse latency
- Consider: latency from checker location vs client location

---

## Observability

### Prometheus Metrics

**Priority:** High  
**Target:** Sprint 2 Story 6 (in progress)  
**Identified:** Sprint 2 planning

**Description:**  
Expose operational metrics via Prometheus endpoint.

**Planned Metrics:**
- `opengslb_dns_queries_total{domain, type, status}`
- `opengslb_health_check_results_total{region, server, result}`
- `opengslb_routing_decisions_total{domain, algorithm, server}`
- `opengslb_health_check_latency_seconds{region, server}`

---

### Grafana Dashboards

**Priority:** Medium  
**Target:** Phase 4  
**Identified:** Sprint planning

**Description:**  
Pre-built Grafana dashboards for OpenGSLB monitoring.

---

## High Availability

### Active/Standby Mode

**Priority:** High  
**Target:** Phase 3 or 4  
**Identified:** Project summary, ADR-005

**Description:**  
Support active/standby failover for regions, not just round-robin.

**User Story:**  
As an operator, I want a primary region to handle all traffic until it fails, then failover to secondary so that I can have predictable traffic patterns.

---

### Keepalived Integration (VIP)

**Priority:** Medium  
**Target:** Phase 4 or 5  
**Identified:** Project summary

**Description:**  
High availability for OpenGSLB itself using keepalived and virtual IP.

---

## API & Integration

### REST API for Runtime Management

**Priority:** Low  
**Target:** Phase 4 or later  
**Identified:** General best practice

**Description:**  
REST API for querying status, triggering reloads, and runtime changes.

**Potential Endpoints:**
- `GET /api/v1/health` - OpenGSLB health
- `GET /api/v1/servers` - List servers with health status
- `GET /api/v1/domains` - List configured domains
- `POST /api/v1/reload` - Trigger config reload

---

### Ansible Module

**Priority:** Low  
**Target:** Phase 5  
**Identified:** Project summary (API-first design for Ansible)

**Description:**  
Ansible module for managing OpenGSLB configuration.

---

## Document History

| Date | Author | Changes |
|------|--------|---------|
| 2025-12-02 | Logan Ross | Initial creation with hot reload and config includes |
