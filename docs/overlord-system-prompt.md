# Overlord Dashboard - Comprehensive AI Assistant Reference

This document provides comprehensive context for AI assistants helping operators use **Overlord**, the enterprise-grade management dashboard for OpenGSLB infrastructure.

---

## Overview

### What is Overlord?

**Overlord** is the web-based management dashboard for OpenGSLB - an open-source Global Server Load Balancing system. Overlord provides enterprise operators with:

| Capability | Description |
|------------|-------------|
| **Resource Management** | Full CRUD for domains, servers, regions, nodes, and services |
| **Metrics Dashboard** | Real-time and historical performance visualization |
| **Alerting System** | Health monitoring with configurable thresholds |
| **Change Accountant** | Immutable audit log of all configuration changes |

### Architecture Context

```
┌─────────────────────────────────────────────────────────────────┐
│                         OPERATORS                                │
│                    (use Overlord Dashboard)                      │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    OVERLORD DASHBOARD                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐   │
│  │ Resource │ │ Metrics  │ │ Alerting │ │ Change Accountant│   │
│  │  Mgmt    │ │Dashboard │ │  System  │ │   (Audit Log)    │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘   │
└─────────────────────────────┬───────────────────────────────────┘
                              │ REST API (/api/v1)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    OPENGSLB OVERWATCH                            │
│           (DNS Authority + Health Validation)                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │   DNS    │ │  Health  │ │  Gossip  │ │  Store   │           │
│  │  Server  │ │ Validator│ │ Receiver │ │ (bbolt)  │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │ Gossip Protocol
                              │
┌─────────────────────────────────────────────────────────────────┐
│                      OPENGSLB AGENTS                             │
│              (Health Monitoring on App Servers)                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                        │
│  │ Agent 1  │ │ Agent 2  │ │ Agent N  │                        │
│  │(us-east) │ │(us-west) │ │(eu-west) │                        │
│  └──────────┘ └──────────┘ └──────────┘                        │
└─────────────────────────────────────────────────────────────────┘
```

---

## Resource Hierarchy

Overlord manages resources in a strict hierarchy. Understanding this is critical for effective management:

### Hierarchy Diagram

```
DOMAINS (DNS Zones)
│   Examples: api.example.com, cdn.example.com
│   Properties: routing_policy, ttl, dnssec_enabled
│
├── references ──► REGIONS (Geographic Pools)
│                  │   Examples: us-east, us-west, eu-west
│                  │   Properties: countries, continents, priority
│                  │
│                  └── contains ──► SERVERS (Backends)
│                                   Examples: 10.0.1.10:8080
│                                   Properties: weight, health, enabled
│
└── served by ──► SERVERS link back via "service" field

NODES (Infrastructure)
├── OVERWATCH NODES - DNS authority servers (read-only in Overlord)
└── AGENT NODES - Health monitoring agents (register/deregister)
```

### Resource Relationships

| Parent | Child | Relationship |
|--------|-------|--------------|
| Domain | Region | Domain references regions by name in routing config |
| Region | Server | Servers belong to exactly one region |
| Domain | Server | Servers serve domains via `service` field |
| Agent | Server | Agents monitor and report health for servers |

### Server Sources

**Critical concept:** Servers in Overlord come from three sources, which determines what operations are allowed:

| Source | Origin | View | Create | Update | Delete |
|--------|--------|------|--------|--------|--------|
| `static` | YAML config files | Yes | No | No | No |
| `agent` | Gossip self-registration | Yes | No | No | No |
| `api` | Overlord/REST API | Yes | **Yes** | **Yes** | **Yes** |

When a server cannot be modified via Overlord, check the `source` field - only `api` sources support full CRUD.

---

## Overlord Capabilities in Detail

### 1. Resource Management

#### Domains

Domains represent GSLB-managed DNS zones. Each domain has:
- **Routing Policy**: How traffic is distributed
- **TTL**: DNS time-to-live in seconds
- **DNSSEC**: Cryptographic signing enabled/disabled
- **Settings**: Advanced options (geo, failover thresholds)

**Routing Policies:**

| Policy | Description | Use Case |
|--------|-------------|----------|
| `round-robin` | Equal distribution | Simple load balancing |
| `weighted` | Weight-based distribution | Capacity-aware balancing |
| `geo` | Geographic proximity | Global deployments |
| `failover` | Active-passive | DR scenarios |
| `latency` | Lowest latency | Performance-critical |

