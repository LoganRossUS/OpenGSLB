# OpenGSLB Cluster Deployment Guide

This guide covers deploying OpenGSLB in cluster mode for high availability across multiple nodes, data centers, or cloud regions.

## Overview

OpenGSLB cluster mode provides:

- **Automatic failover**: If the leader node fails, a new leader is elected within ~2 seconds
- **Distributed health checking**: Health events propagate via gossip protocol
- **Predictive health**: Agents can signal impending failures before they impact traffic
- **External validation**: Overwatch nodes validate agent health claims

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Anycast VIP: 10.99.99.1                     │
└───────────────────────────┬─────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│   Node 1      │   │   Node 2      │   │   Node 3      │
│  (Leader)     │   │  (Follower)   │   │  (Follower)   │
│               │   │               │   │               │
│ ┌───────────┐ │   │ ┌───────────┐ │   │ ┌───────────┐ │
│ │Raft State │ │   │ │Raft State │ │   │ │Raft State │ │
│ └───────────┘ │   │ └───────────┘ │   │ └───────────┘ │
│       │       │   │       │       │   │       │       │
│ ┌───────────┐ │   │ ┌───────────┐ │   │ ┌───────────┐ │
│ │  Gossip   │◄┼───┼─┤  Gossip   │◄┼───┼─┤  Gossip   │ │
│ └───────────┘ │   │ └───────────┘ │   │ └───────────┘ │
│       │       │   │       │       │   │       │       │
│ ┌───────────┐ │   │ ┌───────────┐ │   │ ┌───────────┐ │
│ │DNS Server │ │   │ │DNS Server │ │   │ │DNS Server │ │
│ │(ACTIVE)   │ │   │ │(REFUSED)  │ │   │ │(REFUSED)  │ │
│ └───────────┘ │   │ └───────────┘ │   │ └───────────┘ │
└───────────────┘   └───────────────┘   └───────────────┘
```

## Prerequisites

- **Nodes**: 3 or 5 nodes (odd number for Raft quorum)
- **Network**: All nodes must be able to reach each other on:
  - Raft port (default: 7000)
  - Gossip port (default: 7946)
  - DNS port (53/udp, 53/tcp)
  - API port (default: 9090)
- **Disk**: ~500MB per node for Raft state and logs
- **Time sync**: NTP configured (Raft is sensitive to clock drift)

## Cluster Sizing

| Cluster Size | Fault Tolerance | Recommended For |
|--------------|-----------------|-----------------|
| 3 nodes | 1 node failure | Most deployments |
| 5 nodes | 2 node failures | High availability |
| 7 nodes | 3 node failures | Mission critical |

## Step-by-Step Deployment

### Step 1: Prepare Configuration Files

Create a base configuration file that's shared across nodes. Node-specific settings will be provided via command-line flags or environment variables.

**`/etc/opengslb/config.yaml`** (base configuration):

```yaml
dns:
  listen_address: ":53"
  default_ttl: 60

regions:
  - name: us-east-1
    servers:
      - address: "10.0.1.10"
        port: 80
        weight: 100
      - address: "10.0.1.11"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 10s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2

  - name: us-west-2
    servers:
      - address: "10.0.2.10"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 10s
      timeout: 5s
      path: /health

domains:
  - name: app.example.com
    routing_algorithm: weighted
    regions:
      - us-east-1
      - us-west-2
    ttl: 30

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9090"

api:
  enabled: true
  address: ":8080"
  allowed_networks:
    - "10.0.0.0/8"
    - "172.16.0.0/12"

# Cluster configuration
cluster:
  mode: cluster
  
  raft:
    data_dir: /var/lib/opengslb/raft
    heartbeat_timeout: 1s
    election_timeout: 1s
    
  gossip:
    probe_interval: 1s
    probe_timeout: 500ms
    
  overwatch:
    external_check_interval: 30s
    veto_mode: balanced
    veto_threshold: 3
    
  predictive_health:
    enabled: true
    cpu:
      threshold: 80
    memory:
      threshold: 85
    error_rate:
      threshold: 5
      window: 60s
