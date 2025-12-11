# Agent-Overwatch Deployment Guide

This guide covers deploying OpenGSLB using the agent-overwatch architecture introduced in Sprint 5.

## Architecture Overview

The agent-overwatch model consists of two components:

1. **Agent**: Runs on application servers, monitors local health, gossips state to Overwatch nodes
2. **Overwatch**: Runs adjacent to DNS infrastructure, validates health claims, serves authoritative DNS

### Key Principles

- **No VIPs required**: DNS clients retry automatically (resolv.conf with multiple nameservers)
- **No cluster coordination**: Each Overwatch operates independently
- **Security by default**: Mandatory gossip encryption, TOFU authentication, DNSSEC enabled
- **Overwatch always wins**: External validation overrides agent health claims

## Prerequisites

- Go 1.21+ (for building from source)
- Network connectivity between agents and Overwatches (port 7946 for gossip)
- DNS port access (port 53 or custom) for Overwatch nodes

## Deployment Patterns

### Pattern 1: Simple (1 Overwatch, N Agents)

```
┌─────────────────────────────────────────────────────────┐
│                    DNS Clients                           │
│                         │                                │
│                         ▼                                │
│                   ┌──────────┐                          │
│                   │Overwatch │                          │
│                   │ 10.0.1.53│                          │
│                   └────┬─────┘                          │
│                        │ Gossip                         │
│            ┌───────────┼───────────┐                    │
│            ▼           ▼           ▼                    │
│       ┌────────┐  ┌────────┐  ┌────────┐              │
│       │ Agent  │  │ Agent  │  │ Agent  │              │
│       │ + App  │  │ + App  │  │ + App  │              │
│       └────────┘  └────────┘  └────────┘              │
└─────────────────────────────────────────────────────────┘
```

### Pattern 2: High Availability (Multiple Independent Overwatches)

```
┌─────────────────────────────────────────────────────────┐
│  DNS Clients (resolv.conf with multiple nameservers)     │
│       │           │           │                          │
│       ▼           ▼           ▼                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                 │
│  │Overwatch1│ │Overwatch2│ │Overwatch3│                 │
│  │10.0.1.53 │ │10.0.1.54 │ │10.0.1.55 │                 │
│  └─────┬────┘ └─────┬────┘ └─────┬────┘                 │
│        │            │            │                       │
│        └────────────┼────────────┘                       │
│                     │ Gossip (all agents → all overwatches)
│            ┌────────┼────────┐                          │
│            ▼        ▼        ▼                          │
│       ┌────────┐ ┌────────┐ ┌────────┐                 │
│       │ Agent  │ │ Agent  │ │ Agent  │                 │
│       │ + App  │ │ + App  │ │ + App  │                 │
│       └────────┘ └────────┘ └────────┘                 │
└─────────────────────────────────────────────────────────┘
```

## Step-by-Step Deployment

### Step 1: Generate Shared Secrets

```bash
# Generate gossip encryption key (32 bytes, base64 encoded)
GOSSIP_KEY=$(openssl rand -base64 32)
echo "Gossip Key: $GOSSIP_KEY"

# Generate service token for each application
MYAPP_TOKEN=$(openssl rand -base64 32)
echo "MyApp Token: $MYAPP_TOKEN"
```

### Step 2: Deploy Overwatch Nodes

Create `/etc/opengslb/overwatch.yaml`:

```yaml
mode: overwatch

identity:
  node_id: overwatch-us-east-1
  region: us-east

dns:
  listen_address: "0.0.0.0:53"
  zones:
    - gslb.example.com
  default_ttl: 30

dnssec:
  enabled: true
  key_sync:
    peers:
      - "https://overwatch-2.internal:9090"
      - "https://overwatch-3.internal:9090"
    poll_interval: "1h"

# Service tokens - agents must present matching token
agent_tokens:
  myapp: "${MYAPP_TOKEN}"
  otherapp: "${OTHERAPP_TOKEN}"

gossip:
  bind_address: "0.0.0.0:7946"
  encryption_key: "${GOSSIP_KEY}"  # REQUIRED

validation:
  enabled: true
  check_interval: 30s
  check_timeout: 5s

stale:
  threshold: 30s      # Mark stale after 30s no heartbeat
  remove_after: 5m    # Remove backend after 5m stale

api:
  address: "0.0.0.0:9090"
  allowed_networks:
    - 10.0.0.0/8
    - 192.168.0.0/16

metrics:
  enabled: true
  address: "0.0.0.0:9091"

data_dir: /var/lib/opengslb

logging:
  level: info
  format: json
```

