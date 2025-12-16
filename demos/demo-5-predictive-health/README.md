# OpenGSLB Demo 5: Predictive Health Detection

> "We knew it was going to fail before it did."

This demo showcases OpenGSLB's core differentiator: **predictive health monitoring** that detects problems *before* they impact users.

## The Problem with Traditional GSLB

```
Traditional GSLB:
  App crashes → Health check fails → DNS updated → Users see errors (30-60s)

OpenGSLB:
  CPU spikes → Agent predicts failure → Traffic drains → App crashes → Zero user impact
```

OpenGSLB is **predictive from the inside** (agents know trouble is coming) while remaining **reactive from the outside** (overwatch validates and can veto).

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           DEMO ENVIRONMENT                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │                        DNS CLIENT                                │   │
│   │                     (dig / client container)                     │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                    │                                     │
│                                    ▼                                     │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │                        OVERWATCH                                 │   │
│   │                      (10.50.0.10:53)                             │   │
│   │                                                                  │   │
│   │  • Receives agent gossip (including predictive signals)         │   │
│   │  • Performs external health validation                          │   │
│   │  • Serves authoritative DNS                                     │   │
│   │  • Exposes API on :8080, metrics on :9090                       │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                    ▲                                     │
│                                    │ Gossip (AES-256 encrypted)          │
│                                    │                                     │
│   ┌────────────────────┬───────────┴───────────┬────────────────────┐   │
│   │                    │                       │                    │   │
│   ▼                    ▼                       ▼                    │   │
│ ┌──────────────┐  ┌──────────────┐       ┌──────────────┐          │   │
│ │  BACKEND-1   │  │  BACKEND-2   │       │  BACKEND-3   │          │   │
│ │   (stable)   │  │   (stable)   │       │   (CHAOS)    │          │   │
│ │              │  │              │       │              │          │   │
│ │ ┌──────────┐ │  │ ┌──────────┐ │       │ ┌──────────┐ │          │   │
│ │ │  Agent   │ │  │ │  Agent   │ │       │ │  Agent   │ │          │   │
│ │ └──────────┘ │  │ └──────────┘ │       │ └──────────┘ │          │   │
│ │ ┌──────────┐ │  │ ┌──────────┐ │       │ ┌──────────┐ │          │   │
│ │ │ Demo App │ │  │ │ Demo App │ │       │ │ Demo App │ │◄── Chaos │   │
│ │ │  :8080   │ │  │ │  :8080   │ │       │ │  :8080   │ │    Target│   │
│ │ └──────────┘ │  │ └──────────┘ │       │ └──────────┘ │          │   │
│ └──────────────┘  └──────────────┘       └──────────────┘          │   │
│   10.50.0.21        10.50.0.22              10.50.0.23              │   │
│                                                                          │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │                      PROMETHEUS + GRAFANA                        │   │
│   │                    (Metrics visualization)                       │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.22+ (for building OpenGSLB)
- `dig` command (for DNS queries)
- `curl` and `jq` (for API calls)

### Start the Demo

```bash
# From this directory
./scripts/start-demo.sh
```

This will:
1. Build the OpenGSLB binary
2. Build and start all Docker containers
3. Wait for services to be healthy
4. Display access URLs

### Run the Guided Demo

SSH into the client container for an interactive walkthrough:

```bash
ssh root@localhost -p 2222
# Password: demo

# Inside the container:
./demo.sh
```

### Access Points

| Service | URL/Command |
|---------|-------------|
| Grafana Dashboard | http://localhost:3000 |
| Prometheus | http://localhost:9092 |
| Overwatch API | http://localhost:8080/api/v1/health/servers |
| DNS Query | `dig @localhost -p 5354 app.demo.local +short` |
| Client SSH | `ssh root@localhost -p 2222` (password: demo) |
| Chaos API | http://localhost:8083/chaos/status |

## Demo Script

### Act 1: Baseline

Verify all backends are healthy:

```bash
# Check health via API
curl -s http://localhost:8080/api/v1/health/servers | jq '.servers[] | {address, healthy}'

# DNS returns all 3 backends
dig @localhost -p 5354 app.demo.local +short
```

### Act 2: Trigger CPU Spike

Inject chaos on backend-3:

```bash
# From host machine
curl -X POST "http://localhost:8083/chaos/cpu?intensity=85&duration=60s"

# Or from client container
./chaos.sh cpu 60s 85
```

Watch the Grafana dashboard - you'll see:
- CPU spike on backend-3
- Predictive signal triggers
- Backend begins draining

### Act 3: Traffic Shifts

DNS automatically excludes backend-3:

```bash
# DNS now returns only backend-1 and backend-2
dig @localhost -p 5354 app.demo.local +short

# But health check STILL PASSES!
curl -s http://localhost:8083/health
# Returns: {"status":"healthy"...}

# Overwatch shows backend-3 as draining
curl -s http://localhost:8080/api/v1/health/servers | jq '.servers[] | select(.address | contains("10.50.0.23"))'
```

