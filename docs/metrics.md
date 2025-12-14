# OpenGSLB Metrics Reference

OpenGSLB exposes Prometheus metrics for monitoring and observability. This document describes all available metrics and how to configure them.

## Configuration

Enable the metrics endpoint in your configuration:

```yaml
metrics:
  enabled: true
  address: ":9090"  # Default port
```

Metrics are served at `http://<address>/metrics` in Prometheus text format.

A health check endpoint is also available at `http://<address>/health`.

## Metrics Reference

### DNS Metrics

#### `opengslb_dns_queries_total`
**Type:** Counter  
**Labels:** `domain`, `type`, `status`

Total number of DNS queries received.

| Label | Description |
|-------|-------------|
| `domain` | The queried domain name |
| `type` | DNS query type (A, AAAA, etc.) |
| `status` | Response status: `success`, `nxdomain`, `servfail` |

**Example:**
```
opengslb_dns_queries_total{domain="app.example.com",type="A",status="success"} 1542
opengslb_dns_queries_total{domain="app.example.com",type="AAAA",status="success"} 523
opengslb_dns_queries_total{domain="unknown.com",type="A",status="nxdomain"} 12
```

#### `opengslb_dns_query_duration_seconds`
**Type:** Histogram  
**Labels:** `domain`, `status`

DNS query processing duration in seconds.

**Buckets:** 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1

**Example:**
```
opengslb_dns_query_duration_seconds_bucket{domain="app.example.com",status="success",le="0.001"} 1200
opengslb_dns_query_duration_seconds_sum{domain="app.example.com",status="success"} 0.892
opengslb_dns_query_duration_seconds_count{domain="app.example.com",status="success"} 1542
```

### Health Check Metrics

#### `opengslb_health_check_results_total`
**Type:** Counter  
**Labels:** `region`, `server`, `result`

Total number of health check results.

| Label | Description |
|-------|-------------|
| `region` | Region name |
| `server` | Server address and port (e.g., `10.0.1.10:80`) |
| `result` | Check result: `healthy`, `unhealthy` |

**Example:**
```
opengslb_health_check_results_total{region="us-east-1",server="10.0.1.10:80",result="healthy"} 4521
opengslb_health_check_results_total{region="us-east-1",server="10.0.1.10:80",result="unhealthy"} 3
```

#### `opengslb_health_check_duration_seconds`
**Type:** Histogram  
**Labels:** `region`, `server`

Health check duration in seconds.

**Buckets:** 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5

**Example:**
```
opengslb_health_check_duration_seconds_bucket{region="us-east-1",server="10.0.1.10:80",le="0.1"} 4500
opengslb_health_check_duration_seconds_sum{region="us-east-1",server="10.0.1.10:80"} 135.6
opengslb_health_check_duration_seconds_count{region="us-east-1",server="10.0.1.10:80"} 4524
```

#### `opengslb_healthy_servers`
**Type:** Gauge  
**Labels:** `region`

Current number of healthy servers per region.

**Example:**
```
opengslb_healthy_servers{region="us-east-1"} 3
opengslb_healthy_servers{region="us-west-2"} 2
```

### Routing Metrics

#### `opengslb_routing_decisions_total`
**Type:** Counter  
**Labels:** `domain`, `algorithm`, `server`

Total number of routing decisions made.

| Label | Description |
|-------|-------------|
| `domain` | The domain being routed |
| `algorithm` | Routing algorithm used: `round-robin`, `weighted`, `failover` |
| `server` | Selected server address and port |

**Example:**
```
opengslb_routing_decisions_total{domain="app.example.com",algorithm="round-robin",server="10.0.1.10:80"} 512
opengslb_routing_decisions_total{domain="app.example.com",algorithm="round-robin",server="10.0.1.11:80"} 510
opengslb_routing_decisions_total{domain="critical.example.com",algorithm="failover",server="10.0.2.10:80"} 1000
```

### Configuration Metrics

