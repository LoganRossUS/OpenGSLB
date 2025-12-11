# Capacity Planning Guide

This document provides guidance for sizing OpenGSLB deployments based on workload requirements.

## Overview

OpenGSLB capacity planning involves sizing:

1. **Overwatches**: DNS query throughput, backend count
2. **Agents**: Number of backends monitored
3. **Network**: Gossip traffic, DNS traffic
4. **Storage**: KV store, DNSSEC keys, logs

## Sizing Guidelines

### Overwatch Sizing

#### Resource Requirements by Scale

| Scale | Backends | DNS QPS | CPU | Memory | Disk |
|-------|----------|---------|-----|--------|------|
| Small | < 50 | < 1,000 | 2 cores | 512 MB | 1 GB |
| Medium | 50-200 | 1,000-10,000 | 4 cores | 1 GB | 5 GB |
| Large | 200-1,000 | 10,000-50,000 | 8 cores | 2 GB | 10 GB |
| XLarge | 1,000+ | 50,000+ | 16 cores | 4 GB | 20 GB |

#### Factors Affecting Overwatch Resources

| Factor | Impact | Mitigation |
|--------|--------|------------|
| DNS query rate | CPU, network | Horizontal scaling (more Overwatches) |
| Number of backends | Memory, gossip traffic | Increase memory |
| DNSSEC enabled | CPU for signing | Consider faster algorithm or more CPU |
| Validation enabled | CPU, network | Adjust validation interval |
| GeoIP lookups | CPU, memory | GeoIP database ~5MB in memory |

### Agent Sizing

#### Resource Requirements

| Backends per Agent | CPU | Memory | Notes |
|--------------------|-----|--------|-------|
| 1-5 | 0.1 core | 32 MB | Typical deployment |
| 5-20 | 0.25 core | 64 MB | Multi-service host |
| 20-50 | 0.5 core | 128 MB | High-density |

#### Factors Affecting Agent Resources

| Factor | Impact | Mitigation |
|--------|--------|------------|
| Number of backends | CPU, memory | Linear scaling |
| Health check frequency | CPU, network | Adjust interval |
| Predictive health | CPU | Can be disabled |
| Gossip traffic | Network | Minimal impact |

### Network Requirements

#### Gossip Traffic

Gossip traffic is lightweight:

| Component | Traffic Pattern | Estimated Bandwidth |
|-----------|-----------------|---------------------|
| Heartbeat | Every 10s per backend | ~100 bytes/heartbeat |
| Health update | On status change | ~200 bytes/update |
| Probe | Periodic | ~50 bytes/probe |

**Estimate per agent**: ~1 KB/minute typical, ~5 KB/minute during changes

#### DNS Traffic

| Query Size | Response Size | Typical |
|------------|---------------|---------|
| A record | 40-60 bytes | 100-200 bytes |
| With DNSSEC | 40-60 bytes | 500-1000 bytes |

**Bandwidth calculation**: `QPS × avg_response_size × 8`

Example: 10,000 QPS × 200 bytes × 8 = 16 Mbps

### Storage Requirements

#### Overwatch Storage

| Component | Size | Growth |
|-----------|------|--------|
| Binary | ~15 MB | Per version |
| Configuration | ~10 KB | Stable |
| GeoIP database | ~5 MB | Monthly updates |
| KV store (bbolt) | ~1 MB base + 1 KB/backend | Linear with backends |
| DNSSEC keys | ~10 KB | Per zone |
| Logs | Variable | Configure rotation |

#### Agent Storage

| Component | Size | Growth |
|-----------|------|--------|
| Binary | ~15 MB | Per version |
| Configuration | ~5 KB | Stable |
| Certificate | ~2 KB | Stable |
| Logs | Variable | Configure rotation |

## Scaling Strategies

### Vertical Scaling (Scale Up)

Increase resources on existing nodes:

| Scenario | Solution |
|----------|----------|
| High CPU usage | Add cores, optimize validation interval |
| High memory usage | Add RAM, reduce log retention |
| High disk I/O | Use SSD, reduce logging |

### Horizontal Scaling (Scale Out)

Add more nodes:

| Scenario | Solution |
|----------|----------|
| DNS capacity limit | Add more Overwatches |
| Geographic distribution | Deploy Overwatches per region |
| Agent count limit | More Overwatches (gossip load sharing) |

### Scaling Formulas

#### Overwatches Needed

```
min_overwatches = ceil(peak_qps / qps_per_overwatch)
recommended_overwatches = min_overwatches + 1  # For redundancy
```

