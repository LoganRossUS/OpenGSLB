# OpenGSLB API Reference

OpenGSLB provides a REST API for health status monitoring, external health overrides, DNSSEC key management, and Overwatch agent management.

**Base URL:** `http://localhost:8080/api/v1`

## Table of Contents

- [Security & Authentication](#security--authentication)
- [Configuration](#configuration)
- [Core Health Endpoints](#core-health-endpoints)
  - [GET /api/v1/health/servers](#get-apiv1healthservers)
  - [GET /api/v1/health/regions](#get-apiv1healthregions)
- [Liveness & Readiness Probes](#liveness--readiness-probes)
  - [GET /api/v1/ready](#get-apiv1ready)
  - [GET /api/v1/live](#get-apiv1live)
- [Override API](#override-api)
  - [GET /api/v1/overrides](#get-apiv1overrides)
  - [GET /api/v1/overrides/{service}/{address}](#get-apiv1overridesserviceaddress)
  - [PUT /api/v1/overrides/{service}/{address}](#put-apiv1overridesserviceaddress)
  - [DELETE /api/v1/overrides/{service}/{address}](#delete-apiv1overridesserviceaddress)
- [DNSSEC API](#dnssec-api)
  - [GET /api/v1/dnssec/status](#get-apiv1dnssecstatus)
  - [GET /api/v1/dnssec/ds](#get-apiv1dnssecds)
  - [GET /api/v1/dnssec/keys](#get-apiv1dnsseckeys)
  - [POST /api/v1/dnssec/sync](#post-apiv1dnssecsync)
- [Overwatch API](#overwatch-api)
  - [GET /api/v1/overwatch/backends](#get-apiv1overwatchbackends)
  - [GET /api/v1/overwatch/stats](#get-apiv1overwatchstats)
  - [POST /api/v1/overwatch/backends/{service}/{address}/{port}/override](#post-apiv1overwatchbackendsserviceaddressportoverride)
  - [DELETE /api/v1/overwatch/backends/{service}/{address}/{port}/override](#delete-apiv1overwatchbackendsserviceaddressportoverride)
  - [POST /api/v1/overwatch/validate](#post-apiv1overwatchvalidate)
  - [GET /api/v1/overwatch/agents](#get-apiv1overwatchagents)
  - [GET /api/v1/overwatch/agents/{agent_id}](#get-apiv1overwatchagentsagent_id)
  - [DELETE /api/v1/overwatch/agents/{agent_id}](#delete-apiv1overwatchagentsagent_id)
  - [POST /api/v1/overwatch/agents/{agent_id}/revoke](#post-apiv1overwatchagentsagent_idrevoke)
  - [GET /api/v1/overwatch/agents/expiring](#get-apiv1overwatchagentsexpiring)
- [Error Responses](#error-responses)
- [Usage Examples](#usage-examples)

---

## Security & Authentication

The API exposes internal infrastructure details including server addresses, health states, and failure information. **Secure access appropriately.**

### Access Control

| Endpoint | ACL Protected |
|----------|---------------|
| `/api/v1/health/*` | Yes |
| `/api/v1/overrides/*` | Yes |
| `/api/v1/dnssec/*` | Yes |
| `/api/v1/overwatch/*` | Yes |
| `/api/v1/ready` | **No** |
| `/api/v1/live` | **No** |

By default, the API:
- Binds to `127.0.0.1:8080` (localhost only)
- Allows connections only from localhost (`127.0.0.1/32`, `::1/128`)
- Does not trust proxy headers

For production deployments, see [API Hardening Guide](security/api-hardening.md).

---

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

---

## Core Health Endpoints

### GET /api/v1/health/servers

Returns health status for all configured servers.

**ACL Protected:** Yes

**Response:** `200 OK`

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

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `address` | string | Server IP address |
| `port` | int | Server port |
| `region` | string | Region name (if configured) |
| `healthy` | bool | Current health status |
| `status` | string | Status string: `healthy`, `unhealthy`, `unknown` |
| `last_check` | string | ISO 8601 timestamp of last health check |
| `last_healthy` | string | ISO 8601 timestamp when last healthy |
| `consecutive_failures` | int | Number of consecutive failed checks |
| `consecutive_successes` | int | Number of consecutive successful checks |
| `last_error` | string | Error message from last failed check (null if healthy) |

---

### GET /api/v1/health/regions

Returns health summary aggregated by region.

**ACL Protected:** Yes

**Response:** `200 OK`

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

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Region name |
| `total_servers` | int | Total servers in region |
| `healthy_count` | int | Number of healthy servers |
| `unhealthy_count` | int | Number of unhealthy servers |
| `health_percent` | float | Percentage of healthy servers |

---

## Liveness & Readiness Probes

These endpoints are **not** ACL protected and accessible from anywhere.

### GET /api/v1/ready

Readiness probe for load balancers and orchestrators.

**ACL Protected:** No

**Response (ready):** `200 OK`

```json
{
  "ready": true,
  "dns_ready": true,
  "health_ready": true
}
```

**Response (not ready):** `503 Service Unavailable`

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

---

### GET /api/v1/live

Liveness probe. Returns 200 if the process is running.

**ACL Protected:** No

**Response:** `200 OK`

```json
{
  "alive": true
}
```

---

## Override API

External health overrides for individual servers. Allows external systems to mark servers healthy/unhealthy independent of health checks.

### GET /api/v1/overrides

List all active health overrides.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "overrides": [
    {
      "service": "api-service",
      "address": "10.0.1.10",
      "healthy": false,
      "reason": "Maintenance scheduled",
      "source": "monitoring_tool",
      "created_at": "2025-01-15T10:30:00Z",
      "authority": "192.168.1.50"
    }
  ]
}
```

---

### GET /api/v1/overrides/{service}/{address}

Get a specific override for a service/address combination.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service` | string | Service name |
| `address` | string | Server IP address |

**Response:** `200 OK`

```json
{
  "service": "api-service",
  "address": "10.0.1.10",
  "healthy": false,
  "reason": "Maintenance scheduled",
  "source": "monitoring_tool",
  "created_at": "2025-01-15T10:30:00Z",
  "authority": "192.168.1.50"
}
```

**Error Response:** `404 Not Found`

```json
{
  "error": "override not found",
  "code": 404
}
```

---

### PUT /api/v1/overrides/{service}/{address}

Set or update a health override for a server.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service` | string | Service name |
| `address` | string | Server IP address |

**Request Body:**

```json
{
  "healthy": false,
  "reason": "Maintenance window",
  "source": "external_monitoring_tool"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `healthy` | bool | No | Override health status (default: false) |
| `reason` | string | No | Human-readable reason for override |
| `source` | string | **Yes** | Identifier for the system setting the override |

**Response:** `200 OK`

```json
{
  "service": "api-service",
  "address": "10.0.1.10",
  "healthy": false,
  "reason": "Maintenance window",
  "source": "external_monitoring_tool",
  "created_at": "2025-01-15T10:30:00Z",
  "authority": "192.168.1.50"
}
```

**Error Response:** `400 Bad Request`

```json
{
  "error": "source is required",
  "code": 400
}
```

---

### DELETE /api/v1/overrides/{service}/{address}

Clear a health override for a server.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service` | string | Service name |
| `address` | string | Server IP address |

**Response:** `204 No Content`

**Error Response:** `404 Not Found`

```json
{
  "error": "override not found",
  "code": 404
}
```

---

## DNSSEC API

DNSSEC key management and synchronization endpoints. These endpoints are available when DNSSEC is enabled.

### GET /api/v1/dnssec/status

Returns the current DNSSEC status including key info and sync status.

**ACL Protected:** Yes

**Response (DNSSEC enabled):** `200 OK`

```json
{
  "enabled": true,
  "keys": [
    {
      "zone": "example.com.",
      "key_tag": 12345,
      "algorithm": 8,
      "flags": 256,
      "protocol": 3
    }
  ],
  "sync": {
    "enabled": true,
    "last_sync": "2025-01-15T10:30:00Z",
    "last_sync_error": null,
    "next_sync": "2025-01-15T11:00:00Z",
    "peer_count": 3
  }
}
```

**Response (DNSSEC disabled):** `200 OK`

```json
{
  "enabled": false
}
```

---

### GET /api/v1/dnssec/ds

Returns DS (Delegation Signer) records for all managed zones.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `zone` | string | Optional zone filter |

**Response (DNSSEC enabled):** `200 OK`

```json
{
  "enabled": true,
  "ds_records": [
    {
      "zone": "example.com.",
      "key_tag": 12345,
      "algorithm": 8,
      "digest_type": 2,
      "digest": "ABC123...",
      "ds_record": "example.com. 3600 IN DS 12345 8 2 ABC123...",
      "created_at": "2025-01-15T10:30:00Z"
    }
  ]
}
```

**Response (DNSSEC disabled):** `200 OK`

```json
{
  "enabled": false,
  "message": "DNSSEC is disabled"
}
```

---

### GET /api/v1/dnssec/keys

Returns DNSSEC key pairs for synchronization between Overwatches.

**ACL Protected:** Yes

**Response (DNSSEC enabled):** `200 OK`

```json
{
  "enabled": true,
  "keys": [
    {
      "zone": "example.com.",
      "key_tag": 12345,
      "algorithm": 8,
      "flags": 256,
      "protocol": 3,
      "public_key": "...",
      "created_at": "2025-01-15T10:30:00Z"
    }
  ]
}
```

**Response (DNSSEC disabled):** `200 OK`

```json
{
  "enabled": false,
  "keys": null
}
```

---

### POST /api/v1/dnssec/sync

Trigger an immediate DNSSEC key sync from all peers.

**ACL Protected:** Yes

**Request Body:** Empty

**Response:** `200 OK`

```json
{
  "status": "ok",
  "message": "key sync triggered"
}
```

**Error Response:** `400 Bad Request`

```json
{
  "error": "DNSSEC is disabled",
  "code": 400
}
```

---

## Overwatch API

Agent-based health monitoring with validation and overrides. These endpoints are available in Overwatch mode.

### GET /api/v1/overwatch/backends

List all backends with agent and validation health info.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service` | string | Filter by service name |
| `region` | string | Filter by region |
| `status` | string | Filter by status: `healthy`, `unhealthy`, `stale` |

**Response:** `200 OK`

```json
{
  "backends": [
    {
      "service": "api-service",
      "address": "10.0.1.10",
      "port": 8080,
      "weight": 100,
      "agent_id": "agent-123",
      "region": "us-east-1",
      "effective_status": "healthy",
      "agent_healthy": true,
      "agent_last_seen": "2025-01-15T10:30:00Z",
      "validation_healthy": true,
      "validation_last_check": "2025-01-15T10:29:00Z",
      "validation_error": null,
      "override_status": null,
      "override_reason": "",
      "override_by": "",
      "override_at": null
    }
  ],
  "generated_at": "2025-01-15T10:30:05Z"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `service` | string | Service name |
| `address` | string | Backend IP address |
| `port` | int | Backend port |
| `weight` | int | Load balancing weight |
| `agent_id` | string | Reporting agent ID |
| `region` | string | Region identifier |
| `effective_status` | string | Final computed status: `healthy`, `unhealthy`, `stale` |
| `agent_healthy` | bool | Health status reported by agent |
| `agent_last_seen` | string | Last heartbeat from agent |
| `validation_healthy` | bool | Validation check result (if enabled) |
| `validation_last_check` | string | Last validation timestamp |
| `validation_error` | string | Validation error message |
| `override_status` | bool | Manual override status (null if none) |
| `override_reason` | string | Reason for override |
| `override_by` | string | User/system that set override |
| `override_at` | string | When override was set |

---

### GET /api/v1/overwatch/stats

Get aggregated statistics across all backends.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "total_backends": 10,
  "healthy_backends": 8,
  "unhealthy_backends": 1,
  "stale_backends": 1,
  "active_overrides": 2,
  "validation_enabled": true,
  "validated_backends": 7,
  "disagreement_count": 0,
  "active_agents": 3,
  "unique_services": 2,
  "backends_by_service": {
    "api-service": 5,
    "web-service": 5
  },
  "backends_by_region": {
    "us-east-1": 6,
    "us-west-2": 4
  },
  "generated_at": "2025-01-15T10:30:05Z"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `total_backends` | int | Total registered backends |
| `healthy_backends` | int | Backends with healthy status |
| `unhealthy_backends` | int | Backends with unhealthy status |
| `stale_backends` | int | Backends with stale status (no recent heartbeat) |
| `active_overrides` | int | Number of active manual overrides |
| `validation_enabled` | bool | Whether validation is running |
| `validated_backends` | int | Backends with validation results |
| `disagreement_count` | int | Backends where agent and validation disagree |
| `active_agents` | int | Number of unique reporting agents |
| `unique_services` | int | Number of unique services |
| `backends_by_service` | object | Backend count per service |
| `backends_by_region` | object | Backend count per region |

---

### POST /api/v1/overwatch/backends/{service}/{address}/{port}/override

Set a manual override for a backend's health status.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service` | string | Service name |
| `address` | string | Backend IP address |
| `port` | int | Backend port |

**Request Body:**

```json
{
  "healthy": false,
  "reason": "Manual maintenance"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `healthy` | bool | No | Override health status |
| `reason` | string | No | Human-readable reason |

**Request Headers:**

| Header | Description |
|--------|-------------|
| `X-User` | Optional user identifier for audit trail |

**Response:** `200 OK`

```json
{
  "success": true,
  "message": "Override set: backend marked as unhealthy"
}
```

**Error Response:** `404 Not Found`

```json
{
  "error": "backend not found",
  "code": 404
}
```

---

### DELETE /api/v1/overwatch/backends/{service}/{address}/{port}/override

Clear a manual override for a backend.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service` | string | Service name |
| `address` | string | Backend IP address |
| `port` | int | Backend port |

**Response:** `200 OK`

```json
{
  "success": true,
  "message": "Override cleared"
}
```

**Error Response:** `404 Not Found`

```json
{
  "error": "backend not found",
  "code": 404
}
```

---

### POST /api/v1/overwatch/validate

Trigger immediate validation of all or specific backends.

**ACL Protected:** Yes

**Query Parameters (optional):**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service` | string | Service name (requires all three params) |
| `address` | string | Backend IP address |
| `port` | int | Backend port |

**Request Body:** Empty

**Response (all backends):** `200 OK`

```json
{
  "success": true,
  "message": "Validation triggered for all backends"
}
```

**Response (specific backend):** `200 OK`

```json
{
  "success": true,
  "message": "Validation triggered for specific backend"
}
```

**Error Response:** `503 Service Unavailable`

```json
{
  "error": "validator not available",
  "code": 503
}
```

---

### GET /api/v1/overwatch/agents

List all pinned agent certificates with expiration info.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "agents": [
    {
      "agent_id": "agent-123",
      "fingerprint": "SHA256:abc123...",
      "region": "us-east-1",
      "first_seen": "2025-01-01T10:00:00Z",
      "last_seen": "2025-01-15T10:30:00Z",
      "not_after": "2026-01-01T10:00:00Z",
      "revoked": false,
      "expires_in_hours": 8760
    }
  ],
  "total": 3,
  "revoked": 0,
  "expiring": 1,
  "generated_at": "2025-01-15T10:30:05Z"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `agents` | array | List of agent certificates |
| `total` | int | Total agent count |
| `revoked` | int | Number of revoked certificates |
| `expiring` | int | Certificates expiring within 30 days |

---

### GET /api/v1/overwatch/agents/{agent_id}

Get a specific agent's certificate details.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `agent_id` | string | Agent identifier |

**Response:** `200 OK`

```json
{
  "agent_id": "agent-123",
  "fingerprint": "SHA256:abc123...",
  "region": "us-east-1",
  "first_seen": "2025-01-01T10:00:00Z",
  "last_seen": "2025-01-15T10:30:00Z",
  "not_after": "2026-01-01T10:00:00Z",
  "revoked": false,
  "expires_in_hours": 8760
}
```

**Error Response:** `404 Not Found`

```json
{
  "error": "agent not found",
  "code": 404
}
```

---

### DELETE /api/v1/overwatch/agents/{agent_id}

Delete (unpin) an agent's certificate.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `agent_id` | string | Agent identifier |

**Response:** `200 OK`

```json
{
  "success": true,
  "message": "Agent certificate deleted"
}
```

---

### POST /api/v1/overwatch/agents/{agent_id}/revoke

Revoke an agent's certificate.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `agent_id` | string | Agent identifier |

**Request Body:**

```json
{
  "reason": "Security incident"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `reason` | string | No | Reason for revocation (default: "revoked via API") |

**Response:** `200 OK`

```json
{
  "success": true,
  "message": "Agent certificate revoked"
}
```

**Error Response:** `404 Not Found`

```json
{
  "error": "agent not found",
  "code": 404
}
```

---

### GET /api/v1/overwatch/agents/expiring

List agent certificates expiring within a threshold.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `threshold_days` | int | 30 | Days until expiration threshold |

**Response:** `200 OK`

```json
{
  "expiring": [
    {
      "agent_id": "agent-456",
      "fingerprint": "SHA256:def456...",
      "region": "us-west-2",
      "first_seen": "2025-01-01T10:00:00Z",
      "last_seen": "2025-01-15T10:30:00Z",
      "not_after": "2025-02-10T10:00:00Z",
      "revoked": false,
      "expires_in_hours": 480
    }
  ],
  "count": 1,
  "threshold_days": 30,
  "generated_at": "2025-01-15T10:30:05Z"
}
```

---

## Error Responses

All endpoints return errors in a consistent format:

```json
{
  "error": "error message",
  "code": 405
}
```

### HTTP Status Codes

| Status Code | Description |
|-------------|-------------|
| `200` | Success |
| `204` | Success (no content) |
| `400` | Bad Request - invalid parameters or malformed JSON |
| `403` | Forbidden - IP not in `allowed_networks` |
| `404` | Not Found - resource doesn't exist |
| `405` | Method Not Allowed - wrong HTTP method |
| `500` | Internal Server Error |
| `503` | Service Unavailable - feature disabled or dependency unavailable |

---

## Usage Examples

### curl

```bash
# Check server health
curl http://localhost:8080/api/v1/health/servers

# Check regional health
curl http://localhost:8080/api/v1/health/regions

# Check readiness
curl -w "%{http_code}" http://localhost:8080/api/v1/ready

# Pretty print with jq
curl -s http://localhost:8080/api/v1/health/servers | jq .

# Set a health override
curl -X PUT http://localhost:8080/api/v1/overrides/api-service/10.0.1.10 \
  -H "Content-Type: application/json" \
  -d '{"healthy": false, "reason": "maintenance", "source": "admin"}'

# List all overrides
curl http://localhost:8080/api/v1/overrides

# Clear an override
curl -X DELETE http://localhost:8080/api/v1/overrides/api-service/10.0.1.10

# Get DNSSEC status
curl http://localhost:8080/api/v1/dnssec/status

# Get DS records for a specific zone
curl "http://localhost:8080/api/v1/dnssec/ds?zone=example.com"

# Trigger DNSSEC key sync
curl -X POST http://localhost:8080/api/v1/dnssec/sync

# List Overwatch backends
curl http://localhost:8080/api/v1/overwatch/backends

# Filter backends by service
curl "http://localhost:8080/api/v1/overwatch/backends?service=api-service"

# Get Overwatch stats
curl http://localhost:8080/api/v1/overwatch/stats

# Set backend override
curl -X POST http://localhost:8080/api/v1/overwatch/backends/api-service/10.0.1.10/8080/override \
  -H "Content-Type: application/json" \
  -H "X-User: admin" \
  -d '{"healthy": false, "reason": "deploying new version"}'

# Trigger validation
curl -X POST http://localhost:8080/api/v1/overwatch/validate

# List agent certificates
curl http://localhost:8080/api/v1/overwatch/agents

# Get expiring certificates (custom threshold)
curl "http://localhost:8080/api/v1/overwatch/agents/expiring?threshold_days=60"

# Revoke an agent certificate
curl -X POST http://localhost:8080/api/v1/overwatch/agents/agent-123/revoke \
  -H "Content-Type: application/json" \
  -d '{"reason": "compromised credentials"}'
```

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

### Monitoring Integration

**Prometheus (via blackbox_exporter or custom scraper):**

The API complements the `/metrics` endpoint. Use `/api/v1/ready` as a health check target.

**Alertmanager webhook:**

Poll `/api/v1/health/servers` and alert on `consecutive_failures > 0`.

**Overwatch monitoring:**

Poll `/api/v1/overwatch/stats` and alert on:
- `stale_backends > 0` - agents not reporting
- `disagreement_count > 0` - agent/validation mismatch
- `unhealthy_backends / total_backends > 0.5` - high failure rate
