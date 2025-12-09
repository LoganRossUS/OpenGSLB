# Gossip Protocol

OpenGSLB uses the gossip protocol for fast propagation of health events across cluster nodes. This document describes the gossip architecture, message types, and configuration.

## Overview

The gossip protocol is built on [hashicorp/memberlist](https://github.com/hashicorp/memberlist), providing:

- **Fast event propagation**: Health updates reach all nodes within 500ms
- **Membership detection**: Automatic detection of node joins and leaves
- **Encryption support**: Optional AES-256 encryption for gossip traffic
- **Failure detection**: SWIM-based protocol detects node failures quickly

## Architecture

```
\u250c\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2510
\u2502                        Gossip Cluster                           \u2502
\u2502                                                                 \u2502
\u2502  \u250c\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2510      \u250c\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2510      \u250c\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2510     \u2502
\u2502  \u2502   Node 1    \u2502\u25c4\u2500\u2500\u2500\u2500\u25ba\u2502   Node 2    \u2502\u25c4\u2500\u2500\u2500\u2500\u25ba\u2502   Node 3    \u2502     \u2502
\u2502  \u2502  (Leader)   \u2502      \u2502  (Follower) \u2502      \u2502  (Follower) \u2502     \u2502
\u2502  \u2502             \u2502      \u2502             \u2502      \u2502             \u2502      \u2502
\u2502  \u2502 Health Mgr  \u2502      \u2502 Health Mgr  \u2502      \u2502 Health Mgr  \u2502     \u2502
\u2502  \u2502     \u2502       \u2502      \u2502     \u2502       \u2502      \u2502     \u2502       \u2502     \u2502
\u2502  \u2502     \u25bc       \u2502      \u2502     \u25bc       \u2502      \u2502     \u25bc       \u2502     \u2502
\u2502  \u2502  Gossip \u25c4\u2500\u2500\u2500\u253c\u2500\u2500\u2500\u2500\u2500\u2500\u253c\u2500\u2500\u25ba Gossip \u25c4\u2500\u253c\u2500\u2500\u2500\u2500\u2500\u2500\u253c\u2500\u2500\u25ba Gossip   \u2502     \u2502
\u2502  \u2514\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2518      \u2514\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2518      \u2514\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2518     \u2502
\u2514\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2518
```

### Relationship with Raft

- **Raft**: Used for leader election and replicated state (KV store)
- **Gossip**: Used for fast, eventually-consistent health event propagation

Gossip complements Raft by providing:
- Sub-second health event propagation (vs. Raft's consistency guarantees)
- Reduced load on the Raft leader
- Continued health awareness even during leader elections

## Message Types

### Health Update (`health_update`)

Sent when a server's health status changes.

```json
{
  "type": "health_update",
  "node_id": "gslb-node-1",
  "timestamp": "2025-04-08T10:30:00Z",
  "payload": {
    "server_addr": "10.0.1.10:80",
    "region": "us-east-1",
    "healthy": false,
    "latency": 45000000,
    "error": "connection refused",
    "check_type": "http"
  }
}
```

### Predictive Signal (`predictive`)

Sent when an agent predicts an impending failure ("predictive from the inside").

```json
{
  "type": "predictive",
  "node_id": "gslb-node-1",
  "timestamp": "2025-04-08T10:30:00Z",
  "payload": {
    "node_id": "gslb-node-1",
    "signal": "bleed",
    "reason": "cpu_high",
    "value": 92.5,
    "threshold": 90.0
  }
}
```

**Signal types:**
- `bleed`: Gradual degradation, reduce traffic slowly
- `drain`: Prepare for shutdown, stop accepting new connections
- `critical`: Immediate action required

**Reason codes:**
- `cpu_high`: CPU utilization above threshold
- `memory_pressure`: Memory usage above threshold
- `error_rate`: Error rate above threshold
- `latency_high`: Response latency above threshold

### Override Command (`override`)

Sent by overwatch nodes to override health status ("reactive from the outside").

```json
{
  "type": "override",
  "node_id": "overwatch-1",
  "timestamp": "2025-04-08T10:30:00Z",
  "payload": {
    "target_node": "gslb-node-1",
    "server_addr": "10.0.1.10:80",
    "action": "force_unhealthy",
    "reason": "external validation failed",
    "expiry": 1712577000
  }
}
```

**Action types:**
- `force_healthy`: Override to healthy status
- `force_unhealthy`: Override to unhealthy status
- `clear`: Remove override, use local health check result

### Node State (`node_state`)

Periodic full state synchronization during push/pull.

```json
{
  "type": "node_state",
  "node_id": "gslb-node-1",
  "timestamp": "2025-04-08T10:30:00Z",
  "payload": {
    "node_id": "gslb-node-1",
    "is_leader": true,
    "uptime": 86400000000000,
    "health_states": [
      {
        "server_addr": "10.0.1.10:80",
        "region": "us-east-1",
        "healthy": true,
        "last_check": "2025-04-08T10:29:55Z",
        "last_latency": 45000000,
        "consecutive_fails": 0
      }
    ]
  }
}
```

## Configuration

### Basic Configuration

```yaml
cluster:
  mode: cluster
  node_name: "gslb-node-1"
  bind_address: "10.0.1.10:7946"
  
  gossip:
    enabled: true
    bind_port: 7947
```

### Full Configuration

```yaml
cluster:
  gossip:
    # Enable gossip (default: true in cluster mode)
    enabled: true
    
    # Port for gossip communication
    bind_port: 7947
    advertise_port: 7947
    
    # Encryption key (32 bytes, base64 encoded)
    # Generate: head -c 32 /dev/urandom | base64
    encryption_key: "your-base64-encoded-key"
    
    # Failure detection timing
    probe_interval: "1s"      # Time between probes
    probe_timeout: "500ms"    # Probe timeout
    
    # Message propagation timing
    gossip_interval: "200ms"  # Time between gossip rounds
    push_pull_interval: "30s" # Time between full syncs
```

### Encryption

To enable encryption, generate a 32-byte key:

```bash
# Generate key
head -c 32 /dev/urandom | base64

# Example output: xK7dQm9pR8vLnM3wYhA2cE5fG6jN1sU4tB0oZiXeHrI=
```

Add to all nodes' configuration:

```yaml
cluster:
  gossip:
    encryption_key: "xK7dQm9pR8vLnM3wYhA2cE5fG6jN1sU4tB0oZiXeHrI="
```

**Important**: All nodes must use the same encryption key.

## Metrics

Gossip exposes the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `opengslb_gossip_members_total` | Gauge | Total gossip cluster members |
| `opengslb_gossip_healthy_members` | Gauge | Healthy (alive) members |
| `opengslb_gossip_messages_received_total` | Counter | Messages received by type |
| `opengslb_gossip_messages_sent_total` | Counter | Messages sent by type |
| `opengslb_gossip_message_send_failures_total` | Counter | Failed message sends |
| `opengslb_gossip_node_joins_total` | Counter | Node join events |
| `opengslb_gossip_node_leaves_total` | Counter | Node leave events |
| `opengslb_gossip_health_updates_received_total` | Counter | Health updates received |
| `opengslb_gossip_health_updates_broadcast_total` | Counter | Health updates sent |
| `opengslb_gossip_predictive_signals_total` | Counter | Predictive signals by type |
| `opengslb_gossip_overrides_total` | Counter | Override commands by action |
| `opengslb_gossip_propagation_latency_seconds` | Histogram | Message propagation latency |

## Propagation Latency

With default settings, health events propagate to all nodes within **500ms**:

| Cluster Size | Typical Propagation Time |
|--------------|--------------------------|
| 3 nodes | ~200ms |
| 5 nodes | ~300ms |
| 7 nodes | ~400ms |
| 10+ nodes | ~500ms |

To achieve faster propagation, reduce `gossip_interval`:

```yaml
cluster:
  gossip:
    gossip_interval: "100ms"  # Faster propagation, more network traffic
```

## Troubleshooting

### Nodes Not Discovering Each Other

1. Check firewall rules allow UDP/TCP on gossip port
2. Verify `bind_address` is reachable from other nodes
3. Check `advertise_address` if using NAT

```bash
# Test connectivity
nc -vz 10.0.1.10 7947
```

### High Message Send Failures

Check the `opengslb_gossip_message_send_failures_total` metric:

1. Network connectivity issues
2. Target node is down or unreachable
3. Encryption key mismatch

### Encryption Key Mismatch

If nodes can't communicate after enabling encryption:

```
WARN gossip: failed to decode gossip message error="cipher: message authentication failed"
```

Ensure all nodes use the same `encryption_key` value.

### Health Updates Not Propagating

1. Check `opengslb_gossip_health_updates_broadcast_total` is incrementing
2. Check `opengslb_gossip_messages_sent_total{type="health_update"}` 
3. Verify health manager's `OnStatusChange` callback is configured

## Best Practices

1. **Use encryption in production**: Always enable gossip encryption
2. **Monitor propagation latency**: Alert if P99 exceeds 1 second
3. **Match cluster topology**: Gossip seeds should match Raft join addresses
4. **Size appropriately**: Gossip scales well to 100+ nodes, but latency increases
5. **Separate ports**: Use different ports for Raft (consensus) and gossip (events)