# Configuration Reference

OpenGSLB is configured via a YAML file. By default, it looks for `/etc/opengslb/config.yaml`, but you can specify a different path with the `--config` flag.

## Configuration File Security

OpenGSLB enforces strict file permissions on the configuration file. The config file must **not** be world-readable (no "other" read permission).

```bash
# Correct permissions (owner read/write, group read)
chmod 640 /etc/opengslb/config.yaml

# Or more restrictive (owner only)
chmod 600 /etc/opengslb/config.yaml
```

If the file has insecure permissions, OpenGSLB will refuse to start and display an error message.

## Configuration Sections

### DNS Configuration

Controls the DNS server behavior.

```yaml
dns:
  listen_address: ":53"
  default_ttl: 60
  return_last_healthy: false
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `listen_address` | string | `:53` | Address and port to listen on. Format: `ip:port` or `:port` for all interfaces. |
| `default_ttl` | integer | `60` | Default TTL (seconds) for DNS responses. Clients cache responses for this duration. |
| `return_last_healthy` | boolean | `false` | When all servers are unhealthy: `false` returns SERVFAIL, `true` returns the last known healthy IP. |

**Notes:**
- Lower TTL = faster failover but higher DNS query volume
- Port 53 requires root privileges. Use a high port (e.g., `:5353`) for non-root operation
- `return_last_healthy: true` enables "limp mode" - degraded service instead of complete failure

### Logging Configuration

Controls log output format and verbosity.

```yaml
logging:
  level: info
  format: json
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `format` | string | `json` | Output format: `json` (structured) or `text` (human-readable) |

**Format recommendations:**
- Use `json` for production deployments with log aggregation (ELK, Splunk, Loki)
- Use `text` for development and debugging

### Metrics Configuration

Controls Prometheus metrics exposure.

```yaml
metrics:
  enabled: true
  address: ":9090"
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable/disable the metrics HTTP endpoint |
| `address` | string | `:9090` | Address and port for the metrics server |

When enabled, metrics are available at `http://<address>/metrics` and a health check at `http://<address>/health`.

### Regions Configuration

Defines geographic regions/data centers and their backend servers.

```yaml
regions:
  - name: us-east-1
    servers:
      - address: 10.0.1.10
        port: 80
        weight: 100
      - address: 10.0.1.11
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2
```

#### Region Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier for the region |
| `servers` | list | Yes | List of backend servers in this region |
| `health_check` | object | Yes | Health check configuration for servers in this region |

#### Server Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `address` | string | Required | IP address of the backend server |
| `port` | integer | `80` | Port number for health checks |
| `weight` | integer | `100` | Server weight for weighted routing (1-1000) |
| `host` | string | (empty) | Hostname for HTTPS health checks (for TLS SNI and certificate validation) |

#### Health Check Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `http` | Check type: `http`, `https`, or `tcp` |
| `interval` | duration | `30s` | Time between health checks |
| `timeout` | duration | `5s` | Timeout for each check (must be < interval) |
| `path` | string | `/health` | HTTP/HTTPS path to check |
| `host` | string | (empty) | Host header for HTTPS checks (for TLS SNI and certificate validation) |
| `failure_threshold` | integer | `3` | Consecutive failures before marking unhealthy (1-10) |
| `success_threshold` | integer | `2` | Consecutive successes before marking healthy (1-10) |

**Health check behavior:**
- HTTP/HTTPS checks expect a 2xx response code
- TCP checks only verify successful TCP connection (no data exchange)
- A server starts as healthy and requires `failure_threshold` consecutive failures to become unhealthy
- An unhealthy server requires `success_threshold` consecutive successes to become healthy again
- For HTTPS checks with IP addresses, use `host` to set the Host header for TLS certificate validation

**When to use TCP checks:**
- Services without HTTP endpoints (databases, caches, custom protocols)
- Quick connectivity verification without application-level validation
- Services where the health endpoint isn't exposed

### Domains Configuration

Defines which domains OpenGSLB responds to and how traffic is routed.

