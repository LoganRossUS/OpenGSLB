# Changelog

All notable changes to OpenGSLB will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security
- **Go toolchain moved to 1.26.11** (`go.mod`, all workflows, Dockerfile builder image). With the previous toolchain `govulncheck` reported 39 reachable vulnerabilities in the Go standard library (net/http, crypto/tls, crypto/x509, net/url, encoding/pem); it now reports none. Building from source needs Go 1.26.11 or later; Go 1.21 or later downloads that toolchain automatically unless `GOTOOLCHAIN=local` is set.
- **`golang.org/x/net`, `golang.org/x/sys` and `golang.org/x/text` updated** to releases without known advisories (v0.63.0, v0.50.0, v0.44.0).
- **`govulncheck` now fails the build.** The CI job ran `govulncheck ./... || true` and could not fail. It moved to `.github/workflows/security.yml`, which also runs weekly.
- **Container image is scanned with Trivy before it is published.** A HIGH or CRITICAL vulnerability stops the push. A finding can be accepted by listing it in a `.trivyignore` file with a reason.
- **Runtime base image pinned** to `alpine:3.24.4` (was `alpine:latest`).
- **`gosec` enabled** in golangci-lint. Findings were fixed or annotated with a stated reason.
- **`SECURITY.md`** added: supported versions and how to report a vulnerability privately.
- **Unbounded metric series from DNS queries**: every distinct query name, including names that do not exist or lie outside the served zones, became a value of the `domain` label of `opengslb_dns_queries_total` and `opengslb_dns_query_duration_seconds` and was never removed, so any DNS client could grow the process's memory and `/metrics` by asking for random names. Only registered domains and zone apexes are label values now; every other name is counted under `domain="other"`.
- **Remote crash through gossip**: a `heartbeat` that registered backends with very large weights for a domain with weighted routing made the weighted router panic on the next DNS query for that domain, which stopped the Overwatch. Any host holding the gossip encryption key could send it. Fixed by the gossip input validation below.
- **Unbounded metric series through gossip**: the sender-chosen message type was used as a Prometheus label, so every new type string added a time series that was never removed.
- **Gossip processing stall**: with the learned latency table at its limit, a single `latency_report` with many new subnets scanned the whole table once per subnet while holding the table lock, on the goroutine that processes all gossip.
- **Audit export**: the requested `format` is no longer copied into the `Content-Disposition` header, and the export is served with `X-Content-Type-Options: nosniff`.

### Added
- **Zone apex records**: SOA and NS are served at each zone apex. An entry of `dns.zones` may now be a mapping with `name`, `ttl`, `soa` (`primary_ns`, `contact`, `serial`, `refresh`, `retry`, `expire`, `negative_ttl`) and `nameservers`; the plain string form keeps working. Unset fields default from the node identity. See ADR-021.
- **EDNS(0)**: responses carry an OPT record when the query had one, UDP responses honor the requester's advertised size and are truncated with the TC bit when they do not fit, and the EDNS Client Subnet option is echoed with a scope prefix when ECS is enabled.
- **`dns.max_udp_size`**: largest UDP response the server sends (default 1232, range 512 to 4096).
- **`opengslb_dns_last_healthy_answers_total{domain,type}`**: answers given from unhealthy servers because `dns.return_last_healthy` is true.
- **Per-server health checks through the API**: `health_check` on a server (type, path, host, interval, timeout, thresholds) is stored and used by the overwatch validator. `GET`, `PUT` and `PATCH /api/v1/servers/{id}/health-check` are real.
- **DNS conformance tests** (`test/conformance`, `make test-conformance`): start the real DNS server in-process and assert response code, AA flag, answer and authority sections for a table of queries over UDP and TCP, with and without EDNS. Own CI job.
- **Fuzz tests** (Go native fuzzing) for the code that parses input from the network: `FuzzHandlerServeDNS` (`pkg/dns`), `FuzzReceiverNotifyMsg` and `FuzzReceiverNodeEvents` (`pkg/gossip`), `FuzzGossipHandleMessage` and `FuzzGossipPayloadRoundTrip` (`pkg/overwatch`). The seed corpora run in `go test ./...`; CI fuzzes each target for 60 seconds.
- **CodeQL analysis** for Go (`.github/workflows/codeql.yml`) and **Dependabot** (`.github/dependabot.yml`) for Go modules, GitHub Actions and Docker base images. Dependabot pull requests target `develop`.
- **Container image data directory**: the image creates `/var/lib/opengslb` for its runtime user (UID 1000) and declares it a volume.
- **`api/openapi.yaml`**: OpenAPI 3.1 description of every REST route registered in `pkg/api/server.go`, written from observed handler behaviour. Operations that are not fully real carry an `x-status` extension (`stub`, `not-implemented`, `unavailable`, `unwired`, `broken`). This file is now the single source of truth for the API.
- **API contract test** (`cmd/opengslb/api_contract_test.go`, `test/apicontract`): starts a real overwatch in-process, calls every operation in the spec and validates the real responses against it. Runs in `go test ./...` and as its own `API Contract` CI job.
- **`docs/api-alignment.md`**: per-endpoint status of the overwatch API and of the Overlord portal's use of it, with the mismatches found.
- **`tools/apidocs`**: generator that writes `docs/api.md` from `api/openapi.yaml` (`make api-docs` or `go generate ./tools/apidocs`). Its test fails in `go test ./...` when the committed `docs/api.md` is out of date with the spec.
- **`docs/api-guide.md`**: hand-maintained guide to API access control, configuration, error format and usage examples, moved out of `docs/api.md`. Every statement and example was checked against the code, the spec and a running overwatch.

