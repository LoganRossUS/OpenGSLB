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
- Signal handler for SIGHUP
- Re-parse and validate config before applying
- Atomic swap of configuration in running components
- Components need `Reload(cfg)` method or config subscription pattern
- Log old vs new config diff for audit trail

**Affected Components:**
- `pkg/config` - Add reload capability
- `pkg/dns` - Update domain registry
- `pkg/health` - Add/remove health checkers
- `pkg/routing` - Update server pools

**Why Deferred:**  
Core functionality must be stable before adding runtime reconfiguration. Restart-based config changes acceptable for initial release.

---

### Configuration Includes

**Priority:** Medium  
**Target:** Sprint 4 or later  
**Identified:** 2025-12-02 (Sprint 2 discussion)

**Description:**  
Allow splitting configuration across multiple files for better organization.

**User Story:**  
As an operator with many domains and regions, I want to organize configuration into separate files so that different teams can manage their own domains.

**Example:**
```yaml
# config.yaml
dns:
  listen_address: ":53"

includes:
  - regions/*.yaml
  - domains/*.yaml
```

**Implementation Notes:**
- Add `Includes []string` to config types
- Expand globs in `config.Load()`
- Parse each included file
- Merge into main config (append to slices)
- Validate merged result
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

### OpenTelemetry Integration

**Priority:** Low  
**Target:** Phase 4 or later  
**Identified:** 2025-12-02 (Sprint 2 Story 6 discussion)

**Description:**  
Export logs, metrics, and traces via OpenTelemetry Protocol (OTLP) for unified observability.

**User Story:**  
As an operator using OpenTelemetry, I want OpenGSLB to export telemetry via OTLP so that I can correlate logs, metrics, and traces in my existing observability stack.

**Implementation Notes:**
- Add `go.opentelemetry.io/otel` dependencies
- OTLP exporter for logs (bridge from slog)
- OTLP exporter for metrics (alongside or replacing Prometheus)
- Distributed tracing for DNS query flow
- Configuration for OTel Collector endpoint

**Example Config:**
```yaml
telemetry:
  otlp:
    enabled: true
    endpoint: "localhost:4317"
    insecure: true  # For non-TLS collector
```

**Why Deferred:**  
- Adds operational complexity (requires OTel Collector)
- Conflicts with "no external dependencies" value proposition for simple deployments
- JSON logging + Prometheus metrics cover 90% of enterprise needs
- Can be added later as optional feature without breaking existing deployments

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
**Target:** Sprint 3 or Phase 4  
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
| 2025-12-02 | Logan Ross | Added OpenTelemetry integration |