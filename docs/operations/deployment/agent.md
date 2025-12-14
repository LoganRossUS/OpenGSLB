# Agent Deployment Runbook

This runbook provides step-by-step instructions for deploying OpenGSLB agents on application servers.

## Overview

OpenGSLB agents run alongside your applications to:
- Monitor local backend health
- Report health status to Overwatch nodes via gossip
- Send predictive signals when resource thresholds are exceeded
- Support multiple backends per agent

## Prerequisites

### System Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 1 core | 2 cores |
| Memory | 64 MB | 128 MB |
| Disk | 50 MB | 100 MB |
| Network | Outbound to Overwatch gossip port (7946) | |

### Network Requirements

| Direction | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| Outbound | 7946 | TCP/UDP | Gossip to Overwatch nodes |
| Outbound | 9090 | TCP | API calls (optional) |
| Inbound | 9100 | TCP | Metrics endpoint (optional) |

### Information Needed

Before starting, gather the following from your Overwatch administrator:

- [ ] Gossip encryption key (32-byte base64 string)
- [ ] Service token for your application(s)
- [ ] Overwatch node addresses and gossip ports
- [ ] Region identifier for this deployment

## Installation Methods

### Method 1: Binary Installation (Recommended)

#### Step 1: Download the Binary

```bash
# Set version
VERSION="1.0.0"

# Download for your platform
curl -Lo opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${VERSION}/opengslb-linux-amd64
chmod +x opengslb
sudo mv opengslb /usr/local/bin/
```

#### Step 2: Create System User

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
```

#### Step 3: Create Configuration File

```bash
sudo tee /etc/opengslb/agent.yaml << 'EOF'
mode: agent

agent:
  identity:
    # Pre-shared token - REPLACE with your actual token
    service_token: "YOUR_SERVICE_TOKEN_HERE"
    region: us-east

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
    check_interval: 10s
    cpu:
      threshold: 85
      bleed_duration: 30s
    memory:
      threshold: 90
      bleed_duration: 30s
    error_rate:
      threshold: 10
      window: 60s
      bleed_duration: 60s

  gossip:
    # REPLACE with your actual encryption key
    encryption_key: "YOUR_GOSSIP_ENCRYPTION_KEY_HERE"
    overwatch_nodes:
      - overwatch-1.internal:7946
      - overwatch-2.internal:7946
      - overwatch-3.internal:7946

  heartbeat:
    interval: 10s
    missed_threshold: 3

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: "127.0.0.1:9100"
EOF

# Set secure permissions
sudo chown root:opengslb /etc/opengslb/agent.yaml
sudo chmod 640 /etc/opengslb/agent.yaml
```

#### Step 4: Create systemd Service

```bash
sudo tee /etc/systemd/system/opengslb-agent.service << 'EOF'
[Unit]
Description=OpenGSLB Agent
Documentation=https://opengslb.org/docs
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=opengslb
Group=opengslb
ExecStart=/usr/local/bin/opengslb --config=/etc/opengslb/agent.yaml
ExecReload=/bin/kill -SIGHUP $MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/opengslb

# Environment
Environment="GOMAXPROCS=2"

[Install]
WantedBy=multi-user.target
EOF
```

#### Step 5: Start the Agent

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable and start agent
sudo systemctl enable opengslb-agent
sudo systemctl start opengslb-agent

# Check status
sudo systemctl status opengslb-agent
```

### Method 2: Docker Installation

```bash
docker run -d \
  --name opengslb-agent \
  --restart unless-stopped \
  --network host \
  -v /etc/opengslb/agent.yaml:/etc/opengslb/config.yaml:ro \
  -v /var/lib/opengslb:/var/lib/opengslb \
  ghcr.io/loganrossus/opengslb:latest \
  --config=/etc/opengslb/config.yaml
```

### Method 3: Build from Source

```bash
# Clone repository
git clone https://github.com/loganrossus/OpenGSLB.git
cd OpenGSLB

# Build
make build

# Install
sudo mv opengslb /usr/local/bin/
```

## First Start and TOFU Certificate Generation

On first start, the agent will:

1. Generate a self-signed certificate and private key
2. Connect to Overwatch nodes via gossip
3. Present the service token for initial authentication
4. Overwatch pins the certificate fingerprint (Trust On First Use)

### Verify Certificate Generation

```bash
# Check certificate was created
ls -la /var/lib/opengslb/
# Should show: agent.crt, agent.key

# View certificate details
openssl x509 -in /var/lib/opengslb/agent.crt -noout -text
```

### Verify Registration with Overwatch

```bash
# Using opengslb-cli
opengslb-cli servers --api http://overwatch-1.internal:9090

# Or using curl
curl http://overwatch-1.internal:9090/api/v1/overwatch/backends | jq .
```

Expected output should show your backend registered:

```json
{
  "backends": [
    {
      "service": "myapp",
      "address": "127.0.0.1",
      "port": 8080,
      "agent_id": "agent-abc123",
      "region": "us-east",
      "effective_status": "healthy",
      "agent_healthy": true
    }
  ]
}
```

## Multi-Backend Configuration

A single agent can monitor multiple backends:

```yaml
agent:
  backends:
    - service: webapp
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

    - service: api
      address: 127.0.0.1
      port: 9000
      weight: 100
      health_check:
        type: http
        path: /api/health
        interval: 5s
        timeout: 2s
        failure_threshold: 3
        success_threshold: 2

    - service: grpc
      address: 127.0.0.1
      port: 50051
      weight: 100
      health_check:
        type: tcp
        interval: 10s
        timeout: 3s
        failure_threshold: 2
        success_threshold: 1
```

