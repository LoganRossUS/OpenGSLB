# OpenGSLB Cluster Mode

OpenGSLB supports two runtime modes: **standalone** (single-node) and **cluster** (distributed high-availability). This document describes how to configure and operate cluster mode.

> **Note**: Cluster mode is being implemented in Sprint 4. This document describes the target functionality. Check PROGRESS.md for current implementation status.

## Overview

| Aspect | Standalone | Cluster |
|--------|------------|---------|
| Nodes | 1 | 3-7 (odd number) |
| High Availability | No | Yes (Raft consensus) |
| Use Case | Development, simple deployments | Production, enterprise |
| DNS Serving | Always active | Leader only |
| Health Checks | Local perspective | Distributed + gossip |

## Quick Start

### Standalone Mode (Default)

No special configuration needed. This is the default behavior:

```bash
opengslb --config /etc/opengslb/config.yaml
```

### Cluster Mode

**Node 1 (Bootstrap the cluster):**
```bash
opengslb --mode=cluster --bootstrap --config /etc/opengslb/config.yaml
```

**Node 2 & 3 (Join existing cluster):**
```bash
opengslb --mode=cluster --join=10.0.1.10:7946 --config /etc/opengslb/config.yaml
```

## Command-Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--mode` | Runtime mode: `standalone` or `cluster` | `standalone` |
| `--bootstrap` | Initialize a new cluster (first node only) | `false` |
| `--join` | Comma-separated list of existing cluster nodes | (none) |
| `--config` | Path to configuration file | `/etc/opengslb/config.yaml` |

## Configuration

### Cluster Settings

```yaml
cluster:
  # Runtime mode (can also use --mode flag)
  mode: cluster
  
  # Unique node name (defaults to hostname)
  node_name: "gslb-node-1"
  
  # Address for Raft/gossip communication
  bind_address: "10.0.1.10:7946"
  
  # Address advertised to peers (optional, defaults to bind_address)
  advertise_address: "10.0.1.10:7946"
  
  # Bootstrap new cluster (can also use --bootstrap flag)
  bootstrap: false
  
  # Nodes to join (can also use --join flag)
  join:
    - "10.0.1.11:7946"
    - "10.0.1.12:7946"
  
  # Raft consensus settings
  raft:
    data_dir: "/var/lib/opengslb/raft"
    heartbeat_timeout: "1s"
    election_timeout: "1s"
    snapshot_interval: "120s"
    snapshot_threshold: 8192
  
  # Gossip settings
  gossip:
    encryption_key: ""  # Optional: 32-byte base64 key
  
  # Anycast VIP (all nodes advertise, leader responds)
  anycast_vip: "10.99.99.1"
```

### Flag vs Config Precedence

Command-line flags override configuration file values:

1. `--mode` overrides `cluster.mode`
2. `--bootstrap` overrides `cluster.bootstrap`
3. `--join` overrides `cluster.join`

## Cluster Deployment

### Prerequisites

- 3, 5, or 7 nodes (odd number required for Raft quorum)
- Network connectivity between all nodes on gossip port (default: 7946)
- Shared configuration file (same domains, regions, etc.)
- Unique `node_name` per node (or use hostname)

### Deployment Steps

1. **Prepare configuration** on all nodes with identical domain/region settings

2. **Start first node** with bootstrap:
   ```bash
   opengslb --mode=cluster --bootstrap --config /etc/opengslb/config.yaml
   ```

3. **Wait for first node** to initialize and become leader

4. **Start remaining nodes** with join:
   ```bash
   opengslb --mode=cluster --join=<first-node-ip>:7946 --config /etc/opengslb/config.yaml
   ```

5. **Verify cluster health** via API or logs

### Verifying Cluster Status

Check cluster membership and leader status:

```bash
# Via API (when implemented)
curl http://localhost:8080/api/v1/cluster/status

# Via logs
journalctl -u opengslb | grep -i "leader\|raft\|cluster"
```

## High Availability Behavior

### Leader Election

- Raft consensus elects a single leader
- Only the leader responds to DNS queries
- Followers forward or refuse queries
- Election completes within 1-3 seconds

### Failover Scenarios

| Scenario | Behavior | Recovery Time |
|----------|----------|---------------|
| Leader crashes | New election, new leader serves DNS | ≤2s |
| Network partition | Minority loses quorum, stops serving | Immediate |
| Leader unhealthy | Demoted, new election | ≤2s |