#### `opengslb_config_reloads_total`
**Type:** Counter  
**Labels:** `result`

Total number of configuration reload attempts.

| Label | Description |
|-------|-------------|
| `result` | Reload result: `success`, `failure` |

**Example:**
```
opengslb_config_reloads_total{result="success"} 5
opengslb_config_reloads_total{result="failure"} 1
```

**Use Cases:**
- Track reload activity
- Alert on failed reloads
- Correlate reloads with behavior changes

#### `opengslb_config_reload_timestamp_seconds`
**Type:** Gauge

Unix timestamp of the last successful configuration reload.

**Example:**
```
opengslb_config_reload_timestamp_seconds 1701504615
```

**Use Cases:**
- Verify reload was applied
- Track time since last reload
- Correlate with deployment events

### Application Metrics

#### `opengslb_app_info`
**Type:** Gauge  
**Labels:** `version`

Application version information. Always set to 1.

**Example:**
```
opengslb_app_info{version="1.0.0"} 1
```

#### `opengslb_config_load_timestamp_seconds`
**Type:** Gauge

Unix timestamp of the initial configuration load at startup.

**Example:**
```
opengslb_config_load_timestamp_seconds 1701504000
```

#### `opengslb_configured_domains`
**Type:** Gauge

Number of configured domains.

**Example:**
```
opengslb_configured_domains 5
```

#### `opengslb_configured_servers`
**Type:** Gauge

Total number of configured servers across all regions.

**Example:**
```
opengslb_configured_servers 12
```

## Prometheus Configuration

Add OpenGSLB to your Prometheus scrape configuration:

```yaml
scrape_configs:
  - job_name: 'opengslb'
    static_configs:
      - targets: ['opengslb-host:9090']
    scrape_interval: 15s
```

## Example Queries

### Query Rate
```promql
rate(opengslb_dns_queries_total[5m])
```

### Query Latency (p99)
```promql
histogram_quantile(0.99, rate(opengslb_dns_query_duration_seconds_bucket[5m]))
```

### Error Rate
```promql
sum(rate(opengslb_dns_queries_total{status!="success"}[5m])) 
/ 
sum(rate(opengslb_dns_queries_total[5m]))
```

### Healthy Server Ratio
```promql
opengslb_healthy_servers / opengslb_configured_servers
```

### Health Check Failure Rate
```promql
rate(opengslb_health_check_results_total{result="unhealthy"}[5m])
```

### Configuration Reload Success Rate
```promql
sum(rate(opengslb_config_reloads_total{result="success"}[1h]))
/
sum(rate(opengslb_config_reloads_total[1h]))
```

### Time Since Last Reload
```promql
time() - opengslb_config_reload_timestamp_seconds
```

### Routing Distribution by Algorithm
```promql
sum by (algorithm) (rate(opengslb_routing_decisions_total[5m]))
```

### Failover Events (Traffic to Non-Primary)
```promql
# Track when failover routing sends traffic to secondary servers
rate(opengslb_routing_decisions_total{algorithm="failover"}[5m])
```

## Alerting Examples

### High Error Rate
```yaml
- alert: OpenGSLBHighErrorRate
  expr: |
    sum(rate(opengslb_dns_queries_total{status!="success"}[5m])) 
    / 
    sum(rate(opengslb_dns_queries_total[5m])) > 0.05
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "OpenGSLB error rate above 5%"
```

### No Healthy Servers
```yaml
- alert: OpenGSLBNoHealthyServers
  expr: opengslb_healthy_servers == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "No healthy servers in region {{ $labels.region }}"
```

### High Query Latency
```yaml
- alert: OpenGSLBHighLatency
  expr: |
    histogram_quantile(0.99, rate(opengslb_dns_query_duration_seconds_bucket[5m])) > 0.01
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "OpenGSLB p99 latency above 10ms"
```