Where `qps_per_overwatch` ≈ 50,000 for typical hardware.

#### Memory Estimation

```
base_memory = 256 MB
per_backend_memory = 2 KB
geoip_memory = 5 MB
buffer = 1.5x

total_memory = (base_memory + (backends × per_backend_memory) + geoip_memory) × buffer
```

## Deployment Configurations

### Small Deployment (Startup/Dev)

```
Overwatches: 1-2
Agents: 1-10
Hardware per Overwatch:
  - 2 vCPU
  - 512 MB RAM
  - 10 GB disk
  - Shared/burstable OK
```

### Medium Deployment (SMB/Production)

```
Overwatches: 3
Agents: 10-50
Hardware per Overwatch:
  - 4 vCPU
  - 1 GB RAM
  - 20 GB SSD
  - Dedicated instances
```

### Large Deployment (Enterprise)

```
Overwatches: 5-10 (geo-distributed)
Agents: 50-500
Hardware per Overwatch:
  - 8 vCPU
  - 2 GB RAM
  - 50 GB SSD
  - Dedicated instances
  - 10 Gbps networking
```

### Cloud Instance Recommendations

#### AWS

| Scale | Instance Type | Notes |
|-------|---------------|-------|
| Small | t3.small | Burstable OK for dev |
| Medium | m5.large | Production baseline |
| Large | m5.xlarge | High throughput |
| XLarge | c5.2xlarge | CPU optimized |

#### GCP

| Scale | Machine Type | Notes |
|-------|--------------|-------|
| Small | e2-small | Cost-effective |
| Medium | n2-standard-2 | Balanced |
| Large | n2-standard-4 | Production |
| XLarge | c2-standard-8 | Compute optimized |

#### Azure

| Scale | VM Size | Notes |
|-------|---------|-------|
| Small | B2s | Burstable |
| Medium | D2s_v3 | General purpose |
| Large | D4s_v3 | Production |
| XLarge | F8s_v2 | Compute optimized |

## Capacity Monitoring

### Key Metrics to Watch

```promql
# CPU utilization
rate(process_cpu_seconds_total{job="opengslb"}[5m])

# Memory usage
process_resident_memory_bytes{job="opengslb"}

# DNS query rate
rate(opengslb_dns_queries_total[5m])

# Backend count
opengslb_overwatch_backends_total

# Query latency
histogram_quantile(0.99, rate(opengslb_dns_query_duration_seconds_bucket[5m]))
```

### Capacity Alerts

```yaml
groups:
  - name: opengslb-capacity
    rules:
      - alert: HighCPUUsage
        expr: rate(process_cpu_seconds_total{job="opengslb"}[5m]) > 0.8
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Overwatch CPU usage above 80%"

      - alert: HighMemoryUsage
        expr: process_resident_memory_bytes{job="opengslb"} / process_virtual_memory_max_bytes > 0.8
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Overwatch memory usage above 80%"

      - alert: HighQueryRate
        expr: rate(opengslb_dns_queries_total[5m]) > 40000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Approaching query rate capacity"

      - alert: HighQueryLatency
        expr: histogram_quantile(0.99, rate(opengslb_dns_query_duration_seconds_bucket[5m])) > 0.01
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P99 query latency above 10ms"
```

## Growth Planning

### Capacity Planning Process

1. **Baseline current usage**
   - Measure current QPS, backend count, resource usage

2. **Estimate growth**
   - Project based on business growth
   - Typical: 20-50% annual growth

3. **Plan headroom**
   - Maintain 30-50% headroom for spikes
   - Plan capacity 6-12 months ahead

4. **Review quarterly**
   - Adjust based on actual growth
   - Update projections

### Example Capacity Plan

```
Current (Q1 2025):
  - DNS QPS: 5,000
  - Backends: 50
  - Overwatches: 3

Projected (Q4 2025, 50% growth):
  - DNS QPS: 7,500
  - Backends: 75
  - Overwatches: 3 (sufficient)

Projected (Q4 2026, 100% total):
  - DNS QPS: 10,000
  - Backends: 100
  - Overwatches: 3 (may need 4th for headroom)

Action items:
  - Q3 2025: Upgrade Overwatch instances to next tier
  - Q2 2026: Deploy 4th Overwatch for geographic expansion
```

## Related Documentation

- [Benchmarks](./benchmarks.md)
- [HA Setup Guide](../deployment/ha-setup.md)
- [Overwatch Deployment](../deployment/overwatch.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
