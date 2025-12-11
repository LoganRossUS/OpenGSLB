# High Availability Setup Guide

This guide covers deploying multiple OpenGSLB Overwatch nodes for high availability without requiring cluster coordination.

## Architecture Overview

OpenGSLB achieves high availability through a simple but effective approach:

- **Multiple independent Overwatches**: Each operates autonomously
- **DNS client retry**: Clients automatically retry failed queries
- **Shared state via gossip**: Agents gossip to all Overwatches
- **DNSSEC key sync**: Keys synchronized via API polling

```
┌─────────────────────────────────────────────────────────────┐
│  DNS Clients (resolv.conf with multiple nameservers)         │
│       │           │           │                              │
│       ▼           ▼           ▼                              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                     │
│  │Overwatch1│ │Overwatch2│ │Overwatch3│                     │
│  │10.0.1.53 │ │10.0.1.54 │ │10.0.1.55 │                     │
│  └─────┬────┘ └─────┬────┘ └─────┬────┘                     │
│        │            │            │                           │
│        │    DNSSEC Key Sync (API)│                          │
│        ├────────────┼────────────┤                          │
│        │            │            │                           │
│        └────────────┼────────────┘                           │
│                     │ Gossip (all agents → all overwatches)  │
│            ┌────────┼────────┐                              │
│            ▼        ▼        ▼                              │
│       ┌────────┐ ┌────────┐ ┌────────┐                     │
│       │ Agent  │ │ Agent  │ │ Agent  │                     │
│       │ + App  │ │ + App  │ │ + App  │                     │
│       └────────┘ └────────┘ └────────┘                     │
└─────────────────────────────────────────────────────────────┘
```

## Why No Cluster Coordination?

Traditional approaches use consensus protocols (Raft, Paxos) for coordination. OpenGSLB avoids this complexity because:

1. **DNS is inherently retry-friendly**: Clients automatically retry on timeout
2. **Health data is eventually consistent**: Brief inconsistencies are acceptable
3. **Simplicity reduces failure modes**: No split-brain, no leader election issues
4. **Operational simplicity**: Add/remove nodes without coordination

## Deployment Topology

### Recommended: 3 Overwatches

For most deployments, 3 Overwatch nodes provide good availability:

| Scenario | Availability |
|----------|--------------|
| All healthy | 100% |
| 1 node down | 100% (clients retry) |
| 2 nodes down | 100% (single node serves) |
| 3 nodes down | 0% (no DNS service) |

### Geographic Distribution

For global deployments, distribute Overwatches across regions:

```
US-East: overwatch-us-east-1.internal (10.0.1.53)
US-West: overwatch-us-west-1.internal (10.0.2.53)
EU-West: overwatch-eu-west-1.internal (10.0.3.53)
```

## Step-by-Step HA Setup

### Step 1: Generate Shared Secrets

All Overwatches and agents must share the same gossip encryption key:

```bash
# Generate once, use on all nodes
GOSSIP_KEY=$(openssl rand -base64 32)
echo "Gossip Key: $GOSSIP_KEY"

# Store securely (vault, secrets manager, etc.)
```

Generate service tokens for each application:

```bash
WEBAPP_TOKEN=$(openssl rand -base64 32)
API_TOKEN=$(openssl rand -base64 32)
```

### Step 2: Deploy First Overwatch

Deploy the first Overwatch following the [Overwatch Deployment Guide](./overwatch.md).

Key configuration for HA:

```yaml
# /etc/opengslb/overwatch.yaml on overwatch-1

mode: overwatch

overwatch:
  identity:
    node_id: overwatch-us-east-1
    region: us-east

  agent_tokens:
    webapp: "${WEBAPP_TOKEN}"
    api: "${API_TOKEN}"

  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "${GOSSIP_KEY}"

  dnssec:
    enabled: true
    key_sync:
      # Initially empty - will add peers after they're deployed
      peers: []
      poll_interval: 1h

dns:
  listen_address: "0.0.0.0:53"
  zones:
    - gslb.example.com
```

### Step 3: Deploy Additional Overwatches

Deploy Overwatch 2 and 3 with similar configuration:

```yaml
# /etc/opengslb/overwatch.yaml on overwatch-2

mode: overwatch

overwatch:
  identity:
    node_id: overwatch-us-west-1
    region: us-west

  agent_tokens:
    webapp: "${WEBAPP_TOKEN}"  # Same tokens
    api: "${API_TOKEN}"

  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "${GOSSIP_KEY}"  # Same key

  dnssec:
    enabled: true
    key_sync:
      peers:
        - "https://overwatch-us-east-1.internal:9090"
      poll_interval: 1h
```

