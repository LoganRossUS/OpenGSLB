# OpenGSLB Overlord Dashboard - AI Assistant Context

This document provides context for AI assistants helping operators use the **Overlord Dashboard** - the enterprise-grade management interface for OpenGSLB infrastructure.

## What is Overlord?

**Overlord** is the web-based management dashboard for OpenGSLB that provides:

1. **Resource Management** - CRUD operations for domains, servers, regions, nodes, and services
2. **Metrics Dashboard** - Real-time and historical performance metrics visualization
3. **Alerting System** - Health monitoring and notification configuration
4. **Change Accountant** - Comprehensive audit log tracking all configuration changes

Overlord communicates with OpenGSLB via the REST API at `/api/v1`.

---

## Resource Hierarchy

Overlord manages resources in a hierarchical structure:

```
+------------------+
|     DOMAINS      |  DNS zones with routing policies
|  (api.example.com)|  (weighted, geo, failover, latency)
+--------+---------+
         |
         | references
         v
+------------------+
|     REGIONS      |  Geographic server pools
|   (us-east, eu)  |  (countries, continents, priority)
+--------+---------+
         |
         | contains
         v
+------------------+
|     SERVERS      |  Backend instances
| (10.0.1.10:8080) |  (address, port, weight, health)
+------------------+
         ^
         | monitored by
+------------------+
|   AGENT NODES    |  Health check agents
|  (agent-east-1)  |  (gossip-based reporting)
+------------------+

+------------------+
| OVERWATCH NODES  |  DNS authority servers
| (overwatch-east) |  (query handling, validation)
+------------------+
```

### Server Sources

Servers displayed in Overlord come from three sources:

| Source | Description | Editable in Overlord? |
|--------|-------------|----------------------|
| `static` | Defined in YAML configuration files | Read-only (edit config file) |
| `agent` | Self-registered via gossip protocol | Read-only (agent lifecycle) |
| `api` | Created through Overlord/API | **Full CRUD** |

---

## Overlord Capabilities

### 1. Resource Management

#### Domains
- View all GSLB-managed DNS zones
- Create new domains with routing policies
- Configure TTL, DNSSEC, failover settings
- View backends serving each domain

#### Servers (Backends)
- List all backend servers across regions
- Filter by region, health status, enabled state
- Add new servers (creates `source: api`)
- Modify weights for traffic distribution
- Enable/disable servers
- Delete API-created servers

#### Regions
- View geographic server pools
- Create/edit regions with coordinates
- Assign countries and continents for geo-routing
- Set failover priorities

#### Nodes
- **Overwatch Nodes**: View DNS authority server status
- **Agent Nodes**: Register, view, and deregister health agents
- Manage agent certificates (view, reissue, revoke)

#### Health Overrides
- Manually mark servers healthy/unhealthy
- Used for maintenance windows
- Tracks who set override and why

### 2. Metrics Dashboard

Overlord displays metrics from multiple sources:

| Endpoint | Metrics |
|----------|---------|
| `/api/v1/metrics/overview` | QPS, health check rates, server counts, response times |
| `/api/v1/metrics/history` | Time-series data with configurable resolution |
| `/api/v1/metrics/nodes/{id}` | Per-node performance |
| `/api/v1/metrics/regions/{id}` | Regional statistics |
| `/api/v1/metrics/routing` | Routing decision analytics |
| `/api/v1/overwatch/stats` | Backend health aggregations |

### 3. Alerting System

Health monitoring endpoints for alerting integration:

| Endpoint | Purpose |
|----------|---------|
| `/api/v1/health/servers` | Individual server health with failure counts |
| `/api/v1/health/regions` | Regional health percentages |
| `/api/v1/overwatch/backends` | Detailed status (agent + validation) |
| `/api/v1/overwatch/agents/expiring` | Certificate expiration warnings |

**Alert-worthy conditions:**
- `consecutive_failures > 0` on servers
- `stale_backends > 0` (agents not reporting)
- `disagreement_count > 0` (agent/validation mismatch)
- `unhealthy_backends / total_backends > threshold`

### 4. Change Accountant (Audit Log)

All changes are tracked in the audit log:

```
GET /api/v1/audit-logs
GET /api/v1/audit-logs?actions=create,update,delete
GET /api/v1/audit-logs?resources=domain,server
GET /api/v1/audit-logs?actors=admin@example.com
GET /api/v1/audit-logs/stats
GET /api/v1/audit-logs/export?format=csv
```

Each entry includes:
- Timestamp
- Action (create, update, delete)
- Resource type and ID
- Actor (user/system)
- Actor IP address
- Success/failure status
- Duration

---

## API Operations Reference

### Domains
| Operation | Method | Endpoint |
|-----------|--------|----------|
| List all | GET | `/api/v1/domains` |
| Get one | GET | `/api/v1/domains/{name}` |
| Create | POST | `/api/v1/domains` |
| Update | PUT/PATCH | `/api/v1/domains/{name}` |
| Delete | DELETE | `/api/v1/domains/{name}` |
| List backends | GET | `/api/v1/domains/{name}/backends` |

**Create/Update payload:**
```json
{
  "name": "api.example.com",
  "ttl": 300,
  "routing_policy": "weighted|geo|failover|latency|round-robin",
  "dnssec_enabled": true,
  "enabled": true,
  "description": "Main API endpoint",
  "settings": {
    "geo_routing_enabled": true,
    "failover_enabled": true,
    "failover_threshold": 2
  }
}
```

