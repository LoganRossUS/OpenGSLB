# OpenGSLB Future Ideas

This document captures feature ideas being considered for future development. Unlike `future-features.md` which tracks committed roadmap items, this is a brainstorming space for possibilities that may or may not be implemented.

**Status**: Ideas only - not committed to roadmap  
**Last Updated**: 2025-12-02

---

## DNS Enhancements

### EDNS Client Subnet (ECS) Support

**Problem**: When clients use public DNS resolvers (8.8.8.8, 1.1.1.1), geolocation routing sees the resolver's IP, not the client's actual location.

**Solution**: Implement RFC 7871 EDNS Client Subnet. Resolvers that support ECS include the client's subnet in the query, allowing accurate geographic routing.

**Complexity**: Medium  
**Value**: High for geolocation routing accuracy

**Considerations**:
- Privacy implications (some resolvers strip ECS intentionally)
- Cache key must include client subnet
- Not all resolvers support ECS

---

### DNS-over-HTTPS (DoH) / DNS-over-TLS (DoT)

**Problem**: Traditional DNS is unencrypted, visible to network observers.

**Solution**: Implement RFC 8484 (DoH) and RFC 7858 (DoT) for encrypted DNS transport.

**Complexity**: Medium  
**Value**: Medium - increasingly expected in modern deployments

**Considerations**:
- Certificate management adds operational complexity
- DoH requires HTTP/2 support
- May conflict with "simple deployment" goal
- Could be optional module

---

### Zone Transfer (AXFR/IXFR) Support

**Problem**: Organizations may want OpenGSLB to integrate with existing DNS infrastructure.

**Solution**: Support AXFR (full) and IXFR (incremental) zone transfers, both as primary and secondary.

**Complexity**: High  
**Value**: Medium - enables hybrid deployments

**Use Cases**:
- OpenGSLB as hidden primary, traditional DNS as public-facing
- Existing DNS as primary, OpenGSLB as intelligent secondary
- Migration path from legacy DNS

---

### Split-Horizon DNS

**Problem**: Internal and external clients need different answers for the same domain.

**Solution**: Return different IP addresses based on source IP ranges (internal corporate network vs internet).

**Complexity**: Low-Medium  
**Value**: High for enterprise deployments

**Example Config**:
```yaml
domains:
  - name: app.example.com
    views:
      - name: internal
        source_ranges: ["10.0.0.0/8", "192.168.0.0/16"]
        regions: [internal-dc]
      - name: external
        source_ranges: ["0.0.0.0/0"]
        regions: [public-cloud]
```

---

### Negative Caching Controls

**Problem**: NXDOMAIN responses may be cached too long or not long enough.

**Solution**: Configurable SOA minimum TTL for negative responses, separate from positive response TTL.

**Complexity**: Low  
**Value**: Low-Medium

---

## Advanced Routing Algorithms

### Capacity-Aware Routing

**Problem**: Round-robin and weighted routing don't account for current server load.

**Solution**: Servers report capacity/load via health check responses. Routing prefers servers with available capacity.

**Complexity**: Medium  
**Value**: High for variable-load environments

**Implementation Options**:
- Custom header in health check response (`X-Capacity: 75`)
- Separate capacity endpoint
- Sidecar reporting to OpenGSLB API

---

### Session Affinity (Consistent Hashing)

**Problem**: Some applications benefit from clients consistently reaching the same backend.

**Solution**: Use consistent hashing based on client IP to prefer the same server for a given client, while still supporting failover.

**Complexity**: Medium  
**Value**: Medium

**Considerations**:
- DNS caching already provides some stickiness
- Not true session persistence (client can still hit different servers)
- Hash ring implementation for minimal disruption during server changes

---

### Canary / Traffic Splitting

**Problem**: Gradual rollouts need fine-grained traffic control beyond simple weights.

**Solution**: Explicit percentage-based traffic splitting between server groups.

**Complexity**: Medium  
**Value**: High for CI/CD integration

**Example Config**:
```yaml
domains:
  - name: app.example.com
    traffic_split:
      - group: stable
        percentage: 95
        regions: [us-east-stable]
      - group: canary
        percentage: 5
        regions: [us-east-canary]
```