### Changed
- **DNS response codes** (see ADR-021 and the table in `docs/configuration.md`):
  - A query for a name that exists but has no server of the queried address family (AAAA for an IPv4-only name, and the reverse) answers NOERROR with an empty answer and the zone SOA in the authority section. Was SERVFAIL.
  - A query for a type the server has no data for (TXT, MX, CNAME, HTTPS, ANY, DS and so on) at a name that exists answers NOERROR with an empty answer and the SOA. Was NOTIMP.
  - NXDOMAIN and empty NOERROR responses carry the zone SOA in the authority section, so resolvers can cache them (RFC 2308).
  - A query for a name outside every served zone answers REFUSED with the AA bit clear. Was an authoritative NXDOMAIN.
  - A name that has no records itself but has a domain beneath it answers NOERROR with an empty answer instead of NXDOMAIN (RFC 8020).
  - AXFR and IXFR are refused. Queries with an opcode other than QUERY get NOTIMP, and classes other than IN get REFUSED.
- **Query names are matched case-insensitively.** `APP.GSLB.Example.COM` now resolves a configured `app.gslb.example.com`; the response echoes the querier's spelling.
- **DNS metrics labels**: `status` holds the DNS response code (`NOERROR`, `NXDOMAIN`, `SERVFAIL`, `REFUSED`, ...). `domain` holds the canonical lower-case name of a registered domain or zone apex, or `other`. Documentation that showed `status="success"` was wrong and is corrected.
- **Query-path logging**: registry misses and excluded backends are logged at DEBUG, not INFO.
- **Backend health lookups** on the query path use an index by address and port instead of copying and scanning every backend for each candidate server. An address and port registered under more than one service is healthy only when every backend registered at it is healthy.
- **REST API: servers and domain backends are one object** (see ADR-022 and `docs/api-alignment.md`). `POST /api/v1/domains/{name}/backends` and `POST /api/v1/servers` create the same server (source `api`), listed and writable through both paths. A domain without backends lists an empty array instead of answering 404.
- **REST API: `routing_algorithm`** is the canonical field of a domain. `routing_policy` is still accepted and returned as a deprecated alias.
- **REST API: health-check `interval` and `timeout` are duration strings** such as `"30s"`. **Breaking**: a bare number is refused with 400 (it was decoded as nanoseconds). Unknown health-check fields and out-of-range values are refused with 400.
- **REST API errors**: 404 for an unknown domain or server, 409 for a duplicate and for objects owned by the configuration file or an agent, 400 for an invalid value, and 503 with the directory and reason for a write on a node without a persistent store (was 500 `not implemented`).
- **`GET /api/v1/config/dns`** reports `zones` (each zone of `dns.zones` with its apex SOA and NS settings as served, defaults included), `max_udp_size` and `return_last_healthy`, read from the running DNS server. Zones stay a configuration-file setting; the endpoint is read-only for them.
- **`GET /api/v1/config`** reports `node_id`, `region`, `mode`, `start_time`, `data_dir` and the version of the running binary (was an empty identity and the constant `1.0.0`).
- **Startup without a usable data directory**: a store that cannot be created is logged at WARN (`RUNNING WITHOUT PERSISTENCE`) and the node serves the configuration file read-only. **A store that exists and cannot be opened now stops the start.** A store file locked by another process is reported after five seconds instead of waiting forever.
- **Configuration reload** (see ADR-023): a reload reports, at WARN, the changed settings it cannot apply without a restart instead of logging success. `dns.zones`, `dns.max_udp_size` and `dns.return_last_healthy` are applied on reload. The geolocation region mapping is refreshed on reload.
- **Agent reload** applies `agent.backends`: added backends are checked at once, removed ones are deregistered from the overwatch nodes. Before, an agent reload logged success and changed nothing.
- **Gossip input validation**: an Overwatch ignores a backend in a `heartbeat` or `register` message whose port is outside 1-65535 or whose weight is outside 0-1000000, and logs it at WARN. Unknown message types are counted under `type="unknown"` in `opengslb_gossip_messages_received_total`. See ADR-020.
- **Learned latency table**: when the table is full, one eviction pass removes the oldest 1% of the limit instead of a single subnet.
- **DNSSEC is described as in development, not active** (see ADR-024). With `overwatch.dnssec.enabled: true` the overwatch logs a warning at startup: responses are not signed in this release. `overwatch.dnssec.security_acknowledgment` is no longer required to leave DNSSEC off; it is still accepted and ignored.
- **golangci-lint v2.** `.golangci.yml` is in the v2 format. CI pins golangci-lint v2.15.0 instead of `latest`.
- **Workflows read the Go version from `go.mod`**; `softprops/action-gh-release` updated from v1 to v3.
- **`docs/api.md`** is now generated from `api/openapi.yaml` and must not be edited by hand. It lists all 107 operations with their status (real, stub, not implemented, unavailable, unwired, broken) and the 128 schemas, replacing the hand-written endpoint prose and its example responses. Anchors inside the file have changed.
- **API documentation corrections** (documentation only, no change in behaviour): the DNSSEC endpoints and `/api/health` are no longer presented as available (the binary answers 404), and the `api.enable_cors` / `api.cors` settings are no longer documented because the configuration has no such keys and CORS is never enabled.

