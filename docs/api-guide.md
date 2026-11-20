# OpenGSLB API guide

This guide is written by hand. It covers what the endpoint reference does not: how access to the API is
controlled, how to configure the listener, how errors look, and worked examples.

- **Endpoint reference:** [api.md](api.md), generated from [`api/openapi.yaml`](../api/openapi.yaml). Every
  path, parameter, request body and response schema is listed there and only there.
- **Hardening for production:** [security/api-hardening.md](security/api-hardening.md).

The REST API is served by a node running in overwatch mode. A node in agent mode starts no API server.

## Contents

- [Security and access control](#security-and-access-control)
- [Configuration](#configuration)
- [Endpoint status](#endpoint-status)
- [Error responses](#error-responses)
- [Usage examples](#usage-examples)
  - [curl](#curl)
  - [Kubernetes probes](#kubernetes-probes)
  - [Monitoring](#monitoring)

## Security and access control

The API exposes internal infrastructure details (server addresses, health states, failure reasons) and can
change what DNS answers. **There is no authentication and no TLS.** The only built-in control is an allowlist
of client IP networks (`api.allowed_networks`).

By default the API:

- is disabled (`api.enabled: false`);
- when enabled, binds to `127.0.0.1:8080`;
- accepts clients only from `127.0.0.1/32` and `::1/128`;
- does not trust proxy headers.

A client outside the allowlist receives `403` with the plain-text body `Forbidden` (not the JSON error
format).

The allowlist is **not** applied to the following paths. They answer any client that can reach the listener:

| Path | What it returns |
|------|-----------------|
| `/api/v1/live` | Liveness probe |
| `/api/v1/ready` | Readiness probe |
| `/api/v1/version` | Build and version information |
| `/api`, `/api/v1` | Discovery listings |
| `/api/v1/health`, `/api/v1/geo`, `/api/v1/overwatch`, `/api/v1/cluster` | Discovery listings (the group index only, not the endpoints below it) |
| `/api/health` | Simple health check. Unprotected when attached, but the binary does not attach it today (see [Endpoint status](#endpoint-status)) |

Every other registered endpoint is behind the allowlist, including `/api/v1/health/servers`, `/api/v1/health/regions` and
everything under `/api/v1/overwatch/`, `/api/v1/domains`, `/api/v1/servers`, `/api/v1/regions`,
`/api/v1/overrides`, `/api/v1/geo/`, `/api/v1/nodes/`, `/api/v1/gossip/`, `/api/v1/audit-logs`,
`/api/v1/metrics`, `/api/v1/preferences`, `/api/v1/config` and `/api/v1/routing/`. A path that matches no
endpoint (including the DNSSEC paths the binary does not attach) answers a JSON `404` without an allowlist check.

With `api.trust_proxy_headers: true` the client address used for the allowlist is taken from the first entry
of `X-Forwarded-For`, or from `X-Real-IP` if that header is absent. Any client that can reach the listener
directly can set these headers, so enable this only when the listener is reachable solely through a proxy you
control.

For production deployments (reverse proxy with TLS and authentication, firewalling), see the
[API Hardening Guide](security/api-hardening.md).

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
| `address` | string | `127.0.0.1:8080` | Listen address (`host:port`) |
| `allowed_networks` | []string | `["127.0.0.1/32", "::1/128"]` | CIDR networks allowed to call the protected endpoints |
| `trust_proxy_headers` | bool | `false` | Take the client address from `X-Forwarded-For` / `X-Real-IP` |

These four keys are the whole `api:` section. An invalid `address` or a malformed CIDR in `allowed_networks`
is rejected when the configuration is loaded.

> **Note:** The `allowed_networks` list is matched by IP family. To allow all clients you must include both
> `0.0.0.0/0` (IPv4) and `::/0` (IPv6). Including only `0.0.0.0/0` blocks IPv6 clients, including localhost
> connections via `::1`.

> **CORS is not configurable.** Earlier versions of this document listed `api.enable_cors` and an `api.cors`
> block. The configuration has no such keys: they are ignored without an error, and the binary never enables
> its CORS middleware. Responses carry no `Access-Control-*` headers and an `OPTIONS` preflight is answered
> `405`, so a browser page served from another origin cannot call the API directly.

## Endpoint status

Not every documented endpoint works. The reference marks each operation with a status (real, stub, not
implemented, unavailable, unwired, broken) and explains the terms in its
[overview](api.md#overview); [api-alignment.md](api-alignment.md) lists them per endpoint. In particular, in
the current binary:

- the DNSSEC endpoints (`/api/v1/dnssec/...`) and `/api/health` answer `404`, because the handlers are never
  attached;
- the agent certificate endpoints (`/api/v1/overwatch/agents...`) always answer `503`;
- `DELETE /api/v1/domains/{name}/backends/{id}` always answers `404`.

The examples below use only operations whose status is real.

## Error responses

Handlers report errors as JSON with the HTTP status repeated in `code`
(schema [ErrorResponse](api.md#schema-errorresponse)):

```json
{
  "error": "not found",
  "code": 404
}
```

The exception is the allowlist rejection, which is `403` with the plain-text body `Forbidden`.

Status codes used by the API:

| Status code | Meaning |
|-------------|---------|
| `200` | Success |
| `201` | Created |
| `204` | Success, no content |
| `400` | Invalid parameters or malformed JSON |
| `403` | Client address not in `allowed_networks` (plain text) |
| `404` | Resource or path not found |
| `405` | HTTP method not supported on this path |
| `500` | Internal error. Some client mistakes are also reported as `500` today, for example creating a domain whose name already exists |
| `503` | A dependency is not configured or not ready |

Which of these an operation can return is listed per operation in the [reference](api.md).

## Usage examples

### curl

The examples assume the default listener, `http://localhost:8080`, and were checked against a running
overwatch. Replace the service name, addresses and ports with ones from your configuration.

```bash
# Health of every health-checked server
curl http://localhost:8080/api/v1/health/servers

# Health aggregated by region
curl http://localhost:8080/api/v1/health/regions

# Readiness: print only the status code (200 ready, 503 not ready)
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/v1/ready

# Pretty print with jq
curl -s http://localhost:8080/api/v1/health/servers | jq .

# Version information
curl http://localhost:8080/api/v1/version

# Set an external health override ("source" is required)
curl -X PUT http://localhost:8080/api/v1/overrides/app.example.com/10.0.1.10 \
  -H "Content-Type: application/json" \
  -d '{"healthy": false, "reason": "maintenance", "source": "admin"}'

# List all external overrides
curl http://localhost:8080/api/v1/overrides

# Clear an external override
curl -X DELETE http://localhost:8080/api/v1/overrides/app.example.com/10.0.1.10

# Create a domain
curl -X POST http://localhost:8080/api/v1/domains \
  -H "Content-Type: application/json" \
  -d '{"name": "api.example.com", "ttl": 300, "routing_policy": "latency"}'

# Add a backend to a domain
curl -X POST http://localhost:8080/api/v1/domains/api.example.com/backends \
  -H "Content-Type: application/json" \
  -d '{"address": "10.0.2.10", "port": 443, "weight": 100, "region": "us-west-2"}'

# List the backends of a domain
curl http://localhost:8080/api/v1/domains/api.example.com/backends

# Delete a domain that was created through the API
curl -X DELETE http://localhost:8080/api/v1/domains/api.example.com

# List the backends in the overwatch registry
curl http://localhost:8080/api/v1/overwatch/backends

# Filter them by service
curl "http://localhost:8080/api/v1/overwatch/backends?service=app.example.com"

# Backend statistics
curl http://localhost:8080/api/v1/overwatch/stats

# Force a backend unhealthy (path: service/address/port); X-User is recorded as the author
curl -X POST http://localhost:8080/api/v1/overwatch/backends/app.example.com/10.0.1.10/8080/override \
  -H "Content-Type: application/json" \
  -H "X-User: admin" \
  -d '{"healthy": false, "reason": "deploying new version"}'

# Remove that override
curl -X DELETE http://localhost:8080/api/v1/overwatch/backends/app.example.com/10.0.1.10/8080/override

# Trigger external validation of all backends now
curl -X POST http://localhost:8080/api/v1/overwatch/validate
```

Things to know when adapting these (each is recorded in the reference):

- On domain create and update only `routing_policy` selects the algorithm; if it is omitted the domain is
  created as `round-robin`.
- Listing the backends of an API-created domain that has no backends yet returns `404`.
- Config-file domains cannot be deleted through the API (`500`).
- The backend override returns `404` when the service/address/port is not in the registry.

### Kubernetes probes

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

Both probe paths skip the allowlist, but the kubelet must still be able to reach the listener: with the
default `api.address` of `127.0.0.1:8080` it cannot, so bind the API to the pod address (for example
`0.0.0.0:8080`) and keep `allowed_networks` narrow.

### Monitoring

Prometheus metrics are served by a separate listener, described in [metrics.md](metrics.md). The REST API
complements it:

- Use `/api/v1/ready` as a health check target (for example with `blackbox_exporter`).
- Poll `/api/v1/health/servers` and alert on `consecutive_failures > 0`.
- Poll `/api/v1/overwatch/stats` and alert on:
  - `stale_backends > 0`: agents are not reporting;
  - `disagreement_count > 0`: an agent's report and the overwatch's own validation disagree;
  - `unhealthy_backends / total_backends > 0.5`: high failure rate.