---

### Time-Based Routing

**Problem**: Traffic patterns vary by time of day; different regions are preferred at different times.

**Solution**: Route differently based on time of day, day of week, or calendar schedules.

**Complexity**: Low-Medium  
**Value**: Medium

**Use Cases**:
- Follow-the-sun support routing
- Business hours vs off-hours routing
- Maintenance window avoidance

---

### Cost-Based Routing

**Problem**: Cloud regions have different costs; prefer cheaper options when performance is equal.

**Solution**: Tag servers/regions with cost tiers. When health and latency are equivalent, prefer lower-cost options.

**Complexity**: Low  
**Value**: Medium for cost-conscious deployments

---

### Explicit Failover Chains

**Problem**: Weighted routing doesn't provide predictable failover order.

**Solution**: Define explicit priority ordering: try region A first, then B, then C.

**Complexity**: Low  
**Value**: High for disaster recovery

**Example Config**:
```yaml
domains:
  - name: app.example.com
    routing_algorithm: failover
    failover_order:
      - us-east-1   # Primary
      - us-west-2   # Secondary
      - eu-west-1   # Tertiary
```

---

## Health Checking Enhancements

### gRPC Health Checks

**Problem**: Microservices using gRPC have a standard health checking protocol that HTTP checks don't cover.

**Solution**: Implement gRPC health checking protocol (grpc.health.v1.Health).

**Complexity**: Medium  
**Value**: High for gRPC-based architectures

---

### Custom Script Health Checks

**Problem**: Some health determinations require complex logic beyond HTTP/TCP.

**Solution**: Execute arbitrary scripts that return exit codes indicating health status.

**Complexity**: Low  
**Value**: Medium

**Considerations**:
- Security implications (script execution)
- Timeout enforcement critical
- Script management/deployment
- Could be disabled by default

---

### Passive Health Checking

**Problem**: Active health checks add load to backends and have inherent delays.

**Solution**: Infer health from other signals: DNS query success rates, external monitoring webhooks, or response patterns.

**Complexity**: High  
**Value**: Medium

---

### Health Check Result Sharing

**Problem**: Multiple OpenGSLB instances each probe all backends, multiplying health check traffic.

**Solution**: Share health check results between instances via gossip or shared state.

**Complexity**: Medium (part of clustering work)  
**Value**: High for scaled deployments

---

### Dependency Health Checks

**Problem**: A server may be "up" but its dependencies (database, cache) may be down.

**Solution**: Support chained health checks where a server's health depends on its dependencies.

**Complexity**: Medium  
**Value**: Medium

---

## Operations & Observability

### Control Plane REST API

**Problem**: Operational tasks require signal-based reload or configuration file changes.

**Solution**: REST API for runtime inspection and control.

**Potential Endpoints**:
```
GET  /api/v1/health          # OpenGSLB health
GET  /api/v1/domains         # List domains and their status
GET  /api/v1/servers         # List servers with health status
GET  /api/v1/servers/{id}    # Server detail with health history
POST /api/v1/reload          # Trigger config reload
POST /api/v1/servers/{id}/drain    # Remove from rotation
POST /api/v1/servers/{id}/enable   # Add back to rotation
```

**Complexity**: Medium  
**Value**: High for operational tooling

---

### Audit Logging

**Problem**: Compliance requirements need detailed records of configuration changes and manual interventions.

**Solution**: Structured audit log separate from operational logs, capturing who/what/when for all changes.

**Complexity**: Low-Medium  
**Value**: High for regulated industries

---

### Traffic Analytics

**Problem**: Operators need visibility into query patterns for capacity planning.

**Solution**: Aggregate statistics on query volume by domain, source IP range, response type, and routing decision.

**Complexity**: Medium  
**Value**: Medium

**Considerations**:
- Memory usage for aggregation
- Export format (Prometheus metrics vs separate analytics)
- Privacy considerations for source IP tracking

---

### Built-in Alerting

**Problem**: Health state changes require external monitoring to detect.