```yaml
domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
      - us-west-2
    ttl: 30
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | Required | Fully qualified domain name to respond to |
| `routing_algorithm` | string | `round-robin` | Algorithm: `round-robin`, `weighted` |
| `regions` | list | Required | List of region names to route traffic to |
| `ttl` | integer | Uses `dns.default_ttl` | TTL for this domain's responses (overrides default) |

**Notes:**
- Domain names are matched exactly (no wildcard support currently)
- Queries for unconfigured domains receive NXDOMAIN
- All servers from all listed regions form the candidate pool for routing

## Duration Format

Duration fields accept Go duration strings:
- `30s` - 30 seconds
- `5m` - 5 minutes
- `1h` - 1 hour
- `500ms` - 500 milliseconds

## Complete Example

```yaml
# OpenGSLB Configuration
# /etc/opengslb/config.yaml

dns:
  listen_address: ":53"
  default_ttl: 60
  return_last_healthy: false

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9090"

regions:
  - name: us-east-1
    servers:
      - address: 10.0.1.10
        port: 80
        weight: 100
      - address: 10.0.1.11
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2

  - name: us-west-2
    servers:
      - address: 10.0.2.10
        port: 80
        weight: 150
      - address: 10.0.2.11
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2

  - name: eu-west-1
    servers:
      - address: 10.0.3.10
        port: 8080
        weight: 100
    health_check:
      type: tcp
      interval: 15s
      timeout: 3s

domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
      - us-west-2
    ttl: 30

  - name: api.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
      - us-west-2
      - eu-west-1
    ttl: 60

  - name: static.example.com
    routing_algorithm: round-robin
    regions:
      - us-west-2
    ttl: 300
```

## Example Configurations

### Single Region (Development)

Minimal configuration for development or single-datacenter deployments:

```yaml
dns:
  listen_address: ":5353"
  default_ttl: 30

logging:
  level: debug
  format: text

regions:
  - name: local
    servers:
      - address: 127.0.0.1
        port: 8080
      - address: 127.0.0.1
        port: 8081
    health_check:
      type: http
      interval: 10s
      timeout: 2s
      path: /health
      failure_threshold: 2
      success_threshold: 1

domains:
  - name: myapp.local
    routing_algorithm: round-robin
    regions:
      - local
```

### Multi-Region (Production)

Production configuration with multiple regions and strict health checking:

```yaml
dns:
  listen_address: ":53"
  default_ttl: 60
  return_last_healthy: false

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9090"

regions:
  - name: us-east-1
    servers:
      - address: 10.0.1.10
        port: 443
      - address: 10.0.1.11
        port: 443
      - address: 10.0.1.12
        port: 443
    health_check:
      type: https
      interval: 15s
      timeout: 3s
      path: /healthz
      failure_threshold: 3
      success_threshold: 2

  - name: us-west-2
    servers:
      - address: 10.0.2.10
        port: 443
      - address: 10.0.2.11
        port: 443
    health_check:
      type: https
      interval: 15s
      timeout: 3s
      path: /healthz
      failure_threshold: 3
      success_threshold: 2

  - name: eu-central-1
    servers:
      - address: 10.0.3.10
        port: 443
      - address: 10.0.3.11
        port: 443
    health_check:
      type: https
      interval: 15s
      timeout: 3s
      path: /healthz
      failure_threshold: 3
      success_threshold: 2

domains:
  - name: api.mycompany.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
      - us-west-2
      - eu-central-1
    ttl: 30
```

### High-Availability with Fast Failover

Configuration optimized for rapid failover detection:

```yaml
dns:
  listen_address: ":53"
  default_ttl: 10  # Short TTL for fast client updates

regions:
  - name: primary
    servers:
      - address: 10.0.1.10
        port: 80
      - address: 10.0.1.11
        port: 80
    health_check:
      type: http
      interval: 5s          # Check every 5 seconds
      timeout: 2s
      path: /health
      failure_threshold: 2  # Mark unhealthy after 10 seconds
      success_threshold: 1  # Recover immediately on success

domains:
  - name: critical-app.example.com
    routing_algorithm: round-robin
    regions:
      - primary
    ttl: 5  # Very short TTL