### Configuration Reload Failed
```yaml
- alert: OpenGSLBConfigReloadFailed
  expr: increase(opengslb_config_reloads_total{result="failure"}[5m]) > 0
  for: 0m
  labels:
    severity: warning
  annotations:
    summary: "OpenGSLB configuration reload failed"
    description: "A configuration reload attempt failed. Check logs for details."
```

### Failover Active
```yaml
- alert: OpenGSLBFailoverActive
  expr: |
    opengslb_healthy_servers{region="primary"} == 0 
    and opengslb_healthy_servers{region="secondary"} > 0
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "OpenGSLB failover active - primary region has no healthy servers"
```

### Low Healthy Server Ratio
```yaml
- alert: OpenGSLBLowHealthyRatio
  expr: |
    opengslb_healthy_servers / opengslb_configured_servers < 0.5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Less than 50% of servers are healthy"
```

### Overwatch Metrics (ADR-015)

These metrics are only available in Overwatch mode.

#### `opengslb_overwatch_backends_total`
**Type:** Gauge

Total number of backends in the registry.

**Example:**
```
opengslb_overwatch_backends_total 24
```

#### `opengslb_overwatch_backends_healthy`
**Type:** Gauge

Number of backends with healthy effective status.

**Example:**
```
opengslb_overwatch_backends_healthy 22
```

#### `opengslb_overwatch_agents_registered`
**Type:** Gauge

Number of unique agents currently registered.

**Example:**
```
opengslb_overwatch_agents_registered 8
```

#### `opengslb_overwatch_stale_agents`
**Type:** Gauge

Number of backends marked as stale (no recent heartbeat).

**Example:**
```
opengslb_overwatch_stale_agents 2
```

#### `opengslb_overwatch_overrides_active`
**Type:** Gauge

Number of active manual overrides.

**Example:**
```
opengslb_overwatch_overrides_active 1
```

#### `opengslb_overwatch_validation_total`
**Type:** Counter
**Labels:** `service`, `result`

External validation results.

| Label | Description |
|-------|-------------|
| `service` | Service name |
| `result` | Validation result: `healthy`, `unhealthy` |

**Example:**
```
opengslb_overwatch_validation_total{service="web-service",result="healthy"} 450
opengslb_overwatch_validation_total{service="web-service",result="unhealthy"} 12
```

#### `opengslb_overwatch_veto_total`
**Type:** Counter
**Labels:** `service`, `reason`

Veto events where Overwatch overrode agent health claims.

| Label | Description |
|-------|-------------|
| `service` | Service name |
| `reason` | Veto reason: `validation_unhealthy`, `validation_healthy` |

**Example:**
```
opengslb_overwatch_veto_total{service="web-service",reason="validation_unhealthy"} 5
```

#### `opengslb_overwatch_backends_by_authority`
**Type:** Gauge
**Labels:** `authority`

Backends grouped by health authority source.

| Label | Description |
|-------|-------------|
| `authority` | Source: `agent`, `override`, `stale` |

**Example:**
```
opengslb_overwatch_backends_by_authority{authority="agent"} 20
opengslb_overwatch_backends_by_authority{authority="override"} 1
opengslb_overwatch_backends_by_authority{authority="stale"} 3
```

### Gossip Metrics

#### `opengslb_gossip_messages_received_total`
**Type:** Counter
**Labels:** `type`

Total gossip messages received by type.

**Example:**
```
opengslb_gossip_messages_received_total{type="heartbeat"} 4521
opengslb_gossip_messages_received_total{type="predictive"} 12
```

#### `opengslb_gossip_override_operations_total`
**Type:** Counter
**Labels:** `operation`

Override operations via API.

| Label | Description |
|-------|-------------|
| `operation` | Operation type: `set`, `clear` |

**Example:**
```
opengslb_gossip_override_operations_total{operation="set"} 5
opengslb_gossip_override_operations_total{operation="clear"} 3
```

#### `opengslb_gossip_decryption_failures_total`
**Type:** Counter

Total gossip message decryption failures.

