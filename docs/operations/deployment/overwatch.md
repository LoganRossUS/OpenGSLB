# Overwatch Deployment Runbook

This runbook provides step-by-step instructions for deploying OpenGSLB Overwatch nodes that serve authoritative DNS and validate agent health claims.

## Overview

Overwatch nodes are the core DNS-serving components of OpenGSLB:
- Serve authoritative DNS with GSLB routing decisions
- Receive health updates from agents via gossip
- Perform external validation of agent health claims
- Sign DNS responses with DNSSEC
- Operate independently (no cluster coordination)

## Prerequisites

### System Requirements

| Resource | Minimum | Recommended | High Traffic |
|----------|---------|-------------|--------------|
| CPU | 2 cores | 4 cores | 8 cores |
| Memory | 512 MB | 1 GB | 2 GB |
| Disk | 1 GB | 5 GB | 10 GB |
| Network | Gigabit | Gigabit | 10 Gigabit |

### Network Requirements

| Direction | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| Inbound | 53 | UDP/TCP | DNS queries |
| Inbound | 7946 | TCP/UDP | Gossip from agents |
| Inbound | 8080 | TCP | API endpoint (default: localhost only) |
| Inbound | 9090 | TCP | Metrics endpoint |
| Outbound | 9090 | TCP | DNSSEC key sync (to peers) |
| Outbound | Backend ports | TCP | Health validation |

### DNS Integration Considerations

Before deployment, plan how DNS will be integrated:

1. **Direct Resolution**: Clients point directly to Overwatch nodes
2. **Conditional Forwarding**: Corporate DNS forwards GSLB zones to Overwatch
3. **Stub Zone**: Authoritative DNS delegates GSLB subdomain

### Information Needed

- [ ] DNS zones to serve (e.g., `gslb.example.com`)
- [ ] Gossip encryption key (generate if first Overwatch)
- [ ] Service tokens for each application
- [ ] GeoIP database (for geolocation routing)
- [ ] Peer Overwatch addresses (for HA/DNSSEC sync)

## Installation

### Step 1: Download and Install Binary

```bash
# Set version
VERSION="1.0.0"

# Download for your platform
curl -Lo opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${VERSION}/opengslb-linux-amd64
chmod +x opengslb
sudo mv opengslb /usr/local/bin/

# Also install CLI tool
curl -Lo opengslb-cli https://github.com/loganrossus/OpenGSLB/releases/download/v${VERSION}/opengslb-cli-linux-amd64
chmod +x opengslb-cli
sudo mv opengslb-cli /usr/local/bin/
```

### Step 2: Create System User

```bash
# Create opengslb user and group
sudo useradd --system --no-create-home --shell /bin/false opengslb

# Create data directory
sudo mkdir -p /var/lib/opengslb
sudo chown opengslb:opengslb /var/lib/opengslb
sudo chmod 700 /var/lib/opengslb

# Create config directory
sudo mkdir -p /etc/opengslb
sudo chown root:opengslb /etc/opengslb
sudo chmod 750 /etc/opengslb

# Create GeoIP database directory
sudo mkdir -p /var/lib/opengslb/geoip
sudo chown opengslb:opengslb /var/lib/opengslb/geoip
```

### Step 3: Generate Secrets

```bash
# Generate gossip encryption key (save this securely!)
GOSSIP_KEY=$(openssl rand -base64 32)
echo "Gossip Key: $GOSSIP_KEY"

# Generate service tokens for each application
WEBAPP_TOKEN=$(openssl rand -base64 32)
API_TOKEN=$(openssl rand -base64 32)
echo "WebApp Token: $WEBAPP_TOKEN"
echo "API Token: $API_TOKEN"
```

**Important**: Store these secrets in a secure location (vault, secrets manager). You'll need:
- Gossip key: Shared between all Overwatches and agents
- Service tokens: Shared with respective agent deployments

### Step 4: Set Up GeoIP Database (Optional)

For geolocation routing, download the MaxMind GeoLite2 database:

```bash
# Register at https://www.maxmind.com/en/geolite2/signup
# Download GeoLite2-Country database

# Place database in the correct location
sudo mv GeoLite2-Country.mmdb /var/lib/opengslb/geoip/
sudo chown opengslb:opengslb /var/lib/opengslb/geoip/GeoLite2-Country.mmdb
```

### Step 5: Create Configuration File

```bash
sudo tee /etc/opengslb/overwatch.yaml << 'EOF'
mode: overwatch

overwatch:
  identity:
    node_id: overwatch-us-east-1
    region: us-east

  # Agent authentication tokens
  # REPLACE with your actual tokens
  agent_tokens:
    webapp: "YOUR_WEBAPP_TOKEN_HERE"
    api: "YOUR_API_TOKEN_HERE"

  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "YOUR_GOSSIP_KEY_HERE"
    probe_interval: 1s
    probe_timeout: 500ms
    gossip_interval: 200ms

  validation:
    enabled: true
    check_interval: 30s
    check_timeout: 5s

  stale:
    threshold: 30s
    remove_after: 5m

  dnssec:
    enabled: true
    algorithm: ECDSAP256SHA256
    key_sync:
      peers: []  # Add peer Overwatch URLs for HA
      poll_interval: 1h
      timeout: 30s

  # Geolocation configuration (optional)
  geolocation:
    database_path: "/var/lib/opengslb/geoip/GeoLite2-Country.mmdb"
    default_region: us-east
    ecs_enabled: true
    custom_mappings:
      - cidr: "10.0.0.0/8"
        region: us-east
        comment: "Internal networks default to us-east"

  data_dir: /var/lib/opengslb

# DNS server configuration
dns:
  listen_address: "0.0.0.0:53"
  default_ttl: 30
  return_last_healthy: false
  zones:
    - gslb.example.com

# Region definitions (for static backends or region mapping)
regions:
  - name: us-east
    countries: ["US", "CA", "MX"]
    continents: ["NA", "SA"]
    servers: []  # Populated dynamically from agents

  - name: eu-west
    countries: ["GB", "DE", "FR", "ES", "IT"]
    continents: ["EU"]
    servers: []

  - name: ap-southeast
    continents: ["AS", "OC"]
    servers: []

# Domain routing configuration
domains:
  - name: webapp.gslb.example.com
    routing_algorithm: geolocation
    regions:
      - us-east
      - eu-west
      - ap-southeast
    ttl: 30

  - name: api.gslb.example.com
    routing_algorithm: latency
    regions:
      - us-east
      - eu-west
    ttl: 15
    latency_config:
      smoothing_factor: 0.3
      max_latency_ms: 500
      min_samples: 3

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9090"

api:
  enabled: true
  address: "127.0.0.1:8080"  # Localhost only by default for security
  allowed_networks:
    - 10.0.0.0/8
    - 192.168.0.0/16
    - 127.0.0.1/32
  trust_proxy_headers: false
EOF

# Set secure permissions
sudo chown root:opengslb /etc/opengslb/overwatch.yaml
sudo chmod 640 /etc/opengslb/overwatch.yaml
```

### Step 6: Create systemd Service

```bash
sudo tee /etc/systemd/system/opengslb-overwatch.service << 'EOF'
[Unit]
Description=OpenGSLB Overwatch
Documentation=https://opengslb.org/docs
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=opengslb
Group=opengslb
ExecStart=/usr/local/bin/opengslb --config=/etc/opengslb/overwatch.yaml
ExecReload=/bin/kill -SIGHUP $MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Required for binding to port 53
AmbientCapabilities=CAP_NET_BIND_SERVICE

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/opengslb

# Environment
Environment="GOMAXPROCS=4"

[Install]
WantedBy=multi-user.target
EOF
```

### Step 7: Allow DNS Port Binding

For non-root binding to port 53:

```bash
# Option 1: Using capabilities (recommended)
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/opengslb

# Option 2: Use systemd AmbientCapabilities (already in service file above)
```

### Step 8: Start Overwatch

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable and start Overwatch
sudo systemctl enable opengslb-overwatch
sudo systemctl start opengslb-overwatch