Start Overwatch:

```bash
# Build from source
go build -o opengslb ./cmd/opengslb

# Run as systemd service
./opengslb --config /etc/opengslb/overwatch.yaml
```

### Step 3: Deploy Agents

Create `/etc/opengslb/agent.yaml` on each application server:

```yaml
mode: agent

identity:
  service_token: "${MYAPP_TOKEN}"
  region: us-east
  # Certificate auto-generated on first start at /var/lib/opengslb/

backends:
  - service: myapp
    address: 127.0.0.1
    port: 8080
    weight: 100
    health_check:
      type: http
      path: /health
      interval: 5s
      timeout: 2s
      failure_threshold: 3
      success_threshold: 2

predictive:
  enabled: true
  cpu_threshold: 85
  memory_threshold: 90
  error_rate_threshold: 5
  check_interval: 10s

gossip:
  encryption_key: "${GOSSIP_KEY}"  # Must match Overwatch
  overwatch_nodes:
    - overwatch-1.internal:7946
    - overwatch-2.internal:7946
    - overwatch-3.internal:7946

heartbeat:
  interval: 10s
  missed_threshold: 3

data_dir: /var/lib/opengslb

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: "127.0.0.1:9100"  # Local only for agent metrics
```

Start Agent:

```bash
./opengslb --config /etc/opengslb/agent.yaml
```

### Step 4: Configure DNS Clients

Configure client `/etc/resolv.conf`:

```
nameserver 10.0.1.53
nameserver 10.0.1.54
nameserver 10.0.1.55
options timeout:2 attempts:3
```

Or for corporate networks, configure your DNS server to forward GSLB zones:

**BIND example** (`named.conf`):
```
zone "gslb.example.com" {
    type forward;
    forward only;
    forwarders {
        10.0.1.53;
        10.0.1.54;
        10.0.1.55;
    };
};
```

## Multi-Backend Agent Configuration

An agent can register multiple backends (services):

```yaml
mode: agent

identity:
  service_token: "${TOKEN}"
  region: us-east

backends:
  - service: web
    address: 127.0.0.1
    port: 8080
    weight: 100
    health_check:
      type: http
      path: /health
      interval: 5s
      timeout: 2s

  - service: api
    address: 127.0.0.1
    port: 9090
    weight: 100
    health_check:
      type: http
      path: /api/health
      interval: 5s
      timeout: 2s

  - service: grpc
    address: 127.0.0.1
    port: 50051
    weight: 100
    health_check:
      type: tcp
      interval: 10s
      timeout: 3s

gossip:
  encryption_key: "${GOSSIP_KEY}"
  overwatch_nodes:
    - overwatch-1.internal:7946
```

## Health Authority Hierarchy

Overwatch uses a priority-based health determination:

| Priority | Source | Description |
|----------|--------|-------------|
| 1 (highest) | Manual Override | Via API, persists until cleared |
| 2 | External Tool | CloudWatch, Watcher integration |
| 3 | Overwatch Validation | External health check by Overwatch |
| 4 (lowest) | Agent Claim | Agent's local health check |

**Key behavior**: Overwatch validation ALWAYS wins over agent claims. This prevents lying agents from serving traffic.

### Stale Backend Recovery

If an agent stops sending heartbeats but the backend service is still healthy:

1. Backend marked stale after `stale.threshold` (default: 30s)
2. Overwatch external validation continues checking stale backends
3. If validation succeeds, backend is recovered to healthy status
4. Backend only removed after `stale.remove_after` (default: 5m)

## External Override API

External tools can override health state:

```bash
# Mark backend unhealthy
curl -X PUT http://overwatch:9090/api/v1/overrides/myapp/10.0.1.10 \
  -H "Content-Type: application/json" \
  -d '{"healthy": false, "reason": "High latency from CloudWatch"}'

# Clear override
curl -X DELETE http://overwatch:9090/api/v1/overrides/myapp/10.0.1.10

# List all overrides
curl http://overwatch:9090/api/v1/overrides
```

## DNSSEC Configuration

DNSSEC is enabled by default. To get DS records for parent zone delegation:

```bash
curl http://overwatch:9090/api/v1/dnssec/ds
```

Response:
```json
{
  "zone": "gslb.example.com",
  "ds_records": [
    {
      "key_tag": 12345,
      "algorithm": 13,
      "digest_type": 2,
      "digest": "abc123...",
      "ds_record": "gslb.example.com. IN DS 12345 13 2 abc123..."
    }
  ]
}
```

To disable DNSSEC (not recommended):

```yaml
dnssec:
  enabled: false
  security_acknowledgment: "I understand that disabling DNSSEC allows DNS spoofing attacks"
```

## Monitoring

### Prometheus Metrics

**Agent metrics** (port 9100):
- `opengslb_agent_backends_registered` - Number of backends registered
- `opengslb_agent_heartbeats_sent_total` - Heartbeats sent
- `opengslb_agent_heartbeat_failures_total` - Failed heartbeats
- `opengslb_predictive_bleeding` - Predictive health signal

**Overwatch metrics** (port 9091):
- `opengslb_overwatch_agents_registered` - Registered agents
- `opengslb_overwatch_backends_total` - Total backends
- `opengslb_overwatch_backends_healthy` - Healthy backends
- `opengslb_overwatch_stale_agents_total` - Stale agents
- `opengslb_overwatch_validation_checks_total` - Validation checks
- `opengslb_dns_queries_total` - DNS queries served

### Alerting Examples

```yaml
# Prometheus alerting rules
groups:
  - name: opengslb
    rules:
      - alert: HighStaleAgents
        expr: opengslb_overwatch_stale_agents_total > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Agents are stale"

      - alert: LowHealthyBackends
        expr: opengslb_overwatch_backends_healthy < 2
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Less than 2 healthy backends"
```

## Troubleshooting

### Agent not registering

1. Check gossip connectivity:
   ```bash
   nc -zv overwatch-1.internal 7946
   ```

2. Verify encryption key matches between agent and Overwatch

3. Check service token matches `agent_tokens` in Overwatch config

4. Check agent logs:
   ```bash
   journalctl -u opengslb-agent -f
   ```

### Backend marked stale

1. Check agent is running and sending heartbeats
2. Check heartbeat metrics: `opengslb_agent_heartbeats_sent_total`
3. Check network connectivity between agent and Overwatch
4. Overwatch external validation may recover stale backends if service is actually healthy

### DNS not resolving

1. Verify Overwatch is serving DNS:
   ```bash
   dig @overwatch-1.internal myapp.gslb.example.com
   ```

2. Check registered backends:
   ```bash
   curl http://overwatch:9090/api/v1/backends
   ```

3. Check healthy backends:
   ```bash
   curl http://overwatch:9090/api/v1/backends/healthy
   ```

### DNSSEC validation failing

1. Verify DS records are published in parent zone
2. Check key sync between Overwatches:
   ```bash
   curl http://overwatch:9090/api/v1/dnssec/sync/status
   ```

## Security Checklist

- [ ] Gossip encryption key is securely stored and rotated periodically
- [ ] Service tokens are unique per application
- [ ] API endpoints are IP-restricted
- [ ] DNSSEC is enabled
- [ ] Agent certificates are stored with appropriate permissions
- [ ] Overwatch nodes are in private network
- [ ] Metrics endpoints are not publicly exposed

## Migration from Legacy Mode

If migrating from `--mode=standalone` (Sprint 3 and earlier):

1. Your existing configuration with `regions` and `servers` still works
2. For dynamic registration, deploy agents alongside your applications
3. Overwatches will serve backends from both static config and agent registration
4. Gradually migrate to agent-based registration for full features

---

**Document Version**: 1.0
**Last Updated**: December 2025
