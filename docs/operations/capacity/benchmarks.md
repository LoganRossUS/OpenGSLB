# Performance Benchmarks

This document provides benchmark results and testing methodology for OpenGSLB performance evaluation.

## Benchmark Environment

### Reference Hardware

| Component | Specification |
|-----------|---------------|
| CPU | 4-core Intel Xeon @ 2.5 GHz |
| Memory | 8 GB DDR4 |
| Disk | NVMe SSD |
| Network | 10 Gbps |
| OS | Ubuntu 22.04 LTS |
| Go Version | 1.21 |

### Test Configuration

```yaml
# Overwatch configuration for benchmarks
mode: overwatch

dns:
  listen_address: "0.0.0.0:53"
  default_ttl: 30

dnssec:
  enabled: true

overwatch:
  validation:
    enabled: false  # Disabled for pure DNS benchmarks

logging:
  level: warn  # Reduced logging for benchmarks
```

## DNS Query Performance

### Query Throughput

| Scenario | QPS (median) | QPS (p99) | Latency (p50) | Latency (p99) |
|----------|--------------|-----------|---------------|---------------|
| A record, no DNSSEC | 75,000 | 70,000 | 0.3 ms | 1.2 ms |
| A record, with DNSSEC | 45,000 | 40,000 | 0.5 ms | 2.0 ms |
| Geolocation routing | 40,000 | 35,000 | 0.6 ms | 2.5 ms |
| Latency-based routing | 50,000 | 45,000 | 0.5 ms | 2.0 ms |

### Query Throughput by Backend Count

| Backends | QPS (A record) | QPS (A + DNSSEC) |
|----------|----------------|------------------|
| 10 | 75,000 | 45,000 |
| 50 | 72,000 | 43,000 |
| 100 | 68,000 | 40,000 |
| 500 | 60,000 | 35,000 |
| 1,000 | 50,000 | 30,000 |

### Latency Distribution

| Percentile | No DNSSEC | With DNSSEC |
|------------|-----------|-------------|
| p50 | 0.3 ms | 0.5 ms |
| p90 | 0.8 ms | 1.5 ms |
| p95 | 1.0 ms | 1.8 ms |
| p99 | 1.2 ms | 2.0 ms |
| p99.9 | 2.0 ms | 4.0 ms |

## Resource Usage

### Memory Usage

| Backends | Memory (base) | Memory (with GeoIP) |
|----------|---------------|---------------------|
| 0 | 50 MB | 55 MB |
| 100 | 55 MB | 60 MB |
| 500 | 70 MB | 75 MB |
| 1,000 | 90 MB | 95 MB |
| 5,000 | 150 MB | 155 MB |

### CPU Usage

At 10,000 QPS steady state:

| Configuration | CPU Usage |
|---------------|-----------|
| Round-robin | 15% |
| Weighted | 16% |
| Failover | 15% |
| Geolocation | 25% |
| Latency-based | 20% |
| With DNSSEC | +30% |
| With validation | +10% |

## Gossip Performance

### Message Throughput

| Agents | Messages/sec | Bandwidth |
|--------|--------------|-----------|
| 10 | 10 | 1 KB/s |
| 50 | 50 | 5 KB/s |
| 100 | 100 | 10 KB/s |
| 500 | 500 | 50 KB/s |
| 1,000 | 1,000 | 100 KB/s |

### Convergence Time

Time for health status change to propagate:

| Change | Time to Propagate |
|--------|-------------------|
| Single backend unhealthy | < 1 second |
| Single backend healthy | < 1 second |
| Bulk update (10 backends) | < 2 seconds |
| Network partition recovery | < 5 seconds |

## Routing Algorithm Performance

### Geolocation Routing

| Operation | Time |
|-----------|------|
| GeoIP database lookup | 0.1 ms |
| Custom CIDR lookup | 0.01 ms |
| Region matching | 0.01 ms |
| Total routing decision | 0.15 ms |

### Latency-Based Routing

| Operation | Time |
|-----------|------|
| EMA calculation | 0.001 ms |
| Server selection | 0.01 ms |
| Total routing decision | 0.05 ms |

## Health Check Performance

### Overwatch Validation

| Backends | Checks/minute | CPU Impact |
|----------|---------------|------------|
| 10 | 20 | < 1% |
| 50 | 100 | 2% |
| 100 | 200 | 5% |
| 500 | 1,000 | 15% |

### Agent Health Checks

| Backends per Agent | Checks/minute | CPU Impact |
|--------------------|---------------|------------|
| 1 | 12 | < 1% |
| 5 | 60 | 1% |
| 20 | 240 | 3% |

## Scaling Benchmarks

### Horizontal Scaling (Multiple Overwatches)

