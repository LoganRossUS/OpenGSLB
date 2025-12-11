# Changelog

All notable changes to OpenGSLB will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