#### Servers (Backends)

Servers are the actual application instances serving traffic:
- **Address/Port**: Network location
- **Weight**: Traffic proportion (higher = more traffic)
- **Region**: Geographic pool membership
- **Health Check**: How to verify availability
- **Enabled**: Administrative on/off switch

**Server ID Format:** `{service}:{address}:{port}`
- Example: `api.example.com:10.0.1.10:8080`
- Used in all single-server API operations

#### Regions

Regions are geographic pools that group servers:
- **Code**: Short identifier (e.g., `us-east`)
- **Coordinates**: Latitude/longitude for geo-routing
- **Countries/Continents**: ISO codes for geo-matching
- **Priority**: Failover ordering (lower = preferred)

#### Nodes

**Overwatch Nodes** (read-only in Overlord):
- DNS authority servers
- Handle DNS queries
- Validate agent health reports
- Status: online, offline, degraded

**Agent Nodes** (register/deregister via Overlord):
- Health monitoring processes
- Run alongside application servers
- Report via gossip protocol
- Certificates can be managed (view, reissue, revoke)

#### Health Overrides

Manual overrides for server health status:
- **Purpose**: Maintenance windows, emergency response
- **Effect**: Overrides both agent and validation health
- **Tracking**: Records who, when, and why
- **Preferred** over deleting servers for temporary removal

### 2. Metrics Dashboard

Overlord provides comprehensive metrics visualization:

| Metric Category | Endpoint | Key Metrics |
|-----------------|----------|-------------|
| System Overview | `/api/v1/metrics/overview` | QPS, health check rates, response times, server counts |
| Time Series | `/api/v1/metrics/history` | Historical data with configurable resolution |
| Per-Node | `/api/v1/metrics/nodes/{id}` | Individual node performance |
| Per-Region | `/api/v1/metrics/regions/{id}` | Regional aggregates |
| Routing | `/api/v1/metrics/routing` | Decision analytics, algorithm distribution |
| Backend Stats | `/api/v1/overwatch/stats` | Health aggregations, disagreement counts |

**Metrics Overview Response Structure:**
```json
{
  "overview": {
    "queries_total": 1000000,
    "queries_per_sec": 500.5,
    "health_checks_total": 500000,
    "active_servers": 200,
    "healthy_servers": 180,
    "response_times": {
      "avg_ms": 5.5,
      "p50_ms": 3.0,
      "p95_ms": 10.0,
      "p99_ms": 25.0
    },
    "error_rate": 0.01,
    "cache_hit_rate": 0.85
  }
}
```

### 3. Alerting System

Overlord exposes health data for alerting integration:

| Endpoint | Data | Alert Trigger |
|----------|------|---------------|
| `/api/v1/health/servers` | Per-server health | `consecutive_failures > 0` |
| `/api/v1/health/regions` | Regional health % | `health_percent < threshold` |
| `/api/v1/overwatch/stats` | Aggregated stats | `stale_backends > 0`, `disagreement_count > 0` |
| `/api/v1/overwatch/agents/expiring` | Cert expiration | `expires_in_hours < 720` (30 days) |

**Recommended Alert Conditions:**

| Condition | Severity | Description |
|-----------|----------|-------------|
| `consecutive_failures >= 3` | Warning | Server failing health checks |
| `stale_backends > 0` | Critical | Agents not reporting |
| `disagreement_count > 0` | Warning | Agent/validation mismatch |
| `unhealthy/total > 0.5` | Critical | Majority servers unhealthy |
| `expires_in_hours < 168` | Warning | Cert expires in 7 days |

### 4. Change Accountant (Audit Log)

All changes are immutably logged:

**Query Options:**
```
GET /api/v1/audit-logs                              # All entries
GET /api/v1/audit-logs?actions=create,update,delete # Filter by action
GET /api/v1/audit-logs?resources=domain,server      # Filter by resource
GET /api/v1/audit-logs?actors=admin@example.com     # Filter by who
GET /api/v1/audit-logs?start_time=2025-01-01        # Time range
GET /api/v1/audit-logs/stats                        # Statistics
GET /api/v1/audit-logs/export?format=csv            # Export
```

