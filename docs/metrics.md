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
opengslb_dns_queries_total{domain="app.example.com",type="A",status="nxdomain"} 12
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
| `algorithm` | Routing algorithm used (e.g., `round-robin`) |
| `server` | Selected server address and port |

**Example:**
```
opengslb_routing_decisions_total{domain="app.example.com",algorithm="round-robin",server="10.0.1.10:80"} 512
opengslb_routing_decisions_total{domain="app.example.com",algorithm="round-robin",server="10.0.1.11:80"} 510
```

### Application Metrics

#### `opengslb_app_info`
**Type:** Gauge  
**Labels:** `version`

Application version information. Always set to 1.

**Example:**
```
opengslb_app_info{version="0.1.0-dev"} 1
```

#### `opengslb_config_load_timestamp_seconds`
**Type:** Gauge

Unix timestamp of the last configuration load.

**Example:**
```
opengslb_config_load_timestamp_seconds 1701504615
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