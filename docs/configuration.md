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

## Runtime Mode (ADR-015)

OpenGSLB operates in one of two modes:

| Mode | Description |
|------|-------------|
| `overwatch` | DNS-serving, health-validating authority node. Receives agent heartbeats, validates health claims, serves DNS. |
| `agent` | Health-reporting agent on application servers. Monitors local backends, gossips status to Overwatch nodes. |

```yaml
# Set runtime mode
mode: overwatch  # or "agent"
```

If `mode` is not specified, OpenGSLB defaults to `overwatch` mode.

## Agent Mode Configuration

Agent mode runs on application servers to monitor local backends and report health to Overwatch nodes.

```yaml
mode: agent

agent:
  identity:
    service_token: "pre-shared-token-for-auth"
    region: "us-east-1"
    cert_path: /var/lib/opengslb/agent.crt
    key_path: /var/lib/opengslb/agent.key

  backends:
    - service: "web-service"
      address: "127.0.0.1"
      port: 8080
      weight: 100
      health_check:
        type: http
        interval: 10s
        timeout: 5s
        path: /health
        failure_threshold: 3
        success_threshold: 2

  gossip:
    encryption_key: "base64-encoded-32-byte-key"
    overwatch_nodes:
      - "overwatch-1.internal:7946"
      - "overwatch-2.internal:7946"

  heartbeat:
    interval: 10s
    missed_threshold: 3

  predictive:
    enabled: true
    cpu:
      threshold: 80
      bleed_duration: 30s
    memory:
      threshold: 85
      bleed_duration: 30s
    error_rate:
      threshold: 5
      window: 60s
      bleed_duration: 30s
```

### Agent Identity Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `service_token` | string | Required | Pre-shared token for initial authentication with Overwatch |
| `region` | string | Required | Geographic region this agent belongs to |
| `cert_path` | string | `/var/lib/opengslb/agent.crt` | Path to store/load agent certificate |
| `key_path` | string | `/var/lib/opengslb/agent.key` | Path to store/load agent private key |

### Agent Backend Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `service` | string | Required | Service name (maps to DNS domain) |
| `address` | string | Required | Backend server IP address |
| `port` | integer | Required | Backend server port |
| `weight` | integer | `100` | Routing weight (1-1000) |
| `health_check` | object | Required | Health check configuration |

### Agent Gossip Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `encryption_key` | string | Required | 32-byte base64-encoded encryption key |
| `overwatch_nodes` | list | Required | List of Overwatch gossip addresses |

Generate an encryption key with:
```bash
openssl rand -base64 32
```

### Agent Heartbeat Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `interval` | duration | `10s` | Time between heartbeat messages |
| `missed_threshold` | integer | `3` | Missed heartbeats before deregistration |

### Agent Predictive Health Settings

Predictive health allows agents to signal impending failures before they impact traffic.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable predictive health monitoring |
| `cpu.threshold` | float | `80` | CPU usage percentage to trigger bleed |
| `cpu.bleed_duration` | duration | `30s` | Duration to gradually drain traffic |
| `memory.threshold` | float | `85` | Memory usage percentage to trigger bleed |
| `memory.bleed_duration` | duration | `30s` | Duration to gradually drain traffic |
| `error_rate.threshold` | float | `5` | Error rate percentage to trigger bleed |
| `error_rate.window` | duration | `60s` | Window for error rate calculation |
| `error_rate.bleed_duration` | duration | `30s` | Duration to gradually drain traffic |

### Agent Latency Learning Settings (ADR-017)

Passive latency learning allows agents to collect real client-to-backend TCP RTT data and report it to Overwatch for intelligent routing. This captures actual client experience rather than Overwatch-to-backend latency.

