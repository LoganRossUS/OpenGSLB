# Demos

OpenGSLB includes a comprehensive set of interactive demos that showcase its features and capabilities. Each demo builds on the previous one, introducing new concepts progressively.

## Demo Overview

| Demo | Focus Area | Key Features |
|------|-----------|--------------|
| [Demo 1: Standalone](demo-1-standalone) | Basic Setup | DNS load balancing, health checks, round-robin |
| [Demo 2: Agent-Overwatch](demo-2-agent-overwatch) | Distributed Architecture | Proactive draining, agent gossip, zero-downtime |
| [Demo 3: Latency Routing](demo-3-latency-routing) | Performance Optimization | Latency-based routing, automatic adaptation |
| [Demo 4: GeoIP Routing](demo-4-geoip-routing) | Geographic Distribution | GeoIP routing, EDNS Client Subnet, custom CIDR |
| [Demo 5: Predictive Health](demo-5-predictive-health) | Proactive Health | Predictive monitoring, chaos engineering, Grafana |

## Prerequisites

All demos require:

- Docker and Docker Compose
- Go 1.22+ (for building OpenGSLB)
- Basic CLI tools: `dig`, `curl`, `jq`

### Building the Binary

Each demo requires the OpenGSLB binary built for Alpine Linux (musl libc):

```bash
# From the repository root
CGO_ENABLED=0 GOOS=linux go build -o opengslb ./cmd/opengslb

# Copy to the specific demo directory
cp opengslb demos/demo-X-*/bin/
```

:::{note}
The `CGO_ENABLED=0` flag is required because demo containers use Alpine Linux with musl libc instead of glibc.
:::

## Running a Demo

Each demo follows a consistent pattern:

```bash
# Navigate to the demo directory
cd demos/demo-X-*/

# Start the environment
docker-compose up -d --build

# SSH into the client container (password: demo)
ssh -p 2222 root@localhost

# Run the guided demo script
./demo.sh

# Stop the demo
docker-compose down -v
```

## Learning Path

### Beginner: Start with Demo 1

If you're new to OpenGSLB, start with **Demo 1: Standalone**. It introduces:
- Basic DNS load balancing concepts
- Health check configuration
- The Overwatch component

### Intermediate: Demos 2 & 3

Once comfortable with the basics, explore:
- **Demo 2**: Learn about the agent-overwatch architecture and proactive health signaling
- **Demo 3**: Understand latency-based routing and automatic traffic optimization

### Advanced: Demos 4 & 5

For production-ready scenarios:
- **Demo 4**: Geographic routing with GeoIP databases and custom CIDR mappings
- **Demo 5**: Predictive health monitoring with chaos engineering

## Demo Architecture Comparison

```
Demo 1: Simple               Demo 2-5: Distributed
─────────────────           ─────────────────────────

  ┌──────────┐                 ┌──────────┐
  │ Overwatch│◄────┐           │ Overwatch│◄──────────┐
  └────┬─────┘     │           └────┬─────┘           │
       │           │                │         Gossip  │
       │ Health    │                │                 │
       │ Checks    │                ▼                 │
       ▼           │           ┌─────────┐      ┌─────────┐
  ┌─────────┐      │           │ Backend │      │ Backend │
  │ Backend │      │           │ + Agent │      │ + Agent │
  │ (nginx) │──────┘           └─────────┘      └─────────┘
  └─────────┘
```

## Troubleshooting

### Common Issues

**Binary not found**
```bash
# Rebuild with correct flags
CGO_ENABLED=0 GOOS=linux go build -o demos/demo-X-*/bin/opengslb ./cmd/opengslb
```

**Permission denied on config**
```bash
# Config files require 600 permissions - rebuild the container
docker-compose build
docker-compose up -d
```

**DNS queries fail**
```bash
# Check if Overwatch is running
docker-compose ps
docker-compose logs overwatch
```

**Port conflicts**
```bash
# Stop all demos first
docker-compose down -v

# Check for conflicting services
lsof -i :2222 -i :8080 -i :9090
```

```{toctree}
:maxdepth: 1
:hidden:

demo-1-standalone
demo-2-agent-overwatch
demo-3-latency-routing
demo-4-geoip-routing
demo-5-predictive-health
```
