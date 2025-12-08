# OpenGSLB Future Features Roadmap

This document captures planned features that have been identified but deferred to future sprints. Each feature includes context on why it was deferred and any architectural considerations.

**Last Updated**: April 2025 (Post-Architecture Pivot)

---

## 🎯 Architecture Pivot Notice

**As of Sprint 4, OpenGSLB has pivoted to a distributed agent architecture.**

This pivot means:
- Features are now designed for both standalone and cluster deployment modes
- Some previously planned features have been superseded by the new architecture
- Priorities have been reordered to build on the distributed foundation

See **ADR-012**, **ADR-013**, and **ADR-014** for architectural details.

---

## Recently Completed

### Sprint 3 (December 2025)

| Feature | Notes |
|---------|-------|
| Hot Reload (SIGHUP) | Full implementation with validation |
| TCP Health Checks | Connection-based verification |
| Weighted Routing | Proportional traffic distribution |
| Active/Standby (Failover) Routing | Priority-based with auto-recovery |
| AAAA Record Support | Full IPv6 support |
| Health Status API | REST API with security controls |

### Sprint 4 (April 2025) - In Progress

| Feature | Notes |
|---------|-------|
| `--mode=standalone/cluster` | Runtime mode selection (ADR-014) |
| Raft Consensus | Leader election for HA (ADR-012) |
| Leader-Only DNS | Non-leaders return REFUSED |
| Gossip Protocol | Health event propagation |
| Predictive Health | Agent-side failure prediction |
| External Veto | Overwatch validation of agent claims |
| Embedded KV Store | Raft-replicated runtime state (ADR-013) |

---

## Sprint 5: Advanced Routing (Target: May 2025)

With the distributed foundation complete, Sprint 5 delivers advanced routing algorithms designed for multi-region, multi-cloud deployments.

### Geolocation Routing

**Priority**: High  
**Estimate**: 8 story points

**Description**:  
Route clients to nearest region based on IP geolocation.

**User Story**:  
As an operator, I want clients to be routed to the nearest datacenter so that latency is minimized.

**Implementation Notes**:
- GeoIP database integration (MaxMind GeoLite2-Country)
- Region-to-country/continent mapping configuration
- Fall back to round-robin if geo lookup fails
- Graceful handling of private/unroutable IPs
- **Cluster mode**: Geo decisions made by leader, consistent across cluster

**Configuration Example**:
```yaml
geolocation:
  database_path: "/var/lib/opengslb/GeoLite2-Country.mmdb"
  default_region: us-east-1

regions:
  - name: us-east-1
    countries: ["US", "CA", "MX"]
    continents: ["NA"]
  - name: eu-west-1
    continents: ["EU"]
```

---

### Latency-Based Routing

**Priority**: High  
**Estimate**: 5 story points

**Description**:  
Route to server with lowest measured latency.

**User Story**:  
As an operator, I want traffic routed to the fastest responding server so that users get optimal performance.

**Implementation Notes**:
- Health checks already measure latency
- **Cluster mode**: Latency data aggregated via gossip from all nodes
- Selection: lowest latency with exponential moving average smoothing
- Prevents flapping between servers with similar latency

---

### Grafana Dashboard Templates

**Priority**: High  
**Estimate**: 5 story points

**Description**:  
Pre-built Grafana dashboards for OpenGSLB monitoring.

**Planned Dashboards**:
- **Overview**: Query rate, error rate, latency, healthy servers
- **Health**: Per-region, per-server health status, state transitions
- **Routing**: Algorithm distribution, failover events
- **Cluster** (new): Leader status, Raft health, gossip propagation

---

### Configuration File Includes

**Priority**: Medium  
**Estimate**: 5 story points

**Description**:  
Allow splitting configuration across multiple files for better organization.

**Example**:
```yaml
includes:
  - regions/*.yaml
  - domains/*.yaml
```

**Notes**:
- Merge semantics: append only (ADR-013)
- Works with hot-reload
- Validated before merge

---

## Sprint 6: Operational Excellence (Target: June 2025)

