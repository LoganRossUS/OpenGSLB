# API alignment: OpenGSLB overwatch and Overlord

Status of every REST endpoint on both sides, as of `develop` (OpenGSLB v1.1.10) and Overlord `main`
(`bc68e5b`), recorded on 2026-11-20 for Phase 0 of the enterprise-readiness work.

- The contract is [`api/openapi.yaml`](../api/openapi.yaml). It was written from handler behavior, not from
  the old prose in `docs/api.md`.
- "Go status" was observed by starting a real overwatch in-process and calling each endpoint
  (`cmd/opengslb/api_contract_test.go`). It is not inferred from the code alone.
- Overlord columns come from reading its source. Nothing in Overlord was executed, because it has no tests
  yet (Phase 0 task 5 adds them).

## Summary

| | Count |
|---|---|
| Operations in the spec | 107 (71 paths) |
| Real | 60 |
| Stub (placeholder or always-empty data) | 19 |
| Stub that always fails (`not-implemented`) | 16 |
| Real handler that always answers 503 (`unavailable`, agent certificates) | 5 |
| Handler never attached by the binary (`unwired`, DNSSEC and `/api/health`) | 6 |
| Broken (`DELETE /domains/{name}/backends/{id}`) | 1 |
| Spec operations Overlord calls | 17 (3 of them from dead code) |
| Paths Overlord calls that Go does not serve | 4, plus the Prometheus listener |

Overlord has 14 pages. None talks to an overwatch directly. Two (Domains, Servers) write through to
overwatches; six read Overlord's Postgres, which the node poller fills; four (Health, Overrides, GeoMappings,
DNSSEC) render only `src/data/mockData.js`, which today exports empty placeholders, and simulate their
actions in the browser.

## Mismatches

Numbered so Phase 1E task 3 can refer to them. "Side" is where the fix most likely belongs; none is applied in
Phase 0.

| # | Mismatch | Effect today | Side |
|---|---|---|---|
| M1 | Overlord polls `GET /api/v1/health` as a health document. Go returns the list of health endpoints. | Node "healthy" only means the port answered. `version` and `uptime` come back undefined. | Overlord (use `/api/v1/ready` and `/api/v1/version`) |
| M2 | Overlord reads node region and version from `GET /api/v1/config`. Go returns an empty `region` / `node_id` and the constant version `1.0.0`. | Region is never learned from the node; `1.0.0` can be mistaken for the node version. | Go (populate from the node identity) |
| M3 | `GET /api/v1/servers`: Overlord reads `domain` / `domain_name`; Go sends `name` and `metadata.service`. | Synced servers are not linked to their domain from this call. | Decide in the spec, then both |
| M4 | `GET /api/v1/domains/{name}/backends` returns 404 for an API-created domain with no backends, and carries no source (static, agent, api) or health-check fields. | Overlord labels every synced backend `static` and guesses the check type from the port. | Go |
| M5 | Overlord sends `routing_algorithm` and `regions` on domain create and update. Go reads only `routing_policy`. | Every domain pushed from Overlord is created as `round-robin`; algorithm changes never reach the node. Overlord's poller then reads `routing_policy` back and overwrites its own value. | Decide one field name in the spec, then both |
| M6 | `POST /api/v1/domains/{name}/backends` ignores the `health_check` Overlord sends. | The health check configured in Overlord is not applied on the node. | Go |
| M7 | `DELETE /api/v1/domains/{name}/backends/{id}` is unreachable in Go (always 404). | A server deleted in Overlord stays on every node and keeps being returned in DNS. Overlord reports the delete as successful. | Go |
| M8 | Overlord sends health-check `interval` / `timeout` as seconds; Go's `ServerHealthCheck` decodes integers as nanoseconds and rejects strings such as `"30s"`. Overlord's own guide documents strings. | A 30 second interval pushed from Overlord becomes 30 ns on the node. | Decide the unit in the spec, then both |
| M9 | `pushServer` (dead code) sends `service`; Go requires `name` or `metadata.service` and answers 500. | None today. Would fail if wired up. | Overlord |
| M10 | Overlord polls `/api/v1/status`, `/api/v1/identity` and `/api/v1/overview`, which do not exist. | Three 404s per node per poll, caught and logged. | Overlord (remove; `/api/v1/metrics/overview` has the data) |
| M11 | Overlord polls agent nodes over HTTP (`/api/v1/health`, `/api/v1/backends`). Agent mode starts no API server. | Every agent is marked unhealthy. | Overlord (agents are visible through the overwatch) |
| M12 | Overlord's client assumes plain HTTP on port 8080 with no credentials, and pushes to nodes it has marked unhealthy. Go's default is `127.0.0.1:8080` behind a loopback-only allowlist. | Works only when the operator has opened `api.address` and `api.allowed_networks`. Phase 3A changes this. | Both (Phase 3A) |
| M13 | Push failures are not errors in Overlord: the Postgres write commits and the route returns 2xx with `_pushResults.failed[]`. | Overlord and the nodes drift silently (M5 and M7 make this the normal case). | Overlord (Phase 4 drift work) |
| M14 | Overlord's Overrides, Geo, DNSSEC, Gossip, Audit, Routing and Config routes share paths with the overwatch API but serve only Overlord's Postgres or generated data. | An override or geo mapping made in Overlord has no effect on DNS. | Overlord (Phase 1E task 2, Phase 3B task 6) |