```yaml
agent:
  latency_learning:
    enabled: true
    poll_interval: 10s
    min_connection_age: 5s
    ipv4_prefix: 24
    ipv6_prefix: 48
    ewma_alpha: 0.3
    max_subnets: 100000
    subnet_ttl: 168h
    min_samples: 5
    report_interval: 30s
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable passive latency learning |
| `poll_interval` | duration | `10s` | How often to poll OS for TCP connection RTT data |
| `min_connection_age` | duration | `5s` | Minimum connection age before collecting RTT (new connections have unstable RTT) |
| `ipv4_prefix` | integer | `24` | IPv4 subnet prefix for aggregation (e.g., /24 groups all 10.0.1.x together) |
| `ipv6_prefix` | integer | `48` | IPv6 subnet prefix for aggregation |
| `ewma_alpha` | float | `0.3` | EWMA smoothing factor (0-1). Higher = more responsive to recent samples |
| `max_subnets` | integer | `100000` | Maximum subnets to track (prevents unbounded memory growth) |
| `subnet_ttl` | duration | `168h` | How long to keep subnet entries without updates (7 days default) |
| `min_samples` | integer | `5` | Minimum samples before reporting a subnet's latency |
| `report_interval` | duration | `30s` | How often to send latency reports to Overwatch via gossip |

**Requirements:**
- **Linux**: CAP_NET_ADMIN capability or root privileges
- **Windows**: Administrator privileges (uses GetPerTcpConnectionEStats API)

Grant capability on Linux:
```bash
sudo setcap cap_net_admin+ep /usr/local/bin/opengslb
```

## Overwatch Mode Configuration

Overwatch mode serves DNS and validates health claims from agents.

```yaml
mode: overwatch

overwatch:
  identity:
    node_id: "overwatch-1"
    region: "us-east-1"

  agent_tokens:
    web-service: "pre-shared-token-for-web-service"
    api-service: "pre-shared-token-for-api-service"

  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "base64-encoded-32-byte-key"
    probe_interval: 1s
    probe_timeout: 500ms
    gossip_interval: 200ms

  validation:
    enabled: true
    check_interval: 30s
    check_timeout: 5s

  stale:
    threshold: 30s
    remove_after: 5m

  data_dir: /var/lib/opengslb

  dnssec:
    enabled: true
    algorithm: ECDSAP256SHA256
```

### Overwatch Identity Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `node_id` | string | hostname | Unique identifier for this Overwatch node |
| `region` | string | (empty) | Geographic region this Overwatch serves |

### Overwatch Agent Tokens

Map of service names to authentication tokens. Agents must provide matching tokens to register.

```yaml
overwatch:
  agent_tokens:
    web-service: "token-for-web"
    api-service: "token-for-api"
```

### Overwatch Gossip Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `bind_address` | string | `0.0.0.0:7946` | Address to listen for agent gossip |
| `encryption_key` | string | Required | 32-byte base64-encoded key (must match agents) |
| `probe_interval` | duration | `1s` | Interval between failure probes |
| `probe_timeout` | duration | `500ms` | Timeout for a single probe |
| `gossip_interval` | duration | `200ms` | Interval between gossip messages |

### Overwatch Validation Settings

External validation allows Overwatch to independently verify agent health claims.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable external health validation |
| `check_interval` | duration | `30s` | Frequency of validation checks |
| `check_timeout` | duration | `5s` | Timeout for validation checks |

**Important:** Per ADR-015, Overwatch validation ALWAYS wins over agent claims. This prevents agents from falsely claiming healthy status.

### Overwatch Stale Settings

Configure when backends are considered stale (no recent heartbeat from agent).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `threshold` | duration | `30s` | Time without heartbeat before marking stale |
| `remove_after` | duration | `5m` | Time after which stale backends are removed |

### Overwatch Data Directory

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `data_dir` | string | `/var/lib/opengslb` | Directory for persistent data (bbolt database) |

### Overwatch DNSSEC Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable DNSSEC signing |
| `security_acknowledgment` | string | (empty) | Required if disabling DNSSEC |
| `algorithm` | string | `ECDSAP256SHA256` | DNSSEC signing algorithm |

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
        service: "app.example.com"  # REQUIRED in v1.1.0
      - address: 10.0.1.11
        port: 80
        weight: 100
        service: "app.example.com"  # REQUIRED in v1.1.0
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
| `service` | string | **Required** | **v1.1.0+**: Domain/service this server belongs to (must match a configured domain name) |
| `address` | string | Required | IP address of the backend server |
| `port` | integer | `80` | Port number for health checks |
| `weight` | integer | `100` | Server weight for weighted routing (1-1000) |
| `host` | string | (empty) | Hostname for HTTPS health checks (for TLS SNI and certificate validation) |

**BREAKING CHANGE (v1.1.0):** The `service` field is now required for all servers. This enables the unified server architecture where static, agent-registered, and API-registered servers all use the same validation system. The service field specifies which domain/service the server belongs to.

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
| `routing_algorithm` | string | `round-robin` | Algorithm: `round-robin`, `weighted`, `failover`, `geolocation`, `latency` |
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

## Geolocation Routing

Geolocation routing directs traffic to servers based on the client's geographic location. OpenGSLB uses MaxMind GeoIP2/GeoLite2 databases to resolve client IP addresses to geographic regions.

### Configuration

```yaml
domains:
  - name: app.example.com
    routing_algorithm: geolocation
    regions:
      - us-east-1
      - eu-west-1
      - ap-southeast-1

