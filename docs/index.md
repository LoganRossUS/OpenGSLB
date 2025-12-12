# OpenGSLB Documentation

**Open-source Global Server Load Balancer with DNS-based traffic management**

OpenGSLB is an enterprise-grade DNS-based load balancer that provides intelligent traffic distribution across multiple backend servers. It uses a simplified agent-overwatch architecture for reliability and ease of operation.

## Key Features

- **DNS-based Load Balancing**: Route traffic using DNS responses without proxying
- **Multiple Routing Algorithms**: Round-robin, weighted, failover, geolocation, and latency-based routing
- **Geolocation Routing**: Route traffic based on client geographic location using MaxMind GeoIP2 databases with EDNS Client Subnet (ECS) support
- **Latency-Based Routing**: Route to lowest-latency servers with exponential moving average (EMA) smoothing
- **Health Checking**: HTTP, HTTPS, and TCP health checks with configurable thresholds
- **Agent-Overwatch Architecture**: Distributed health monitoring with centralized DNS serving
- **DNSSEC Support**: Cryptographic authentication of DNS responses
- **Predictive Health**: CPU, memory, and error rate monitoring for proactive failover
- **External Overrides**: API for CloudWatch, Watcher, or custom tool integration
- **Multi-File Configuration**: Split configuration across multiple files with glob pattern support
- **CLI Management Tool**: Command-line tool for status monitoring, overrides, and configuration validation

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│  DNS Clients (resolv.conf with multiple nameservers)    │
│       │           │           │                         │
│       ▼           ▼           ▼                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                │
│  │Overwatch1│ │Overwatch2│ │Overwatch3│                │
│  └─────┬────┘ └─────┬────┘ └─────┬────┘                │
│        │            │            │                      │
│        └────────────┼────────────┘                      │
│                     │ Gossip                            │
│            ┌────────┼────────┐                          │
│            ▼        ▼        ▼                          │
│       ┌────────┐ ┌────────┐ ┌────────┐                 │
│       │ Agent  │ │ Agent  │ │ Agent  │                 │
│       │ + App  │ │ + App  │ │ + App  │                 │
│       └────────┘ └────────┘ └────────┘                 │
└─────────────────────────────────────────────────────────┘
```

## Getting Started

### Installation

```bash
# Clone the repository
git clone https://github.com/LoganRossUS/OpenGSLB.git
cd OpenGSLB

# Build
go build -o opengslb ./cmd/opengslb

# Run
./opengslb --config config.yaml
```

### Basic Configuration

```yaml
mode: overwatch

dns:
  listen_address: "0.0.0.0:53"
  zones:
    - gslb.example.com

gossip:
  bind_address: "0.0.0.0:7946"
  encryption_key: "your-32-byte-base64-key"

validation:
  enabled: true
  check_interval: 30s
```

## License

OpenGSLB is dual-licensed:

1. **AGPLv3** - Free for open-source and internal use
2. **Commercial License** - Available for proprietary integration

See [LICENSE](https://github.com/LoganRossUS/OpenGSLB/blob/main/LICENSE) for details.

```{toctree}
:maxdepth: 2
:caption: Getting Started

configuration
docker
cli
```

```{toctree}
:maxdepth: 2
:caption: Architecture

ARCHITECTURE_DECISIONS
deployment/agent-overwatch
gossip
```

```{toctree}
:maxdepth: 2
:caption: API Reference

api
metrics
```

```{toctree}
:maxdepth: 2
:caption: Operations

operations/index
testing
troubleshooting
```

```{toctree}
:maxdepth: 2
:caption: Security

security/api-hardening
```

```{toctree}
:maxdepth: 2
:caption: Development

PROGRESS
roadmap/future-features
```