Go-side findings that do not involve Overlord are listed under "Found, not fixed" in the Phase 0 pull request.

## Endpoints

One row per operation in the spec, then the paths Overlord calls that are not in it. "Go status" values:
real; stub (placeholder data); stub (always fails); real handler, always 503; handler not attached (404);
broken; missing.

| Endpoint | In spec | Go status | Called by Overlord (file and function) | Overlord page | Mismatch |
|---|---|---|---|---|---|
| `GET /api` | yes | real | not called | none |  |
| `GET /api/health` | yes | handler not attached (404) | not called | none |  |
| `GET /api/v1` | yes | real | not called | none |  |
| `GET /api/v1/health` | yes | real | `server/services/nodePoller.js` `pollNode` | Nodes, Overview (node status) | **M1** Used as the node liveness check and read for `.version` / `.uptime`. Go returns the endpoint listing, so both are undefined; it only proves the port answers. |
| `GET /api/v1/geo` | yes | real | not called | none |  |
| `GET /api/v1/overwatch` | yes | real | not called | none |  |
| `GET /api/v1/cluster` | yes | real | not called | none |  |
| `GET /api/v1/version` | yes | real | `server/services/nodePoller.js` `pollNode`; `server/routes/nodes.js` `GET /nodes/:id/version?fresh=true` | Nodes | None. |
| `GET /api/v1/live` | yes | real | not called | none |  |
| `GET /api/v1/ready` | yes | real | not called | none |  |
| `GET /api/v1/health/servers` | yes | real | not called | none |  |
| `GET /api/v1/health/regions` | yes | real | not called | none |  |
| `GET /api/v1/overwatch/backends` | yes | real | not called | none |  |
| `POST /api/v1/overwatch/backends/{service}/{address}/{port}/override` | yes | real | not called | none |  |
| `DELETE /api/v1/overwatch/backends/{service}/{address}/{port}/override` | yes | real | not called | none |  |
| `GET /api/v1/overwatch/stats` | yes | real | not called | none |  |
| `POST /api/v1/overwatch/validate` | yes | real | not called | none |  |
| `GET /api/v1/overwatch/latency` | yes | real | not called | none |  |
| `POST /api/v1/overwatch/latency` | yes | real | not called | none |  |
| `GET /api/v1/overwatch/agents` | yes | real handler, always 503 | not called | none |  |
| `GET /api/v1/overwatch/agents/expiring` | yes | real handler, always 503 | not called | none |  |
| `GET /api/v1/overwatch/agents/{agent_id}` | yes | real handler, always 503 | not called | none |  |
| `DELETE /api/v1/overwatch/agents/{agent_id}` | yes | real handler, always 503 | not called | none |  |
| `POST /api/v1/overwatch/agents/{agent_id}/revoke` | yes | real handler, always 503 | not called | none |  |
| `GET /api/v1/cluster/status` | yes | real | not called | none |  |
| `GET /api/v1/overrides` | yes | real | not called | none |  |
| `GET /api/v1/overrides/{service}/{address}` | yes | real | not called | none |  |
| `PUT /api/v1/overrides/{service}/{address}` | yes | real | not called | none |  |
| `DELETE /api/v1/overrides/{service}/{address}` | yes | real | not called | none |  |
| `GET /api/v1/dnssec` | yes | handler not attached (404) | not called | none |  |
| `GET /api/v1/dnssec/status` | yes | handler not attached (404) | not called | none |  |
| `GET /api/v1/dnssec/ds` | yes | handler not attached (404) | not called | none |  |
| `GET /api/v1/dnssec/keys` | yes | handler not attached (404) | not called | none |  |
| `POST /api/v1/dnssec/sync` | yes | handler not attached (404) | not called | none |  |
| `GET /api/v1/geo/mappings` | yes | real | not called | none |  |
| `PUT /api/v1/geo/mappings` | yes | real | not called | none |  |
| `POST /api/v1/geo/mappings` | yes | real | not called | none |  |
| `DELETE /api/v1/geo/mappings/{cidr}` | yes | real | not called | none |  |
| `GET /api/v1/geo/test` | yes | real | not called | none |  |
| `GET /api/v1/domains` | yes | real | `server/services/nodePoller.js` `pollNode` -> `syncDomains` | Domains | None for `name`, `ttl`, `enabled`, `routing_policy`. |
| `POST /api/v1/domains` | yes | real | `server/services/openGslbClient.js` `pushDomain` (from `routes/domains.js` POST) | Domains | **M5** Overlord sends `routing_algorithm` and `regions`; Go reads `routing_policy` only and ignores both, so every domain pushed by Overlord becomes `round-robin` on the node. A duplicate returns 500. |
| `GET /api/v1/domains/{name}` | yes | real | not called | none |  |
| `PUT /api/v1/domains/{name}` | yes | real | `server/services/openGslbClient.js` `updateDomainOnNodes` (from `routes/domains.js` PUT) | Domains | **M5** Same: `routing_algorithm` and `regions` are ignored, so an algorithm change in Overlord never reaches the node. |
| `PATCH /api/v1/domains/{name}` | yes | real | not called | none |  |
| `DELETE /api/v1/domains/{name}` | yes | real | `server/services/openGslbClient.js` `deleteDomainFromNodes` (from `routes/domains.js` DELETE) | Domains | None for API-created domains. Config-file domains return 500. |
| `GET /api/v1/domains/{name}/backends` | yes | real | `server/services/nodePoller.js` `pollNode` -> `syncServerFromBackend` | Servers, Domains | **M4** Go returns 404 for an API-created domain with no backends. Overlord sets `source='static'` on every row and guesses the health-check type from the port; Go exposes neither a source nor the check on this endpoint. Domain name is not URL-encoded. |
| `POST /api/v1/domains/{name}/backends` | yes | real | `server/services/openGslbClient.js` `pushBackend` (from `routes/servers.js` POST) | Servers | **M6** The `health_check` object Overlord sends is ignored. An unknown domain returns 500, not 404. |
| `DELETE /api/v1/domains/{name}/backends/{id}` | yes | broken (always 404) | `server/services/openGslbClient.js` `deleteBackendFromNodes` (from `routes/servers.js` DELETE) | Servers | **M7** Always 404 on the Go side (routing defect), so a server deleted in Overlord stays on every node and keeps being answered in DNS. |
| `GET /api/v1/servers` | yes | real | `server/services/nodePoller.js` `pollNode` -> `syncServers` | Servers | **M3** Overlord reads `domain` / `domain_name`; Go sends the owning domain in `name` and `metadata.service`, so the domain link is not set from this call. |
| `POST /api/v1/servers` | yes | real | `server/services/openGslbClient.js` `pushServer` (never called) | none | **M9** Dead code. If called it would fail: it sends `service`, which Go ignores, and Go then returns 500 `service is required`. |
| `GET /api/v1/servers/{id}` | yes | real | not called | none |  |
| `PUT /api/v1/servers/{id}` | yes | real | `server/services/openGslbClient.js` `updateServerOnNodes` (from `routes/servers.js` PUT) | Servers | **M8** Overlord sends `health_check.interval` / `timeout` as seconds (30, 5); Go decodes them as nanoseconds. `PATCH /servers/:id` and `PUT /servers/:id/health-check` in Overlord are not pushed at all. |
| `PATCH /api/v1/servers/{id}` | yes | real | not called | none |  |
| `DELETE /api/v1/servers/{id}` | yes | real | `server/services/openGslbClient.js` `deleteServerFromNodes` (never called) | none | Dead code. |
| `GET /api/v1/servers/{id}/health-check` | yes | real | not called | none |  |
| `PUT /api/v1/servers/{id}/health-check` | yes | real | not called | none |  |
| `PATCH /api/v1/servers/{id}/health-check` | yes | real | not called | none |  |
| `GET /api/v1/regions` | yes | real | `server/services/nodePoller.js` `pollNode` -> `syncRegions` | Servers, Domains (region lists) | None. Overlord also looks for nested `servers[]`, which Go does not send. |
| `POST /api/v1/regions` | yes | real | `server/services/openGslbClient.js` `pushRegion` (never called) | none | Dead code. Regions created in Overlord are never pushed. |
| `GET /api/v1/regions/{id}` | yes | real | not called | none |  |
| `PUT /api/v1/regions/{id}` | yes | real | not called | none |  |
| `PATCH /api/v1/regions/{id}` | yes | real | not called | none |  |
| `DELETE /api/v1/regions/{id}` | yes | real | not called | none |  |
| `GET /api/v1/nodes/overwatch` | yes | stub | not called | none |  |
| `GET /api/v1/nodes/overwatch/{id}` | yes | stub | not called | none |  |
| `GET /api/v1/nodes/agent` | yes | stub | not called | none |  |
| `POST /api/v1/nodes/agent` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/nodes/agent/{id}` | yes | stub | not called | none |  |
| `DELETE /api/v1/nodes/agent/{id}` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/nodes/agent/{id}/certificate` | yes | stub (always fails) | not called | none |  |
| `POST /api/v1/nodes/agent/{id}/certificate` | yes | stub (always fails) | not called | none |  |
| `DELETE /api/v1/nodes/agent/{id}/certificate` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/gossip/nodes` | yes | stub | not called | none |  |
| `GET /api/v1/gossip/nodes/{id}` | yes | stub | not called | none |  |
| `GET /api/v1/gossip/config` | yes | stub | not called | none |  |
| `PUT /api/v1/gossip/config` | yes | stub (always fails) | not called | none |  |
| `PATCH /api/v1/gossip/config` | yes | stub (always fails) | not called | none |  |
| `POST /api/v1/gossip/generate-key` | yes | real | not called | none |  |
| `GET /api/v1/audit-logs` | yes | stub | not called | none |  |
| `GET /api/v1/audit-logs/stats` | yes | stub | not called | none |  |
| `GET /api/v1/audit-logs/export` | yes | stub | not called | none |  |
| `GET /api/v1/audit-logs/{id}` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/metrics` | yes | real | `server/services/nodePoller.js` `pollNode` (only if the Prometheus scrape fails) | Overview, Metrics | None for the fields under `overview`. The flat fallbacks (`backends_managed`, `latency_p50`, ...) do not exist in Go. |
| `GET /api/v1/metrics/overview` | yes | real | not called | none |  |
| `GET /api/v1/metrics/history` | yes | stub | not called | none |  |
| `GET /api/v1/metrics/nodes/{id}` | yes | real | not called | none |  |
| `GET /api/v1/metrics/regions/{id}` | yes | real | not called | none |  |
| `GET /api/v1/metrics/routing` | yes | real | not called | none |  |
| `GET /api/v1/preferences` | yes | stub | not called | none |  |
| `PUT /api/v1/preferences` | yes | stub (always fails) | not called | none |  |
| `PATCH /api/v1/preferences` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/config` | yes | stub | `server/services/nodePoller.js` `pollNode` | Nodes (region, version) | **M2** Scanned for `config.region`, `config.identity.region`, `config.version`. Go returns an empty region and the constant `1.0.0`, which is not the node version. |
| `GET /api/v1/config/system` | yes | stub | not called | none |  |
| `GET /api/v1/config/dns` | yes | real | not called | none |  |
| `PUT /api/v1/config/dns` | yes | stub (always fails) | not called | none |  |
| `PATCH /api/v1/config/dns` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/config/health-check` | yes | real | not called | none |  |
| `PUT /api/v1/config/health-check` | yes | stub (always fails) | not called | none |  |
| `PATCH /api/v1/config/health-check` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/config/logging` | yes | real | not called | none |  |
| `PUT /api/v1/config/logging` | yes | stub (always fails) | not called | none |  |
| `PATCH /api/v1/config/logging` | yes | stub (always fails) | not called | none |  |
| `GET /api/v1/routing/algorithms` | yes | stub | not called | none |  |
| `GET /api/v1/routing/algorithms/{id}` | yes | stub | not called | none |  |
| `POST /api/v1/routing/test` | yes | stub | not called | none |  |
| `GET /api/v1/routing/decisions` | yes | stub | not called | none |  |
| `GET /api/v1/routing/flows` | yes | stub | not called | none |  |
| `GET /api/v1/status` | no | missing | `server/services/nodePoller.js` `pollNode` | Nodes | **M10** Not a Go route (404). Overlord reads `version`, `uptime`, `backends_count`, `qps`. |
| `GET /api/v1/identity` | no | missing | `server/services/nodePoller.js` `pollNode` | Nodes (region) | **M10** Not a Go route (404). |
| `GET /api/v1/overview` | no | missing | `server/services/nodePoller.js` `pollNode` | Overview | **M10** Not a Go route (404). The data Overlord wants is at `/api/v1/metrics/overview`. |
| `GET /api/v1/backends` | no | missing | `server/services/nodePoller.js` `pollNode` (agent nodes) | Nodes (agents) | **M11** Not a Go route. Agent mode starts no API server at all, so every agent poll fails and agents show as unhealthy. |
| `GET :9090/metrics` | no | missing | `server/services/nodePoller.js` `pollNode` -> Prometheus parser | Overview, Metrics | Outside the REST API and outside the spec (separate metrics listener). The seven metric names Overlord parses all exist in Go. `routing_decisions_total` has an extra `domain` label Overlord does not read. |

`GET /api/v1/nodes`, `/api/v1/gossip` and `/api/v1/routing` (no trailing segment) are not registered. Go
answers 301 to the same path with a trailing slash, which then returns 404.

## Overlord routes (`server/routes/`)

Every route file, with where its data comes from today. Paths are as mounted in `server/index.js`.
"Postgres" means Overlord's own database. "Poller" means Postgres rows written by `nodePoller.js` from
overwatch responses.

| Route file | Paths | Data source today |
|---|---|---|
| `domains.js` | `GET /api/v1/domains`, `GET /:id`, `GET /:id/servers` | Postgres (poller plus local writes) |
| `domains.js` | `POST /api/v1/domains`, `PUT /:id`, `DELETE /:id` | Postgres, then overwatch API (`pushDomain`, `updateDomainOnNodes`, `deleteDomainFromNodes`) |
| `domains.js` | `POST /:id/servers`, `DELETE /:id/servers/:serverId` | Postgres only (not pushed) |
| `servers.js` | `GET /api/v1/servers`, `GET /:id` | Postgres (poller plus local writes) |
| `servers.js` | `POST /api/v1/servers`, `PUT /:id`, `DELETE /:id` | Postgres, then overwatch API (`pushBackend`, `updateServerOnNodes`, `deleteBackendFromNodes`) |
| `servers.js` | `PATCH /:id`, `GET` / `PUT /:id/health-check` | Postgres only (not pushed) |
| `regions.js` | `/api/v1/regions` CRUD | Postgres only (poller-synced; writes not pushed) |
| `nodes.js` | `/api/v1/nodes/overwatch`, `/nodes/agent`, certificates, `POST /nodes` | Postgres `nodes` (poller updates status) |
| `nodes.js` | `GET /api/v1/nodes/:id/version?fresh=true` | Overwatch API (`/api/v1/version`), otherwise Postgres cache |
| `nodes.js` | `POST /api/v1/nodes/agent/:id/heartbeat` | Inbound from agents, into Postgres |
| `polling.js` | `/api/v1/polling/status`, `/start`, `/stop`, `/trigger`, `/cleanup` | In-memory poller state; `/trigger` polls overwatches |
| `health.js` | `/api/v1/health/servers`, `/health/regions`, `/ready`, `/live` | Postgres (poller); `/live` static |
| `overwatch.js` | `/api/v1/overwatch/backends`, `/stats`, override, agents | Postgres only |
| `overwatch.js` | `POST /api/v1/overwatch/validate` | Fabricated: copies `agent_healthy` into `validation_healthy` in Postgres; no node is asked |
| `overrides.js` | `/api/v1/overrides` | Postgres only; never sent to an overwatch |
| `geo.js` | `/api/v1/geo/mappings`, `/geo/test` | Postgres only; never sent to an overwatch |
| `dnssec.js` | `/api/v1/dnssec/status`, `/keys`, `/ds` | Postgres only |
| `dnssec.js` | `POST /api/v1/dnssec/sync` | Fabricated: reports success after a 2 second timer |
| `gossip.js` | `/api/v1/gossip/nodes`, `/config`, `/generate-key` | Postgres only; the key is generated locally |
| `auditLogs.js` | `/api/v1/audit-logs`, `/stats`, `/export`, `/:id` | Postgres (Overlord's own audit log) |
| `metrics.js` | `/api/v1/metrics/history`, `/nodes/:id`, `/regions/:id` | Postgres `metrics_snapshots` (poller) |
| `metrics.js` | `GET /api/v1/metrics/overview` | Postgres (poller), with hard-coded `cache_hit_rate: 0.85`, `dnssec_enabled: true`, `gossip_enabled: true`; memory, CPU and uptime are the Overlord host's |
| `metrics.js` | `GET /api/v1/metrics/routing` | Postgres `routing_decisions`, which nothing writes, so always empty |
| `routing.js` | `GET /api/v1/routing/decisions` | Postgres `routing_decisions` (always empty) |
| `routing.js` | `GET /api/v1/routing/flows` | Fabricated: random sample flows for three hard-coded regions when the table is empty, which is always |
| `routing.js` | `GET /api/v1/routing/algorithms` | Hard-coded list |
| `routing.js` | `POST /api/v1/routing/test` | Fabricated: simulated from Postgres with `Math.random`; no node is asked |
| `config.js` | `/api/v1/config/dns`, `/health-check`, `/logging`, `/system` | Postgres `settings`; `/system` is the Overlord process |
| `preferences.js` | `/api/v1/preferences` | Postgres |
| `auth.js`, `users.js`, `oauth.js`, `settings.js` | `/api/v1/auth`, `/users`, `/oauth`, `/settings` | Postgres (portal's own; no overwatch equivalent) |

Outside `server/routes/`, `server/index.js` serves `/api/push-to-node`, `/api/test-connection` and
`/api/install-opengslb` over SSH (`ssh2`), and `/api/github-releases` from the GitHub API.

## Overlord pages (`src/pages/`)

| Page | Overlord routes it calls | Data source today |
|---|---|---|
| `Overview.jsx` | `/metrics/overview`, `/health/regions`, `/metrics/history`, `/audit-logs` | Postgres (poller), including the hard-coded overview fields above |
| `Metrics.jsx` | `/metrics/overview`, `/metrics/history`, `/nodes/overwatch` | Postgres (poller). The routing chart is a hard-coded split (35/30/20/10/5) because `routingAPI.getStats` does not exist |
| `Nodes.jsx` | `/nodes/overwatch`, `/nodes/agent`, `/nodes`, `/polling/*` | Postgres `nodes` (poller) and poller state |
| `Domains.jsx` | `/domains` CRUD | Postgres; writes go to the overwatch API |
| `Servers.jsx` | `/servers` CRUD, `/servers/:id/health-check` | Postgres; create, update and delete go to the overwatch API |
| `Gossip.jsx` | `/gossip/*` | Postgres only |
| `AuditLogs.jsx` | `/audit-logs`, `/stats`, `/export` | Postgres (Overlord's own log) |
| `RoutingDecisions.jsx` | `/metrics/routing`, `/regions` | Postgres; always empty |
| `Health.jsx` | none | `src/data/mockData.js` (empty). "Validate" is simulated in the browser with a timer and `Math.random` |
| `Overrides.jsx` | none | `src/data/mockData.js` (empty). Adds and deletes change browser state only |
| `GeoMappings.jsx` | none | `src/data/mockData.js` (empty). The IP test is simulated in the browser |
| `DNSSEC.jsx` | none | `src/data/mockData.js` (`enabled: false`, no keys) |
| `Preferences.jsx` | none | Browser localStorage |
| `Login.jsx` | `/auth/*`, `/oauth/*` | Postgres |

## Disagreements between the two repos' own documents

- Overlord's `AI_ASSISTANT_GUIDE.md` documents health-check `interval` and `timeout` as strings (`"30s"`).
  Overlord's client sends numbers, and Go accepts only integer nanoseconds (M8).
- Overlord's `ALIGNMENT_PLAN.md` describes aligning Overlord's own API shapes with OpenGSLB (wrapped lists,
  `routing_policy`). Its write client was not updated to match and still sends `routing_algorithm` (M5).
- The old `docs/api.md` in this repo documents the DNSSEC and `/api/health` endpoints as available. The
  binary does not serve them.
- `config/reference-overwatch.yaml` shows `api.address: ":9090"`, which is also the default metrics address;
  the code default for the API is `127.0.0.1:8080`.
