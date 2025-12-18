# OpenGSLB API Reference

OpenGSLB provides a comprehensive REST API for health status monitoring, external health overrides, DNSSEC key management, Overwatch agent management, domain/server/region configuration, metrics, audit logging, and routing.

**Base URL:** `http://localhost:8080/api/v1`

## Table of Contents

- [Security & Authentication](#security--authentication)
- [Configuration](#configuration)
- [Simple Health Check](#simple-health-check)
  - [GET /api/health](#get-apihealth)
- [Core Health Endpoints](#core-health-endpoints)
  - [GET /api/v1/health/servers](#get-apiv1healthservers)
  - [GET /api/v1/health/regions](#get-apiv1healthregions)
- [Liveness & Readiness Probes](#liveness--readiness-probes)
  - [GET /api/v1/ready](#get-apiv1ready)
  - [GET /api/v1/live](#get-apiv1live)
- [Version Endpoint](#version-endpoint)
  - [GET /api/v1/version](#get-apiv1version)
- [Domain API](#domain-api)
  - [GET /api/v1/domains](#get-apiv1domains)
  - [GET /api/v1/domains/{name}](#get-apiv1domainsname)
  - [POST /api/v1/domains](#post-apiv1domains)
  - [PUT /api/v1/domains/{name}](#put-apiv1domainsname)
  - [DELETE /api/v1/domains/{name}](#delete-apiv1domainsname)
  - [GET /api/v1/domains/{name}/backends](#get-apiv1domainsnamebackends)
  - [POST /api/v1/domains/{name}/backends](#post-apiv1domainsnamebackends)
  - [DELETE /api/v1/domains/{name}/backends/{id}](#delete-apiv1domainsnamebackendsid)
- [Server API](#server-api)
  - [GET /api/v1/servers](#get-apiv1servers)
  - [GET /api/v1/servers/{id}](#get-apiv1serversid)
  - [POST /api/v1/servers](#post-apiv1servers)
  - [PUT /api/v1/servers/{id}](#put-apiv1serversid)
  - [DELETE /api/v1/servers/{id}](#delete-apiv1serversid)
  - [GET /api/v1/servers/{id}/health-check](#get-apiv1serversidhealth-check)
  - [PUT /api/v1/servers/{id}/health-check](#put-apiv1serversidhealth-check)
- [Region API](#region-api)
  - [GET /api/v1/regions](#get-apiv1regions)
  - [GET /api/v1/regions/{id}](#get-apiv1regionsid)
  - [POST /api/v1/regions](#post-apiv1regions)
  - [PUT /api/v1/regions/{id}](#put-apiv1regionsid)
  - [DELETE /api/v1/regions/{id}](#delete-apiv1regionsid)
- [Node API](#node-api)
  - [GET /api/v1/nodes/overwatch](#get-apiv1nodesoverwatch)
  - [GET /api/v1/nodes/overwatch/{id}](#get-apiv1nodesoverwatchid)
  - [GET /api/v1/nodes/agent](#get-apiv1nodesagent)
  - [POST /api/v1/nodes/agent](#post-apiv1nodesagent)
  - [GET /api/v1/nodes/agent/{id}](#get-apiv1nodesagentid)
  - [DELETE /api/v1/nodes/agent/{id}](#delete-apiv1nodesagentid)
  - [GET /api/v1/nodes/agent/{id}/certificate](#get-apiv1nodesagentidcertificate)
  - [POST /api/v1/nodes/agent/{id}/certificate](#post-apiv1nodesagentidcertificate)
  - [DELETE /api/v1/nodes/agent/{id}/certificate](#delete-apiv1nodesagentidcertificate)
- [Gossip API](#gossip-api)
  - [GET /api/v1/gossip/nodes](#get-apiv1gossipnodes)
  - [GET /api/v1/gossip/nodes/{id}](#get-apiv1gossipnodesid)
  - [GET /api/v1/gossip/config](#get-apiv1gossipconfig)
  - [PUT /api/v1/gossip/config](#put-apiv1gossipconfig)
  - [POST /api/v1/gossip/generate-key](#post-apiv1gossipgenerate-key)
- [Audit Log API](#audit-log-api)
  - [GET /api/v1/audit-logs](#get-apiv1audit-logs)
  - [GET /api/v1/audit-logs/{id}](#get-apiv1audit-logsid)
  - [GET /api/v1/audit-logs/stats](#get-apiv1audit-logsstats)
  - [GET /api/v1/audit-logs/export](#get-apiv1audit-logsexport)
- [Metrics API](#metrics-api)
  - [GET /api/v1/metrics/overview](#get-apiv1metricsoverview)
  - [GET /api/v1/metrics/history](#get-apiv1metricshistory)
  - [GET /api/v1/metrics/nodes/{id}](#get-apiv1metricsnodesid)
  - [GET /api/v1/metrics/regions/{id}](#get-apiv1metricsregionsid)
  - [GET /api/v1/metrics/routing](#get-apiv1metricsrouting)
- [Config API](#config-api)
  - [GET /api/v1/preferences](#get-apiv1preferences)
  - [PUT /api/v1/preferences](#put-apiv1preferences)
  - [GET /api/v1/config/system](#get-apiv1configsystem)
  - [GET /api/v1/config/dns](#get-apiv1configdns)
  - [PUT /api/v1/config/dns](#put-apiv1configdns)
  - [GET /api/v1/config/health-check](#get-apiv1confighealth-check)
  - [PUT /api/v1/config/health-check](#put-apiv1confighealth-check)
  - [GET /api/v1/config/logging](#get-apiv1configlogging)
  - [PUT /api/v1/config/logging](#put-apiv1configlogging)
- [Routing API](#routing-api)
  - [GET /api/v1/routing/algorithms](#get-apiv1routingalgorithms)
  - [GET /api/v1/routing/algorithms/{id}](#get-apiv1routingalgorithmsid)
  - [POST /api/v1/routing/test](#post-apiv1routingtest)
  - [GET /api/v1/routing/decisions](#get-apiv1routingdecisions)
  - [GET /api/v1/routing/flows](#get-apiv1routingflows)
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
- [Geolocation API](#geolocation-api)
  - [GET /api/v1/geo/mappings](#get-apiv1geomappings)
  - [PUT /api/v1/geo/mappings](#put-apiv1geomappings)
  - [DELETE /api/v1/geo/mappings/{cidr}](#delete-apiv1geomappingscidr)
  - [GET /api/v1/geo/test](#get-apiv1geotest)
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
| `/api/health` | **No** |
| `/api/v1/health/*` | Yes |
| `/api/v1/domains/*` | Yes |
| `/api/v1/servers/*` | Yes |
| `/api/v1/regions/*` | Yes |
| `/api/v1/nodes/*` | Yes |
| `/api/v1/gossip/*` | Yes |
| `/api/v1/audit-logs/*` | Yes |
| `/api/v1/metrics/*` | Yes |
| `/api/v1/preferences` | Yes |
| `/api/v1/config/*` | Yes |
| `/api/v1/routing/*` | Yes |
| `/api/v1/overrides/*` | Yes |
| `/api/v1/dnssec/*` | Yes |
| `/api/v1/geo/*` | Yes |
| `/api/v1/overwatch/*` | Yes |
| `/api/v1/ready` | **No** |
| `/api/v1/live` | **No** |
| `/api/v1/version` | **No** |

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
  enable_cors: false
  cors:
    allowed_origins:
      - "*"
    allowed_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "PATCH"
      - "DELETE"
      - "OPTIONS"
    allowed_headers:
      - "Accept"
      - "Authorization"
      - "Content-Type"
      - "X-Requested-With"
    allow_credentials: false
    max_age: 86400
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the API server |
| `address` | string | `127.0.0.1:8080` | Listen address (IP:port) |
| `allowed_networks` | []string | `["127.0.0.1/32", "::1/128"]` | CIDR networks allowed to connect |
| `trust_proxy_headers` | bool | `false` | Trust X-Forwarded-For headers |
| `enable_cors` | bool | `false` | Enable CORS middleware for cross-origin requests |
| `cors.allowed_origins` | []string | `["*"]` | Origins allowed to access the API |
| `cors.allowed_methods` | []string | See example | HTTP methods allowed |
| `cors.allowed_headers` | []string | See example | Request headers allowed |
| `cors.allow_credentials` | bool | `false` | Allow credentials in requests |
| `cors.max_age` | int | `86400` | Preflight cache duration in seconds |

> **Note:** The `allowed_networks` list is matched exactly by IP family. If you want to allow all IPs,
> you must include both `0.0.0.0/0` (IPv4) and `::/0` (IPv6). Including only `0.0.0.0/0` will block
> IPv6 clients, including localhost connections via `::1`.

---

## Simple Health Check

### GET /api/health

Simple health endpoint with version and uptime information. This endpoint is **not** ACL protected.

**ACL Protected:** No

**Response:** `200 OK`

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 86400,
  "timestamp": "2025-01-15T10:30:00Z"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Health status: `healthy`, `degraded`, `unhealthy` |
| `version` | string | Application version |
| `uptime_seconds` | int | Seconds since application start |
| `timestamp` | string | ISO 8601 timestamp |

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

## Version Endpoint

### GET /api/v1/version

Returns build version and information. This endpoint is **not** ACL protected.

**ACL Protected:** No

**Response:** `200 OK`

```json
{
  "version": "1.1.2",
  "go_version": "go1.21.0",
  "git_commit": "abc123def",
  "build_date": "2025-12-18T10:30:00Z"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | OpenGSLB version |
| `go_version` | string | Go compiler version used to build |
| `git_commit` | string | Git commit hash of the build |
| `build_date` | string | ISO 8601 timestamp of when the binary was built |

---

## Domain API

Manage DNS domain configurations.

**NEW in v1.1.1:** Full CRUD support for domains and backends. API-created domains are automatically registered with the DNS server for immediate DNS resolution (v1.1.2).

### GET /api/v1/domains

List all configured domains.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "domains": [
    {
      "id": "d1",
      "name": "api.example.com",
      "ttl": 300,
      "routing_policy": "weighted",
      "health_check_id": "hc1",
      "dnssec_enabled": true,
      "enabled": true,
      "description": "Main API endpoint",
      "tags": ["production"],
      "backend_count": 3,
      "healthy_backends": 2,
      "created_at": "2025-01-01T10:00:00Z",
      "updated_at": "2025-01-15T10:00:00Z"
    }
  ],
  "total": 1,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/domains/{name}

Get a specific domain by name.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "domain": {
    "id": "d1",
    "name": "api.example.com",
    "ttl": 300,
    "routing_policy": "weighted",
    "dnssec_enabled": true,
    "enabled": true,
    "settings": {
      "geo_routing_enabled": true,
      "failover_enabled": true,
      "failover_threshold": 2,
      "load_balancing_method": "round-robin"
    }
  }
}
```

---

### POST /api/v1/domains

Create a new domain.

**ACL Protected:** Yes

**Request Body:**

```json
{
  "name": "api.example.com",
  "ttl": 300,
  "routing_policy": "weighted",
  "dnssec_enabled": true,
  "enabled": true,
  "description": "Main API endpoint"
}
```

**Response:** `201 Created`

---

### PUT /api/v1/domains/{name}

Update an existing domain.

**ACL Protected:** Yes

**Request Body:**

```json
{
  "ttl": 600,
  "enabled": false
}
```

**Response:** `200 OK`

---

### DELETE /api/v1/domains/{name}

Delete a domain.

**ACL Protected:** Yes

**Response:** `204 No Content`

---

### GET /api/v1/domains/{name}/backends

Get backends for a domain.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "backends": [
    {
      "id": "b1",
      "address": "10.0.1.10",
      "port": 80,
      "weight": 100,
      "priority": 1,
      "region": "us-east-1",
      "healthy": true,
      "enabled": true,
      "last_check": "2025-01-15T10:30:00Z"
    }
  ],
  "total": 1,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### POST /api/v1/domains/{name}/backends

Add a backend server to a domain (**v1.1.1**). The backend is automatically registered with the DNS server for immediate DNS resolution (**v1.1.2**).

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Domain name |

**Request Body:**

```json
{
  "address": "10.0.2.10",
  "port": 443,
  "weight": 100,
  "region": "us-west-2"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | **Yes** | Server IP address |
| `port` | int | **Yes** | Server port |
| `weight` | int | No | Load balancing weight (default: 1) |
| `region` | string | No | Geographic region |

**Response:** `201 Created`

```json
{
  "message": "backend added",
  "id": "api.example.com:10.0.2.10:443"
}
```

**Error Responses:**

- `400 Bad Request` - Domain not found or backend already exists
- `501 Not Implemented` - Store not configured

---

### DELETE /api/v1/domains/{name}/backends/{id}

Remove a backend from a domain (**v1.1.1**). The backend is automatically deregistered from the DNS server (**v1.1.2**).

**Note:** Only API-created backends can be deleted. Config-based backends cannot be deleted via API.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Domain name |
| `id` | string | Backend ID format: `{domain}:{address}:{port}` |

**Example:** `DELETE /api/v1/domains/api.example.com/backends/api.example.com:10.0.2.10:443`

**Response:** `204 No Content`

**Error Responses:**

- `400 Bad Request` - Cannot delete config-based backend
- `404 Not Found` - Backend not found
- `501 Not Implemented` - Store not configured

---

## Server API

**NEW in v1.1.0:** Manage backend server configurations dynamically. The unified server architecture allows creating, updating, and deleting servers via API without configuration file changes.

### GET /api/v1/servers

List all configured servers (static, agent-registered, and API-created).

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "servers": [
    {
      "id": "app.example.com:10.0.1.10:80",
      "service": "app.example.com",
      "address": "10.0.1.10",
      "port": 80,
      "weight": 100,
      "region": "us-east-1",
      "source": "static",
      "effective_status": "healthy",
      "agent_healthy": null,
      "draining": false,
      "created_at": "2025-01-15T10:00:00Z",
      "updated_at": "2025-01-15T10:00:00Z"
    },
    {
      "id": "app.example.com:10.0.2.10:80",
      "service": "app.example.com",
      "address": "10.0.2.10",
      "port": 80,
      "weight": 150,
      "region": "us-west-2",
      "source": "api",
      "effective_status": "healthy",
      "agent_healthy": null,
      "draining": false,
      "created_at": "2025-01-15T11:00:00Z",
      "updated_at": "2025-01-15T11:00:00Z"
    }
  ]
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Server ID format: `{service}:{address}:{port}` |
| `service` | string | Domain/service this server belongs to |
| `address` | string | Server IP address |
| `port` | int | Server port |
| `weight` | int | Load balancing weight (1-1000) |
| `region` | string | Geographic region |
| `source` | string | Registration source: `static`, `agent`, or `api` |
| `effective_status` | string | Current health status: `healthy`, `unhealthy`, `stale` |
| `agent_healthy` | bool | Agent-reported health (null for static/API servers) |
| `draining` | bool | Whether server is being drained |

---

### POST /api/v1/servers

Create a new server dynamically (**v1.1.0**).

**ACL Protected:** Yes

**Request Body:**

```json
{
  "service": "app.example.com",
  "address": "10.0.3.10",
  "port": 80,
  "weight": 100,
  "region": "eu-west-1"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `service` | string | **Yes** | Domain/service name (must match configured domain) |
| `address` | string | **Yes** | Server IP address |
| `port` | int | **Yes** | Server port |
| `weight` | int | No | Load balancing weight (default: 100) |
| `region` | string | **Yes** | Geographic region |

**Response:** `201 Created`

```json
{
  "message": "server created",
  "id": "app.example.com:10.0.3.10:80"
}
```

**Error Response:** `400 Bad Request`

```json
{
  "error": "server already exists: app.example.com:10.0.3.10:80"
}
```

---

### GET /api/v1/servers/{id}

Get a specific server by ID.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Server ID format: `{service}:{address}:{port}` |

**Example:** `GET /api/v1/servers/app.example.com:10.0.1.10:80`

**Response:** `200 OK`

```json
{
  "id": "app.example.com:10.0.1.10:80",
  "service": "app.example.com",
  "address": "10.0.1.10",
  "port": 80,
  "weight": 100,
  "region": "us-east-1",
  "source": "static",
  "effective_status": "healthy"
}
```

---

### PATCH /api/v1/servers/{id}

Update a server's weight and region (**v1.1.0**).

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Server ID format: `{service}:{address}:{port}` |

**Request Body:**

```json
{
  "weight": 150,
  "region": "us-east-1"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `weight` | int | No | New load balancing weight |
| `region` | string | No | New geographic region |

**Response:** `200 OK`

```json
{
  "message": "server updated",
  "id": "app.example.com:10.0.1.10:80"
}
```

---

### DELETE /api/v1/servers/{id}

Delete a dynamically-created server (**v1.1.0**).

**Note:** Only servers created via API (`source: "api"`) can be deleted. Static servers from configuration cannot be deleted via API.

**ACL Protected:** Yes

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Server ID format: `{service}:{address}:{port}` |

**Response:** `204 No Content`

**Error Response:** `400 Bad Request`

```json
{
  "error": "cannot delete static server"
}
```

---

### GET /api/v1/servers/{id}/health-check

Get health check configuration for a server.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "server_id": "s1",
  "health_check": {
    "enabled": true,
    "type": "http",
    "path": "/health",
    "interval": "30s",
    "timeout": "5s",
    "healthy_threshold": 2,
    "unhealthy_threshold": 3,
    "expected_status": 200
  }
}
```

---

### PUT /api/v1/servers/{id}/health-check

Update health check configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

---

## Region API

Manage geographic region configurations.

### GET /api/v1/regions

List all configured regions.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `enabled` | bool | Filter by enabled status |
| `continent` | string | Filter by continent |

**Response:** `200 OK`

```json
{
  "regions": [
    {
      "id": "r1",
      "name": "US East",
      "code": "us-east-1",
      "description": "Virginia region",
      "latitude": 37.7749,
      "longitude": -122.4194,
      "continent": "NA",
      "countries": ["US"],
      "enabled": true,
      "priority": 1,
      "server_count": 5,
      "healthy_servers": 4
    }
  ],
  "total": 1,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### POST /api/v1/regions

Create a new region.

**ACL Protected:** Yes

**Request Body:**

```json
{
  "name": "US East",
  "code": "us-east-1",
  "description": "Virginia region",
  "latitude": 37.7749,
  "longitude": -122.4194,
  "continent": "NA",
  "enabled": true,
  "priority": 1
}
```

**Response:** `201 Created`

---

### GET /api/v1/regions/{id}

Get a specific region.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### PUT /api/v1/regions/{id}

Update a region.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### DELETE /api/v1/regions/{id}

Delete a region.

**ACL Protected:** Yes

**Response:** `204 No Content`

---

## Node API

Manage Overwatch and Agent nodes.

### GET /api/v1/nodes/overwatch

List all Overwatch nodes.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status: `online`, `offline`, `degraded` |
| `region` | string | Filter by region |

**Response:** `200 OK`

```json
{
  "nodes": [
    {
      "id": "ow1",
      "name": "overwatch-east-1",
      "address": "10.0.1.100",
      "port": 53,
      "api_port": 8080,
      "region": "us-east-1",
      "status": "online",
      "version": "1.0.0",
      "uptime_seconds": 86400,
      "last_seen": "2025-01-15T10:30:00Z",
      "queries_total": 1000000,
      "queries_per_sec": 500.5,
      "dnssec_enabled": true
    }
  ],
  "total": 1,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/nodes/overwatch/{id}

Get a specific Overwatch node.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### GET /api/v1/nodes/agent

List all Agent nodes.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status: `online`, `offline`, `pending` |
| `region` | string | Filter by region |

**Response:** `200 OK`

```json
{
  "nodes": [
    {
      "id": "ag1",
      "name": "agent-east-1",
      "address": "10.0.2.100",
      "port": 8443,
      "region": "us-east-1",
      "status": "online",
      "version": "1.0.0",
      "uptime_seconds": 86400,
      "last_seen": "2025-01-15T10:30:00Z",
      "checks_total": 500000,
      "checks_per_sec": 100.5,
      "active_checks": 50,
      "target_count": 25,
      "certificate_expiry": "2026-01-15T10:30:00Z",
      "registered_at": "2025-01-01T10:00:00Z"
    }
  ],
  "total": 1,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### POST /api/v1/nodes/agent

Register a new Agent node.

**ACL Protected:** Yes

**Request Body:**

```json
{
  "name": "agent-east-1",
  "address": "10.0.2.100",
  "port": 8443,
  "region": "us-east-1",
  "version": "1.0.0"
}
```

**Response:** `201 Created`

---

### GET /api/v1/nodes/agent/{id}

Get a specific Agent node.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### DELETE /api/v1/nodes/agent/{id}

Deregister an Agent node.

**ACL Protected:** Yes

**Response:** `204 No Content`

---

### GET /api/v1/nodes/agent/{id}/certificate

Get certificate information for an Agent.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "certificate": {
    "agent_id": "ag1",
    "serial": "ABC123",
    "not_before": "2025-01-01T10:00:00Z",
    "not_after": "2026-01-01T10:00:00Z",
    "fingerprint": "SHA256:...",
    "status": "valid",
    "issued_at": "2025-01-01T10:00:00Z"
  }
}
```

---

### POST /api/v1/nodes/agent/{id}/certificate

Reissue certificate for an Agent.

**ACL Protected:** Yes

**Response:** `201 Created`

---

### DELETE /api/v1/nodes/agent/{id}/certificate

Revoke certificate for an Agent.

**ACL Protected:** Yes

**Response:** `204 No Content`

---

## Gossip API

Manage gossip protocol cluster communication.

### GET /api/v1/gossip/nodes

List all nodes in the gossip cluster.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status: `alive`, `suspect`, `dead` |
| `region` | string | Filter by region |

**Response:** `200 OK`

```json
{
  "nodes": [
    {
      "id": "node1",
      "name": "overwatch-1",
      "address": "10.0.1.100",
      "port": 7946,
      "status": "alive",
      "state": "leader",
      "region": "us-east-1",
      "version": "1.0.0",
      "rtt_ms": 5,
      "last_seen": "2025-01-15T10:30:00Z",
      "joined_at": "2025-01-01T10:00:00Z"
    }
  ],
  "total": 3,
  "healthy": 3,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/gossip/nodes/{id}

Get a specific gossip node.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### GET /api/v1/gossip/config

Get gossip protocol configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "config": {
    "enabled": true,
    "bind_address": "0.0.0.0",
    "bind_port": 7946,
    "cluster_name": "opengslb",
    "encryption_enabled": true,
    "gossip_interval_ms": 200,
    "probe_interval_ms": 1000,
    "probe_timeout_ms": 500,
    "seeds": ["10.0.1.101:7946", "10.0.1.102:7946"]
  }
}
```

---

### PUT /api/v1/gossip/config

Update gossip protocol configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### POST /api/v1/gossip/generate-key

Generate a new encryption key for gossip protocol.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "key": "base64-encoded-key",
  "algorithm": "AES-256-GCM",
  "bits": 256
}
```

---

## Audit Log API

Access audit logs for compliance and troubleshooting.

### GET /api/v1/audit-logs

List audit log entries with pagination and filtering.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `start_time` | string | ISO 8601 start time |
| `end_time` | string | ISO 8601 end time |
| `actions` | string | Comma-separated list of actions |
| `resources` | string | Comma-separated list of resources |
| `actors` | string | Comma-separated list of actors |
| `status` | string | Filter by status: `success`, `failure` |
| `limit` | int | Results per page (default: 100) |
| `offset` | int | Pagination offset |

**Response:** `200 OK`

```json
{
  "entries": [
    {
      "id": "e1",
      "timestamp": "2025-01-15T10:30:00Z",
      "action": "create",
      "resource": "domain",
      "resource_id": "d1",
      "actor": "admin",
      "actor_type": "user",
      "actor_ip": "192.168.1.50",
      "status": "success",
      "duration_ms": 50
    }
  ],
  "total": 100,
  "limit": 100,
  "offset": 0,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/audit-logs/{id}

Get a specific audit log entry.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### GET /api/v1/audit-logs/stats

Get audit log statistics.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "stats": {
    "total_entries": 10000,
    "entries_last_24h": 500,
    "entries_last_7d": 3000,
    "by_action": {
      "create": 1000,
      "update": 5000,
      "delete": 500
    },
    "by_resource": {
      "domain": 2000,
      "server": 3000
    },
    "by_status": {
      "success": 9500,
      "failure": 500
    },
    "retention_days": 90
  },
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/audit-logs/export

Export audit logs in CSV or JSON format.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `format` | string | Export format: `json`, `csv` (default: json) |
| (plus all filter params from GET /api/v1/audit-logs) |

**Response:** `200 OK` with `Content-Disposition: attachment`

---

## Metrics API

System and performance metrics.

### GET /api/v1/metrics/overview

Get system metrics overview.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "overview": {
    "timestamp": "2025-01-15T10:30:00Z",
    "uptime_seconds": 86400,
    "queries_total": 1000000,
    "queries_per_sec": 500.5,
    "health_checks_total": 500000,
    "health_checks_per_sec": 100.5,
    "active_domains": 50,
    "active_servers": 200,
    "healthy_servers": 180,
    "unhealthy_servers": 20,
    "active_regions": 5,
    "overwatch_nodes": 3,
    "agent_nodes": 10,
    "dnssec_enabled": true,
    "gossip_enabled": true,
    "response_times": {
      "avg_ms": 5.5,
      "p50_ms": 3.0,
      "p95_ms": 10.0,
      "p99_ms": 25.0,
      "max_ms": 100.0
    },
    "error_rate": 0.01,
    "cache_hit_rate": 0.85,
    "memory": {
      "used_bytes": 500000000,
      "available_bytes": 1500000000,
      "total_bytes": 2000000000,
      "percent": 25.0
    },
    "cpu": {
      "used_percent": 15.5,
      "cores": 8
    }
  }
}
```

---

### GET /api/v1/metrics/history

Get historical metrics data.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `start_time` | string | ISO 8601 start time |
| `end_time` | string | ISO 8601 end time |
| `duration` | string | Duration shorthand: `1h`, `24h`, `7d` |
| `metrics` | string | Comma-separated list of metrics |
| `resolution` | string | Data resolution: `1m`, `5m`, `1h`, `1d` |
| `node_id` | string | Filter by node |
| `region_id` | string | Filter by region |

**Response:** `200 OK`

```json
{
  "data_points": [
    {
      "timestamp": "2025-01-15T10:00:00Z",
      "values": {
        "queries_per_sec": 500.5,
        "error_rate": 0.01
      }
    }
  ],
  "total": 60,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/metrics/nodes/{id}

Get metrics for a specific node.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### GET /api/v1/metrics/regions/{id}

Get metrics for a specific region.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### GET /api/v1/metrics/routing

Get routing decision statistics.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "stats": {
    "timestamp": "2025-01-15T10:30:00Z",
    "total_decisions": 1000000,
    "by_algorithm": {
      "weighted": 500000,
      "geo": 300000,
      "failover": 200000
    },
    "by_region": {
      "us-east-1": 400000,
      "us-west-2": 300000,
      "eu-west-1": 300000
    },
    "geo_routing_hits": 300000,
    "failover_events": 500,
    "avg_decision_time_us": 50.5
  }
}
```

---

## Config API

System configuration and user preferences.

### GET /api/v1/preferences

Get user preferences for the dashboard.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "preferences": {
    "theme": "dark",
    "language": "en",
    "timezone": "UTC",
    "date_format": "YYYY-MM-DD",
    "time_format": "HH:mm:ss",
    "refresh_interval_seconds": 30,
    "notifications_enabled": true,
    "default_view": "overview",
    "updated_at": "2025-01-15T10:30:00Z"
  }
}
```

---

### PUT /api/v1/preferences

Update user preferences.

**ACL Protected:** Yes

**Request Body:**

```json
{
  "theme": "light",
  "refresh_interval_seconds": 60
}
```

**Response:** `200 OK`

---

### GET /api/v1/config/system

Get system configuration (read-only).

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "config": {
    "version": "1.0.0",
    "build_info": {
      "version": "1.0.0",
      "git_commit": "abc123",
      "build_date": "2025-01-01",
      "go_version": "1.21"
    },
    "mode": "overwatch",
    "node_id": "ow1",
    "node_name": "overwatch-1",
    "region": "us-east-1",
    "features": ["dnssec", "gossip", "geo"],
    "start_time": "2025-01-15T00:00:00Z"
  }
}
```

---

### GET /api/v1/config/dns

Get DNS configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "config": {
    "listen_address": "0.0.0.0",
    "listen_port": 53,
    "enable_tcp": true,
    "enable_udp": true,
    "enable_dot": false,
    "enable_doh": false,
    "max_udp_size": 4096,
    "default_ttl": 300,
    "cache_enabled": true,
    "cache_size": 10000,
    "rate_limit_enabled": true,
    "rate_limit_qps": 1000
  }
}
```

---

### PUT /api/v1/config/dns

Update DNS configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### GET /api/v1/config/health-check

Get health check configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "config": {
    "default_interval_seconds": 30,
    "default_timeout_seconds": 5,
    "healthy_threshold": 2,
    "unhealthy_threshold": 3,
    "max_concurrent_checks": 100,
    "default_protocol": "http"
  }
}
```

---

### PUT /api/v1/config/health-check

Update health check configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### GET /api/v1/config/logging

Get logging configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "config": {
    "level": "info",
    "format": "json",
    "output": "stdout",
    "enable_audit": true,
    "audit_retention_days": 90
  }
}
```

---

### PUT /api/v1/config/logging

Update logging configuration.

**ACL Protected:** Yes

**Response:** `200 OK`

---

## Routing API

Traffic routing algorithms and testing.

### GET /api/v1/routing/algorithms

List available routing algorithms.

**ACL Protected:** Yes

**Response:** `200 OK`

```json
{
  "algorithms": [
    {
      "id": "weighted",
      "name": "Weighted Round Robin",
      "description": "Distribute traffic based on server weights",
      "type": "weighted",
      "enabled": true,
      "default": true
    },
    {
      "id": "geo",
      "name": "Geographic Routing",
      "description": "Route to nearest region based on client location",
      "type": "geo",
      "enabled": true,
      "default": false
    },
    {
      "id": "failover",
      "name": "Active-Passive Failover",
      "description": "Route to primary, failover to secondary on failure",
      "type": "failover",
      "enabled": true,
      "default": false
    }
  ],
  "total": 3,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/routing/algorithms/{id}

Get a specific routing algorithm.

**ACL Protected:** Yes

**Response:** `200 OK`

---

### POST /api/v1/routing/test

Test routing for a given request.

**ACL Protected:** Yes

**Request Body:**

```json
{
  "domain": "api.example.com",
  "client_ip": "8.8.8.8",
  "query_type": "A"
}
```

**Response:** `200 OK`

```json
{
  "result": {
    "domain": "api.example.com",
    "client_ip": "8.8.8.8",
    "client_region": "us-east-1",
    "client_country": "US",
    "algorithm": "geo",
    "selected_backend": {
      "address": "10.0.1.10",
      "port": 80,
      "region": "us-east-1",
      "weight": 100,
      "priority": 1,
      "healthy": true
    },
    "alternatives": [],
    "factors": [
      {
        "name": "geo_distance",
        "type": "geo",
        "value": "500km",
        "weight": 1.0,
        "impact": "positive"
      }
    ],
    "decision": "success",
    "decision_time_us": 50,
    "ttl": 300,
    "timestamp": "2025-01-15T10:30:00Z"
  }
}
```

---

### GET /api/v1/routing/decisions

Get recent routing decisions.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `start_time` | string | ISO 8601 start time |
| `end_time` | string | ISO 8601 end time |
| `domain` | string | Filter by domain |
| `algorithm` | string | Filter by algorithm |
| `region` | string | Filter by selected region |
| `outcome` | string | Filter by outcome: `success`, `failover`, `fallback` |
| `limit` | int | Results per page |
| `offset` | int | Pagination offset |

**Response:** `200 OK`

```json
{
  "decisions": [
    {
      "id": "rd1",
      "timestamp": "2025-01-15T10:30:00Z",
      "domain": "api.example.com",
      "client_ip": "8.8.8.8",
      "client_region": "us-east-1",
      "algorithm": "geo",
      "selected_server": "10.0.1.10:80",
      "selected_region": "us-east-1",
      "decision_time_us": 50,
      "outcome": "success"
    }
  ],
  "total": 1000,
  "limit": 100,
  "offset": 0,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

---

### GET /api/v1/routing/flows

Get traffic flow information between regions.

**ACL Protected:** Yes

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `start_time` | string | ISO 8601 start time |
| `end_time` | string | ISO 8601 end time |
| `source_region` | string | Filter by source region |
| `dest_region` | string | Filter by destination region |

**Response:** `200 OK`

```json
{
  "flows": [
    {
      "source_region": "us-east-1",
      "destination_region": "us-west-2",
      "request_count": 100000,
      "bytes_transferred": 1000000000,
      "avg_latency_ms": 50.5,
      "error_rate": 0.01,
      "timestamp": "2025-01-15T10:30:00Z"
    }
  ],
  "total": 10,
  "generated_at": "2025-01-15T10:30:00Z"
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

# Get version information
curl http://localhost:8080/api/v1/version

# Create a new domain
curl -X POST http://localhost:8080/api/v1/domains \
  -H "Content-Type: application/json" \
  -d '{"name": "api.example.com", "ttl": 300, "routing_policy": "latency"}'

# Add a backend to a domain
curl -X POST http://localhost:8080/api/v1/domains/api.example.com/backends \
  -H "Content-Type: application/json" \
  -d '{"address": "10.0.2.10", "port": 443, "weight": 100, "region": "us-west-2"}'

# List backends for a domain
curl http://localhost:8080/api/v1/domains/api.example.com/backends

# Remove a backend from a domain
curl -X DELETE http://localhost:8080/api/v1/domains/api.example.com/backends/api.example.com:10.0.2.10:443

# Delete a domain
curl -X DELETE http://localhost:8080/api/v1/domains/api.example.com

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