```yaml
# /etc/opengslb/overwatch.yaml on overwatch-3

mode: overwatch

overwatch:
  identity:
    node_id: overwatch-eu-west-1
    region: eu-west

  agent_tokens:
    webapp: "${WEBAPP_TOKEN}"
    api: "${API_TOKEN}"

  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "${GOSSIP_KEY}"

  dnssec:
    enabled: true
    key_sync:
      peers:
        - "https://overwatch-us-east-1.internal:9090"
        - "https://overwatch-us-west-1.internal:9090"
      poll_interval: 1h
```

### Step 4: Update First Overwatch with Peers

After all Overwatches are deployed, update the first node to include peers:

```yaml
# Update /etc/opengslb/overwatch.yaml on overwatch-1

dnssec:
  enabled: true
  key_sync:
    peers:
      - "https://overwatch-us-west-1.internal:9090"
      - "https://overwatch-eu-west-1.internal:9090"
    poll_interval: 1h
```

Reload configuration:

```bash
sudo systemctl reload opengslb-overwatch
```

### Step 5: Configure Agents for HA

Agents should gossip to ALL Overwatch nodes:

```yaml
# Agent configuration
agent:
  gossip:
    encryption_key: "${GOSSIP_KEY}"
    overwatch_nodes:
      - overwatch-us-east-1.internal:7946
      - overwatch-us-west-1.internal:7946
      - overwatch-eu-west-1.internal:7946
```

### Step 6: Configure DNS Clients

#### Option A: Direct Client Configuration

```bash
# /etc/resolv.conf
nameserver 10.0.1.53    # Overwatch 1
nameserver 10.0.2.53    # Overwatch 2
nameserver 10.0.3.53    # Overwatch 3
options timeout:2 attempts:3
```

#### Option B: Corporate DNS Forwarding

Configure your DNS servers to forward GSLB zones:

```
# BIND named.conf
zone "gslb.example.com" {
    type forward;
    forward only;
    forwarders {
        10.0.1.53;
        10.0.2.53;
        10.0.3.53;
    };
};
```

#### Option C: Load Balancer (Optional)

For environments requiring a single VIP:

```yaml
# HAProxy example (not recommended but possible)
frontend dns
    bind *:53
    mode tcp
    default_backend overwatches

backend overwatches
    mode tcp
    balance roundrobin
    server ow1 10.0.1.53:53 check
    server ow2 10.0.2.53:53 check
    server ow3 10.0.3.53:53 check
```

**Note**: Load balancers add complexity and a single point of failure. Direct multi-nameserver configuration is preferred.

### Step 7: Verify DNSSEC Key Sync

Check that all Overwatches have the same DNSSEC keys:

```bash
# On each Overwatch
curl http://localhost:9090/api/v1/dnssec/status | jq '.keys[].key_tag'

# Should return the same key_tag on all nodes
```

Trigger manual sync if needed:

```bash
curl -X POST http://localhost:9090/api/v1/dnssec/sync
```

## DNSSEC Key Synchronization

### How Key Sync Works

1. First Overwatch to start generates DNSSEC keys
2. Other Overwatches poll peers for existing keys
3. Keys are imported and used for signing
4. All Overwatches sign with identical keys

### Key Sync Configuration

```yaml
dnssec:
  enabled: true
  key_sync:
    peers:
      - "https://overwatch-2.internal:9090"
      - "https://overwatch-3.internal:9090"
    poll_interval: 1h    # How often to check for new keys
    timeout: 30s         # Timeout for sync requests
```

### Verifying Sync Status

```bash
curl http://localhost:9090/api/v1/dnssec/status | jq '.sync'
```

```json
{
  "enabled": true,
  "last_sync": "2025-01-15T10:30:00Z",
  "last_sync_error": null,
  "next_sync": "2025-01-15T11:30:00Z",
  "peer_count": 2
}
```

## Handling Node Failures

### Single Node Failure

**Impact**: None for DNS clients (automatic retry)

**Detection**:
```bash
# Check metrics
curl http://overwatch-1:9091/metrics | grep up
```

**Recovery**:
1. Investigate root cause
2. Restart service: `sudo systemctl restart opengslb-overwatch`
3. Verify registration: `curl http://localhost:9090/api/v1/ready`

### Multiple Node Failure

**Impact**: Reduced redundancy, potential service degradation

**Immediate Actions**:
1. Verify remaining nodes are healthy
2. Route DNS traffic to healthy nodes
3. Bring failed nodes back online

### Complete Cluster Failure

**Impact**: DNS service unavailable

**Recovery**:
1. Start any single Overwatch
2. DNS service resumes immediately
3. Start remaining nodes
4. Verify DNSSEC key sync