### CLI Management Tool

**Priority**: High  
**Estimate**: 5 story points

**Description**:  
Command-line tool for cluster management and debugging.

**Commands**:
```bash
opengslb-cli status              # Overall cluster status
opengslb-cli servers             # List servers with health
opengslb-cli domains             # List configured domains
opengslb-cli cluster members     # Show cluster membership
opengslb-cli cluster leader      # Show current leader
opengslb-cli override set ...    # Set weight override (KV)
opengslb-cli override list       # List active overrides
opengslb-cli config validate     # Validate config file
```

---

### Operational Runbooks

**Priority**: High  
**Estimate**: 3 story points

**Description**:  
Production deployment guides and incident response playbooks.

**Documents**:
- Standalone deployment guide
- Cluster deployment guide (3-node, 5-node)
- Upgrade procedures (standalone → cluster migration)
- Incident response playbook
- Capacity planning guidelines

---

### Dynamic Service Registration API

**Priority**: Medium  
**Estimate**: 5 story points

**Description**:  
Allow services to register themselves dynamically via API.

**User Story**:  
As a service owner, I want my application to register itself with OpenGSLB so that I don't need to update configuration files for every deployment.

**API Example**:
```bash
# Register service
curl -X PUT http://opengslb:9090/api/v1/services/web \
  -d '{
    "address": "10.0.1.50",
    "port": 8080,
    "region": "us-east-1",
    "health_check": {"type": "http", "path": "/health"},
    "ttl": "60s"
  }'

# Service sends heartbeats to maintain registration
curl -X POST http://opengslb:9090/api/v1/services/web/10.0.1.50:8080/heartbeat
```

**Notes**:
- Stored in KV store (ADR-013)
- Replicated via Raft in cluster mode
- TTL-based expiration if heartbeat stops

---

### EDNS Client Subnet (ECS) Support

**Priority**: Medium  
**Estimate**: 5 story points

**Description**:  
Implement RFC 7871 for accurate geolocation when clients use public DNS resolvers.

**Implementation Notes**:
- Parse ECS option from DNS queries
- Use client subnet instead of resolver IP for geo decisions
- Cache by client subnet prefix
- Privacy mode: strip ECS from upstream queries

---

## Phase 4: Enterprise Features (Target: Q3 2025)

### Multi-Datacenter Federation

**Priority**: Medium  
**Estimate**: 13 story points

**Description**:  
Connect multiple OpenGSLB clusters across datacenters with WAN gossip.

**User Story**:  
As an enterprise operator, I want separate OpenGSLB clusters per datacenter that share health information so that I can have both local autonomy and global awareness.

**Architecture**:
```
┌─────────────────┐     WAN Gossip     ┌─────────────────┐
│  DC1 Cluster    │◄──────────────────►│  DC2 Cluster    │
│  (3 nodes)      │                    │  (3 nodes)      │
│  Raft: local    │                    │  Raft: local    │
└─────────────────┘                    └─────────────────┘
         │                                      │
         └──────────────┬───────────────────────┘
                        │
                        ▼
              Global Health View
              (Cross-DC routing decisions)
```

**Notes**:
- Each DC has independent Raft cluster
- WAN gossip for cross-DC health sharing
- Local decisions with global awareness

---

### Capacity-Aware Routing

**Priority**: Medium  
**Estimate**: 5 story points

**Description**:  
Route based on current server capacity reported by agents.

**Implementation Notes**:
- Agents report capacity via predictive health system
- Custom health check header: `X-Capacity: 75`
- Routing prefers servers with available capacity
- Integrates with weighted routing

---

### Web UI Dashboard

**Priority**: Medium  
**Estimate**: 8 story points

**Description**:  
Read-only web dashboard for monitoring and debugging.

**Features**:
- Cluster overview (leader, members, health)
- Domain list with routing status
- Server health visualization
- Real-time query metrics
- Configuration viewer

**Notes**:
- Served by leader node only
- Simple server-rendered HTML (no heavy JS framework)
- Secured by API access controls

---