geolocation:
  database_path: "/var/lib/opengslb/geoip/GeoLite2-Country.mmdb"
  default_region: us-east-1
  ecs_enabled: true
  custom_mappings:
    - cidr: "10.0.0.0/8"
      region: us-east-1
    - cidr: "172.16.0.0/12"
      region: eu-west-1
```

### Geolocation Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `database_path` | string | Required | Path to MaxMind GeoIP2/GeoLite2 database file (.mmdb) |
| `default_region` | string | Required | Fallback region when geolocation lookup fails |
| `ecs_enabled` | boolean | `true` | Enable EDNS Client Subnet support for accurate client location |
| `custom_mappings` | list | (empty) | Custom CIDR-to-region mappings |

### Custom CIDR Mappings

Custom mappings override GeoIP lookups for specific IP ranges. This is useful for:
- Internal networks that should route to specific regions
- Known customer IP ranges with preferred regions
- Overriding incorrect GeoIP data

```yaml
custom_mappings:
  - cidr: "10.0.0.0/8"        # Internal US network
    region: us-east-1
  - cidr: "192.168.0.0/16"    # Internal EU network
    region: eu-west-1
  - cidr: "203.0.113.0/24"    # Customer's APAC network
    region: ap-southeast-1
```

Custom mappings use longest-prefix matching—the most specific CIDR match wins.

### Region Configuration for Geolocation

Regions must specify which countries or continents they serve:

```yaml
regions:
  - name: us-east-1
    countries: ["US", "CA", "MX"]
    continents: ["NA", "SA"]
    servers:
      - address: "10.0.1.10"
        port: 8080

  - name: eu-west-1
    countries: ["GB", "DE", "FR", "NL", "BE"]
    continents: ["EU"]
    servers:
      - address: "10.0.2.10"
        port: 8080

  - name: ap-southeast-1
    countries: ["SG", "MY", "TH", "VN", "ID"]
    continents: ["AS", "OC"]
    servers:
      - address: "10.0.3.10"
        port: 8080
```

| Field | Type | Description |
|-------|------|-------------|
| `countries` | list | ISO 3166-1 alpha-2 country codes served by this region |
| `continents` | list | Continent codes: AF, AN, AS, EU, NA, OC, SA |

### EDNS Client Subnet (ECS) Support

When `ecs_enabled: true`, OpenGSLB extracts client location from ECS information in DNS queries. This provides more accurate geolocation when queries come from recursive resolvers (like Google DNS or Cloudflare) that include client subnet data.

### GeoIP Database Setup

Download a MaxMind GeoLite2 database:

```bash
# Register at maxmind.com for a free license key
# Download GeoLite2-Country.mmdb