**Solution**: Webhook notifications or built-in alerting rules for health transitions.

**Complexity**: Low-Medium  
**Value**: Medium

**Considerations**:
- May duplicate Prometheus alerting capabilities
- Useful for deployments without full monitoring stack

---

### Dry-Run Mode

**Problem**: Configuration changes may have unintended effects; no way to preview.

**Solution**: Load configuration, validate, and report what would change without applying.

**Complexity**: Low  
**Value**: Medium

**Use Cases**:
- CI/CD pipeline validation
- Pre-deployment review
- Training/learning

---

### Configuration Diff on Reload

**Problem**: After hot reload, unclear what actually changed.

**Solution**: Log detailed diff between old and new configuration on reload.

**Complexity**: Low  
**Value**: Low-Medium (operational nicety)

---

## High Availability & Clustering

### Native Clustering with Raft

**Problem**: High availability requires external systems (etcd, Consul) or manual coordination.

**Solution**: Built-in Raft consensus for leader election and state replication.

**Complexity**: High  
**Value**: High - aligns with "no external dependencies" philosophy

**Considerations**:
- Significant complexity increase
- hashicorp/raft library available
- Bootstrap and membership management
- Split-brain prevention

---

### Gossip-Based Health Sharing

**Problem**: Single-point-of-view health checking can have false positives from network issues.

**Solution**: Multiple OpenGSLB instances share health observations via gossip protocol, make consensus decisions.

**Complexity**: Medium  
**Value**: High for distributed deployments

**Note**: Already identified in roadmap with hashicorp/memberlist

---

### Anycast Deployment Guide

**Problem**: True geographic distribution requires anycast, which is complex to set up.

**Solution**: Documentation and reference architecture for deploying OpenGSLB with BGP anycast.

**Complexity**: Low (documentation only)  
**Value**: Medium

---

### Quorum-Based Failover

**Problem**: Single checker deciding health can cause flapping or false failovers.

**Solution**: Require N of M checkers to agree before changing health state.

**Complexity**: Medium (depends on clustering)  
**Value**: High for reliability

---

## Security

### DNSSEC Signing

**Problem**: DNS responses can be spoofed; DNSSEC provides authentication.

**Solution**: Sign responses with DNSSEC keys.

**Complexity**: High  
**Value**: Medium-High for security-conscious deployments

**Considerations**:
- Key management complexity
- Key rotation procedures
- Performance impact of signing
- NSEC/NSEC3 for authenticated denial

---

### Rate Limiting

**Problem**: DNS amplification attacks and query floods can overwhelm the server.

**Solution**: Per-source-IP and global rate limits with configurable thresholds.

**Complexity**: Low-Medium  
**Value**: High for public-facing deployments

**Example Config**:
```yaml
security:
  rate_limiting:
    enabled: true
    per_ip_qps: 100
    global_qps: 10000
    burst: 50
```

---

### Access Control Lists

**Problem**: Some domains should only be queryable by specific networks.

**Solution**: Per-domain ACLs restricting which source IPs can query.

**Complexity**: Low  
**Value**: Medium for internal deployments

---

### mTLS for Health Checks

**Problem**: Health check endpoints may need authentication.

**Solution**: Support client certificates for health check requests.

**Complexity**: Low-Medium  
**Value**: Medium for high-security environments

---

### Query Logging with PII Controls

**Problem**: Query logs are useful for debugging but may contain sensitive client IPs.

**Solution**: Configurable anonymization (truncate IPs, hash, or omit) for query logging.

**Complexity**: Low  
**Value**: Medium for privacy-conscious deployments

---

## Integration & Ecosystem

### Kubernetes Operator

**Problem**: Kubernetes-native deployments expect Custom Resource Definitions.

**Solution**: Operator that manages OpenGSLB configuration via CRDs.

**Complexity**: High  
**Value**: High for Kubernetes shops

**Example CRD**:
```yaml
apiVersion: gslb.opengslb.io/v1
kind: GlobalService
metadata:
  name: my-app
spec:
  domain: app.example.com
  routingAlgorithm: round-robin
  regions:
    - name: us-east
      endpoints:
        - address: 10.0.1.10
          port: 80
```