### gRPC Health Checks

**Priority**: Low  
**Estimate**: 3 story points

**Description**:  
Support gRPC health checking protocol for microservices.

**Implementation Notes**:
- `grpc.health.v1.Health` service check
- Config: `type: grpc` in health_check section

---

## Phase 5: Production Hardening (Target: Q4 2025)

### Rate Limiting & DDoS Protection

**Priority**: High  
**Estimate**: 5 story points

**Description**:  
Protect against DNS amplification attacks and query floods.

**Configuration Example**:
```yaml
security:
  rate_limiting:
    enabled: true
    per_ip_qps: 100
    global_qps: 50000
    burst: 50
```

---

### DNS-over-HTTPS (DoH) / DNS-over-TLS (DoT)

**Priority**: Medium  
**Estimate**: 8 story points

**Description**:  
Encrypted DNS transport per RFC 8484 (DoH) and RFC 7858 (DoT).

**Notes**:
- Optional feature (certificate management complexity)
- Served by leader only in cluster mode

---

### DNSSEC Signing

**Priority**: Low  
**Estimate**: 13 story points

**Description**:  
Sign DNS responses with DNSSEC keys.

**Notes**:
- Significant complexity (key management, rotation)
- Performance impact of signing
- Deferred until core features stable

---

### Kubernetes Operator

**Priority**: Medium  
**Estimate**: 13 story points

**Description**:  
Manage OpenGSLB via Kubernetes Custom Resource Definitions.

**Example CRD**:
```yaml
apiVersion: gslb.opengslb.io/v1
kind: GlobalService
metadata:
  name: my-app
spec:
  domain: app.example.com
  routingAlgorithm: geolocation
  regions:
    - name: us-east
      endpoints:
        - address: 10.0.1.10
          port: 80
```

---

### Terraform Provider

**Priority**: Low  
**Estimate**: 8 story points

**Description**:  
Terraform provider for infrastructure-as-code management.

---

## ⚠️ Superseded / Removed Features

The following features from the original roadmap have been superseded by the distributed architecture:

| Original Feature | Status | Replacement |
|-----------------|--------|-------------|
| Health Check Consensus | **Superseded** | Built into ADR-012 (gossip + overwatch veto) |
| Keepalived Integration (VIP) | **Superseded** | Native anycast with Raft leader election |
| Native Clustering with Raft | **Completed** | Sprint 4 (ADR-012) |

---

## Feature Priority Matrix

| Feature | User Value | Complexity | Dependency | Target |
|---------|-----------|------------|------------|--------|
| Geolocation Routing | High | Medium | Distributed foundation | Sprint 5 |
| Latency-Based Routing | High | Low | Distributed foundation | Sprint 5 |
| Grafana Dashboards | High | Low | Cluster metrics | Sprint 5 |
| CLI Tool | High | Medium | API complete | Sprint 6 |
| Dynamic Registration | Medium | Medium | KV store | Sprint 6 |
| Multi-DC Federation | High | High | Cluster stable | Phase 4 |
| Web UI | Medium | Medium | API complete | Phase 4 |
| Rate Limiting | High | Low | None | Phase 5 |
| Kubernetes Operator | Medium | High | API stable | Phase 5 |

---

## Contributing Ideas

Have an idea for OpenGSLB? Consider:

1. **Problem**: What problem does this solve?
2. **Users**: Who benefits from this feature?
3. **Mode Compatibility**: Does it work in both standalone and cluster mode?
4. **Distributed Design**: How does it leverage gossip/Raft?
5. **Alignment**: Does it fit the "predictive + reactive" philosophy?

Ideas that leverage the distributed architecture and align with OpenGSLB's dual-perspective health model are most likely to be prioritized.

---

## Document History

| Date | Author | Changes |
|------|--------|---------|
| 2025-12-02 | Logan Ross | Initial creation |
| 2025-12-05 | Logan Ross | Sprint 3 completion updates |
| 2025-04-08 | Logan Ross | **Major revision**: Architecture pivot to distributed model, reordered priorities, marked superseded features |