## Health Check Configuration

### HTTP Health Check

```yaml
health_check:
  type: http
  path: /health           # Required: health endpoint path
  interval: 5s            # How often to check
  timeout: 2s             # Timeout for each check
  failure_threshold: 3    # Mark unhealthy after N failures
  success_threshold: 2    # Mark healthy after N successes
  host: "custom-host"     # Optional: Host header override
```

### TCP Health Check

```yaml
health_check:
  type: tcp
  interval: 10s
  timeout: 3s
  failure_threshold: 2
  success_threshold: 1
```

## Predictive Health Tuning

Predictive health sends early warning signals before failures occur:

```yaml
predictive:
  enabled: true
  check_interval: 10s    # How often to check metrics

  cpu:
    threshold: 85        # Start bleeding at 85% CPU
    bleed_duration: 30s  # Gradually reduce traffic over 30s

  memory:
    threshold: 90        # Start bleeding at 90% memory
    bleed_duration: 30s

  error_rate:
    threshold: 10        # Errors per minute
    window: 60s          # Measurement window
    bleed_duration: 60s
```

**Tuning Guidelines:**

| Scenario | CPU Threshold | Memory Threshold | Error Rate |
|----------|---------------|------------------|------------|
| CPU-bound app | 75% | 90% | 5 |
| Memory-bound app | 90% | 80% | 5 |
| High-traffic API | 80% | 85% | 10 |
| Background worker | 90% | 90% | 3 |

## Log Rotation Setup

### Using logrotate

```bash
sudo tee /etc/logrotate.d/opengslb << 'EOF'
/var/log/opengslb/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 opengslb opengslb
    postrotate
        systemctl reload opengslb-agent 2>/dev/null || true
    endscript
}
EOF
```

### Using journald (recommended for systemd)

Logs go to journald by default. Configure retention:

```bash
# Edit /etc/systemd/journald.conf
sudo tee -a /etc/systemd/journald.conf << 'EOF'
SystemMaxUse=500M
MaxRetentionSec=7day
EOF

sudo systemctl restart systemd-journald
```

View logs:

```bash
# Follow agent logs
journalctl -u opengslb-agent -f

# Last 100 lines
journalctl -u opengslb-agent -n 100

# Errors only
journalctl -u opengslb-agent -p err
```

## Verification Steps

### 1. Check Agent is Running

```bash
sudo systemctl status opengslb-agent
```

### 2. Check Gossip Connectivity

```bash
# Test connectivity to Overwatch
nc -zv overwatch-1.internal 7946
nc -zv overwatch-2.internal 7946
```

### 3. Check Metrics Endpoint

```bash
curl http://localhost:9100/metrics | grep opengslb
```

Expected metrics:

```
opengslb_agent_backends_registered 1
opengslb_agent_heartbeats_sent_total 150
opengslb_agent_heartbeat_failures_total 0
```

### 4. Check Backend Registration

```bash
# Query Overwatch API
curl http://overwatch-1.internal:9090/api/v1/overwatch/backends?service=myapp | jq .
```

## Troubleshooting

### Agent Not Starting

1. **Check configuration syntax:**
   ```bash
   opengslb --config=/etc/opengslb/agent.yaml --validate
   ```

2. **Check file permissions:**
   ```bash
   ls -la /etc/opengslb/agent.yaml
   # Should be: -rw-r----- root opengslb
   ```

3. **Check logs:**
   ```bash
   journalctl -u opengslb-agent -n 50 --no-pager
   ```

### Agent Not Registering

1. **Verify gossip connectivity:**
   ```bash
   nc -zv overwatch-1.internal 7946
   ```

2. **Check encryption key matches:**
   - Agent and Overwatch must use identical gossip encryption keys
   - Check for extra whitespace or encoding issues

3. **Check service token:**
   - Token must match `agent_tokens` in Overwatch config
   - Tokens are case-sensitive

4. **Check certificate issues:**
   ```bash
   # View agent certificate
   openssl x509 -in /var/lib/opengslb/agent.crt -noout -text
   ```

### Backend Marked Unhealthy

1. **Check backend is running:**
   ```bash
   curl http://localhost:8080/health
   ```

2. **Check health check configuration:**
   - Verify path, port, and timeout settings
   - Ensure health endpoint returns 2xx status

3. **Check agent logs for health check failures:**
   ```bash
   journalctl -u opengslb-agent | grep "health check"
   ```

### Metrics Not Available

1. **Check metrics endpoint is enabled:**
   ```yaml
   metrics:
     enabled: true
     address: "127.0.0.1:9100"
   ```

2. **Check firewall rules:**
   ```bash
   sudo iptables -L -n | grep 9100
   ```

## Configuration Reference

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OPENGSLB_CONFIG` | Config file path | `/etc/opengslb/config.yaml` |
| `GOMAXPROCS` | Max CPU cores | All available |

### Configuration Options

See [Configuration Reference](../../configuration.md) for complete configuration options.

## Security Considerations

- [ ] Store gossip encryption key securely (not in version control)
- [ ] Use strong service tokens (minimum 32 characters)
- [ ] Restrict metrics endpoint to localhost or monitoring network
- [ ] Set appropriate file permissions (640 for config, 700 for data dir)
- [ ] Use systemd security hardening options

## Related Documentation

- [Overwatch Deployment](./overwatch.md)
- [HA Setup Guide](./ha-setup.md)
- [Security Hardening](../security/hardening.md)
- [Troubleshooting](../../troubleshooting.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
