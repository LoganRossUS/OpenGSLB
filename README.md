# OpenGSLB

[![CI](https://github.com/loganrossus/OpenGSLB/actions/workflows/ci.yml/badge.svg)](https://github.com/loganrossus/OpenGSLB/actions/workflows/ci.yml) [![Docker Build](https://github.com/loganrossus/opengslb/actions/workflows/docker-build.yml/badge.svg)](https://github.com/loganrossus/opengslb/actions/workflows/docker-build.yml) [![Release](https://img.shields.io/github/v/release/loganrossus/OpenGSLB?label=stable)](https://github.com/loganrossus/OpenGSLB/releases)

**v1.1.9 Stable Release** - Production-ready DNS-first global load balancing.

[Website](https://opengslb.org) | [Documentation](https://docs.opengslb.org) | [Discord](https://discord.gg/H4s8RG6Az) | [Reddit](https://www.reddit.com/r/OpenGSLB)

## Overview

OpenGSLB is an open-source, self-hosted Global Server Load Balancing (GSLB) system designed for intelligent traffic distribution across multiple data centers and cloud regions. Built for organizations that require complete control over their infrastructure, OpenGSLB provides enterprise-grade global load balancing without vendor lock-in or dependency on third-party services.

## Licensing

OpenGSLB is **dual-licensed**:

- **AGPLv3** – Free forever for open-source projects, internal use, and anyone willing to share modifications  
- **Commercial License** – For proprietary products, appliances, SaaS, or if you prefer not to comply with AGPL obligations → licensing@opengslb.org

See [LICENSE](LICENSE) for full terms.

## Features

### DNS Server
- **A and AAAA Records**: Full IPv4 and IPv6 support with automatic address family filtering
- **UDP and TCP**: Handles both transport protocols
- **Configurable TTL**: Per-domain TTL settings with global default fallback
- **Authoritative Responses**: Returns proper NXDOMAIN, SERVFAIL, and NOERROR responses
- **DNSSEC**: Automatic key management with zone signing and DS record generation

### Routing Algorithms
- **Round-Robin**: Even distribution across healthy servers with per-domain rotation
- **Weighted**: Proportional traffic distribution based on server capacity (weight 0-1000)
- **Failover (Active/Standby)**: Predictable primary → secondary → tertiary failover with automatic return-to-primary
- **Geolocation-Based**: Route clients to nearest region using MaxMind GeoIP2 with custom CIDR overrides
- **Latency-Based**: Dynamically route to lowest-latency backends with EMA smoothing to prevent flapping
- **Learned Latency**: Route based on real client-to-backend TCP RTT data collected by agents (ADR-017)
- **EDNS Client Subnet (ECS)**: Extract client location from recursive resolvers for accurate geo-routing

### Health Checking
- **HTTP/HTTPS**: Configurable endpoint path, expected status codes, and TLS support
- **TCP**: Connection-based health checks for non-HTTP services (databases, custom protocols)
- **Configurable Thresholds**: Separate failure and success thresholds to prevent flapping
- **Per-Region Configuration**: Different health check settings for different server tiers
- **Agent-Based Monitoring**: Distributed agents report health from edge locations with gossip-based sync
- **External Validation**: Overwatch nodes independently verify agent health claims

### Operations
- **Hot Reload**: Update configuration without restart via SIGHUP signal
- **Structured Logging**: JSON or text format with configurable log levels
- **Prometheus Metrics**: DNS queries, health check results, routing decisions, and more
- **Health Status API**: JSON endpoint for current server health status
- **Server Management API**: CRUD operations for dynamic server management
- **CLI Management Tool**: Full-featured CLI for status, overrides, and management

### Deployment
- **Single Binary**: No runtime dependencies
- **Docker Support**: Official container images on GitHub Container Registry
- **Minimal Resources**: Lightweight footprint suitable for edge deployment

## Quick Start

### From Source

```bash
# Clone and build
git clone https://github.com/loganrossus/OpenGSLB.git
cd OpenGSLB
go build -o opengslb ./cmd/opengslb

# Run with example config
./opengslb --config config/example.yaml
```

### Docker

```bash
# Pull the latest image
docker pull ghcr.io/loganrossus/opengslb:latest

# Run with your configuration
docker run -d \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 9090:9090 \
  -v $(pwd)/config.yaml:/etc/opengslb/config.yaml \
  ghcr.io/loganrossus/opengslb:latest
```

### Test It

```bash
# Query for IPv4
dig @localhost -p 53 app.example.com A +short

# Query for IPv6
dig @localhost -p 53 app.example.com AAAA +short

# Check metrics
curl http://localhost:9090/metrics
```

## Configuration Example

```yaml
dns:
  listen_address: ":53"
  default_ttl: 60

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9090"

regions:
  - name: us-east
    servers:
      - address: "10.0.1.10"
        port: 80
        weight: 100
        service: "app.example.com"  # Required in v1.1.0
      - address: "10.0.1.11"
        port: 80
        weight: 100
        service: "app.example.com"
      - address: "2001:db8::1"    # IPv6 support
        port: 80
        weight: 100
        service: "app.example.com"
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2

  - name: us-west
    servers:
      - address: "10.0.2.10"
        port: 80
        weight: 100
        service: "app.example.com"
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health

  - name: database
    servers:
      - address: "10.0.3.10"
        port: 5432
        service: "db.example.com"
    health_check:
      type: tcp              # TCP health check for non-HTTP
      interval: 15s
      timeout: 3s

domains:
  - name: app.example.com
    routing_algorithm: weighted
    regions: [us-east, us-west]
    ttl: 30

  - name: api.example.com
    routing_algorithm: failover    # Active/standby
    regions: [us-east, us-west]
    ttl: 15

  - name: db.example.com
    routing_algorithm: round-robin
    regions: [database]
    ttl: 60
```

## Documentation

Full documentation is available at **[docs.opengslb.org](https://docs.opengslb.org)**

- [Configuration Reference](https://docs.opengslb.org/en/latest/configuration/) - Complete configuration options
- [Interactive Demos](https://docs.opengslb.org/en/latest/demos/) - Hands-on tutorials
- [Docker Deployment](https://docs.opengslb.org/en/latest/docker/) - Container deployment guide
- [API Reference](https://docs.opengslb.org/en/latest/api/) - REST API documentation
- [Troubleshooting](https://docs.opengslb.org/en/latest/troubleshooting/) - Common issues and solutions

## Roadmap

### v1.1.9 Stable (Current)
- ✅ DNS server with A and AAAA record support
- ✅ Round-robin, weighted, and failover routing
- ✅ Geolocation-based routing (GeoIP + custom CIDR mappings)
- ✅ Latency-based routing (EMA smoothing)
- ✅ Learned latency routing (passive TCP RTT from agents)
- ✅ EDNS Client Subnet (ECS) support
- ✅ HTTP and TCP health checks
- ✅ Predictive health monitoring (CPU, memory, error rate)
- ✅ Agent-Overwatch distributed architecture
- ✅ DNSSEC support with automatic key management
- ✅ Server management CRUD API
- ✅ Multi-file configuration with includes
- ✅ Prometheus metrics and structured logging
- ✅ CLI management tool
- ✅ Docker deployment

### Planned
- 🔲 Web UI dashboard (Overlord)
- 🔲 Kubernetes operator
- 🔲 DNS-over-HTTPS/TLS

## Target Use Cases

- **Private Cloud Deployments**: Multi-region infrastructure with full control
- **Hybrid Cloud**: Intelligent routing between on-premises and cloud
- **Regulated Industries**: Data sovereignty requirements (finance, healthcare, government)
- **High-Security Environments**: No external dependencies or data sharing
- **Cost-Conscious Enterprises**: Enterprise features without SaaS pricing

## Community

- [Discord](https://discord.gg/H4s8RG6Az) - Real-time chat and support
- [Reddit](https://www.reddit.com/r/OpenGSLB) - Discussions and announcements
- [GitHub Discussions](https://github.com/LoganRossUS/OpenGSLB/discussions) - Q&A and ideas

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and workflow.

## Support the Project

If OpenGSLB is useful to you, consider supporting continued development:

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-FFDD00?style=flat&logo=buy-me-a-coffee&logoColor=black)](https://www.buymeacoffee.com/OpenGSLB) 
