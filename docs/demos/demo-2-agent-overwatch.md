# Demo 2: Agent-Overwatch Architecture

This demo showcases OpenGSLB's distributed architecture where **agents run alongside applications** and can PROACTIVELY signal health issues.

## What You'll Learn

- Agent-Overwatch architecture
- Proactive health signaling (drain mode)
- Zero-downtime traffic management
- Gossip protocol communication

## The Key Insight

| Traditional GSLB | Agent-based GSLB |
|------------------|------------------|
| External health check fails | Agent knows problem is coming |
| DNS removes backend | Gossips "I'm unhealthy" |
| **Some requests already failed** | DNS removes backend first |
| Reactive | **Zero failed requests** |

## Architecture

```
+-----------------------------------------------------------------------------+
|  webapp1 (10.20.0.21)   webapp2 (10.20.0.22)   webapp3 (10.20.0.23)        |
|  +-----------------+    +-----------------+    +-----------------+         |
|  | nginx + agent   |    | nginx + agent   |    | nginx + agent   |         |
|  +--------+--------+    +--------+--------+    +--------+--------+         |
|           +----------------------+----------------------+                   |
|                                  | gossip (:7946)                          |
|                                  v                                         |
|                        overwatch (10.20.0.10)                              |
|                        +-----------------------+                           |
|                        |  DNS :53 | API :8080  |                           |
|                        +-----------------------+                           |
|                                  ^                                         |
|                                  | dig queries                             |
|                        client (10.20.0.100)                                |
|                        +-- SSH port 2222                                   |
+-----------------------------------------------------------------------------+
```

## Quick Start

### 1. Build the Binary

```bash
# From the repository root
CGO_ENABLED=0 GOOS=linux go build -o demos/demo-2-agent-overwatch/bin/opengslb ./cmd/opengslb
```

### 2. Start the Demo

```bash
cd demos/demo-2-agent-overwatch
docker-compose up -d --build

# SSH into client (password: demo)
ssh -p 2222 root@localhost

# Run the guided demo
./demo.sh
```

## Demo Scenarios

### Scenario 1: Traditional Reactive Failure

```bash
# Stop nginx - external health check eventually catches it
docker exec webapp2 nginx -s stop

# Wait ~10s, then check DNS - webapp2 removed
dig @10.20.0.10 app.demo.local +short

# Restart nginx
docker exec webapp2 nginx
```

### Scenario 2: Proactive Drain (The Key Feature)

This is what makes agent-based GSLB special:

```bash
# Trigger drain - agent reports unhealthy, nginx STILL RUNNING
./scripts/drain.sh webapp2 on

# Prove nginx is still healthy
curl http://10.20.0.22/health    # Returns 200!

# But it's out of DNS rotation
dig @10.20.0.10 app.demo.local +short   # No 10.20.0.22!

# Recover
./scripts/drain.sh webapp2 off
```

:::{important}
Notice that the health check still passes, but the server is removed from DNS. This is proactive draining - the agent signals "I'm about to have problems" before any actual failure occurs.
:::

## Why Agents?

Traditional GSLB relies on external health checks. The problem:

1. External check runs every N seconds
2. Server fails between checks
3. Requests hit failed server until next check
4. **Result: Failed requests during the gap**

Agent-based architecture:

1. Agent runs ON the server with inside knowledge
2. Agent sees: CPU spike, memory pressure, error rates, maintenance mode
3. Agent signals "I'm about to fail" BEFORE external check would catch it
4. **Result: Zero failed requests**

## The Drain File

Each agent's startup script watches for `/tmp/drain`. When this file exists:
- Agent stops running (stops sending gossip)
- Overwatch removes backend from DNS (no gossip = unhealthy)
- nginx is still running and serving traffic
- Perfect for maintenance drains

This simulates any "inside knowledge" the agent might have:
- High CPU/memory (predictive)
- Increasing error rates (trending)
- Upcoming maintenance (planned)
- Application-specific signals

## Components

**Overwatch (10.20.0.10)**
- Receives gossip from agents on port 7946
- Serves DNS queries on port 53
- Exposes API on port 8080
- Exposes metrics on port 9090

**Webapp Containers (10.20.0.21-23)**
- Run nginx serving HTTP on port 80
- Run OpenGSLB agent as sidecar
- Agent monitors localhost:80 and gossips to Overwatch
- Agent respects /tmp/drain for proactive health signaling

**Client (10.20.0.100)**
- SSH access on host port 2222
- Has dig, curl, jq for testing
- Contains demo.sh guided walkthrough

## Commands Cheat Sheet

| Action | Command |
|--------|---------|
| SSH to client | `ssh -p 2222 root@localhost` (password: demo) |
| Query DNS | `dig @10.20.0.10 app.demo.local +short` |
| Check health API | `curl http://10.20.0.10:8080/api/v1/health/servers \| jq` |
| Enable drain | `./scripts/drain.sh webapp2 on` |
| Disable drain | `./scripts/drain.sh webapp2 off` |
| View agent logs | `docker logs webapp2` |
| View overwatch logs | `docker logs overwatch` |
| Reset demo | `docker-compose down && docker-compose up -d` |

## Troubleshooting

### Agent not connecting to Overwatch

```bash
# Check agent logs
docker logs webapp1

# Verify gossip port is open
docker exec overwatch netstat -ln | grep 7946
```

### DNS not returning backends

```bash
# Check if agents registered
curl http://10.20.0.10:8080/api/v1/health/servers | jq

# View overwatch logs
docker logs overwatch
```

### Drain not working

```bash
# Verify drain file exists
docker exec webapp2 ls -la /tmp/drain

# Check agent process
docker exec webapp2 pgrep -la opengslb
```

## Cleanup

```bash
docker-compose down -v
```

## Next Steps

Continue to [Demo 3: Latency-Based Routing](demo-3-latency-routing) to learn about automatic performance optimization.
