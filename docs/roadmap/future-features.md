# OpenGSLB Future Features Roadmap

This document captures planned features that have been identified but deferred to future sprints. Each feature includes context on why it was deferred and any architectural considerations.

**Last Updated**: December 2025 (Post-Sprint 3)

---

## Recently Completed (Sprint 3)

The following features were previously on this roadmap and are now complete:

| Feature | Completed In | Notes |
|---------|--------------|-------|
| Hot Reload (SIGHUP) | Sprint 3 | Full implementation with validation |
| TCP Health Checks | Sprint 3 | Connection-based verification |
| Weighted Routing | Sprint 3 | Proportional traffic distribution |
| Active/Standby (Failover) Routing | Sprint 3 | Priority-based with auto-recovery |
| AAAA Record Support | Sprint 3 | Full IPv6 support |
| Health Status API | Sprint 3 | REST API with security controls |

---

## Configuration & Operations

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

### Health Check Consensus

**Priority:** Low  
**Target:** Phase 4 or later  
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

### gRPC Health Checks

**Priority:** Low  
**Target:** Phase 4 or later  
**Identified:** Future ideas brainstorming

**Description:**  
Support gRPC health checking protocol (grpc.health.v1.Health) for microservices.

**Implementation Notes:**
- Add `GRPCChecker` implementing `health.Checker` interface
- Config: `type: grpc` in health_check section
- Use standard gRPC health checking protocol

**Why Deferred:**  
HTTP and TCP cover most use cases. gRPC is a specialized need.

---

## Routing Algorithms

### Geolocation Routing

**Priority:** High  
**Target:** Sprint 4  
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
- How to handle EDNS Client Subnet for accurate client location?

---

### Latency-Based Routing

**Priority:** Medium  
**Target:** Sprint 4 or Phase 3  
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

### Capacity-Aware Routing

**Priority:** Low  
**Target:** Phase 4 or later  
**Identified:** Future ideas brainstorming

**Description:**  
Route based on current server load/capacity reported via health checks.

**Implementation Notes:**
- Custom header in health check response (`X-Capacity: 75`)
- Integrate capacity into routing decisions
- Combine with weighted routing

---

## Observability

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
**Target:** Sprint 4  
**Identified:** Sprint planning

**Description:**  
Pre-built Grafana dashboards for OpenGSLB monitoring.

**Planned Dashboards:**
- Overview dashboard (query rate, error rate, healthy servers)
- Health check dashboard (per-region, per-server health status)
- Routing decisions dashboard (algorithm distribution, failover events)

---

## High Availability

### Keepalived Integration (VIP)

**Priority:** Medium  
**Target:** Phase 4 or 5  
**Identified:** Project summary

**Description:**  
High availability for OpenGSLB itself using keepalived and virtual IP.

**Implementation Notes:**
- Documentation for keepalived configuration
- Health check script for keepalived
- VIP failover between OpenGSLB instances

---

### Native Clustering with Raft

**Priority:** Low  
**Target:** Phase 5 or later  
**Identified:** Future ideas brainstorming

**Description:**  
Built-in Raft consensus for leader election and state replication.

**Why Deferred:**  
Significant complexity. VIP-based HA covers most needs initially.

---

## DNS Enhancements

### EDNS Client Subnet (ECS) Support

**Priority:** Medium  
**Target:** Phase 4  
**Identified:** Future ideas brainstorming

**Description:**  
Implement RFC 7871 EDNS Client Subnet for accurate geolocation when clients use public resolvers.

**Implementation Notes:**
- Parse ECS option from DNS queries
- Use client subnet instead of resolver IP for geo decisions
- Privacy implications (some resolvers strip ECS intentionally)

---

### DNS-over-HTTPS (DoH) / DNS-over-TLS (DoT)

**Priority:** Low  
**Target:** Phase 5 or later  
**Identified:** Future ideas brainstorming

**Description:**  
Implement RFC 8484 (DoH) and RFC 7858 (DoT) for encrypted DNS transport.

**Why Deferred:**  
- Adds operational complexity (certificate management)
- May conflict with "simple deployment" goal
- Could be optional module

---

### CNAME Record Support

**Priority:** Low  
**Target:** Phase 4 or later  
**Identified:** Technical debt

**Description:**  
Support CNAME records in addition to A/AAAA.

---

## API & Integration

### Ansible Module

**Priority:** Low  
**Target:** Phase 5  
**Identified:** Project summary (API-first design for Ansible)

**Description:**  
Ansible module for managing OpenGSLB configuration.

---

### Terraform Provider

**Priority:** Low  
**Target:** Phase 5  
**Identified:** Future ideas brainstorming

**Description:**  
Terraform provider for OpenGSLB configuration.

---

### Kubernetes Operator

**Priority:** Medium  
**Target:** Phase 5  
**Identified:** Project summary

**Description:**  
Operator that manages OpenGSLB configuration via Custom Resource Definitions.

**Example CRD:**
```yaml
apiVersion: gslb.opengslb.io/v1
kind: GlobalService
metadata:
  name: my-app
spec:
  domain: app.example.com
  routingAlgorithm: weighted
  regions:
    - name: us-east
      endpoints:
        - address: 10.0.1.10
          port: 80
          weight: 100
```

---

### Web UI Dashboard

**Priority:** Medium  
**Target:** Phase 4 or 5  
**Identified:** Future ideas brainstorming

**Description:**  
Read-only web dashboard showing domains, servers, health status, and recent routing decisions.

**Considerations:**
- Keep it simple (server-rendered HTML or minimal JS)
- Security of dashboard endpoint
- Could be separate optional component

---

## Document History

| Date | Author | Changes |
|------|--------|---------|
| 2025-12-02 | Logan Ross | Initial creation with hot reload and config includes |
| 2025-12-02 | Logan Ross | Added OpenTelemetry integration |
| 2025-12-05 | Logan Ross | Sprint 3 completion - marked completed features, reorganized priorities |