mkdir -p /var/lib/opengslb/geoip
mv GeoLite2-Country.mmdb /var/lib/opengslb/geoip/
chown opengslb:opengslb /var/lib/opengslb/geoip/GeoLite2-Country.mmdb
```

For production deployments, automate database updates using MaxMind's `geoipupdate` tool. See the GeoIP maintenance runbook in the operations documentation.

### Monitoring Geolocation Routing

Monitor these metrics:

- `opengslb_geo_routing_decision{country="...",continent="...",region="..."}` - Routing decisions by location
- `opengslb_geo_fallback{reason="..."}` - Fallback events and reasons
- `opengslb_geo_custom_mapping_hit{region="..."}` - Custom CIDR mapping matches

## Latency-Based Routing

Latency-based routing directs traffic to the server with the lowest measured latency. This algorithm continuously measures latency during health checks and uses exponential moving average (EMA) smoothing to prevent routing flapping.

### Configuration

```yaml
domains:
  - name: app.example.com
    routing_algorithm: latency
    regions:
      - us-east-1
      - us-west-2

latency_config:
  smoothing_factor: 0.3
  max_latency_ms: 500
  min_samples: 3
```

### Latency Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `smoothing_factor` | float | `0.3` | EMA smoothing factor (0.0-1.0). Higher = more responsive, lower = more stable |
| `max_latency_ms` | integer | `500` | Maximum acceptable latency in milliseconds. Servers exceeding this are excluded |
| `min_samples` | integer | `3` | Minimum latency samples required before using server for routing |

### How It Works

1. **Latency measurement**: During each health check, OpenGSLB measures the TCP connection time to the backend
2. **EMA smoothing**: Latency values are smoothed using exponential moving average to prevent routing flapping from transient spikes
3. **Server selection**: The server with the lowest smoothed latency is selected
4. **Threshold enforcement**: Servers with latency exceeding `max_latency_ms` are excluded from selection
5. **Automatic fallback**: Falls back to round-robin when insufficient latency data is available

### Smoothing Factor

The smoothing factor controls how responsive the latency calculation is to new measurements:

| Factor | Behavior |
|--------|----------|
| `0.1` | Very stable, slow to react to changes |
| `0.3` | Balanced (default) |
| `0.5` | Moderate responsiveness |
| `0.8` | Highly responsive, may flap on spikes |

Formula: `new_latency = (smoothing_factor * measured) + ((1 - smoothing_factor) * previous)`

### Use Cases

- **Global deployments**: Route users to the fastest regional server
- **Multi-cloud**: Route to the cloud provider with best current performance
- **Hybrid deployments**: Balance between on-premises and cloud based on network conditions

### Example: Multi-Region Latency Routing

```yaml
dns:
  default_ttl: 30

regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 8080
      - address: "10.0.1.11"
        port: 8080
    health_check:
      type: http
      interval: 10s
      timeout: 5s
      path: /health

  - name: us-west-2
    servers:
      - address: "10.0.2.10"
        port: 8080
    health_check:
      type: http
      interval: 10s
      timeout: 5s
      path: /health

  - name: eu-west-1
    servers:
      - address: "10.0.3.10"
        port: 8080
    health_check:
      type: http
      interval: 10s
      timeout: 5s
      path: /health

domains:
  - name: api.example.com
    routing_algorithm: latency
    regions:
      - us-east-1
      - us-west-2
      - eu-west-1
    ttl: 30

latency_config:
  smoothing_factor: 0.3
  max_latency_ms: 200
  min_samples: 5