```

## Command Line Options

```
./opengslb [options]

Options:
  --config string    Path to configuration file (default "/etc/opengslb/config.yaml")
  --version          Show version information and exit
```

## Environment Variables

Currently, OpenGSLB does not support environment variable substitution in configuration files. All values must be specified directly in the YAML file.

## Validation

OpenGSLB validates configuration on startup and will fail with descriptive error messages for:
- Missing required fields
- Invalid duration formats
- Invalid port numbers
- Invalid log levels or formats
- Domains referencing non-existent regions
- Timeout >= interval for health checks
- Out-of-range threshold values

## Weighted Routing

Weighted routing distributes traffic proportionally based on server weights. Servers with higher weights receive more traffic.

### Configuration

```yaml
domains:
  - name: app.example.com
    routing_algorithm: weighted
    regions:
      - my-region
```

### How It Works

Traffic distribution is proportional to server weights:

| Server | Weight | Traffic Share |
|--------|--------|---------------|
| server1 | 150 | 50% |
| server2 | 100 | 33% |
| server3 | 50 | 17% |

The algorithm uses weighted random selection. On each DNS query, a server is randomly selected with probability proportional to its weight. Over many queries, the distribution matches the weight ratios.

### Weight Behavior

- **Weight > 0**: Server participates in selection with given weight
- **Weight = 0**: Server is excluded from selection (useful for soft-disabling)
- **Unhealthy servers**: Excluded regardless of weight

### Use Cases

- **Capacity-based distribution**: Route more traffic to higher-capacity servers
- **Gradual migrations**: Shift traffic by adjusting weights over time
- **Cost optimization**: Send less traffic to more expensive regions

### Comparison with Round-Robin

| Aspect | Round-Robin | Weighted |
|--------|-------------|----------|
| Distribution | Equal | Proportional to weight |
| Server weights | Ignored | Respected |
| Predictability | Deterministic rotation | Probabilistic |
| Use case | Homogeneous servers | Heterogeneous capacity |

### Example: Gradual Traffic Shift

To gradually shift traffic from old to new servers:

```yaml
# Week 1: 90% old, 10% new
servers:
  - address: "10.0.1.10"  # old
    weight: 90
  - address: "10.0.2.10"  # new
    weight: 10

# Week 2: 50% old, 50% new
servers:
  - address: "10.0.1.10"
    weight: 50
  - address: "10.0.2.10"
    weight: 50

# Week 3: 10% old, 90% new
servers:
  - address: "10.0.1.10"
    weight: 10
  - address: "10.0.2.10"
    weight: 90
```

## Failover (Active/Standby) Routing

Failover routing directs all traffic to the highest-priority healthy server. When that server becomes unhealthy, traffic automatically fails over to the next server in priority order.

### Configuration

```yaml
domains:
  - name: critical-app.example.com
    routing_algorithm: failover
    regions:
      - my-region
```

### How It Works

Server priority is determined by the order in the configuration file:

```yaml
servers:
  - address: "10.0.1.10"  # Priority 1 (Primary)
  - address: "10.0.1.11"  # Priority 2 (Secondary)  
  - address: "10.0.1.12"  # Priority 3 (Tertiary)
