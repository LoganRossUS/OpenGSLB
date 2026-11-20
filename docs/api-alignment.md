# API alignment: OpenGSLB overwatch and Overlord

Status of every REST endpoint on both sides, as of `develop` (OpenGSLB v1.1.10) and Overlord `main`
(`bc68e5b`), recorded on 2026-11-20 for Phase 0 of the enterprise-readiness work.

- The contract is [`api/openapi.yaml`](../api/openapi.yaml). It was written from handler behavior, not from
  the old prose in `docs/api.md`.
- "Go status" was observed by starting a real overwatch in-process and calling each endpoint
  (`cmd/opengslb/api_contract_test.go`). It is not inferred from the code alone.
- Overlord columns come from reading its source. The rows where Overlord calls the overwatch are now pinned by
  Overlord's contract suite (`test/contract`), which runs its real client and poller code against an overwatch
  container. Mismatch tests there are named by M-number and fail when the mismatch is fixed.
- Phase 1 (1E task 3) fixed the Go side of M2 to M8, M15 and M16. Their rows below say what the contract is
  now. The Overlord columns still describe Overlord `main` (`bc68e5b`); its side of each of these is pending in
  the paired change, and until then its contract tests for them fail against this overwatch, as designed.
  The Go status of every endpoint these fixes touched is updated; `cmd/opengslb/api_alignment_test.go` pins
  each fix by M-number.

## Summary

| | Count |
|---|---|
| Operations in the spec | 107 (71 paths) |
| Real | 63 |
| Stub (placeholder, always-empty or no-op) | 17 |
| Stub that always fails (`not-implemented`) | 16 |
| Real handler that always answers 503 (`unavailable`, agent certificates) | 5 |
| Handler never attached by the binary (`unwired`, DNSSEC and `/api/health`) | 6 |
| Broken | 0 (was 1: `DELETE /domains/{name}/backends/{id}`, fixed in Phase 1) |
| Spec operations Overlord calls | 17 (3 of them from dead code) |
| Paths Overlord calls that Go does not serve | 4, plus the Prometheus listener |

Overlord has 14 pages. None talks to an overwatch directly. Two (Domains, Servers) write through to
overwatches; six read Overlord's Postgres, which the node poller fills; four (Health, Overrides, GeoMappings,
DNSSEC) render only `src/data/mockData.js`, which today exports empty placeholders, and simulate their
actions in the browser.

## Mismatches

Numbered so Phase 1E task 3 can refer to them. "Side" is where the fix belongs. Rows marked **Go side fixed**
describe the contract as it is now; the "Effect" column of those rows is what remains until Overlord follows.