# Check status
sudo systemctl status opengslb-overwatch
```

## DNS Integration Patterns

### Pattern 1: Direct Client Resolution

Configure clients to use Overwatch directly:

```bash
# Client /etc/resolv.conf
nameserver 10.0.1.53    # Overwatch 1
nameserver 10.0.1.54    # Overwatch 2
nameserver 10.0.1.55    # Overwatch 3
options timeout:2 attempts:3
```

### Pattern 2: BIND Conditional Forwarding

```
# named.conf
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

### Pattern 3: Unbound Stub Zone

```
# unbound.conf
stub-zone:
    name: "gslb.example.com"
    stub-addr: 10.0.1.53
    stub-addr: 10.0.1.54
    stub-addr: 10.0.1.55
```

### Pattern 4: Parent Zone Delegation

In your parent zone (e.g., `example.com`):

```
; NS records for delegation
gslb    IN  NS  ns1.gslb.example.com.
gslb    IN  NS  ns2.gslb.example.com.
gslb    IN  NS  ns3.gslb.example.com.

; Glue records
ns1.gslb    IN  A   10.0.1.53
ns2.gslb    IN  A   10.0.1.54
ns3.gslb    IN  A   10.0.1.55

; DS record for DNSSEC (get from Overwatch API)
gslb    IN  DS  12345 13 2 abc123...
```

## DNSSEC Setup

DNSSEC is enabled by default. After starting Overwatch:

### Get DS Records for Parent Zone

```bash
# Using CLI
opengslb-cli dnssec ds --zone gslb.example.com --api http://localhost:8080

# Using curl
curl http://localhost:8080/api/v1/dnssec/ds | jq .
```

Output:

```json
{
  "enabled": true,
  "ds_records": [
    {
      "zone": "gslb.example.com.",
      "key_tag": 12345,
      "algorithm": 13,
      "digest_type": 2,
      "digest": "abc123def456...",
      "ds_record": "gslb.example.com. IN DS 12345 13 2 abc123def456..."
    }
  ]
}
```

Add the DS record to your parent zone to enable DNSSEC chain of trust.

### DNSSEC Key Synchronization

For multiple Overwatches, configure key sync:

```yaml
dnssec:
  enabled: true
  key_sync:
    peers:
      - "https://overwatch-2.internal:9090"
      - "https://overwatch-3.internal:9090"
    poll_interval: 1h
    timeout: 30s
```

## API Security Configuration

### Network Restrictions

```yaml
api:
  enabled: true
  address: ":9090"
  allowed_networks:
    - 10.0.0.0/8        # Internal network
    - 192.168.0.0/16    # VPN/corporate
    - 127.0.0.1/32      # Localhost
  trust_proxy_headers: false
```

### Behind a Load Balancer

If API is behind a reverse proxy:

```yaml
api:
  trust_proxy_headers: true
  allowed_networks:
    - 10.0.0.0/8
```

The proxy must set `X-Forwarded-For` header.

## Metrics and Monitoring

### Prometheus Configuration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'opengslb-overwatch'
    static_configs:
      - targets:
        - 'overwatch-1.internal:9090'
        - 'overwatch-2.internal:9090'
        - 'overwatch-3.internal:9090'
    scrape_interval: 15s
```

### Key Metrics to Monitor

```promql
# DNS query rate
rate(opengslb_dns_queries_total[5m])

# DNS error rate
sum(rate(opengslb_dns_queries_total{status!="success"}[5m])) / sum(rate(opengslb_dns_queries_total[5m]))

# Healthy backends
opengslb_overwatch_backends_healthy

# Stale agents
opengslb_overwatch_stale_agents
```

### Alert Examples

```yaml
groups:
  - name: opengslb-overwatch
    rules:
      - alert: OpenGSLBLowHealthyBackends
        expr: opengslb_overwatch_backends_healthy < 2
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Less than 2 healthy backends"

      - alert: OpenGSLBStaleAgents
        expr: opengslb_overwatch_stale_agents > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Agents are stale"