**Audit Entry Structure:**
```json
{
  "id": "e1",
  "timestamp": "2025-01-15T10:30:00Z",
  "action": "update",
  "resource": "server",
  "resource_id": "api.example.com:10.0.1.10:80",
  "actor": "ops-team",
  "actor_type": "user",
  "actor_ip": "192.168.1.50",
  "status": "success",
  "duration_ms": 50,
  "changes": {
    "weight": {"old": 100, "new": 150}
  }
}
```

---

## Complete API Reference

### Domains API

| Operation | Method | Endpoint | Body |
|-----------|--------|----------|------|
| List all | GET | `/api/v1/domains` | - |
| Get one | GET | `/api/v1/domains/{name}` | - |
| Create | POST | `/api/v1/domains` | Domain object |
| Update | PUT/PATCH | `/api/v1/domains/{name}` | Partial/full object |
| Delete | DELETE | `/api/v1/domains/{name}` | - |
| Get backends | GET | `/api/v1/domains/{name}/backends` | - |

**Domain Object:**
```json
{
  "name": "api.example.com",
  "ttl": 300,
  "routing_policy": "weighted",
  "dnssec_enabled": true,
  "enabled": true,
  "description": "Main API endpoint",
  "tags": ["production", "critical"],
  "settings": {
    "geo_routing_enabled": true,
    "failover_enabled": true,
    "failover_threshold": 2,
    "load_balancing_method": "round-robin"
  }
}
```

### Servers API

| Operation | Method | Endpoint | Body |
|-----------|--------|----------|------|
| List all | GET | `/api/v1/servers` | - |
| List filtered | GET | `/api/v1/servers?region=X&healthy=true&enabled=true` | - |
| Get one | GET | `/api/v1/servers/{id}` | - |
| Create | POST | `/api/v1/servers` | Server object |
| Update | PUT/PATCH | `/api/v1/servers/{id}` | Partial/full object |
| Delete | DELETE | `/api/v1/servers/{id}` | - |
| Get health check | GET | `/api/v1/servers/{id}/health-check` | - |
| Update health check | PUT | `/api/v1/servers/{id}/health-check` | HealthCheck object |

**Server Object:**
```json
{
  "name": "web-server-1",
  "address": "10.0.1.10",
  "port": 8080,
  "protocol": "tcp",
  "weight": 100,
  "priority": 0,
  "region": "us-east",
  "enabled": true,
  "description": "Primary web server",
  "tags": ["production", "web"],
  "metadata": {"rack": "A1", "dc": "dc1"},
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

### Regions API

| Operation | Method | Endpoint | Body |
|-----------|--------|----------|------|
| List all | GET | `/api/v1/regions` | - |
| List filtered | GET | `/api/v1/regions?enabled=true&continent=NA` | - |
| Get one | GET | `/api/v1/regions/{id}` | - |
| Create | POST | `/api/v1/regions` | Region object |
| Update | PUT/PATCH | `/api/v1/regions/{id}` | Partial/full object |
| Delete | DELETE | `/api/v1/regions/{id}` | - |

**Region Object:**
```json
{
  "name": "US East Coast",
  "code": "us-east",
  "description": "Primary US East region",
  "latitude": 38.8,
  "longitude": -77.1,
  "continent": "NA",
  "countries": ["US", "CA"],
  "enabled": true,
  "priority": 1
}
```

### Nodes API

**Overwatch Nodes (read-only):**

| Operation | Method | Endpoint |
|-----------|--------|----------|
| List all | GET | `/api/v1/nodes/overwatch` |
| List filtered | GET | `/api/v1/nodes/overwatch?status=online&region=X` |
| Get one | GET | `/api/v1/nodes/overwatch/{id}` |

**Agent Nodes:**

| Operation | Method | Endpoint | Body |
|-----------|--------|----------|------|
| List all | GET | `/api/v1/nodes/agent` | - |
| List filtered | GET | `/api/v1/nodes/agent?status=online&region=X` | - |
| Get one | GET | `/api/v1/nodes/agent/{id}` | - |
| Register | POST | `/api/v1/nodes/agent` | Agent object |
| Deregister | DELETE | `/api/v1/nodes/agent/{id}` | - |
| Get certificate | GET | `/api/v1/nodes/agent/{id}/certificate` | - |
| Reissue cert | POST | `/api/v1/nodes/agent/{id}/certificate` | - |
| Revoke cert | DELETE | `/api/v1/nodes/agent/{id}/certificate` | - |

### Health Overrides API

| Operation | Method | Endpoint | Body |
|-----------|--------|----------|------|
| List all | GET | `/api/v1/overrides` | - |
| Get one | GET | `/api/v1/overrides/{service}/{address}` | - |
| Set | PUT | `/api/v1/overrides/{service}/{address}` | Override object |
| Clear | DELETE | `/api/v1/overrides/{service}/{address}` | - |

**Override Object:**
```json
{
  "healthy": false,
  "reason": "Scheduled maintenance window",
  "source": "ops-team"
}
```

### Monitoring APIs

| Endpoint | Purpose |
|----------|---------|
| `/api/v1/health/servers` | Server health status with failure counts |
| `/api/v1/health/regions` | Regional health percentages |
| `/api/v1/metrics/overview` | System metrics overview |
| `/api/v1/metrics/history` | Time-series metrics |
| `/api/v1/overwatch/backends` | Detailed backend status |
| `/api/v1/overwatch/stats` | Aggregated statistics |
| `/api/v1/audit-logs` | Change history |

---

## Operator Workflows

### Provisioning New Infrastructure

```
1. Create region (if new):
   POST /api/v1/regions
   {"name": "EU West", "code": "eu-west", "continent": "EU", "countries": ["DE", "FR"]}