| # | Mismatch | Effect today | Side |
|---|---|---|---|
| M1 | Overlord polls `GET /api/v1/health` as a health document. Go returns the list of health endpoints. | Node "healthy" only means the port answered. `version` and `uptime` come back undefined. | Overlord (use `/api/v1/ready` and `/api/v1/version`) |
| M2 | **Go side fixed.** `GET /api/v1/config` (and `/config/system`) returns `config.node_id` and `config.region` from `overwatch.identity`, `config.version` and `config.build_info.version` equal to `GET /api/v1/version`, `config.mode`, `config.start_time` and `config.data_dir`. Was: empty `region` / `node_id` and the constant version `1.0.0`. | None on the node. Overlord's poller already reads `config.region` and `config.version`, so it now learns both; its contract test for M2 pins the old values and fails. | Go: done. Overlord: update the test, pending in the paired change |
| M3 | **Decided, Go side fixed.** A server carries the name of its domain in `service` (the name the configuration file and the server ID use). `name` and `metadata.service` stay as deprecated aliases with the same value. The objects under `/servers` and under `/domains/{name}/backends` are the same `BackendServer`. There is no `domain` or `domain_name` field. | Overlord still reads `domain` / `domain_name`, so servers synced from `GET /servers` are not linked to their domain until it reads `service`. | Go: done. Overlord: read `service`, pending in the paired change |
| M4 | **Go side fixed.** `GET /api/v1/domains/{name}/backends` returns 200 with `"backends": []` for a domain without backends (404 only for an unknown domain). Every backend carries `source` (`static`, `agent`, `api`) and `health_check` (the check in effect: `enabled`, `type`, `path`, `host`, `interval`, `timeout`, thresholds). | Overlord still labels every synced backend `static` and guesses the check type from the port until it reads the two fields. | Go: done. Overlord: read `source` and `health_check`, pending in the paired change |
| M5 | **Decided, Go side fixed.** The canonical field is `routing_algorithm`, the key and the values of the configuration file. `routing_policy` is accepted on input as a deprecated alias (two different values: 400) and is still returned, always equal to `routing_algorithm`. An unknown algorithm is 400. `regions` is not a field of an API domain (its regions are those of its servers) and is ignored on input. | The algorithm Overlord pushes now reaches the node. Overlord's poller reads `routing_policy` back, which still works; it should read `routing_algorithm` and stop sending `regions`. | Go: done. Overlord: pending in the paired change |
| M6 | **Go side fixed.** The `health_check` of `POST /api/v1/domains/{name}/backends`, `POST /api/v1/servers`, `PUT` / `PATCH /api/v1/servers/{id}` and `PUT` / `PATCH /api/v1/servers/{id}/health-check` is stored and applied: the overwatch probes that server with it. A field the overwatch cannot apply is rejected with 400, not ignored. | Overlord's push is rejected with 400 until it sends durations as strings (M8) and drops fields that are not in the contract. | Go: done. Overlord: pending in the paired change |
| M7 | **Go side fixed.** `DELETE /api/v1/domains/{name}/backends/{id}` is reachable: 204, and the address leaves DNS answers with the call (`id` is the server ID, `{domain}:{address}:{port}`). 404 for an unknown backend, 409 for a backend of the configuration file. | None once Overlord's contract test for M7, which pins the 404, is updated. | Go: done. Overlord: update the test, pending in the paired change |
| M8 | **Decided, Go side fixed.** `interval` and `timeout` are duration strings with a unit (`"30s"`, `"500ms"`), as in the configuration file and in Overlord's own guide, in requests and in responses. A bare number is rejected with 400 and a message that shows the string form. Out of range (`interval` 1s to 1h, `timeout` 100ms to 1m) and a timeout longer than the interval are 400. | Overlord still sends numbers of seconds, so its backend and server pushes that carry a health check are rejected with 400 until it sends strings. | Go: done. Overlord: send strings, pending in the paired change |
| M9 | `pushServer` (dead code) sends `service`. Go now reads `service` as the domain (M3), so that part is resolved on the Go side; the call still fails with 400 because of its numeric health-check durations (M8). Was: Go required `name` or `metadata.service` and answered 500. | None today. | Overlord (durations, with M8) |
| M10 | Overlord polls `/api/v1/status`, `/api/v1/identity` and `/api/v1/overview`, which do not exist. | Three 404s per node per poll, caught and logged. | Overlord (remove; `/api/v1/metrics/overview` has the data) |
| M11 | Overlord polls agent nodes over HTTP (`/api/v1/health`, `/api/v1/backends`). Agent mode starts no API server. | Every agent is marked unhealthy. | Overlord (agents are visible through the overwatch) |
| M12 | Overlord's client assumes plain HTTP on port 8080 with no credentials, and pushes to nodes it has marked unhealthy. Go's default is `127.0.0.1:8080` behind a loopback-only allowlist. | Works only when the operator has opened `api.address` and `api.allowed_networks`. Phase 3A changes this. | Both (Phase 3A) |
| M13 | Push failures are not errors in Overlord: the Postgres write commits and the route returns 2xx with `_pushResults.failed[]`. | Overlord and the nodes drift silently (M5 and M7 make this the normal case). | Overlord (Phase 4 drift work) |
| M15 | **Go side fixed.** One model: a backend created with `POST /api/v1/domains/{name}/backends` is a server (`source: api`). It is listed by `GET /api/v1/servers`, and `GET`, `PUT`, `PATCH` and `DELETE /api/v1/servers/{domain}:{address}:{port}` work on it. A server created with `POST /api/v1/servers` is a backend of the domain in its `service`. Backends stored by 1.1.10 are migrated at startup. | Overlord's server update reaches the node, provided the body is valid (M8). | Go and spec: done. Overlord: update the test, pending in the paired change |
| M16 | **Go side fixed.** The image creates `/var/lib/opengslb` for its runtime user (UID 1000) and declares it a volume, so the default `overwatch.data_dir` works. An overwatch that still cannot create its store starts read-only, logs `RUNNING WITHOUT PERSISTENCE` at WARN and answers every create, update and delete of domains, servers and regions with 503 and a message that names the directory (was: 500 `not implemented`). An existing store that cannot be opened stops the start. | Overlord's contract container no longer needs its `data_dir` override under `/tmp`; removing it is optional. | Go: done. Overlord: optional cleanup in the paired change |
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
| `GET /api/v1/domains` | yes | real | `server/services/nodePoller.js` `pollNode` -> `syncDomains` | Domains | None for `name`, `ttl`, `enabled`, `routing_policy` (deprecated alias of `routing_algorithm`, still returned). |
| `POST /api/v1/domains` | yes | real | `server/services/openGslbClient.js` `pushDomain` (from `routes/domains.js` POST) | Domains | **M5** Go side fixed: `routing_algorithm` is read and applied. `regions` is ignored. A duplicate returns 409. |
| `GET /api/v1/domains/{name}` | yes | real | not called | none |  |
| `PUT /api/v1/domains/{name}` | yes | real | `server/services/openGslbClient.js` `updateDomainOnNodes` (from `routes/domains.js` PUT) | Domains | **M5** Go side fixed: an algorithm change reaches the node. A domain of the configuration file returns 409. |
| `PATCH /api/v1/domains/{name}` | yes | real | not called | none |  |
| `DELETE /api/v1/domains/{name}` | yes | real | `server/services/openGslbClient.js` `deleteDomainFromNodes` (from `routes/domains.js` DELETE) | Domains | None for API-created domains; the servers created for the domain through the API are deleted with it. Config-file domains return 409. |
| `GET /api/v1/domains/{name}/backends` | yes | real | `server/services/nodePoller.js` `pollNode` -> `syncServerFromBackend` | Servers, Domains | **M4** Go side fixed: an empty list for a domain without backends, and `source` and `health_check` on every backend. Overlord still sets `source='static'` on every row and guesses the health-check type from the port (pending). Domain name is not URL-encoded. |
| `POST /api/v1/domains/{name}/backends` | yes | real | `server/services/openGslbClient.js` `pushBackend` (from `routes/servers.js` POST) | Servers | **M6**, **M8** Go side fixed: the `health_check` is applied. Overlord sends `interval` / `timeout` as numbers, which is now rejected with 400 (pending). An unknown domain returns 404, a duplicate 409. |
| `DELETE /api/v1/domains/{name}/backends/{id}` | yes | real | `server/services/openGslbClient.js` `deleteBackendFromNodes` (from `routes/servers.js` DELETE) | Servers | **M7** Go side fixed: 204, and the server leaves DNS answers with the call. |
| `GET /api/v1/servers` | yes | real | `server/services/nodePoller.js` `pollNode` -> `syncServers` | Servers | **M3** Decided: the owning domain is in `service` (`name` and `metadata.service` are deprecated aliases). Overlord reads `domain` / `domain_name`, so the domain link is not set from this call until it reads `service` (pending). |
| `POST /api/v1/servers` | yes | real | `server/services/openGslbClient.js` `pushServer` (never called) | none | **M9** Dead code. With the M3 decision the `service` it sends is read as the domain (a missing one is 400, an unknown domain 404). The call still fails, now with 400, because it sends the health-check durations as numbers (M8). |
| `GET /api/v1/servers/{id}` | yes | real | not called | none |  |
| `PUT /api/v1/servers/{id}` | yes | real | `server/services/openGslbClient.js` `updateServerOnNodes` (from `routes/servers.js` PUT) | Servers | **M15** Go side fixed: servers Overlord created with `POST /domains/{name}/backends` exist under `/servers` and are updated. **M8** Overlord sends `health_check.interval` / `timeout` as seconds (30, 5), which is now rejected with 400 until it sends `"30s"`, `"5s"` (pending). `PATCH /servers/:id` and `PUT /servers/:id/health-check` in Overlord are not pushed at all. |
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
| `GET /api/v1/config` | yes | real | `server/services/nodePoller.js` `pollNode` | Nodes (region, version) | **M2** Go side fixed. Scanned for `config.region`, `config.identity.region`, `config.version`; Go now returns the node's region and the version of the running binary. |
| `GET /api/v1/config/system` | yes | real | not called | none |  |
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
  Overlord's client sends numbers. Resolved in the spec for the guide: Go accepts and returns strings and
  rejects numbers (M8). Overlord's client is pending.
- Overlord's `ALIGNMENT_PLAN.md` describes aligning Overlord's own API shapes with OpenGSLB (wrapped lists,
  `routing_policy`). Its write client still sends `routing_algorithm`. Resolved in the spec for the write
  client and the configuration file: `routing_algorithm` is canonical and `routing_policy` a deprecated alias
  (M5). Overlord's plan and its read side are pending.
- The old `docs/api.md` in this repo documents the DNSSEC and `/api/health` endpoints as available. The
  binary does not serve them.
- `config/reference-overwatch.yaml` shows `api.address: ":9090"`, which is also the default metrics address;
  the code default for the API is `127.0.0.1:8080`.