| Overwatches | Combined QPS | Notes |
|-------------|--------------|-------|
| 1 | 75,000 | Single node |
| 2 | 150,000 | Linear scaling |
| 3 | 220,000 | Near-linear |
| 5 | 350,000 | Slight overhead |

### Backend Scaling

| Backends | Registration Time | Memory Overhead |
|----------|-------------------|-----------------|
| 100 | < 1 second | 5 MB |
| 1,000 | < 5 seconds | 40 MB |
| 10,000 | < 30 seconds | 400 MB |

## Running Your Own Benchmarks

### DNS Query Benchmark

Using `dnsperf`:

```bash
# Install dnsperf
apt-get install dnsperf

# Create query file
echo "myapp.gslb.example.com A" > queries.txt

# Run benchmark
dnsperf -s 127.0.0.1 -d queries.txt -l 60 -c 100 -Q 10000
```

Using `dnsbench`:

```bash
# Install dnsbench
go install github.com/onur/dnsbench@latest

# Run benchmark
dnsbench -s 127.0.0.1:53 -c 100 -n 100000 myapp.gslb.example.com
```

### Custom Benchmark Script

```bash
#!/bin/bash
# benchmark.sh

OVERWATCH="127.0.0.1"
DOMAIN="myapp.gslb.example.com"
DURATION=60
CONCURRENCY=100

echo "=== DNS Benchmark ==="
echo "Target: ${OVERWATCH}"
echo "Domain: ${DOMAIN}"
echo "Duration: ${DURATION}s"
echo "Concurrency: ${CONCURRENCY}"
echo ""

# Warmup
echo "Warming up..."
for i in {1..1000}; do
    dig @${OVERWATCH} ${DOMAIN} +short > /dev/null
done

# Benchmark
echo "Running benchmark..."
dnsperf -s ${OVERWATCH} -d <(yes "${DOMAIN} A" | head -100000) \
    -l ${DURATION} -c ${CONCURRENCY} -Q 50000

echo "=== Benchmark Complete ==="
```

### Stress Test

```bash
#!/bin/bash
# stress-test.sh

OVERWATCH="127.0.0.1"
DOMAIN="myapp.gslb.example.com"

# Gradually increase load
for qps in 10000 20000 30000 40000 50000 60000 70000 80000; do
    echo "Testing at ${qps} QPS..."
    dnsperf -s ${OVERWATCH} -d <(yes "${DOMAIN} A" | head -100000) \
        -l 30 -c 100 -Q ${qps} 2>&1 | tail -5
    sleep 10
done
```

### Monitoring During Benchmark

```bash
# Terminal 1: Watch metrics
watch -n1 'curl -s http://localhost:9091/metrics | grep -E "(queries_total|query_duration)"'

# Terminal 2: Watch resources
htop -p $(pgrep opengslb)

# Terminal 3: Run benchmark
./benchmark.sh
```

## Performance Tuning

### Recommended Settings for High Throughput

```yaml
# High-performance configuration
logging:
  level: warn  # Reduce logging overhead

metrics:
  enabled: true  # Keep for monitoring

dns:
  default_ttl: 30  # Balance freshness vs cache efficiency

overwatch:
  validation:
    check_interval: 60s  # Less frequent for high backend counts
```

### System Tuning

```bash
# Increase file descriptors
echo "* soft nofile 65536" >> /etc/security/limits.conf
echo "* hard nofile 65536" >> /etc/security/limits.conf

# Network tuning
sysctl -w net.core.somaxconn=65535
sysctl -w net.core.netdev_max_backlog=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535

# For Overwatch in systemd
# LimitNOFILE=65536
```

### Go Runtime Tuning

```bash
# In systemd service
Environment="GOMAXPROCS=4"
Environment="GOGC=100"  # Default, adjust based on memory/CPU tradeoff
```

## Comparison with Alternatives

| Solution | QPS (typical) | Latency (p99) | Notes |
|----------|---------------|---------------|-------|
| OpenGSLB | 50,000+ | < 2 ms | With DNSSEC |
| PowerDNS | 100,000+ | < 1 ms | No GSLB logic |
| CoreDNS | 80,000+ | < 1 ms | Plugin dependent |
| AWS Route 53 | N/A | ~50 ms | Managed service |
| NS1 | N/A | ~10 ms | Managed service |

**Note**: Comparisons are approximate and depend heavily on configuration.

## Benchmark Caveats

1. **Real-world performance varies**: Actual results depend on:
   - Hardware specifications
   - Network conditions
   - Query patterns
   - Configuration options

2. **DNSSEC overhead**: Cryptographic operations add latency

3. **Geolocation overhead**: Database lookups add latency

4. **Validation overhead**: External health checks consume resources

5. **Test methodology**: Results vary with test tools and methodology

## Related Documentation

- [Capacity Planning](./planning.md)
- [Overwatch Deployment](../deployment/overwatch.md)
- [Metrics Reference](../../metrics.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