**Example:**
```
opengslb_gossip_decryption_failures_total 3
```

**Use Cases:**
- Monitor for encryption key mismatches
- Detect potential security issues with gossip communication

### Geolocation Routing Metrics (Sprint 6)

#### `opengslb_routing_geo_decisions_total`
**Type:** Counter
**Labels:** `domain`, `country`, `continent`, `region`

Total geolocation routing decisions by location.

| Label | Description |
|-------|-------------|
| `domain` | The domain being routed |
| `country` | ISO country code (e.g., "US", "GB") |
| `continent` | Continent code (e.g., "NA", "EU") |
| `region` | Selected region name |

**Example:**
```
opengslb_routing_geo_decisions_total{domain="app.example.com",country="US",continent="NA",region="us-east-1"} 1542
opengslb_routing_geo_decisions_total{domain="app.example.com",country="GB",continent="EU",region="eu-west-1"} 523
```

#### `opengslb_routing_geo_fallback_total`
**Type:** Counter
**Labels:** `domain`, `reason`

Total geolocation routing fallbacks by reason.

| Label | Description |
|-------|-------------|
| `domain` | The domain being routed |
| `reason` | Fallback reason: `no_client_ip`, `no_resolver`, `lookup_failed`, `no_servers_in_region`, `no_match` |

**Example:**
```
opengslb_routing_geo_fallback_total{domain="app.example.com",reason="no_servers_in_region"} 12
opengslb_routing_geo_fallback_total{domain="app.example.com",reason="lookup_failed"} 5
```

#### `opengslb_routing_geo_custom_hits_total`
**Type:** Counter
**Labels:** `domain`, `region`, `cidr`

Total custom CIDR mapping matches in geolocation routing.

| Label | Description |
|-------|-------------|
| `domain` | The domain being routed |
| `region` | Matched region from custom mapping |
| `cidr` | The matched CIDR range |

**Example:**
```
opengslb_routing_geo_custom_hits_total{domain="app.example.com",region="us-chicago",cidr="10.1.0.0/16"} 450
opengslb_routing_geo_custom_hits_total{domain="app.example.com",region="us-dallas",cidr="10.2.0.0/16"} 230
```

### Latency Routing Metrics (Sprint 6)

#### `opengslb_routing_latency_selected_ms`
**Type:** Gauge
**Labels:** `domain`, `server`

Smoothed latency in milliseconds of the selected server for latency-based routing.

**Example:**
```
opengslb_routing_latency_selected_ms{domain="perf-critical.example.com",server="10.0.1.10:8080"} 45.5
```

#### `opengslb_routing_latency_rejected_total`
**Type:** Counter
**Labels:** `domain`, `server`, `reason`

Total servers rejected due to latency threshold or insufficient data.

| Label | Description |
|-------|-------------|
| `domain` | The domain being routed |
| `server` | Server address (host:port) |
| `reason` | Rejection reason: `above_threshold`, `no_data` |

**Example:**
```
opengslb_routing_latency_rejected_total{domain="perf-critical.example.com",server="10.0.2.10:8080",reason="above_threshold"} 5
opengslb_routing_latency_rejected_total{domain="perf-critical.example.com",server="10.0.3.10:8080",reason="no_data"} 12
```

#### `opengslb_routing_latency_fallback_total`
**Type:** Counter
**Labels:** `domain`, `reason`

Total fallbacks to round-robin when latency data is unavailable.

| Label | Description |
|-------|-------------|
| `domain` | The domain being routed |
| `reason` | Fallback reason: `no_provider`, `no_latency_data` |

**Example:**
```
opengslb_routing_latency_fallback_total{domain="perf-critical.example.com",reason="no_latency_data"} 3
```

#### `opengslb_backend_smoothed_latency_ms`
**Type:** Gauge
**Labels:** `service`, `address`

Current smoothed (EMA) latency in milliseconds for each backend.

