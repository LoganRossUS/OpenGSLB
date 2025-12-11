# Gossip Protocol

OpenGSLB uses the gossip protocol for communication between Agents and Overwatch nodes. This document describes the gossip architecture, message types, and configuration.

## Overview

The gossip protocol is built on [hashicorp/memberlist](https://github.com/hashicorp/memberlist), providing:

- **Fast event propagation**: Health updates reach Overwatch within 500ms
- **Encrypted communication**: Required AES-256 encryption for gossip traffic
- **Failure detection**: SWIM-based protocol detects agent failures quickly
- **Heartbeat mechanism**: Agents send periodic heartbeats to maintain registration

## Architecture (ADR-015)

```
                    ┌─────────────────────────────────────────────────────────┐
                    │              Overwatch Nodes (DNS Authority)             │
                    │                                                          │
                    │   ┌──────────────┐         ┌──────────────┐             │
                    │   │ Overwatch-1  │         │ Overwatch-2  │             │
                    │   │              │         │              │             │
                    │   │ ┌──────────┐ │         │ ┌──────────┐ │             │
                    │   │ │  Gossip  │ │         │ │  Gossip  │ │             │
                    │   │ │ Receiver │ │         │ │ Receiver │ │             │
                    │   │ └────▲─────┘ │         │ └────▲─────┘ │             │
                    │   └──────┼───────┘         └──────┼───────┘             │
                    │          │                        │                      │
                    └──────────┼────────────────────────┼──────────────────────┘
                               │                        │
         ┌─────────────────────┼────────────────────────┼─────────────────────┐
         │                     │                        │                      │
         │            Gossip Messages (Encrypted)                             │
         │                     │                        │                      │
         │    ┌────────────────┼────────────────────────┼────────────────┐    │
         │    │                │                        │                │    │
         │    ▼                ▼                        ▼                ▼    │
    ┌─────────────┐    ┌─────────────┐          ┌─────────────┐   ┌─────────────┐
    │  Agent-1    │    │  Agent-2    │          │  Agent-3    │   │  Agent-4    │
    │ (App Server)│    │ (App Server)│          │ (App Server)│   │ (App Server)│
    │             │    │             │          │             │   │             │
    │ ┌─────────┐ │    │ ┌─────────┐ │          │ ┌─────────┐ │   │ ┌─────────┐ │
    │ │ Health  │ │    │ │ Health  │ │          │ │ Health  │ │   │ │ Health  │ │
    │ │ Monitor │ │    │ │ Monitor │ │          │ │ Monitor │ │   │ │ Monitor │ │
    │ └─────────┘ │    │ └─────────┘ │          │ └─────────┘ │   │ └─────────┘ │
    └─────────────┘    └─────────────┘          └─────────────┘   └─────────────┘
         │                   │                        │                  │
         ▼                   ▼                        ▼                  ▼
    [Backend Svc]      [Backend Svc]            [Backend Svc]      [Backend Svc]
```

### Communication Flow

1. **Agents** run on application servers alongside backends
2. **Agents** monitor local backend health
3. **Agents** send heartbeat messages to Overwatch nodes via gossip
4. **Overwatch** receives heartbeats and maintains backend registry
5. **Overwatch** optionally validates health claims independently

There is **no Agent-to-Agent communication**. Each Agent connects directly to Overwatch nodes.

## Message Types

### Heartbeat (`heartbeat`)

Sent periodically by agents to register backends and report health status.

```json
{
  "type": "heartbeat",
  "agent_id": "agent-abc123",
  "timestamp": "2025-04-08T10:30:00Z",
  "payload": {
    "agent_id": "agent-abc123",
    "region": "us-east-1",
    "fingerprint": "sha256:...",
    "backends": [
      {
        "service": "web-service",
        "address": "10.0.1.10",
        "port": 80,
        "weight": 100,
        "healthy": true,
        "latency_ms": 45,
        "last_check": "2025-04-08T10:29:55Z"
      }
    ]
  }
}
```

### Predictive Signal (`predictive`)

Sent when an agent predicts an impending failure based on resource metrics.

```json
{
  "type": "predictive",
  "agent_id": "agent-abc123",
  "timestamp": "2025-04-08T10:30:00Z",
  "payload": {
    "agent_id": "agent-abc123",
    "service": "web-service",
    "address": "10.0.1.10",
    "port": 80,
    "signal": "bleed",
    "reason": "cpu_high",
    "value": 92.5,
    "threshold": 90.0,
    "bleed_weight": 50
  }
}
```

**Signal types:**
- `bleed`: Gradual degradation, reduce traffic weight
- `drain`: Prepare for shutdown, stop accepting new traffic
- `recovered`: Signal cleared, return to normal operation

**Reason codes:**
- `cpu_high`: CPU utilization above threshold
- `memory_pressure`: Memory usage above threshold
- `error_rate`: Error rate above threshold

### Deregister (`deregister`)

Sent by agent during graceful shutdown to remove backends from registry.

```json
{
  "type": "deregister",
  "agent_id": "agent-abc123",
  "timestamp": "2025-04-08T10:30:00Z",
  "payload": {
    "agent_id": "agent-abc123",
    "reason": "shutdown"
  }
}
```

## Configuration

### Agent Gossip Configuration

```yaml
mode: agent

agent:
  gossip:
    # Required: 32-byte base64-encoded encryption key
    encryption_key: "xK7dQm9pR8vLnM3wYhA2cE5fG6jN1sU4tB0oZiXeHrI="

    # Required: Overwatch nodes to connect to
    overwatch_nodes:
      - "overwatch-1.internal:7946"
      - "overwatch-2.internal:7946"
```

### Overwatch Gossip Configuration

```yaml
mode: overwatch

overwatch:
  gossip:
    # Address to bind for receiving gossip
    bind_address: "0.0.0.0:7946"

    # Required: Must match agent encryption key
    encryption_key: "xK7dQm9pR8vLnM3wYhA2cE5fG6jN1sU4tB0oZiXeHrI="

    # Failure detection timing
    probe_interval: 1s
    probe_timeout: 500ms

    # Gossip message timing
    gossip_interval: 200ms
```

### Encryption

Encryption is **required** in production. Generate a 32-byte key:

```bash
# Generate key
openssl rand -base64 32

# Example output: xK7dQm9pR8vLnM3wYhA2cE5fG6jN1sU4tB0oZiXeHrI=
```

**Important**: All Agents and Overwatch nodes must use the same encryption key.

## Metrics

Gossip exposes the following Prometheus metrics:

### Agent Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `opengslb_gossip_heartbeats_sent_total` | Counter | Heartbeats sent to Overwatch |
| `opengslb_gossip_heartbeat_failures_total` | Counter | Failed heartbeat attempts |
| `opengslb_gossip_predictive_signals_total` | Counter | Predictive signals sent |
| `opengslb_gossip_connected_overwatches` | Gauge | Connected Overwatch nodes |

### Overwatch Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `opengslb_gossip_messages_received_total` | Counter | Messages received by type |
| `opengslb_gossip_heartbeats_received_total` | Counter | Heartbeats received from agents |
| `opengslb_gossip_agents_registered` | Gauge | Currently registered agents |
| `opengslb_gossip_message_processing_errors_total` | Counter | Message processing errors |

## Heartbeat Behavior

### Interval and Timeout

```yaml
agent:
  heartbeat:
    interval: 10s         # Send heartbeat every 10 seconds
    missed_threshold: 3   # Deregistered after 3 missed heartbeats (30s)
```

### Staleness Detection

Overwatch marks backends as stale based on heartbeat activity:

```yaml
overwatch:
  stale:
    threshold: 30s      # Mark stale after 30s without heartbeat
    remove_after: 5m    # Remove backend after 5m stale
```

**Status progression:**
1. **Healthy/Unhealthy**: Recent heartbeat from agent
2. **Stale**: No heartbeat within `stale.threshold`
3. **Removed**: No heartbeat within `stale.remove_after`

## Troubleshooting

### Agent Cannot Connect to Overwatch

1. Check firewall rules allow TCP/UDP on gossip port (default: 7946)
2. Verify Overwatch bind address is reachable from agent
3. Test connectivity:

```bash
# From agent server
nc -zv overwatch-ip 7946
```

### Encryption Key Mismatch

If agents can't communicate with Overwatch:

```
WARN gossip: failed to decode gossip message error="cipher: message authentication failed"
```

Ensure all nodes use the same `encryption_key` value.

### Backends Going Stale

1. Check agent process is running: `systemctl status opengslb`
2. Check agent metrics: `curl http://localhost:9090/metrics | grep gossip`
3. Verify network connectivity to Overwatch
4. Review agent logs: `journalctl -u opengslb | grep gossip`

### High Heartbeat Failures

Check the `opengslb_gossip_heartbeat_failures_total` metric:

1. Network connectivity issues between agent and Overwatch
2. Overwatch node is down or unreachable
3. Encryption key mismatch
4. Overwatch gossip port not listening

## Best Practices

1. **Always use encryption**: Gossip encryption is required in production
2. **Multiple Overwatch nodes**: Configure agents to connect to multiple Overwatch nodes for redundancy
3. **Monitor heartbeat metrics**: Alert if `opengslb_gossip_heartbeat_failures_total` increases
4. **Tune stale thresholds**: Balance between quick detection and avoiding false positives
5. **Separate networks**: Consider using a management network for gossip traffic

## See Also

- [ADR-015: Agent-Overwatch Architecture](ARCHITECTURE_DECISIONS.md#adr-015)
- [Troubleshooting: Agent Mode Issues](troubleshooting.md#agent-mode-issues)
- [Troubleshooting: Overwatch Mode Issues](troubleshooting.md#overwatch-mode-issues)