```

The routing behavior:

| Primary | Secondary | Tertiary | Traffic Goes To |
|---------|-----------|----------|-----------------|
| ✅ Healthy | ✅ Healthy | ✅ Healthy | Primary |
| ❌ Unhealthy | ✅ Healthy | ✅ Healthy | Secondary |
| ❌ Unhealthy | ❌ Unhealthy | ✅ Healthy | Tertiary |
| ❌ Unhealthy | ❌ Unhealthy | ❌ Unhealthy | SERVFAIL |

### Return-to-Primary Behavior

When a higher-priority server recovers, traffic **automatically returns** to it. This is the default and expected behavior for most disaster recovery scenarios.

Example timeline:
1. **T=0**: Primary healthy → traffic to Primary
2. **T=5**: Primary fails health checks → traffic to Secondary
3. **T=10**: Primary recovers → traffic returns to Primary

### Use Cases

- **Disaster Recovery**: Primary datacenter with hot standby
- **Maintenance Windows**: Graceful failover during updates
- **Cost Optimization**: Use expensive standby only when needed
- **Regulatory Compliance**: Ensure traffic stays in primary region when possible

### Comparison with Other Algorithms

| Aspect | Round-Robin | Weighted | Failover |
|--------|-------------|----------|----------|
| Traffic pattern | Distributed | Proportional | Single server |
| Predictability | Rotates | Probabilistic | Deterministic |
| Failover | Automatic | Automatic | Automatic |
| Recovery | N/A | N/A | Returns to primary |
| Use case | Load distribution | Capacity-based | DR/Active-standby |

### Health Check Recommendations

For failover routing, consider:

- **Short intervals** (10-15s): Detect failures quickly
- **Low failure threshold** (2-3): Fail over promptly
- **Higher success threshold** (3-5): Avoid flapping on recovery
- **Short DNS TTL** (15-30s): Clients update quickly after failover

```yaml
health_check:
  interval: 10s
  failure_threshold: 2   # Fail fast
  success_threshold: 3   # Recover carefully

domains:
  - name: app.example.com
    ttl: 15  # Short TTL for failover scenarios
```

### Monitoring Failover Events

Monitor these metrics to track failover:

- `opengslb_routing_decisions_total{algorithm="failover",server="..."}` - Which server is receiving traffic
- `opengslb_health_check_results_total{result="unhealthy"}` - Health check failures

A spike in traffic to the secondary server indicates a failover event.

## Configuration Hot-Reload

OpenGSLB supports reloading configuration without restarting the service. This allows you to add/remove domains and servers, change routing algorithms, and update health check settings with zero downtime.

### Triggering a Reload

Send `SIGHUP` to the OpenGSLB process:

```bash
# Find the process ID
pgrep opengslb

# Send SIGHUP
kill -HUP $(pgrep opengslb)

# Or in one command
pkill -HUP opengslb
```

### What Can Be Reloaded

| Setting | Hot-Reload | Notes |
|---------|------------|-------|
| Domains | ✅ Yes | Add, remove, or modify domains |
| Servers | ✅ Yes | Add, remove, or modify servers |
| Regions | ✅ Yes | Add, remove, or modify regions |
| Health check settings | ✅ Yes | Interval, timeout, thresholds |
| Routing algorithm | ✅ Yes | Change algorithm for domains |
| DNS TTL | ✅ Yes | Per-domain or default TTL |
| DNS listen address | ❌ No | Requires restart |
| Metrics port | ❌ No | Requires restart |

### Reload Behavior

1. **Validation first**: The new configuration is fully validated before any changes are applied
2. **Atomic swap**: Changes are applied atomically—partial updates don't happen
3. **Health state preserved**: Existing servers retain their health state during reload
4. **No query disruption**: In-flight DNS queries are not affected

### Reload Process

When you send SIGHUP:

1. OpenGSLB reads and validates the configuration file
2. If validation fails, the old configuration continues (error logged)
3. If validation succeeds:
   - DNS registry is updated with new domains
   - Health checks are started for new servers
   - Health checks are stopped for removed servers
   - Router is updated if algorithm changed
4. Success/failure is logged and recorded in metrics

### Monitoring Reloads

Check reload metrics in Prometheus:

```promql
# Total reload attempts by result
opengslb_config_reloads_total{result="success"}
opengslb_config_reloads_total{result="failure"}