```

## Verification Steps

### 1. Check Service Status

```bash
sudo systemctl status opengslb-overwatch
```

### 2. Verify DNS is Responding

```bash
# Query Overwatch directly
dig @localhost webapp.gslb.example.com +short

# Query with DNSSEC validation
dig @localhost webapp.gslb.example.com +dnssec
```

### 3. Check API is Accessible

```bash
# Health check
curl http://localhost:8080/api/v1/live

# Readiness check
curl http://localhost:8080/api/v1/ready

# List backends
curl http://localhost:8080/api/v1/overwatch/backends | jq .
```

### 4. Check Metrics Endpoint

```bash
curl http://localhost:9090/metrics | grep opengslb
```

### 5. Verify Gossip is Listening

```bash
ss -tulnp | grep 7946
```

## Smoke Tests

Run these after deployment to verify functionality:

```bash
#!/bin/bash
# smoke-test.sh

OVERWATCH="localhost"
DNS_PORT="53"
API_PORT="8080"
METRICS_PORT="9090"
DOMAIN="webapp.gslb.example.com"

echo "=== OpenGSLB Overwatch Smoke Test ==="

# Test 1: DNS query
echo -n "DNS Query: "
if dig @${OVERWATCH} -p ${DNS_PORT} ${DOMAIN} +short | grep -q "."; then
    echo "PASS"
else
    echo "FAIL"
fi

# Test 2: API liveness
echo -n "API Liveness: "
if curl -s http://${OVERWATCH}:${API_PORT}/api/v1/live | grep -q "alive"; then
    echo "PASS"
else
    echo "FAIL"
fi

# Test 3: API readiness
echo -n "API Readiness: "
if curl -s http://${OVERWATCH}:${API_PORT}/api/v1/ready | grep -q "ready"; then
    echo "PASS"
else
    echo "FAIL"
fi

# Test 4: DNSSEC
echo -n "DNSSEC: "
if dig @${OVERWATCH} -p ${DNS_PORT} ${DOMAIN} +dnssec | grep -q "RRSIG"; then
    echo "PASS"
else
    echo "FAIL (may need DS in parent zone)"
fi

# Test 5: Metrics
echo -n "Metrics: "
if curl -s http://${OVERWATCH}:${METRICS_PORT}/metrics | grep -q "opengslb_dns_queries_total"; then
    echo "PASS"
else
    echo "FAIL"
fi

echo "=== Smoke Test Complete ==="
```

## Troubleshooting

### DNS Not Resolving

1. **Check Overwatch is listening:**
   ```bash
   ss -tulnp | grep :53
   ```

2. **Check for port conflicts:**
   ```bash
   sudo lsof -i :53
   # May need to disable systemd-resolved
   sudo systemctl stop systemd-resolved
   ```

3. **Test directly:**
   ```bash
   dig @127.0.0.1 webapp.gslb.example.com
   ```

### Agents Not Registering

1. **Check gossip is listening:**
   ```bash
   ss -tulnp | grep 7946
   ```

2. **Verify encryption key:**
   - Must match between Overwatch and agents

3. **Check agent tokens:**
   - Tokens in `agent_tokens` must match agent configuration

### API Not Accessible

1. **Check binding:**
   ```bash
   ss -tulnp | grep 8080
   ```

2. **Check allowed networks:**
   - Your IP must be in `allowed_networks` CIDR ranges

3. **Check firewall:**
   ```bash
   sudo iptables -L -n | grep 8080
   ```

### DNSSEC Issues

1. **Verify keys exist:**
   ```bash
   curl http://localhost:8080/api/v1/dnssec/status | jq .
   ```

2. **Check DS record in parent:**
   ```bash
   dig DS gslb.example.com +trace
   ```

## Configuration Reference

See [Configuration Reference](../../configuration.md) for complete configuration options.

## Related Documentation

- [Agent Deployment](./agent.md)
- [HA Setup Guide](./ha-setup.md)
- [DNSSEC Key Rotation](../security/key-rotation.md)
- [Backup and Restore](../maintenance/backup-restore.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