**Key insight**: The backend's health check is still passing, but we're proactively draining because the agent predicted trouble.

### Act 4: Overwatch Validates

Trigger actual errors to validate the prediction:

```bash
curl -X POST "http://localhost:8083/chaos/errors?rate=100&duration=30s"
```

Now Overwatch's external validation confirms the failure. The agent's prediction was correct.

### Act 5: Recovery

Stop all chaos:

```bash
curl -X POST "http://localhost:8083/chaos/stop"

# Wait 15 seconds for recovery
sleep 15

# All 3 backends should be back
dig @localhost -p 5354 app.demo.local +short
```

## Chaos Injection API

The demo app on backend-3 (exposed at http://localhost:8083) provides chaos injection endpoints:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/chaos/cpu` | POST | Trigger CPU spike |
| `/chaos/memory` | POST | Trigger memory pressure |
| `/chaos/errors` | POST | Inject HTTP 500 errors |
| `/chaos/latency` | POST | Add response latency |
| `/chaos/stop` | POST | Stop all chaos |
| `/chaos/status` | GET | View current chaos state |

### Parameters

**CPU Spike**
```bash
curl -X POST "http://localhost:8083/chaos/cpu?duration=60s&intensity=85"
# intensity: 1-100 (% CPU to consume)
# duration: how long to run
```

**Memory Pressure**
```bash
curl -X POST "http://localhost:8083/chaos/memory?duration=60s&amount=500"
# amount: MB to allocate
# duration: how long to hold
```

**Error Injection**
```bash
curl -X POST "http://localhost:8083/chaos/errors?duration=60s&rate=50"
# rate: 1-100 (% of /health requests that return 500)
# duration: how long to inject errors
```

**Latency Injection**
```bash
curl -X POST "http://localhost:8083/chaos/latency?duration=60s&latency=500"
# latency: milliseconds to add to all requests
# duration: how long to inject latency
```

## Key Talking Points

### "Why is this better than traditional health checks?"

Traditional GSLB waits for failure. Three failed health checks at 10-second intervals = 30 seconds of users hitting a degraded server. OpenGSLB's agent sees the warning signs—CPU spiking, memory filling, error rates climbing—and starts draining traffic before the crash. Zero user impact.

### "Can't the agent lie?"

Great question. The agent can claim anything, but Overwatch always validates externally. If the agent says "healthy" but Overwatch's check sees 500 errors, Overwatch wins. If the agent cries wolf with bleed signals, Overwatch's external check can override. Trust but verify.

### "What if the Overwatch goes down?"

DNS clients have built-in redundancy. Configure multiple Overwatches in resolv.conf, clients automatically retry. No VRRP, no Raft, no complexity. DNS has been doing failover for 40 years.

## Predictive Health Configuration

The agent configuration enables predictive monitoring:

```yaml
agent:
  predictive:
    enabled: true
    check_interval: 5s
    cpu:
      threshold: 80        # Bleed when CPU exceeds 80%
      bleed_duration: 30s
    memory:
      threshold: 85        # Bleed when memory exceeds 85%
      bleed_duration: 30s
    error_rate:
      threshold: 5         # Bleed when error rate exceeds 5/min
      window: 60s
      bleed_duration: 30s
```

## Cleanup

```bash
./scripts/cleanup.sh
```

This stops and removes all containers.

## Troubleshooting

### DNS queries timeout

Make sure Overwatch is running:
```bash
docker logs overwatch
```

### Backends not registering

Check agent logs:
```bash
docker logs backend-3
```

### Chaos not working

Verify backend-3 is accessible:
```bash
curl http://localhost:8083/chaos/status
```

## Files

```
demo-5-predictive-health/
├── docker-compose.yml          # Container orchestration
├── Dockerfile.overwatch        # Overwatch container
├── Dockerfile.backend          # Backend container (demo-app + agent)
├── Dockerfile.client           # Client container for SSH access
├── demo-app/
│   ├── main.go                 # Demo app with chaos endpoints
│   ├── go.mod
│   └── Dockerfile              # Standalone demo-app build
├── config/
│   ├── overwatch.yaml          # Overwatch configuration
│   ├── agent1.yaml             # Agent config for backend-1
│   ├── agent2.yaml             # Agent config for backend-2
│   ├── agent3.yaml             # Agent config for backend-3 (chaos)
│   ├── prometheus.yml          # Prometheus scrape config
│   └── grafana/
│       ├── provisioning/       # Grafana auto-provisioning
│       └── dashboards/         # Pre-built dashboards
├── scripts/
│   ├── start-demo.sh           # Build and start everything
│   ├── cleanup.sh              # Stop and clean up
│   ├── start-backend.sh        # Backend startup script
│   ├── demo.sh                 # Guided demo walkthrough
│   ├── chaos.sh                # Chaos injection helper
│   └── query-dns.sh            # DNS query monitor
└── README.md                   # This file
```
