# OpenGSLB API Reference

OpenGSLB provides a REST API for health status monitoring and integration with external systems.

## Security Considerations

The API exposes internal infrastructure details including server addresses, health states, and failure information. **Secure access appropriately.**

By default, the API:
- Binds to `127.0.0.1:8080` (localhost only)
- Allows connections only from localhost
- Does not trust proxy headers

For production deployments, see [API Hardening Guide](security/api-hardening.md).

## Configuration

```yaml
api:
  enabled: true
  address: "127.0.0.1:8080"
  allowed_networks:
    - "127.0.0.1/32"
    - "::1/128"
  trust_proxy_headers: false
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the API server |
| `address` | string | `127.0.0.1:8080` | Listen address (IP:port) |
| `allowed_networks` | []string | `["127.0.0.1/32", "::1/128"]` | CIDR networks allowed to connect |
| `trust_proxy_headers` | bool | `false` | Trust X-Forwarded-For headers |

## Endpoints

### GET /api/v1/health/servers

Returns health status for all configured servers.

**Response:**

```json
{
  "servers": [
    {
      "address": "10.0.1.10",
      "port": 80,
      "region": "us-east-1",
      "healthy": true,
      "status": "healthy",
      "last_check": "2025-01-15T10:30:00Z",
      "last_healthy": "2025-01-15T10:30:00Z",
      "consecutive_failures": 0,
      "consecutive_successes": 5,
      "last_error": null
    },
    {
      "address": "10.0.1.11",
      "port": 80,
      "region": "us-east-1",
      "healthy": false,
      "status": "unhealthy",
      "last_check": "2025-01-15T10:30:00Z",
      "last_healthy": "2025-01-15T10:25:00Z",
      "consecutive_failures": 3,
      "consecutive_successes": 0,
      "last_error": "connection refused"
    }
  ],
  "generated_at": "2025-01-15T10:30:05Z"
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `address` | string | Server IP address |
| `port` | int | Server port |
| `region` | string | Region name (if configured) |
| `healthy` | bool | Current health status |
| `status` | string | Status string: "healthy", "unhealthy", "unknown" |
| `last_check` | string | ISO 8601 timestamp of last health check |
| `last_healthy` | string | ISO 8601 timestamp when last healthy |
| `consecutive_failures` | int | Number of consecutive failed checks |
| `consecutive_successes` | int | Number of consecutive successful checks |
| `last_error` | string | Error message from last failed check (null if healthy) |

### GET /api/v1/health/regions

Returns health summary aggregated by region.

**Response:**

```json
{
  "regions": [
    {
      "name": "us-east-1",
      "total_servers": 3,
      "healthy_count": 2,
      "unhealthy_count": 1,
      "health_percent": 66.67
    },
    {
      "name": "us-west-2",
      "total_servers": 2,
      "healthy_count": 2,
      "unhealthy_count": 0,
      "health_percent": 100.0
    }
  ],
  "generated_at": "2025-01-15T10:30:05Z"
}
```

### GET /api/v1/ready

Readiness probe for load balancers and orchestrators. Returns 200 when ready, 503 when not ready.

**Response (ready):**

```json
{
  "ready": true,
  "dns_ready": true,
  "health_ready": true
}
```

**Response (not ready):**

```json
{
  "ready": false,
  "message": "health checks not ready",
  "dns_ready": true,
  "health_ready": false
}
```

**Readiness Criteria:**
- DNS server is initialized and listening
- At least one health check cycle has completed

### GET /api/v1/live

Liveness probe. Returns 200 if the process is running.

**Response:**

```json
{
  "alive": true
}
```

## Error Responses

All endpoints return errors in a consistent format:

```json
{
  "error": "method not allowed",
  "code": 405
}
```

| Status Code | Description |
|-------------|-------------|
| 403 | Forbidden - IP not in allowed_networks |
| 405 | Method not allowed - use GET |
| 500 | Internal server error |
| 503 | Service unavailable (for /ready endpoint) |

## Usage Examples

### curl

```bash
# Check server health
curl http://localhost:8080/api/v1/health/servers

# Check readiness
curl -w "%{http_code}" http://localhost:8080/api/v1/ready

# Pretty print with jq
curl -s http://localhost:8080/api/v1/health/servers | jq .
```

### Monitoring Integration

**Prometheus (via blackbox_exporter or custom scraper):**

The API complements the `/metrics` endpoint. Use `/api/v1/ready` as a health check target.

**Alertmanager webhook:**

Poll `/api/v1/health/servers` and alert on `consecutive_failures > 0`.

### Kubernetes Probes

```yaml
livenessProbe:
  httpGet:
    path: /api/v1/live
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /api/v1/ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
```