## Adding a New Overwatch

1. **Deploy new node** following [Overwatch Deployment Guide](./overwatch.md)

2. **Configure with same secrets**:
   - Same gossip encryption key
   - Same agent tokens
   - Add existing Overwatches as DNSSEC peers

3. **Update existing Overwatches** to include new peer in key_sync

4. **Update agents** to include new Overwatch in gossip nodes

5. **Update DNS configuration** to include new nameserver

## Removing an Overwatch

1. **Remove from DNS configuration** (resolv.conf, forwarding)

2. **Remove from agent configurations** (gossip.overwatch_nodes)

3. **Remove from other Overwatches** (dnssec.key_sync.peers)

4. **Stop and remove the node**:
   ```bash
   sudo systemctl stop opengslb-overwatch
   sudo systemctl disable opengslb-overwatch
   ```

## Monitoring HA Health

### Key Metrics

```promql
# DNS query distribution across nodes
sum by (instance) (rate(opengslb_dns_queries_total[5m]))

# DNSSEC sync status (0 = sync failure)
opengslb_dnssec_sync_success

# Agent registration per Overwatch
opengslb_overwatch_agents_registered
```

### Alerts

```yaml
groups:
  - name: opengslb-ha
    rules:
      - alert: OverwatchDown
        expr: up{job="opengslb-overwatch"} == 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Overwatch node {{ $labels.instance }} is down"

      - alert: AllOverwatchesDown
        expr: sum(up{job="opengslb-overwatch"}) == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "All Overwatch nodes are down - DNS service unavailable"

      - alert: DNSSECSyncFailed
        expr: time() - opengslb_dnssec_last_sync_timestamp > 7200
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "DNSSEC key sync hasn't succeeded in 2 hours"
```

## Testing HA

### Test 1: Node Failure Simulation

```bash
# Stop one Overwatch
sudo systemctl stop opengslb-overwatch

# Verify DNS still works (on client)
dig @10.0.1.53 webapp.gslb.example.com  # Should fail
dig webapp.gslb.example.com             # Should work (retry to other nodes)

# Restart
sudo systemctl start opengslb-overwatch
```

### Test 2: Network Partition

```bash
# Block gossip traffic to one Overwatch
sudo iptables -A INPUT -p tcp --dport 7946 -j DROP
sudo iptables -A INPUT -p udp --dport 7946 -j DROP

# Verify:
# - DNS still works
# - Agents still register to other Overwatches
# - Blocked Overwatch becomes stale

# Remove block
sudo iptables -D INPUT -p tcp --dport 7946 -j DROP
sudo iptables -D INPUT -p udp --dport 7946 -j DROP
```

### Test 3: Rolling Restart

```bash
# Restart each Overwatch one at a time
for host in overwatch-{1,2,3}; do
    echo "Restarting $host..."
    ssh $host "sudo systemctl restart opengslb-overwatch"
    sleep 30
    # Verify
    ssh $host "curl -s http://localhost:9090/api/v1/ready"
done
```

## Best Practices

### Do

- Deploy at least 3 Overwatches for production
- Distribute across failure domains (availability zones, racks)
- Use the same gossip key and agent tokens on all nodes
- Monitor all nodes and alert on failures
- Test failover regularly

### Don't

- Run a single Overwatch in production
- Put all Overwatches in the same failure domain
- Use different secrets on different nodes
- Ignore DNSSEC sync failures
- Skip HA testing

## Troubleshooting

### Inconsistent DNS Responses

**Symptom**: Different Overwatches return different records

**Causes**:
1. Agent not gossiping to all Overwatches
2. Stale data on one Overwatch
3. Different configuration

**Resolution**:
```bash
# Compare backend lists
for host in overwatch-{1,2,3}; do
    echo "=== $host ==="
    ssh $host "curl -s http://localhost:9090/api/v1/overwatch/backends | jq '.backends | length'"
done
```

### DNSSEC Validation Failures

**Symptom**: DNSSEC validation fails on some Overwatches

**Cause**: Key sync issue

**Resolution**:
```bash
# Check key tags match
for host in overwatch-{1,2,3}; do
    echo "=== $host ==="
    ssh $host "curl -s http://localhost:9090/api/v1/dnssec/status | jq '.keys[].key_tag'"
done

# Force sync
curl -X POST http://localhost:9090/api/v1/dnssec/sync
```

## Related Documentation

- [Overwatch Deployment](./overwatch.md)
- [Agent Deployment](./agent.md)
- [DNSSEC Key Rotation](../security/key-rotation.md)
- [Incident Response](../incident-response/playbook.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