### Fixed
- **Reload dropped API-created state**: after SIGHUP, domains and servers created through the REST API answered NXDOMAIN until the next restart, and agent-registered servers left DNS until their next heartbeat. Startup and reload now build DNS state with the same function from the configuration file, the store and the backend registry.
- **Static servers removed from the configuration file** kept being served after a restart, because they were loaded back from the store.
- **API-created servers were removed after `remove_after`** (5 minutes by default), because the stale check treated them as agent backends.
- **`DELETE /api/v1/domains/{name}/backends/{id}`** was unreachable and always answered 404, so a deleted server stayed in DNS.
- **`dns.return_last_healthy`** was parsed and never read. When true and every server of the queried family is unhealthy, the answer is now chosen from all configured servers of that family.
- **Data race between DNS queries and server registration**: registering, deregistering or reweighting a server changed a domain entry that queries were reading. Entries are now replaced, not modified in place.
- **Routers created by a reload or for API-created domains had no learned latency provider**, so `learned_latency` silently fell back to round-robin for them.
- **Agent shutdown during startup**: a stop that arrived while the agent was still starting raced with the start, and could leave the heartbeat sender running after the agent reported itself stopped. Start and stop are now serialized. A start that fails no longer leaves the agent marked as running.
- **TTLs above 2^32 wrapped around** when set through the API (4294967297 became 1). TTLs are limited to 0 to 2^31 - 1.
- **`agent_auth` gossip messages**: the certificate was not base64-decoded on receipt. This path is not used by the shipped binary today.
- **`pkg/health` TCP checker tests no longer depend on the host network.** Test-only change.
- **Data race at overwatch start**: `Registry.loadFromStore` wrote the backend map without holding the registry lock while status-change callbacks read it from other goroutines. Found by the contract test under `-race`. No change in behaviour.

---

## [1.1.10] - 2026-10-25

### Added

#### Per-backend health gauge
- **`opengslb_backend_healthy{service,region,address}`**: 1 when a backend is in the DNS answer pool, 0 otherwise. Enables region-level alerting such as `min by (service, region) (opengslb_backend_healthy) == 0`. The series is removed when the backend is deregistered.

#### Validation thresholds
- **`overwatch.validation.failure_threshold`** and **`overwatch.validation.success_threshold`**: consecutive validation results required before a backend leaves or re-enters DNS. Both default to `1`, which is the existing behaviour (the latest probe is the verdict, so failover and failback complete within one `check_interval` plus TTL). Negative values are rejected at config load.
- `GET /api/v1/overwatch/backends` now reports `authority` (which layer of the ADR-015 hierarchy decided `effective_status`) and `validation_consecutive_passes` / `validation_consecutive_fails`.