```

### Monitoring Latency Routing

Monitor these metrics:

- `opengslb_latency_routing_decision{server="...",latency_ms="..."}` - Selected server and its latency
- `opengslb_latency_rejection{server="...",reason="..."}` - Servers excluded due to high latency or insufficient samples
- `opengslb_health_check_latency_seconds{server="..."}` - Raw health check latency measurements

### Combining with Geolocation

For optimal performance, consider using geolocation routing with latency as a secondary factor. Configure regions geographically, and latency routing will select the fastest server within the client's region.

## Learned Latency Routing (ADR-017)

Learned latency routing uses **passive TCP RTT data** collected by agents to route clients to the backend with the lowest measured latency. Unlike standard latency routing (which measures Overwatch-to-backend latency), this captures the actual client-to-backend experience.

### How It Differs from Standard Latency Routing

| Aspect | Standard Latency | Learned Latency |
|--------|-----------------|-----------------|
| **What's measured** | Overwatch → Backend | Client → Backend |
| **Measurement method** | Active health check probes | Passive TCP RTT from OS |
| **Accuracy** | Proxy's perspective | Client's actual experience |
| **Data source** | Overwatch only | Agent gossip |
| **Cold start** | Falls back to round-robin | Falls back to geolocation |

### Configuration

```yaml
domains:
  - name: app.example.com
    routing_algorithm: learned_latency
    regions:
      - us-east
      - us-west
      - eu-west
      - ap-southeast
    ttl: 60
    latency_config:
      max_latency_ms: 300
      min_samples: 5
```

**Important**: Learned latency routing requires agents with `latency_learning.enabled: true` to collect and gossip RTT data.

### Learned Latency Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_latency_ms` | integer | `500` | Exclude backends with latency above this threshold |
| `min_samples` | integer | `3` | Minimum samples required before using learned data for a subnet |

### How It Works

1. **Agents collect TCP RTT**: When clients connect to backends, agents read TCP connection RTT from the OS kernel
2. **Subnet aggregation**: RTT samples are aggregated by client subnet (default /24 for IPv4)
3. **Gossip to Overwatch**: Agents periodically send latency reports to all Overwatch nodes
4. **DNS routing**: When a query arrives, Overwatch looks up learned latency for that client's subnet and selects the lowest-latency backend
5. **Cold start fallback**: If no learned data exists for a subnet, falls back to geolocation routing

### Viewing Learned Latency Data

Query the Overwatch API to see collected latency data:

```bash
curl http://localhost:9090/api/v1/overwatch/latency | jq .
```

Example response:
```json
{
  "entries": [
    {
      "subnet": "10.1.2.0/24",
      "domain": "app.example.com",
      "region": "eu-west",
      "rtt_ms": 85,
      "samples": 150,
      "last_updated": "2025-12-19T10:05:00Z"
    }
  ]
}
```

### Use Cases

- **True client optimization**: Route based on actual client experience, not proxy measurements
- **CDN-like behavior**: Automatically route clients to their lowest-latency backend
- **Multi-cloud arbitrage**: Discover which cloud provider is fastest for each client subnet
- **ISP-aware routing**: Different ISPs may have different latency to your backends

### Example: Full Learned Latency Deployment

**Overwatch configuration:**
```yaml
mode: overwatch

domains:
  - name: app.example.com
    routing_algorithm: learned_latency
    regions:
      - us-east
      - eu-west
      - ap-southeast
    latency_config:
      max_latency_ms: 300
      min_samples: 5

overwatch:
  geolocation:
    database_path: /var/lib/opengslb/GeoLite2-Country.mmdb
    default_region: us-east
```

**Agent configuration:**
```yaml
mode: agent

agent:
  backends:
    - service: "app.example.com"
      address: "127.0.0.1"
      port: 80
      weight: 100

  latency_learning:
    enabled: true
    poll_interval: 10s
    min_connection_age: 5s
    report_interval: 30s
```

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

## Multi-File Configuration (Includes)

For large deployments with many domains managed by different teams, OpenGSLB supports splitting configuration across multiple files. This enables team-based configuration management while maintaining centralized infrastructure settings.

### Basic Usage

Use the `includes` directive in your main configuration to include additional files:

```yaml
# /etc/opengslb/config.yaml
mode: overwatch

dns:
  listen_address: ":53"
  zones:
    - gslb.example.com

includes:
  - regions/*.yaml          # Load all region files
  - domains/**/*.yaml       # Recursively load domain files
  - tokens.yaml             # Load agent tokens
```

### Glob Patterns

The `includes` directive supports glob patterns:

| Pattern | Matches |
|---------|---------|
| `*.yaml` | All YAML files in the current directory |
| `regions/*.yaml` | All YAML files in the regions/ subdirectory |
| `domains/**/*.yaml` | All YAML files recursively under domains/ |
| `tokens.yaml` | Specific file |

Patterns are relative to the main configuration file's directory.

### Merge Semantics

When multiple files are loaded, content is merged according to these rules:

| Field | Merge Behavior |
|-------|----------------|
| `regions` | Arrays are concatenated |
| `domains` | Arrays are concatenated |
| `agent_tokens` | Maps are merged (later values override) |
| `agent.backends` | Arrays are concatenated |
| `geolocation.custom_mappings` | Arrays are concatenated |
| Other scalars | Only from main file (includes cannot override) |

### Example Directory Structure

```
/etc/opengslb/
├── config.yaml              # Main configuration
├── regions/
│   ├── us-east.yaml         # US East region
│   ├── us-west.yaml         # US West region
│   └── eu-west.yaml         # EU West region
├── domains/
│   ├── team-a/
│   │   └── app.yaml         # Team A's application domain
│   └── team-b/
│       └── api.yaml         # Team B's API domain
└── tokens.yaml              # Agent authentication tokens
```

### Example Files

**Main config (config.yaml)**:
```yaml
mode: overwatch

dns:
  listen_address: ":53"
  zones:
    - gslb.example.com
  default_ttl: 30

overwatch:
  gossip:
    encryption_key: "YOUR_KEY_HERE"
  dnssec:
    enabled: true

logging:
  level: info
  format: json

includes:
  - regions/*.yaml
  - domains/**/*.yaml
  - tokens.yaml
```

**Region file (regions/us-east.yaml)**:
```yaml
regions:
  - name: us-east-1
    countries: ["US", "CA", "MX"]
    continents: ["NA", "SA"]
    servers:
      - address: "10.0.1.10"
        port: 8080
      - address: "10.0.1.11"
        port: 8080
    health_check:
      type: http
      path: /health
      interval: 30s
```

**Domain file (domains/team-a/app.yaml)**:
```yaml
domains:
  - name: app.gslb.example.com
    routing_algorithm: round-robin
    regions:
      - us-east-1
      - us-west-2
    ttl: 30
```

**Tokens file (tokens.yaml)**:
```yaml
overwatch:
  agent_tokens:
    team-a-app: "secret-token-for-team-a"
    team-b-api: "secret-token-for-team-b"
```

### Error Handling

OpenGSLB provides clear error messages with file context:

**Duplicate region name**:
```
regions/backup.yaml: duplicate region name "us-east-1"
```

**Circular include**:
```
circular include detected: config.yaml -> base.yaml -> config.yaml
```

**Permission error**:
```
regions/insecure.yaml: permission check failed: file is world-writable, which is a security risk
```

### Security

- Included files undergo the same permission checks as the main config
- World-writable files are rejected
- Maximum include depth is 10 levels (prevents infinite recursion)
- Circular includes are detected and rejected

### Hot-Reload with Includes

When you send SIGHUP, all included files are re-read along with the main configuration. This means:

1. Changes to any included file take effect on reload
2. New files matching glob patterns are automatically included
3. Removed files are no longer included

```bash
# Reload after editing any config file
kill -HUP $(pgrep opengslb)
```

### Nested Includes

Included files can themselves contain `includes` directives:

```yaml
# base.yaml
includes:
  - regions/*.yaml
```

This allows for modular configuration hierarchies, but be careful not to create circular dependencies.

### Best Practices

1. **Separate by responsibility**: Keep infrastructure settings in the main file, let teams manage their domains
2. **Use descriptive directories**: `domains/team-a/` is clearer than `domains/a/`
3. **Document ownership**: Add comments indicating who manages each file
4. **Secure sensitive files**: Keep tokens in a separate file with restrictive permissions
5. **Version control**: Track all configuration files in git for audit trail

### Validating Configuration

Validate your multi-file configuration before deploying:

```bash
opengslb-cli config validate --config /etc/opengslb/config.yaml
```

This will load all included files and report any validation errors.

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