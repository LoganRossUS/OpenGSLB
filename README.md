# OpenGSLB

[![CI](https://github.com/loganrossus/OpenGSLB/actions/workflows/ci.yml/badge.svg)](https://github.com/loganrossus/OpenGSLB/actions/workflows/ci.yml)
[![Docker Build](https://github.com/loganrossus/opengslb/actions/workflows/docker-build.yml/badge.svg)](https://github.com/loganrossus/opengslb/actions/workflows/docker-build.yml)

OpenGSLB is an open-source, self-hosted Global Server Load Balancer (GSLB) that provides DNS-based traffic distribution across multiple data centers. Built for organizations that need complete control over their infrastructure without vendor lock-in.

## Quick Start

### Prerequisites

- Go 1.21+ (for building from source)
- Docker (for container deployment)

### Option 1: Build from Source

```bash
# Clone the repository
git clone https://github.com/loganrossus/OpenGSLB.git
cd OpenGSLB

# Build the binary
make build

# Create configuration directory
sudo mkdir -p /etc/opengslb
sudo cp config/example.yaml /etc/opengslb/config.yaml
sudo chmod 640 /etc/opengslb/config.yaml

# Edit configuration for your environment
sudo nano /etc/opengslb/config.yaml

# Run OpenGSLB (requires root for port 53, or use a high port)
sudo ./opengslb --config /etc/opengslb/config.yaml
```

### Option 2: Docker

```bash
# Pull the image
docker pull ghcr.io/loganrossus/opengslb:latest

# Create your configuration
mkdir -p ./config
cat > ./config/config.yaml << 'EOF'
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
  - name: primary
    servers:
      - address: 10.0.1.10
        port: 80
      - address: 10.0.1.11
        port: 80
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health

domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - primary
EOF

# Run the container
docker run -d \
  --name opengslb \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 9090:9090 \
  -v $(pwd)/config:/etc/opengslb:ro \
  ghcr.io/loganrossus/opengslb:latest
```

### Verify It's Working

```bash
# Query OpenGSLB for a configured domain
dig @localhost app.example.com A +short

# Check metrics endpoint
curl http://localhost:9090/metrics | grep opengslb

# View logs
docker logs opengslb
```

## Features

- **DNS-Based Load Balancing**: Returns appropriate server IPs based on routing algorithms
- **Health Monitoring**: Continuous HTTP health checks with configurable thresholds
- **Round-Robin Routing**: Even distribution across healthy servers
- **Automatic Failover**: Unhealthy servers automatically excluded from responses
- **Prometheus Metrics**: Full observability with query rates, latencies, and health status
- **Self-Hosted**: Deploy on your own infrastructure with no external dependencies
- **Single Binary**: Simple deployment with minimal operational overhead

## Configuration

OpenGSLB uses YAML configuration. See the [Configuration Reference](docs/configuration.md) for all options.

**Minimal example:**

```yaml
dns:
  listen_address: ":5353"  # Use high port for non-root
  default_ttl: 60

regions:
  - name: datacenter-1
    servers:
      - address: 192.168.1.10
        port: 80
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health

domains:
  - name: myapp.internal
    routing_algorithm: round-robin
    regions:
      - datacenter-1
```

## Documentation

- [Configuration Reference](docs/configuration.md) - All configuration options
- [Metrics Reference](docs/metrics.md) - Prometheus metrics and alerting examples
- [Docker Deployment](docs/docker.md) - Container deployment guide
- [Architecture Decisions](docs/ARCHITECTURE_DECISIONS.md) - Design rationale
- [Testing Guide](docs/testing.md) - Running tests
- [Troubleshooting](docs/troubleshooting.md) - Common issues and solutions
- [Contributing](CONTRIBUTING.md) - Development setup and workflow

## How It Works

1. **DNS Query**: Client queries OpenGSLB for a domain (e.g., `app.example.com`)
2. **Health Check**: OpenGSLB continuously monitors backend servers via HTTP/TCP checks
3. **Server Selection**: Router selects a healthy server using the configured algorithm
4. **DNS Response**: Client receives an A record pointing to the selected server
5. **Direct Connection**: Client connects directly to the backend (OpenGSLB is not in the data path)

```
┌────────┐         ┌──────────┐         ┌─────────────┐
│ Client │─DNS────▶│ OpenGSLB │         │ Backend 1   │
└────────┘ Query   └──────────┘         └─────────────┘
    │                   │ Health              ▲
    │                   │ Checks              │
    │                   ▼                     │
    │              ┌─────────────┐            │
    │              │ Backend 2   │            │
    │              └─────────────┘            │
    │                                         │
    └────────Direct HTTP Connection───────────┘
```

## Project Status

OpenGSLB is under active development. Current capabilities:

✅ DNS server (A records)  
✅ HTTP health checks  
✅ Round-robin routing  
✅ Prometheus metrics  
✅ Structured logging (JSON/text)  
✅ Docker deployment  

Coming soon:

- Weighted routing
- TCP health checks  
- Configuration hot-reload (SIGHUP)
- Geolocation-based routing
- Active/Standby failover mode

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and workflow.