---

### Terraform Provider

**Problem**: Infrastructure-as-code teams use Terraform for DNS management.

**Solution**: Terraform provider for OpenGSLB configuration.

**Complexity**: Medium  
**Value**: Medium

---

### Consul/etcd Backend

**Problem**: Large deployments may want external state storage.

**Solution**: Optional backends for configuration and state storage.

**Complexity**: Medium  
**Value**: Medium

**Considerations**:
- Conflicts with "no external dependencies" for simple deployments
- Could be optional/pluggable

---

### Service Mesh Integration

**Problem**: Service meshes handle internal traffic; external traffic needs different routing.

**Solution**: Integration points with Istio/Linkerd for external-to-mesh traffic routing.

**Complexity**: High  
**Value**: Medium

---

### Cloud Provider Health Sources

**Problem**: Cloud load balancers already perform health checks; duplicating is wasteful.

**Solution**: Import health status from AWS ALB, GCP Load Balancer, Azure Traffic Manager.

**Complexity**: Medium per provider  
**Value**: Medium for cloud deployments

---

## Developer Experience

### Configuration Schema Publishing

**Problem**: YAML configuration has no IDE support for validation or autocomplete.

**Solution**: Publish JSON Schema for configuration file; integrate with editors.

**Complexity**: Low  
**Value**: Medium

---

### Web UI Dashboard

**Problem**: Command-line only visibility into system state.

**Solution**: Read-only web dashboard showing domains, servers, health status, recent routing decisions.

**Complexity**: Medium  
**Value**: Medium-High

**Considerations**:
- Adds frontend complexity (or use simple server-rendered HTML)
- Security of dashboard endpoint
- Could be separate optional component

---

### CLI Tool

**Problem**: Operational tasks require signals, API calls, or config changes.

**Solution**: Dedicated CLI tool for common operations.

**Commands**:
```bash
opengslb-cli status              # Overall status
opengslb-cli servers             # List servers with health
opengslb-cli domains             # List domains
opengslb-cli reload              # Trigger config reload
opengslb-cli drain <server>      # Remove from rotation
opengslb-cli enable <server>     # Add to rotation
opengslb-cli config validate     # Validate config file
```

**Complexity**: Low-Medium  
**Value**: Medium

---

### Simulation Mode

**Problem**: Hard to predict how configuration changes will affect routing.

**Solution**: Feed historical query logs and see how different configurations would have routed traffic.

**Complexity**: Medium  
**Value**: Low-Medium

**Use Cases**:
- Capacity planning
- Configuration tuning
- What-if analysis

---

## Ideas Parking Lot

Lower-priority or speculative ideas that may warrant future discussion:

- **Multi-tenancy**: Isolated configuration per tenant with resource quotas
- **GraphQL API**: Alternative to REST for flexible querying
- **Plugin System**: Dynamic loading of custom routers/checkers
- **DNS Query Rewriting**: Transform queries before routing (CNAME flattening, etc.)
- **Response Policy Zones (RPZ)**: DNS-based filtering/blocking
- **Synthetic Monitoring Integration**: Use external probes (Pingdom, etc.) as health signals
- **Machine Learning Routing**: Predictive routing based on historical patterns
- **WebAssembly Plugins**: Safe custom logic execution
- **DNS Load Testing Tool**: Built-in tool for performance testing

---

## Contributing Ideas

Have an idea for OpenGSLB? Consider:

1. **Problem**: What problem does this solve?
2. **Users**: Who benefits from this feature?
3. **Complexity**: How difficult is implementation?
4. **Dependencies**: Does it require external systems?
5. **Alignment**: Does it fit the "self-hosted, no vendor lock-in" philosophy?

Ideas that align with OpenGSLB's core values (self-hosted, simple deployment, enterprise-grade, no external dependencies for basic functionality) are most likely to be prioritized.

---

## Document History

| Date | Author | Changes |
|------|--------|---------|
| 2025-12-02 | Logan Ross | Initial creation from brainstorming session |