### Servers
| Operation | Method | Endpoint |
|-----------|--------|----------|
| List all | GET | `/api/v1/servers` |
| List filtered | GET | `/api/v1/servers?region=X&healthy=true` |
| Get one | GET | `/api/v1/servers/{id}` |
| Create | POST | `/api/v1/servers` |
| Update | PUT/PATCH | `/api/v1/servers/{id}` |
| Delete | DELETE | `/api/v1/servers/{id}` |

**Server ID format:** `{service}:{address}:{port}` (e.g., `api.example.com:10.0.1.10:80`)

**Create payload:**
```json
{
  "name": "web-server-1",
  "address": "10.0.1.10",
  "port": 8080,
  "region": "us-east",
  "weight": 100,
  "enabled": true,
  "health_check": {
    "type": "http",
    "path": "/health",
    "interval": "30s",
    "timeout": "5s"
  }
}
```

### Regions
| Operation | Method | Endpoint |
|-----------|--------|----------|
| List all | GET | `/api/v1/regions` |
| Get one | GET | `/api/v1/regions/{id}` |
| Create | POST | `/api/v1/regions` |
| Update | PUT/PATCH | `/api/v1/regions/{id}` |
| Delete | DELETE | `/api/v1/regions/{id}` |

**Create payload:**
```json
{
  "name": "US East Coast",
  "code": "us-east",
  "continent": "NA",
  "countries": ["US", "CA"],
  "latitude": 38.8,
  "longitude": -77.1,
  "enabled": true,
  "priority": 1
}
```

### Health Overrides
| Operation | Method | Endpoint |
|-----------|--------|----------|
| List all | GET | `/api/v1/overrides` |
| Get one | GET | `/api/v1/overrides/{service}/{address}` |
| Set | PUT | `/api/v1/overrides/{service}/{address}` |
| Clear | DELETE | `/api/v1/overrides/{service}/{address}` |

**Set override payload:**
```json
{
  "healthy": false,
  "reason": "Scheduled maintenance window",
  "source": "ops-team"
}
```

### Agent Nodes
| Operation | Method | Endpoint |
|-----------|--------|----------|
| List all | GET | `/api/v1/nodes/agent` |
| Get one | GET | `/api/v1/nodes/agent/{id}` |
| Register | POST | `/api/v1/nodes/agent` |
| Deregister | DELETE | `/api/v1/nodes/agent/{id}` |
| Get certificate | GET | `/api/v1/nodes/agent/{id}/certificate` |
| Reissue cert | POST | `/api/v1/nodes/agent/{id}/certificate` |
| Revoke cert | DELETE | `/api/v1/nodes/agent/{id}/certificate` |

---

## Common Operator Workflows

### Adding Infrastructure

**Add a new region:**
```
POST /api/v1/regions
{"name": "EU West", "code": "eu-west", "continent": "EU", "countries": ["DE", "FR", "NL"]}
```

**Add servers to the region:**
```
POST /api/v1/servers
{"address": "10.1.1.10", "port": 80, "region": "eu-west", "weight": 100, "enabled": true}
```

**Create a domain using the region:**
```
POST /api/v1/domains
{"name": "api.example.com", "routing_policy": "geo", "enabled": true}
```

### Maintenance Operations

**Take server offline (preserves config):**
```
PUT /api/v1/overrides/api.example.com/10.0.1.10
{"healthy": false, "reason": "Kernel upgrade", "source": "ops-team"}
```

**Return to service:**
```
DELETE /api/v1/overrides/api.example.com/10.0.1.10
```

### Traffic Management

**Drain traffic from server (gradual):**
```
PATCH /api/v1/servers/api.example.com:10.0.1.10:80
{"weight": 0}
```

**Shift traffic between regions:**
```
PATCH /api/v1/regions/us-east {"priority": 2}
PATCH /api/v1/regions/us-west {"priority": 1}
```

**Change routing algorithm:**
```
PATCH /api/v1/domains/api.example.com
{"routing_policy": "latency"}
```

### Monitoring & Troubleshooting

**Check overall health:**
```
GET /api/v1/overwatch/stats
```

**Find unhealthy servers:**
```
GET /api/v1/servers?healthy=false
```

**Review recent changes:**
```
GET /api/v1/audit-logs?limit=50
```

**Check agent status:**
```
GET /api/v1/nodes/agent?status=offline
```

---

## Response Patterns

**Single resource:**
```json
{"domain": {...}}
{"server": {...}}
```

**List response:**
```json
{
  "servers": [...],
  "total": 25,
  "generated_at": "2025-01-15T10:30:00Z"
}
```

**Error response:**
```json
{"error": "descriptive message", "code": 400}
```

**Delete success:** HTTP 204 No Content

---

## Important Constraints

1. **Static servers are read-only** - Cannot modify/delete via API; edit YAML config instead
2. **Agent servers are lifecycle-managed** - Created/removed by agent registration
3. **Server IDs are composite** - Format: `{service}:{address}:{port}`
4. **PATCH for partial updates** - Only include fields being changed
5. **Audit trail is immutable** - All changes logged, cannot be deleted