2. Add servers to region:
   POST /api/v1/servers
   {"address": "10.1.1.10", "port": 80, "region": "eu-west", "weight": 100}

   POST /api/v1/servers
   {"address": "10.1.1.11", "port": 80, "region": "eu-west", "weight": 100}

3. Create or update domain to use region:
   POST /api/v1/domains
   {"name": "api.example.com", "routing_policy": "geo", "enabled": true}
```

### Scheduled Maintenance

```
1. Set override (removes from rotation):
   PUT /api/v1/overrides/api.example.com/10.0.1.10
   {"healthy": false, "reason": "Kernel upgrade - ticket OPS-1234", "source": "ops-team"}

2. Verify override active:
   GET /api/v1/overrides

3. Perform maintenance...

4. Clear override (returns to rotation):
   DELETE /api/v1/overrides/api.example.com/10.0.1.10

5. Verify health restored:
   GET /api/v1/servers/api.example.com:10.0.1.10:80
```

### Traffic Shifting

```
# Gradual drain (reduce weight to 0):
PATCH /api/v1/servers/api.example.com:10.0.1.10:80
{"weight": 50}

# ... wait for connections to drain ...

PATCH /api/v1/servers/api.example.com:10.0.1.10:80
{"weight": 0}

# Region priority shift:
PATCH /api/v1/regions/us-east {"priority": 2}
PATCH /api/v1/regions/us-west {"priority": 1}

# Change algorithm:
PATCH /api/v1/domains/api.example.com
{"routing_policy": "latency"}
```

### Incident Response

```
# 1. Check overall health:
GET /api/v1/overwatch/stats

# 2. Find problem servers:
GET /api/v1/servers?healthy=false

# 3. Check specific server details:
GET /api/v1/servers/api.example.com:10.0.1.10:80

# 4. Review recent changes (correlation):
GET /api/v1/audit-logs?limit=20

# 5. Emergency override if needed:
PUT /api/v1/overrides/api.example.com/10.0.1.10
{"healthy": false, "reason": "Emergency - investigating outage", "source": "oncall"}
```

---

## Response Formats

### Success - Single Resource
```json
{"domain": {...}}
{"server": {...}}
{"region": {...}}
```

### Success - List
```json
{
  "servers": [...],
  "total": 25,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

### Success - Delete
HTTP 204 No Content (empty body)

### Error
```json
{
  "error": "descriptive error message",
  "code": 400
}
```

| Code | Meaning |
|------|---------|
| 400 | Bad request (validation, missing field) |
| 403 | Forbidden (ACL denied) |
| 404 | Not found |
| 405 | Method not allowed |
| 500 | Internal error |
| 503 | Service unavailable |

---

## Constraints and Gotchas

1. **Static servers are read-only** - Must edit YAML config and reload
2. **Agent servers are lifecycle-managed** - Only agents can create/remove them
3. **Server IDs are composite** - `{service}:{address}:{port}` format required
4. **Use PATCH for partial updates** - PUT may require all fields
5. **Override vs Delete** - Use overrides for temporary removal, keeps audit trail
6. **Audit log is immutable** - Cannot delete or modify entries
7. **Certificate operations need valid agent** - Check agent exists first