**Example:**
```
opengslb_backend_smoothed_latency_ms{service="myapp",address="10.0.1.10:8080"} 45.5
opengslb_backend_smoothed_latency_ms{service="myapp",address="10.0.1.11:8080"} 52.3
```

#### `opengslb_backend_latency_samples`
**Type:** Gauge
**Labels:** `service`, `address`

Number of latency samples collected for each backend.

**Example:**
```
opengslb_backend_latency_samples{service="myapp",address="10.0.1.10:8080"} 150
```

### Per-Agent Connectivity Metrics (Sprint 6)

#### `opengslb_agent_connected`
**Type:** Gauge
**Labels:** `agent_id`, `region`

Agent connection status (1=connected, 0=disconnected).

**Example:**
```
opengslb_agent_connected{agent_id="agent-1",region="us-east-1"} 1
opengslb_agent_connected{agent_id="agent-2",region="eu-west-1"} 0
```

#### `opengslb_agent_heartbeat_age_seconds`
**Type:** Gauge
**Labels:** `agent_id`

Seconds since last heartbeat per agent.

**Example:**
```
opengslb_agent_heartbeat_age_seconds{agent_id="agent-1"} 5.2
opengslb_agent_heartbeat_age_seconds{agent_id="agent-2"} 45.8
```

#### `opengslb_agent_backends_registered_per_agent`
**Type:** Gauge
**Labels:** `agent_id`

Number of backends registered by each agent.

**Example:**
```
opengslb_agent_backends_registered_per_agent{agent_id="agent-1"} 4
opengslb_agent_backends_registered_per_agent{agent_id="agent-2"} 2
```

#### `opengslb_agent_stale_events_total`
**Type:** Counter
**Labels:** `agent_id`

Total stale events per agent.

**Example:**
```
opengslb_agent_stale_events_total{agent_id="agent-1"} 2
```

### Override Metrics with Service Granularity (Sprint 6)

#### `opengslb_overrides_active`
**Type:** Gauge
**Labels:** `service`

Number of active overrides per service.

**Example:**
```
opengslb_overrides_active{service="myapp"} 1
opengslb_overrides_active{service="otherapp"} 0
```

#### `opengslb_overrides_changes_total`
**Type:** Counter
**Labels:** `service`, `action`

Total override changes by service and action.

| Label | Description |
|-------|-------------|
| `service` | Service name |
| `action` | Action type: `set`, `clear` |

**Example:**
```
opengslb_overrides_changes_total{service="myapp",action="set"} 5
opengslb_overrides_changes_total{service="myapp",action="clear"} 3
```

### Enhanced DNSSEC Metrics (Sprint 6)

#### `opengslb_dnssec_signatures_total`
**Type:** Counter
**Labels:** `zone`

Total DNSSEC signatures generated per zone.

**Example:**
```
opengslb_dnssec_signatures_total{zone="gslb.example.com"} 15420
```

#### `opengslb_dnssec_key_age_by_zone_seconds`
**Type:** Gauge
**Labels:** `zone`, `key_tag`

Age of DNSSEC signing keys in seconds, per zone and key tag.

| Label | Description |
|-------|-------------|
| `zone` | DNS zone name |
| `key_tag` | DNSSEC key tag identifier |

**Example:**
```
opengslb_dnssec_key_age_by_zone_seconds{zone="gslb.example.com",key_tag="12345"} 86400
```

## Overwatch Alerting Examples

### No Registered Agents
```yaml
- alert: OpenGSLBNoAgents
  expr: opengslb_overwatch_agents_registered == 0
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "No agents registered with Overwatch"
```

### High Stale Backend Count
```yaml
- alert: OpenGSLBHighStaleBackends
  expr: |
    opengslb_overwatch_stale_agents / opengslb_overwatch_backends_total > 0.2
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "More than 20% of backends are stale"
```

### Validation Disagreement Rate
```yaml
- alert: OpenGSLBHighVetoRate
  expr: |
    rate(opengslb_overwatch_veto_total[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "High rate of validation vetoes - agent health claims being overridden"
```