### Changed
- **Standalone overwatch logging**: `validation disagrees with agent` is no longer written for backends defined in `regions` or created through the API; they have no agent to disagree with. For agent-registered backends it is written once when the validation verdict changes and at debug level while the disagreement persists. Disagreement counts in `/api/v1/overwatch/stats` follow the same rule.
- **`opengslb_overwatch_backends_by_authority`**: the `authority` label now reports the layer that produced the status (`override`, `draining`, `validation`, `stale`, `agent`, `static`). Previously every backend that was not overridden or stale was counted as `agent`, including config-defined backends in standalone deployments. All six series are always published.
- **`opengslb_overwatch_agents_registered`** no longer counts the empty agent ID of static and API backends as an agent.

### Documentation
- Configuration, metrics and troubleshooting guides describe how the DNS answer is decided in standalone overwatch mode, why `/api/v1/health/regions` can differ from it, and how to diagnose a slow return to the primary.

---

## [1.1.2] - 2025-12-18

### Added

#### Dynamic DNS Registration for API-Created Domains
- **Immediate DNS resolution**: Domains created via `POST /api/v1/domains` are now automatically registered with the DNS server
- **Dynamic backend registration**: Backends added via `POST /api/v1/domains/{name}/backends` immediately appear in DNS responses
- **Automatic cleanup**: Deleting domains or backends via API automatically deregisters them from the DNS server
- **No restart required**: API-created domains work immediately without SIGHUP or restart

### Changed
- Domain provider now integrates with DNS registry for real-time updates
- Added `RegisterDomainDynamic` method to DNS registry for API integration

---

## [1.1.1] - 2025-12-18

### Added

#### Full Domain CRUD API
- **POST /api/v1/domains**: Create new domains dynamically without config changes
- **PUT /api/v1/domains/{name}**: Update domain settings (TTL, routing policy, etc.)
- **DELETE /api/v1/domains/{name}**: Remove API-created domains
- **POST /api/v1/domains/{name}/backends**: Add backends to domains
- **DELETE /api/v1/domains/{name}/backends/{id}**: Remove backends from domains

#### Version Endpoint
- **GET /api/v1/version**: Returns build version, Go version, git commit, and build date
- Endpoint is not ACL-protected for easy health monitoring integration

#### Persistent Storage
- API-created domains and backends persist across restarts using bbolt storage
- Store prefixes: `domains/` for domains, `domain-backends/` for backends

---

## [1.1.0] - 2025-12-18

### Added

#### Unified Server Architecture
- **Unified backend registry**: Static, agent-registered, and API-registered servers all use the same validation and health tracking system
- **Dynamic DNS registration**: API-created and agent-registered servers automatically appear in DNS responses
- **Persistent server storage**: API-created servers persist across restarts using bbolt storage
- **Unified health authority**: Manual overrides > Predictive > Validation > Staleness > Agent claim (applies to all server types)
- **CRUD API for servers**: Full create, read, update, delete operations for server management via REST API

#### Server Management API
- **POST /api/v1/servers**: Create new servers dynamically without config changes
- **GET /api/v1/servers**: List all servers (static, agent, and API-registered)
- **GET /api/v1/servers/:id**: Get details for a specific server
- **PATCH /api/v1/servers/:id**: Update server weight and region
- **DELETE /api/v1/servers/:id**: Remove dynamically-created servers

#### CLI Server Management
- **gslbctl servers list**: Display all registered servers with their source and status
- **gslbctl servers create**: Add new servers via API
- **gslbctl servers update**: Modify server weight/region
- **gslbctl servers delete**: Remove API-created servers

### Changed

#### Breaking Changes
- **REQUIRED `service` field**: All servers in configuration must now specify which domain/service they belong to
  ```yaml
  servers:
    - address: "10.0.1.10"
      port: 80
      weight: 100
      service: "app.example.com"  # REQUIRED in v1.1.0
  ```
- **Migration required**: Existing configurations must add `service` field to all server definitions
- **Validation enforced**: OpenGSLB will refuse to start if any server is missing the `service` field

### Fixed
- **DNS registration for dynamic servers**: API and agent-registered servers now correctly appear in DNS responses
- **Persistence loading**: Servers loaded from storage on restart are now properly registered in DNS
- **Status change callbacks**: Backend registry callbacks now trigger DNS registration/deregistration

### Migration Guide

To upgrade from v0.6.0 to v1.1.0:

1. Add `service` field to all server definitions in your configuration:
   ```yaml
   # Before (v0.6.0)
   regions:
     - name: us-east
       servers:
         - address: "10.0.1.10"
           port: 80
           weight: 100

   domains:
     - name: app.example.com
       regions: [us-east]

   # After (v1.1.0)
   regions:
     - name: us-east
       servers:
         - address: "10.0.1.10"
           port: 80
           weight: 100
           service: "app.example.com"  # Add service field

   domains:
     - name: app.example.com
       regions: [us-east]
   ```