### Split-Brain Prevention

Raft requires a quorum (majority) to elect a leader:
- 3 nodes: requires 2 for quorum
- 5 nodes: requires 3 for quorum
- 7 nodes: requires 4 for quorum

If the cluster partitions, only the partition with quorum can serve DNS. The minority partition will refuse queries, preventing inconsistent responses.

## Anycast VIP

In cluster mode, all nodes can advertise the same virtual IP (anycast). Only the Raft leader responds to DNS queries on this VIP.

### How It Works

1. All cluster nodes bind the anycast VIP (e.g., `10.99.99.1`)
2. Network routes traffic to nearest node
3. Non-leaders return `REFUSED` or drop packets
4. When leadership changes, new leader immediately responds

### Configuration

```yaml
cluster:
  anycast_vip: "10.99.99.1"
```

### Network Setup

Configure your network to route the anycast VIP to all cluster nodes. This typically involves:
- BGP announcements from each node
- Or load balancer health checks that detect leader status

## Monitoring

### Prometheus Metrics

Cluster mode exposes additional metrics:

```
# Raft state
opengslb_cluster_is_leader{node="gslb-node-1"} 1
opengslb_cluster_raft_state{node="gslb-node-1"} leader

# Cluster membership
opengslb_cluster_members_total 3
opengslb_cluster_healthy_members 3

# Leader elections
opengslb_cluster_elections_total 2
opengslb_cluster_last_election_timestamp 1704067200
```

### Health API

```bash
# Cluster status endpoint
curl http://localhost:8080/api/v1/cluster/status
```

```json
{
  "mode": "cluster",
  "node_name": "gslb-node-1",
  "is_leader": true,
  "leader_address": "10.0.1.10:7946",
  "members": [
    {"name": "gslb-node-1", "address": "10.0.1.10:7946", "status": "alive"},
    {"name": "gslb-node-2", "address": "10.0.1.11:7946", "status": "alive"},
    {"name": "gslb-node-3", "address": "10.0.1.12:7946", "status": "alive"}
  ],
  "raft": {
    "state": "Leader",
    "term": 5,
    "commit_index": 1234
  }
}
```

## Troubleshooting

### Node Won't Join Cluster

1. **Check network connectivity**: `nc -zv <leader-ip> 7946`
2. **Verify bind_address** is reachable from other nodes
3. **Check firewall rules** for port 7946 (TCP and UDP)
4. **Review logs**: `journalctl -u opengslb | grep -i error`

### Cluster Stuck Without Leader

1. **Check quorum**: At least (N/2)+1 nodes must be healthy
2. **Verify Raft data directory** permissions
3. **Check for clock skew** between nodes
4. **Force new election** by restarting one node

### DNS Queries Failing

1. **Verify leader status**: Check `/api/v1/cluster/status`
2. **Check anycast VIP routing** to leader node
3. **Review DNS listener logs** for errors

## Migration

### Standalone → Cluster

1. Deploy two additional nodes
2. Stop the standalone node
3. Restart it with `--mode=cluster --bootstrap`
4. Join other nodes with `--mode=cluster --join=...`

### Cluster → Standalone

1. Drain traffic from cluster
2. Stop all nodes except one
3. Restart remaining node with `--mode=standalone`

## Feature Availability by Mode

| Feature | Standalone | Cluster |
|---------|------------|---------|
| DNS serving | ✅ | ✅ (leader only) |
| All routing algorithms | ✅ | ✅ |
| HTTP/TCP health checks | ✅ | ✅ |
| Hot reload (SIGHUP) | ✅ | ✅ |
| Prometheus metrics | ✅ | ✅ |
| Health status API | ✅ | ✅ |
| Predictive health signals | ❌ | ✅ |
| External health veto | ❌ | ✅ |
| Dynamic service registration | ❌ | ✅ |
| Automatic failover | ❌ | ✅ |
| Multi-node coordination | ❌ | ✅ |

## See Also

- [ADR-012: Distributed Agent Architecture](ARCHITECTURE_DECISIONS.md#adr-012)
- [ADR-013: Hybrid Configuration & KV Store](ARCHITECTURE_DECISIONS.md#adr-013)
- [ADR-014: Runtime Mode Semantics](ARCHITECTURE_DECISIONS.md#adr-014)
- [Operations: Deployment Guide](operations/deployment.md)