```

### Step 2: Bootstrap First Node

On the first node, start OpenGSLB in bootstrap mode:

```bash
# Node 1 (10.0.1.1)
opengslb \
  --config /etc/opengslb/config.yaml \
  --mode cluster \
  --node-name node-1 \
  --bind-address 10.0.1.1:7000 \
  --bootstrap
```

Or using environment variables:

```bash
export OPENGSLB_CLUSTER_MODE=cluster
export OPENGSLB_CLUSTER_NODE_NAME=node-1
export OPENGSLB_CLUSTER_BIND_ADDRESS=10.0.1.1:7000
export OPENGSLB_CLUSTER_BOOTSTRAP=true

opengslb --config /etc/opengslb/config.yaml
```

**Expected output:**
```
INFO OpenGSLB starting version=1.0.0 mode=cluster
INFO initializing cluster mode components node_name=node-1
INFO Raft node started
INFO leader elected leader_id=node-1
INFO this node is now the cluster leader
INFO DNS server initialized address=:53 cluster_mode=true
INFO OpenGSLB running
```

### Step 3: Join Additional Nodes

On nodes 2 and 3, start OpenGSLB with `--join` pointing to existing cluster members:

```bash
# Node 2 (10.0.1.2)
opengslb \
  --config /etc/opengslb/config.yaml \
  --mode cluster \
  --node-name node-2 \
  --bind-address 10.0.1.2:7000 \
  --join 10.0.1.1:7000
```

```bash
# Node 3 (10.0.1.3)
opengslb \
  --config /etc/opengslb/config.yaml \
  --mode cluster \
  --node-name node-3 \
  --bind-address 10.0.1.3:7000 \
  --join 10.0.1.1:7000,10.0.1.2:7000
```

**Expected output:**
```
INFO OpenGSLB starting version=1.0.0 mode=cluster
INFO initializing cluster mode components node_name=node-2
INFO Raft node started
INFO joined cluster leader_id=node-1
INFO this node is now a follower
INFO DNS queries will be refused (not leader)
```

### Step 4: Verify Cluster Health

Check cluster status via API:

```bash
# From any node
curl http://localhost:8080/api/v1/cluster/status | jq
```

**Expected response:**
```json
{
  "mode": "cluster",
  "node_id": "node-1",
  "is_leader": true,
  "leader_id": "node-1",
  "leader_address": "10.0.1.1:7000",
  "cluster_size": 3,
  "state": "leader"
}
```

List cluster members:

```bash
curl http://localhost:8080/api/v1/cluster/members | jq
```

**Expected response:**
```json
{
  "members": [
    {"id": "node-1", "address": "10.0.1.1:7000", "state": "Leader", "is_voter": true},
    {"id": "node-2", "address": "10.0.1.2:7000", "state": "Follower", "is_voter": true},
    {"id": "node-3", "address": "10.0.1.3:7000", "state": "Follower", "is_voter": true}
  ]
}
```

### Step 5: Configure DNS Clients

Point your DNS clients to the anycast VIP or use round-robin DNS across all nodes.

**Option A: Anycast VIP (Recommended)**

Configure all nodes to advertise the same VIP (e.g., `10.99.99.1`) via BGP or keepalived. Only the leader will respond to queries.

**Option B: Round-Robin DNS**

Configure clients with all node IPs. Non-leaders return `REFUSED`, causing clients to retry with another server.

```
nameserver 10.0.1.1
nameserver 10.0.1.2
nameserver 10.0.1.3
```

## Systemd Service Configuration

Create `/etc/systemd/system/opengslb.service`:

```ini
[Unit]
Description=OpenGSLB DNS Load Balancer
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=opengslb
Group=opengslb
ExecStart=/usr/local/bin/opengslb --config /etc/opengslb/config.yaml
ExecReload=/bin/kill -SIGHUP $MAINPID
Restart=always
RestartSec=5
LimitNOFILE=65535