2. Update your configuration files before upgrading
3. Test configuration with `opengslb --config config.yaml --validate`
4. Restart OpenGSLB with updated configuration

## [0.6.0] - 2025-12-11

### Added

#### Geolocation Routing (Sprint 5)
- **GeoIP-based routing**: Route traffic based on client geographic location using MaxMind GeoIP2/GeoLite2 databases
- **Custom CIDR mappings**: Define custom IP ranges to region mappings that override GeoIP lookups
- **Continent and country resolution**: Full resolution of client location to country, continent, and mapped region
- **Configurable default region**: Fallback region when geolocation fails or no mapping exists
- **EDNS Client Subnet (ECS) support**: Use ECS information from recursive resolvers for more accurate client location

#### Latency-Based Routing (Sprint 5)
- **Active latency measurement**: Continuous latency validation during health checks using TCP connection time
- **Exponential moving average (EMA)**: Smoothed latency values to prevent routing flapping
- **Configurable thresholds**: Maximum latency threshold to exclude slow backends
- **Minimum samples requirement**: Require minimum number of latency samples before using for routing
- **Automatic fallback**: Falls back to round-robin when insufficient latency data available

#### Agent-Overwatch Architecture (Sprint 5)
- **Distributed health checking**: Deploy agents in edge locations for local health monitoring
- **Gossip-based state sync**: Serf-based gossip protocol for real-time health state propagation
- **Encrypted communication**: AES-256-GCM encryption for all gossip traffic
- **Agent lifecycle management**: Automatic detection of agent failures and stale data
- **Overwatch coordination**: Centralized DNS server aggregates health data from all agents

#### Enhanced Observability Metrics (Sprint 6)
- **Geolocation routing metrics**: Track routing decisions by country, continent, and region
- **Geo fallback metrics**: Monitor fallback reasons (no_client_ip, no_resolver, lookup_failed, etc.)
- **Custom CIDR hit metrics**: Track custom mapping matches
- **Latency routing metrics**: Record selected server latency and rejection reasons
- **Per-agent connectivity metrics**: Monitor individual agent connection status and backends registered
- **Agent heartbeat age metrics**: Track freshness of agent health data
- **Override metrics with service labels**: Track active overrides and changes per service
- **Enhanced DNSSEC metrics**: Key age tracking per zone and key tag
- **Gossip decryption failure counter**: Monitor encrypted gossip communication issues

#### Multi-File Configuration (Sprint 5)
- **Config includes support**: Split configuration across multiple files using `includes` directive
- **Glob pattern matching**: Use wildcards to include multiple config files (e.g., `config.d/*.yaml`)
- **Environment variable expansion**: Use `${VAR}` syntax for environment-based configuration
- **Layered configuration**: Override settings by loading configs in sequence

#### CLI Tools (Sprint 5)
- **gslbctl command-line tool**: Manage OpenGSLB from the command line
- **Health status commands**: Query current health status of backends
- **Override management**: Set and clear manual routing overrides
- **Configuration validation**: Validate configuration files before deployment
- **Agent status commands**: Monitor connected agents and their state

### Changed
- Improved health check latency tracking with sub-millisecond precision
- Enhanced logging with structured fields for routing decisions
- Updated metrics documentation with Sprint 6 examples and alerting guides

### Fixed
- Race condition in concurrent latency provider access
- Memory leak in geo resolver cache under high load

## [0.5.0] - 2025-11-01

### Added
- DNSSEC support with automatic key management
- Zone signing with NSEC/NSEC3 authenticated denial of existence
- Key rotation support with configurable intervals
- TLS support for API endpoints

## [0.4.0] - 2025-10-01

### Added
- REST API for health status and management
- Hot reload support via SIGHUP signal
- Kubernetes deployment manifests and Helm chart
- Docker Compose examples for quick deployment

## [0.3.0] - 2025-09-01

### Added
- Prometheus metrics for DNS queries, health checks, and routing
- Structured JSON logging with configurable log levels
- TCP health check support for non-HTTP services

## [0.2.0] - 2025-08-01

### Added
- Weighted routing algorithm with configurable server weights (0-1000)
- Failover (active/standby) routing with automatic primary recovery
- AAAA record support for IPv6 addresses
- Per-domain TTL configuration

## [0.1.0] - 2025-07-01

### Added
- Initial release
- DNS server with A record support
- Round-robin routing algorithm
- HTTP/HTTPS health checks with configurable paths and status codes
- Multi-region server configuration
- Docker container support