# Timestamp of last successful reload
opengslb_config_reload_timestamp_seconds
```

### Logs

Successful reload:
```
level=INFO msg="received SIGHUP, reloading configuration"
level=INFO msg="reloading configuration" old_domains=2 new_domains=3 old_regions=1 new_regions=2
level=INFO msg="health manager reconfigured" added=2 removed=0 updated=0 total=5
level=INFO msg="configuration reload complete" domains=3 servers=5
level=INFO msg="configuration reloaded successfully"
```

Failed reload (invalid config):
```
level=INFO msg="received SIGHUP, reloading configuration"
level=ERROR msg="configuration reload failed" error="failed to load configuration: validation error: ..."
```

### Best Practices

1. **Validate before reload**: Test your config changes with `opengslb --config /path/to/new/config.yaml --validate` (if available) or in a staging environment

2. **Use version control**: Keep your configuration in git to track changes and enable rollback

3. **Monitor after reload**: Watch metrics and logs after reloading to confirm expected behavior

4. **Gradual changes**: Make incremental config changes rather than large rewrites

5. **Backup config**: Keep a known-good configuration file as backup

### Example: Adding a New Server

Original config:
```yaml
regions:
  - name: primary
    servers:
      - address: "10.0.1.10"
        port: 80
```

Updated config:
```yaml
regions:
  - name: primary
    servers:
      - address: "10.0.1.10"
        port: 80
      - address: "10.0.1.11"  # New server
        port: 80
```

Reload:
```bash
kill -HUP $(pgrep opengslb)
```

The new server will immediately begin health checks and be added to rotation once healthy.

### Example: Changing Routing Algorithm

```yaml
domains:
  - name: app.example.com
    routing_algorithm: weighted  # Changed from round-robin
```

After reload, traffic distribution will change to respect server weights.

## IPv6 Support

OpenGSLB supports both IPv4 and IPv6 addresses for backend servers. The DNS server automatically handles A (IPv4) and AAAA (IPv6) queries, returning only addresses of the appropriate family.

### Configuration

Simply configure servers with IPv6 addresses:

```yaml
regions:
  - name: us-east
    servers:
      - address: "10.0.1.10"      # IPv4
        port: 80
        weight: 100
      - address: "10.0.1.11"      # IPv4
        port: 80
        weight: 100
      - address: "2001:db8::1"    # IPv6
        port: 80
        weight: 100
      - address: "2001:db8::2"    # IPv6
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
```

### Query Behavior

| Query Type | Servers Considered | Response |
|------------|-------------------|----------|
| A (IPv4) | Only IPv4 servers | A record with IPv4 address |
| AAAA (IPv6) | Only IPv6 servers | AAAA record with IPv6 address |

### Mixed Environments

In environments with both IPv4 and IPv6 servers:

- **A queries** return only IPv4 addresses
- **AAAA queries** return only IPv6 addresses
- Each address family is load-balanced independently
- Health checks work for both IPv4 and IPv6 endpoints

### IPv4-Only or IPv6-Only Domains

If a domain only has servers of one address family:

- Queries for the available family return addresses normally
- Queries for the unavailable family return `NOERROR` with an empty answer section

This is standard DNS behavior indicating the domain exists but has no records of the requested type.

### Example: Dual-Stack Configuration

```yaml
regions:
  - name: primary-dc
    servers:
      # IPv4 servers
      - address: "192.168.1.10"
        port: 443
        weight: 100
      - address: "192.168.1.11"
        port: 443
        weight: 100
      # IPv6 servers
      - address: "2001:db8:1::10"
        port: 443
        weight: 100
      - address: "2001:db8:1::11"
        port: 443
        weight: 100
    health_check:
      type: http
      interval: 15s
      timeout: 3s
      path: /health

domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - primary-dc
    ttl: 30
```

### Testing IPv6

```bash
# Query for IPv4 address
dig @localhost -p 15353 app.example.com A +short
# Returns: 192.168.1.10 (or .11)

# Query for IPv6 address
dig @localhost -p 15353 app.example.com AAAA +short
# Returns: 2001:db8:1::10 (or ::11)
```

### Health Checks for IPv6

Health checks work identically for IPv6 servers. The health check URL is constructed using the IPv6 address in bracket notation:

```
http://[2001:db8:1::10]:443/health
```

TCP health checks connect to the IPv6 address directly.

### Notes

- IPv4-mapped IPv6 addresses (e.g., `::ffff:192.168.1.1`) are treated as IPv4
- Ensure your network infrastructure supports IPv6 if configuring IPv6 servers
- Health checks must be reachable via the configured address family