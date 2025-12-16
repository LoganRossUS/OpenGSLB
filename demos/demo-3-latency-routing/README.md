# OpenGSLB Demo 3: Latency-Based Routing

## Overview

This demo showcases OpenGSLB's **latency-based routing algorithm** that
automatically routes traffic to the fastest responding backend.

Using Linux traffic control (`tc`), we simulate different network latencies
on each backend and watch OpenGSLB adapt in real-time.

## Key Concept

OpenGSLB measures the response time of each health check. With latency-based
routing enabled, it automatically selects the server with the lowest measured
latency for each DNS query.

**No configuration changes needed** - routing adapts automatically to network
conditions!

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│   ┌───────────────┐  ┌───────────────┐  ┌───────────────┐          │
│   │   webapp1     │  │   webapp2     │  │   webapp3     │          │
│   │  FAST (5ms)   │  │ MEDIUM (50ms) │  │ SLOW (150ms)  │          │
│   │ ┌───────────┐ │  │ ┌───────────┐ │  │ ┌───────────┐ │          │
│   │ │   nginx   │ │  │ │   nginx   │ │  │ │   nginx   │ │          │
│   │ │    :80    │ │  │ │    :80    │ │  │ │    :80    │ │          │
│   │ └───────────┘ │  │ └───────────┘ │  │ └───────────┘ │          │
│   │ ┌───────────┐ │  │ ┌───────────┐ │  │ ┌───────────┐ │          │
│   │ │   agent   │ │  │ │   agent   │ │  │ │   agent   │ │          │
│   │ └─────┬─────┘ │  │ └─────┬─────┘ │  │ └─────┬─────┘ │          │
│   │       │       │  │       │       │  │       │       │          │
│   │  tc netem 5ms │  │ tc netem 50ms │  │tc netem 150ms │          │
│   └───────┼───────┘  └───────┼───────┘  └───────┼───────┘          │
│           │                  │                  │                   │
│           └──────────────────┼──────────────────┘                   │
│                              │ gossip (:7946)                       │
│                              ▼                                      │
│                    ┌───────────────────┐                            │
│                    │     overwatch     │                            │
│                    │ • receives gossip │                            │
│                    │ • measures latency│                            │
│                    │ • routes to FAST  │                            │
│                    └───────────────────┘                            │
│                              ▲                                      │
│                              │ DNS queries                          │
│                    ┌───────────────────┐                            │
│                    │      client       │  ← SSH in here             │
│                    └───────────────────┘                            │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Prerequisites

Build the OpenGSLB binary (from repo root):

```bash
CGO_ENABLED=0 GOOS=linux go build -o demos/demo-3-latency-routing/bin/opengslb ./cmd/opengslb
```

## Quick Start

```bash
cd demos/demo-3-latency-routing
docker-compose up -d --build

# Set initial latencies (from host terminal)
./scripts/set-latency.sh webapp1 5
./scripts/set-latency.sh webapp2 50
./scripts/set-latency.sh webapp3 150

# SSH into the client container (password: demo)
ssh -p 2222 root@localhost
```

## Guided Demo

Run the interactive demo script from the client container:

```bash
./demo.sh
```

Or follow these manual steps:

### Step 1: Verify Initial State

```bash
# Check latency measurements via API
curl -s http://10.30.0.10:8080/api/v1/health/servers | jq

# Query DNS - should return fastest (webapp1 at 5ms)
for i in {1..6}; do
  dig @10.30.0.10 app.demo.local +short
  sleep 0.5
done
```

### Step 2: Change the Fastest Server

```bash
# From HOST terminal, make webapp2 fastest
./scripts/set-latency.sh webapp2 2

# Wait ~5 seconds, then query again from client
dig @10.30.0.10 app.demo.local +short
# Should now return 10.30.0.22 (webapp2)
```

### Step 3: Simulate Network Problems

```bash
# From HOST terminal, add high latency to webapp1
./scripts/set-latency.sh webapp1 500

# Traffic automatically avoids the slow server
dig @10.30.0.10 app.demo.local +short
```

## Commands Cheat Sheet

| Action | Command |
|--------|---------|
| SSH to client | `ssh -p 2222 root@localhost` (password: demo) |
| Query DNS | `dig @10.30.0.10 app.demo.local +short` |
| Check latencies | `curl http://10.30.0.10:8080/api/v1/health/servers \| jq` |
| Set latency | `./scripts/set-latency.sh webapp1 50` (from host) |
| Remove latency | `./scripts/set-latency.sh webapp1 0` (from host) |
| View all latencies | `./scripts/set-latency.sh` (no args, from host) |
| View agent logs | `docker logs webapp1` |
| View overwatch logs | `docker logs overwatch` |
| Reset demo | `docker-compose down && docker-compose up -d` |

## How tc Works

Linux Traffic Control (`tc`) with `netem` (network emulator) adds artificial
delay to network packets:

```bash
# Add 50ms delay
tc qdisc add dev eth0 root netem delay 50ms

# Change to 100ms
tc qdisc change dev eth0 root netem delay 100ms

# Remove delay
tc qdisc del dev eth0 root
```

This affects ALL traffic on the interface, including health check responses,
which is exactly what we want to demonstrate latency-based routing.

## Architecture Explanation

### Why Latency-Based Routing?

- **Round-robin** distributes load evenly, but ignores performance
- **Weighted routing** requires manual configuration

**Latency-based routing automatically optimizes for performance:**

1. Health checks measure actual response times
2. Router tracks exponential moving average of latency per server
3. DNS queries return the server with lowest latency
4. System adapts automatically as network conditions change

### Use Cases

- **Multi-cloud deployments**: Route to the cloud with best connectivity
- **Global CDN**: Direct users to fastest edge server
- **Network failover**: Automatically avoid congested paths
- **Performance optimization**: No manual tuning required

## Network Layout

| Container | IP Address | Role |
|-----------|-----------|------|
| overwatch | 10.30.0.10 | DNS server, gossip receiver |
| webapp1 | 10.30.0.21 | Fast server (5ms) |
| webapp2 | 10.30.0.22 | Medium server (50ms) |
| webapp3 | 10.30.0.23 | Slow server (150ms) |
| client | 10.30.0.100 | SSH access for demo |

## Port Mappings

| Host Port | Container Port | Service |
|-----------|----------------|---------|
| 2222 | 22 | SSH to client container |
| 8080 | 8080 | Overwatch API |
| 9090 | 9090 | Overwatch Metrics |

> **Note**: DNS (port 53) is NOT exposed to host to avoid conflicts. Use the client container.

## Cleanup

```bash
docker-compose down -v
```

## Troubleshooting

### tc command fails

Ensure the container has `NET_ADMIN` capability. This is set in docker-compose.yml.

### Latency not changing DNS results

Wait for health checks to run (every 2 seconds) and for the exponential moving
average to stabilize (~3 samples minimum).

### Permission denied on config file

OpenGSLB requires config files to have 600 permissions. The startup script
copies the config with correct permissions.

### Binary not found

Build the binary first:
```bash
CGO_ENABLED=0 GOOS=linux go build -o demos/demo-3-latency-routing/bin/opengslb ./cmd/opengslb
```