# Environment file for node-specific settings
EnvironmentFile=-/etc/opengslb/node.env

[Install]
WantedBy=multi-user.target
```

Create `/etc/opengslb/node.env` on each node:

```bash
# Node 1
OPENGSLB_CLUSTER_NODE_NAME=node-1
OPENGSLB_CLUSTER_BIND_ADDRESS=10.0.1.1:7000
OPENGSLB_CLUSTER_BOOTSTRAP=true

# Node 2 (different file)
OPENGSLB_CLUSTER_NODE_NAME=node-2
OPENGSLB_CLUSTER_BIND_ADDRESS=10.0.1.2:7000
OPENGSLB_CLUSTER_JOIN=10.0.1.1:7000
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable opengslb
sudo systemctl start opengslb
```

## Monitoring

### Prometheus Metrics

Key cluster metrics to monitor:

| Metric | Description |
|--------|-------------|
| `opengslb_cluster_is_leader` | 1 if this node is leader, 0 otherwise |
| `opengslb_cluster_state` | Current Raft state (leader/follower/candidate) |
| `opengslb_cluster_leader_changes_total` | Number of leader elections |
| `opengslb_cluster_members` | Total cluster members |
| `opengslb_gossip_members` | Gossip cluster size |
| `opengslb_gossip_health_updates_received_total` | Health events received via gossip |
| `opengslb_dns_refused_total` | DNS queries refused (non-leader) |

### Alerting Rules

```yaml
groups:
  - name: opengslb-cluster
    rules:
      - alert: OpenGSLBNoLeader
        expr: sum(opengslb_cluster_is_leader) == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "No OpenGSLB leader elected"
          
      - alert: OpenGSLBClusterSizeReduced
        expr: opengslb_cluster_members < 3
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "OpenGSLB cluster size below 3"
          
      - alert: OpenGSLBLeaderFlapping
        expr: increase(opengslb_cluster_leader_changes_total[5m]) > 3
        labels:
          severity: warning
        annotations:
          summary: "Frequent leader elections detected"
```

## Operational Procedures

### Adding a New Node

1. Deploy OpenGSLB binary and configuration to new node
2. Start with `--join` flag pointing to existing members
3. Node automatically joins as follower
4. Verify via cluster status API

### Removing a Node

1. Stop the OpenGSLB process on the node to remove
2. The node will leave gracefully if possible
3. Remaining nodes will re-elect if leader was removed
4. No manual intervention required for clean removal

### Performing Rolling Updates

1. Update followers first, one at a time
2. Wait for each follower to rejoin cluster before next update
3. Update leader last (will trigger brief re-election)
4. Monitor for leader changes during update

```bash
# On each follower node
sudo systemctl stop opengslb
# ... deploy new binary ...
sudo systemctl start opengslb
# Wait for cluster to stabilize
sleep 30

# After all followers updated, update leader
sudo systemctl stop opengslb
# ... deploy new binary ...
sudo systemctl start opengslb
```

### Disaster Recovery

If quorum is lost (majority of nodes unavailable):

1. **Do not** attempt to bootstrap a new cluster with stale data
2. Restore from backup or wait for nodes to recover
3. If recovery impossible, bootstrap new cluster and restore configuration

## Security Considerations

### Gossip Encryption

Enable gossip encryption for production deployments:

```yaml
cluster:
  gossip:
    encryption_key: "your-32-byte-base64-encoded-key"
```

Generate a key:
```bash
openssl rand -base64 32
```

### Network Segmentation

- Place Raft and gossip ports on internal/management network
- Only expose DNS port (53) to clients
- Consider firewall rules restricting Raft/gossip traffic to cluster nodes only

### API Authentication

The API should be protected via:
- `allowed_networks` configuration
- Reverse proxy with authentication (nginx, traefik)
- Network policies (Kubernetes, cloud provider)

## Troubleshooting

See [troubleshooting.md](../troubleshooting.md) for common cluster issues including:

- Leader election failures
- Gossip communication issues
- Split-brain scenarios
- Node join failures