## Sprint 6 Example Queries

### Geolocation Traffic Distribution
```promql
# Traffic distribution by region
sum by (region) (rate(opengslb_routing_geo_decisions_total[5m]))

# Traffic from custom CIDR mappings
sum by (region, cidr) (rate(opengslb_routing_geo_custom_hits_total[5m]))

# Geolocation fallback rate
sum(rate(opengslb_routing_geo_fallback_total[5m])) / sum(rate(opengslb_routing_geo_decisions_total[5m]))
```

### Latency Routing Analysis
```promql
# Average selected latency by domain
avg by (domain) (opengslb_routing_latency_selected_ms)

# Servers frequently rejected due to high latency
topk(5, sum by (server) (rate(opengslb_routing_latency_rejected_total{reason="above_threshold"}[1h])))

# Latency routing fallback rate
sum by (domain) (rate(opengslb_routing_latency_fallback_total[5m])) / sum by (domain) (rate(opengslb_routing_decisions_total{algorithm="latency"}[5m]))
```

### Agent Health Monitoring
```promql
# Agents not connected
opengslb_agent_connected == 0

# Agents with stale heartbeats (>30s)
opengslb_agent_heartbeat_age_seconds > 30

# Stale events by agent
rate(opengslb_agent_stale_events_total[1h])
```

### Override Activity
```promql
# Current override count by service
opengslb_overrides_active

# Override change rate
sum by (service, action) (rate(opengslb_overrides_changes_total[1h]))
```

## Sprint 6 Alerting Examples

### High Geolocation Fallback Rate
```yaml
- alert: OpenGSLBHighGeoFallbackRate
  expr: |
    sum(rate(opengslb_routing_geo_fallback_total[5m])) /
    sum(rate(opengslb_routing_geo_decisions_total[5m])) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Geolocation routing fallback rate above 10%"
    description: "Many geolocation lookups are failing or falling back to default."
```

### Agent Heartbeat Stale
```yaml
- alert: OpenGSLBAgentStale
  expr: opengslb_agent_heartbeat_age_seconds > 60
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "Agent {{ $labels.agent_id }} heartbeat stale"
    description: "No heartbeat received from agent for over 60 seconds."
```

### High Latency Server Selection
```yaml
- alert: OpenGSLBHighLatencySelected
  expr: opengslb_routing_latency_selected_ms > 200
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Selected servers have high latency for {{ $labels.domain }}"
    description: "Latency-based routing is selecting servers with >200ms latency."
```

### Gossip Decryption Failures
```yaml
- alert: OpenGSLBGossipDecryptionFailures
  expr: increase(opengslb_gossip_decryption_failures_total[5m]) > 0
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "Gossip message decryption failures detected"
    description: "This may indicate encryption key mismatches between nodes."
```

## Metric Cardinality

Be aware of metric cardinality when configuring monitoring:

| Metric | Cardinality Factors |
|--------|---------------------|
| `opengslb_dns_queries_total` | domains × query_types × status |
| `opengslb_routing_decisions_total` | domains × algorithms × servers |
| `opengslb_health_check_results_total` | regions × servers × results |
| `opengslb_routing_geo_decisions_total` | domains × countries × continents × regions |
| `opengslb_routing_geo_custom_hits_total` | domains × regions × cidrs |
| `opengslb_routing_latency_rejected_total` | domains × servers × reasons |
| `opengslb_agent_connected` | agents × regions |
| `opengslb_overrides_changes_total` | services × actions |
| `opengslb_dnssec_key_age_by_zone_seconds` | zones × key_tags |

For large deployments with many domains or servers, consider:
- Aggregating by region instead of individual servers
- Using recording rules to pre-aggregate high-cardinality metrics
- Limiting label values in Prometheus configuration
- The geolocation metrics can grow with country/continent combinations - monitor cardinality