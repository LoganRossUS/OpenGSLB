# Demo 1: Standalone Overwatch

This is the simplest OpenGSLB deployment: a single Overwatch node performing external health checks on nginx backend servers. No agents, no clustering, no Overlord.

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │           DNS Clients                   │
                    │         dig app.demo.local              │
                    └───────────────┬─────────────────────────┘
                                    │
                                    ▼
                    ┌─────────────────────────────────────────┐
                    │           Overwatch                     │
                    │         10.10.0.10                      │
                    │                                         │
                    │   DNS:53  API:8080  Metrics:9090        │
                    │                                         │
                    │   - Serves DNS queries                  │
                    │   - Performs health checks              │
                    │   - Returns only healthy backends       │
                    └───────────────┬─────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    │               │               │
                    ▼               ▼               ▼
            ┌───────────┐   ┌───────────┐   ┌───────────┐
            │  webapp1  │   │  webapp2  │   │  webapp3  │
            │ 10.10.0.21│   │ 10.10.0.22│   │ 10.10.0.23│
            │   :80     │   │   :80     │   │   :80     │
            └───────────┘   └───────────┘   └───────────┘
```

## Network Layout

| Service   | IP Address   | Ports                        |
|-----------|--------------|------------------------------|
| overwatch | 10.10.0.10   | DNS:53, API:8080, Metrics:9090 |
| webapp1   | 10.10.0.21   | HTTP:80                      |
| webapp2   | 10.10.0.22   | HTTP:80                      |
| webapp3   | 10.10.0.23   | HTTP:80                      |
| client    | 10.10.0.100  | (test container)             |

## Host Port Mappings

| Host Port | Container Port | Service        |
|-----------|----------------|----------------|
| 8080      | 8080           | API            |
| 9090      | 9090           | Metrics        |
| 8081      | 80             | webapp1 nginx  |
| 8082      | 80             | webapp2 nginx  |
| 8083      | 80             | webapp3 nginx  |

> **Note:** DNS (port 53) is not exposed to the host to avoid conflicts with system services like mDNS/Avahi. Use the client container for DNS queries: `docker exec client dig app.demo.local`

## Quick Start

### 1. Build the OpenGSLB Binary

```bash
# From the repository root
make build

# Copy to demo directory
cp opengslb demos/demo-1-standalone/bin/
```

### 2. Start the Demo

```bash
cd demos/demo-1-standalone

# Interactive demo with explanations
./scripts/demo.sh demo

# Or just start the environment
./scripts/demo.sh start
```

### 3. Stop the Demo

```bash
./scripts/demo.sh stop
```

## Demo Scenarios

### 1. DNS Round-Robin

Query the DNS multiple times to see round-robin load balancing:

```bash
# Query 6 times - see different IPs each time
for i in {1..6}; do
    docker exec client dig app.demo.local +short
done
```

Expected output:
```
10.10.0.21
10.10.0.22
10.10.0.23
10.10.0.21
10.10.0.22
10.10.0.23
```

### 2. Failure Detection

Stop a backend and watch it leave DNS rotation:

```bash
# Check current health
curl http://localhost:8080/api/v1/health/servers | jq

# Stop webapp2
docker stop webapp2

# Wait 10-15 seconds for health checks to fail
sleep 15

# Check health again - webapp2 should be unhealthy
curl http://localhost:8080/api/v1/health/servers | jq

# Query DNS - webapp2's IP should be gone
for i in {1..4}; do
    docker exec client dig app.demo.local +short
done
```

### 3. Recovery

Restart the backend and watch it return:

```bash
# Start webapp2
docker start webapp2

# Wait 5-10 seconds for health checks to pass
sleep 10

# Check health - webapp2 should be healthy again
curl http://localhost:8080/api/v1/health/servers | jq

# Query DNS - all 3 IPs should be back
for i in {1..6}; do
    docker exec client dig app.demo.local +short
done
```

### 4. API Exploration

```bash
# Liveness check
curl http://localhost:8080/api/v1/live

# Readiness check
curl http://localhost:8080/api/v1/ready

# List all backends with health status
curl http://localhost:8080/api/v1/health/servers | jq

# Get healthy servers only
curl http://localhost:8080/api/v1/health/servers | jq '.servers[] | select(.healthy == true)'
```

### 5. Metrics

```bash
# View all OpenGSLB metrics
curl http://localhost:9090/metrics | grep opengslb

# DNS query count
curl -s http://localhost:9090/metrics | grep opengslb_dns_queries_total

# Health check results
curl -s http://localhost:9090/metrics | grep opengslb_health_check
```

## Commands Cheat Sheet

```bash
# Start environment
docker-compose up -d

# Stop environment
docker-compose down

# View logs
docker-compose logs -f overwatch
docker-compose logs -f webapp1

# Query DNS (from client container)
docker exec client dig app.demo.local +short
docker exec client dig app.demo.local

# Check API health
curl http://localhost:8080/api/v1/health/servers | jq

# View metrics
curl http://localhost:9090/metrics | grep opengslb

# Stop a backend
docker stop webapp2

# Start a backend
docker start webapp2

# Restart everything
docker-compose restart

# Shell into client container (has dig, curl, jq pre-installed)
docker exec -it client sh

# From inside client (uses Overwatch as DNS automatically)
dig app.demo.local
curl app.demo.local
```

## How to Reset

```bash
# Full reset
docker-compose down -v
docker-compose up -d --build
```

## Configuration Details

The Overwatch is configured with:

| Setting | Value | Purpose |
|---------|-------|---------|
| DNS Zone | demo.local | Domain suffix for GSLB |
| Domain | app.demo.local | Load-balanced endpoint |
| Routing | round-robin | Rotate through backends |
| Health Check | HTTP /health | Check backend health |
| Check Interval | 5s | How often to check |
| Check Timeout | 2s | Max time for check |
| Failure Threshold | 2 | Failures before unhealthy |
| Success Threshold | 1 | Successes before healthy |
| DNS TTL | 5s | Low for demo visibility |

## Troubleshooting

### DNS queries fail

```bash
# Check if Overwatch is running
docker-compose ps

# Check Overwatch logs
docker-compose logs overwatch

# Verify DNS port is listening
docker-compose exec overwatch netstat -tulpn | grep 53
```

### Backends always unhealthy

```bash
# Test health endpoint directly
curl http://localhost:8081/health
curl http://localhost:8082/health
curl http://localhost:8083/health

# Check if backends are running
docker-compose ps

# Check backend logs
docker-compose logs webapp1
```

### API not responding

```bash
# Check if port 8080 is exposed
docker-compose ps

# Try from inside the network
docker exec -it client wget -qO- http://10.10.0.10:8080/api/v1/live
```

## Next Steps

After exploring this standalone demo, try:

- **Demo 2**: Agent-Overwatch (adds distributed health checking)
- **Demo 3**: Multi-Region (geolocation routing)
- **Demo 4**: Full HA (multiple Overwatches, DNSSEC)
