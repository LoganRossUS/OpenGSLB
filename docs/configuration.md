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
| `address` | string | Required | IP address or hostname of the backend server |
| `port` | integer | `80` | Port number for health checks |
| `weight` | integer | `100` | Server weight for weighted routing (1-1000) |

#### Health Check Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `http` | Check type: `http`, `https`, or `tcp` |
| `interval` | duration | `30s` | Time between health checks |
| `timeout` | duration | `5s` | Timeout for each check (must be < interval) |
| `path` | string | `/health` | HTTP/HTTPS path to check |
| `failure_threshold` | integer | `3` | Consecutive failures before marking unhealthy (1-10) |
| `success_threshold` | integer | `2` | Consecutive successes before marking healthy (1-10) |

**Health check behavior:**
- HTTP/HTTPS checks expect a 2xx response code
- TCP checks only verify successful TCP connection (no data exchange)
- A server starts as healthy and requires `failure_threshold` consecutive failures to become unhealthy
- An unhealthy server requires `success_threshold` consecutive successes to become healthy again

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