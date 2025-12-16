# Demo 1: Standalone Overwatch

This is the simplest OpenGSLB deployment: a single Overwatch node performing external health checks on nginx backend servers. No agents, no clustering.

## What You'll Learn

- Basic DNS-based load balancing
- Health check configuration
- Round-robin traffic distribution
- Failure detection and recovery

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

| Host Port | Container Port | Service             |
|-----------|----------------|---------------------|
| 2222      | 22             | Client SSH access   |
| 8080      | 8080           | API                 |
| 9090      | 9090           | Metrics             |
| 8081      | 80             | webapp1 nginx       |
| 8082      | 80             | webapp2 nginx       |
| 8083      | 80             | webapp3 nginx       |

:::{note}
DNS (port 53) is internal to the Docker network. SSH into the client container to query DNS.
:::

## Quick Start

### 1. Build the OpenGSLB Binary

```bash
# From the repository root - build a static binary for Alpine Linux
CGO_ENABLED=0 go build -o opengslb ./cmd/opengslb

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

### 3. SSH into the Client

```bash
ssh -p 2222 root@localhost
# Password: demo
```

You'll see a welcome banner with available commands. The client uses Overwatch as its DNS server, so you can query `app.demo.local` directly.

### 4. Stop the Demo

```bash
./scripts/demo.sh stop
```

## Demo Scenarios

### 1. DNS Round-Robin

SSH into the client and query DNS multiple times to see round-robin:

```bash
# SSH in first
ssh -p 2222 root@localhost

# Query 6 times - see different IPs each time
for i in {1..6}; do dig app.demo.local +short; done
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
# From your host terminal - stop webapp2
docker stop webapp2

# Wait 10-15 seconds for health checks to fail, then check health
curl http://localhost:8080/api/v1/health/servers | jq

# From your SSH session in the client - query DNS (webapp2 should be gone)
for i in {1..4}; do dig app.demo.local +short; done
```

### 3. Recovery

Restart the backend and watch it return:

```bash
# From your host terminal - start webapp2
docker start webapp2

# Wait 5-10 seconds, then check health
curl http://localhost:8080/api/v1/health/servers | jq

# From your SSH session - all 3 IPs should be back
for i in {1..6}; do dig app.demo.local +short; done
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

## Configuration Details

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

## Commands Cheat Sheet

| Action | Command |
|--------|---------|
| Start environment | `docker-compose up -d` |
| Stop environment | `docker-compose down` |
| SSH to client | `ssh -p 2222 root@localhost` (password: demo) |
| Query DNS | `dig app.demo.local +short` |
| Check API health | `curl http://localhost:8080/api/v1/health/servers \| jq` |
| View metrics | `curl http://localhost:9090/metrics \| grep opengslb` |
| View logs | `docker-compose logs -f overwatch` |
| Stop a backend | `docker stop webapp2` |
| Start a backend | `docker start webapp2` |
| Full reset | `docker-compose down -v && docker-compose up -d --build` |

## Troubleshooting

### Config permission errors

If you see "config file has insecure permissions" errors:

```bash
# Rebuild the overwatch container to apply config with correct permissions
docker-compose build overwatch
docker-compose up -d
```

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

# Check backend logs
docker-compose logs webapp1
```

## Next Steps

After exploring this standalone demo, continue to [Demo 2: Agent-Overwatch](demo-2-agent-overwatch) to learn about distributed health checking.
