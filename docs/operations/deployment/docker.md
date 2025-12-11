# Docker Deployment Guide

This guide covers deploying OpenGSLB using Docker containers for both Agent and Overwatch modes.

## Image Information

| Registry | Image | Tags |
|----------|-------|------|
| GitHub Container Registry | `ghcr.io/loganrossus/opengslb` | `latest`, `v0.6.0`, `sha-*` |

## Quick Start

### Overwatch Mode

```bash
# Create configuration directory
mkdir -p ./config

# Create overwatch configuration (see Configuration section)
# Then run:
docker run -d \
  --name opengslb-overwatch \
  --restart unless-stopped \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 7946:7946/tcp \
  -p 7946:7946/udp \
  -p 9090:9090 \
  -p 9091:9091 \
  -v ./config/overwatch.yaml:/etc/opengslb/config.yaml:ro \
  -v ./data:/var/lib/opengslb \
  ghcr.io/loganrossus/opengslb:latest
```

### Agent Mode

```bash
docker run -d \
  --name opengslb-agent \
  --restart unless-stopped \
  --network host \
  -v ./config/agent.yaml:/etc/opengslb/config.yaml:ro \
  -v ./data:/var/lib/opengslb \
  ghcr.io/loganrossus/opengslb:latest
```

## Port Reference

### Overwatch Ports

| Port | Protocol | Purpose | Required |
|------|----------|---------|----------|
| 53 | UDP | DNS queries (primary) | Yes |
| 53 | TCP | DNS queries (large responses, zone transfers) | Yes |
| 7946 | TCP/UDP | Gossip communication | Yes |
| 9090 | TCP | REST API | Yes |
| 9091 | TCP | Prometheus metrics | Recommended |

### Agent Ports

| Port | Protocol | Purpose | Required |
|------|----------|---------|----------|
| 9100 | TCP | Prometheus metrics | Optional |

## Configuration

### Overwatch Configuration

```yaml
# config/overwatch.yaml
mode: overwatch

overwatch:
  identity:
    node_id: overwatch-docker-1
    region: us-east

  agent_tokens:
    myapp: "secret-token-for-myapp"

  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "YOUR_BASE64_GOSSIP_KEY_HERE"

  validation:
    enabled: true
    check_interval: 30s
    check_timeout: 5s

  stale:
    threshold: 30s
    remove_after: 5m

  dnssec:
    enabled: true

  data_dir: /var/lib/opengslb

dns:
  listen_address: "0.0.0.0:53"
  default_ttl: 30
  zones:
    - gslb.example.com

regions:
  - name: us-east
    servers: []

domains:
  - name: myapp.gslb.example.com
    routing_algorithm: round-robin
    regions:
      - us-east

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9091"

api:
  enabled: true
  address: ":9090"
  allowed_networks:
    - 0.0.0.0/0  # Allow all (restrict in production)
```

### Agent Configuration

```yaml
# config/agent.yaml
mode: agent

agent:
  identity:
    service_token: "secret-token-for-myapp"
    region: us-east

  backends:
    - service: myapp
      address: host.docker.internal  # or actual backend IP
      port: 8080
      weight: 100
      health_check:
        type: http
        path: /health
        interval: 5s
        timeout: 2s
        failure_threshold: 3
        success_threshold: 2

  gossip:
    encryption_key: "YOUR_BASE64_GOSSIP_KEY_HERE"
    overwatch_nodes:
      - overwatch:7946

  heartbeat:
    interval: 10s

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9100"
```

## Docker Compose

### Development Environment

```yaml
# docker-compose.yml
version: '3.8'

services:
  overwatch:
    image: ghcr.io/loganrossus/opengslb:latest
    container_name: opengslb-overwatch
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "7946:7946/tcp"
      - "7946:7946/udp"
      - "9090:9090"
      - "9091:9091"
    volumes:
      - ./config/overwatch.yaml:/etc/opengslb/config.yaml:ro
      - overwatch-data:/var/lib/opengslb
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9090/api/v1/live"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s

  agent:
    image: ghcr.io/loganrossus/opengslb:latest
    container_name: opengslb-agent
    network_mode: host
    volumes:
      - ./config/agent.yaml:/etc/opengslb/config.yaml:ro
      - agent-data:/var/lib/opengslb
    restart: unless-stopped
    depends_on:
      - overwatch

  # Example backend service
  backend:
    image: nginx:alpine
    container_name: backend
    ports:
      - "8080:80"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/"]
      interval: 10s
      timeout: 5s

volumes:
  overwatch-data:
  agent-data:
```

### Production Environment (HA)

```yaml
# docker-compose.ha.yml
version: '3.8'

services:
  overwatch-1:
    image: ghcr.io/loganrossus/opengslb:latest
    container_name: opengslb-overwatch-1
    hostname: overwatch-1
    ports:
      - "10.0.1.53:53:53/udp"
      - "10.0.1.53:53:53/tcp"
      - "10.0.1.53:7946:7946/tcp"
      - "10.0.1.53:7946:7946/udp"
      - "10.0.1.53:9090:9090"
      - "10.0.1.53:9091:9091"
    volumes:
      - ./config/overwatch-1.yaml:/etc/opengslb/config.yaml:ro
      - ./geoip:/var/lib/opengslb/geoip:ro
      - overwatch-1-data:/var/lib/opengslb
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"

  overwatch-2:
    image: ghcr.io/loganrossus/opengslb:latest
    container_name: opengslb-overwatch-2
    hostname: overwatch-2
    ports:
      - "10.0.1.54:53:53/udp"
      - "10.0.1.54:53:53/tcp"
      - "10.0.1.54:7946:7946/tcp"
      - "10.0.1.54:7946:7946/udp"
      - "10.0.1.54:9090:9090"
      - "10.0.1.54:9091:9091"
    volumes:
      - ./config/overwatch-2.yaml:/etc/opengslb/config.yaml:ro
      - ./geoip:/var/lib/opengslb/geoip:ro
      - overwatch-2-data:/var/lib/opengslb
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 1G

  overwatch-3:
    image: ghcr.io/loganrossus/opengslb:latest
    container_name: opengslb-overwatch-3
    hostname: overwatch-3
    ports:
      - "10.0.1.55:53:53/udp"
      - "10.0.1.55:53:53/tcp"
      - "10.0.1.55:7946:7946/tcp"
      - "10.0.1.55:7946:7946/udp"
      - "10.0.1.55:9090:9090"
      - "10.0.1.55:9091:9091"
    volumes:
      - ./config/overwatch-3.yaml:/etc/opengslb/config.yaml:ro
      - ./geoip:/var/lib/opengslb/geoip:ro
      - overwatch-3-data:/var/lib/opengslb
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 1G

volumes:
  overwatch-1-data:
  overwatch-2-data:
  overwatch-3-data:
```

### With Monitoring Stack

```yaml
# docker-compose.monitoring.yml
version: '3.8'

services:
  overwatch:
    image: ghcr.io/loganrossus/opengslb:latest
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "7946:7946"
      - "9090:9090"
      - "9091:9091"
    volumes:
      - ./config/overwatch.yaml:/etc/opengslb/config.yaml:ro
      - overwatch-data:/var/lib/opengslb
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9092:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--storage.tsdb.retention.time=15d'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_USERS_ALLOW_SIGN_UP=false
    volumes:
      - grafana-data:/var/lib/grafana
    restart: unless-stopped
    depends_on:
      - prometheus

volumes:
  overwatch-data:
  prometheus-data:
  grafana-data:
```

Prometheus configuration:

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'opengslb-overwatch'
    static_configs:
      - targets: ['overwatch:9091']

  - job_name: 'opengslb-agent'
    static_configs:
      - targets: ['agent:9100']
```

## Networking

### Bridge Network (Default)

With bridge networking, containers can reach external IPs but not the host directly.

```yaml
# For backend on external network
backends:
  - service: myapp
    address: 10.0.1.100  # External IP works
    port: 8080
```

### Host Network Mode

For agents that need to reach local backends:

```bash
docker run -d \
  --name opengslb-agent \
  --network host \
  -v ./config/agent.yaml:/etc/opengslb/config.yaml:ro \
  ghcr.io/loganrossus/opengslb:latest
```

### Accessing Host Services

**Docker Desktop (Mac/Windows)**:
```yaml
backends:
  - service: myapp
    address: host.docker.internal
    port: 8080
```

**Linux**:
```bash
# Get host IP
docker network inspect bridge | grep Gateway
# Use that IP in configuration
```

## Volume Management

### Data Persistence

```bash
# Create named volumes
docker volume create opengslb-overwatch-data
docker volume create opengslb-agent-data

# Run with named volumes
docker run -d \
  -v opengslb-overwatch-data:/var/lib/opengslb \
  ...
```

### Backup Volumes

```bash
# Backup volume to tar
docker run --rm \
  -v opengslb-overwatch-data:/data:ro \
  -v $(pwd):/backup \
  alpine tar czf /backup/overwatch-backup.tar.gz -C /data .

# Restore from tar
docker run --rm \
  -v opengslb-overwatch-data:/data \
  -v $(pwd):/backup:ro \
  alpine tar xzf /backup/overwatch-backup.tar.gz -C /data
```

## Security Considerations

### Read-Only Root Filesystem

```yaml
services:
  overwatch:
    image: ghcr.io/loganrossus/opengslb:latest
    read_only: true
    tmpfs:
      - /tmp
    volumes:
      - ./config/overwatch.yaml:/etc/opengslb/config.yaml:ro
      - overwatch-data:/var/lib/opengslb
```

### Non-Root User

The container runs as non-root by default. If you need to verify:

```bash
docker exec opengslb-overwatch id
# Should show: uid=65532(nonroot) gid=65532(nonroot)
```

### Secret Management

For production, use Docker secrets or environment variable injection:

```yaml
services:
  overwatch:
    image: ghcr.io/loganrossus/opengslb:latest
    environment:
      - GOSSIP_KEY_FILE=/run/secrets/gossip_key
    secrets:
      - gossip_key

secrets:
  gossip_key:
    external: true
```

## Health Checks

```yaml
services:
  overwatch:
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9090/api/v1/live"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

Or using DNS:

```yaml
healthcheck:
  test: ["CMD", "dig", "@localhost", "configured-domain.example.com", "+short", "+time=2"]
  interval: 30s
  timeout: 10s
  retries: 3
```

## Logging

### View Logs

```bash
# Follow logs
docker logs -f opengslb-overwatch

# Last 100 lines
docker logs --tail 100 opengslb-overwatch

# Since timestamp
docker logs --since 2025-01-01T00:00:00 opengslb-overwatch
```

### Log Rotation

```yaml
services:
  overwatch:
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"
```

### External Logging

```yaml
services:
  overwatch:
    logging:
      driver: "syslog"
      options:
        syslog-address: "tcp://logserver:514"
        tag: "opengslb-overwatch"
```

## Upgrading

### Rolling Upgrade with Compose

```bash
# Pull new image
docker-compose pull

# Recreate containers one at a time
docker-compose up -d --no-deps overwatch-1
sleep 30
docker-compose up -d --no-deps overwatch-2
sleep 30
docker-compose up -d --no-deps overwatch-3
```

### Blue-Green Deployment

```bash
# Start new version
docker run -d --name opengslb-overwatch-new \
  -p 5353:53/udp \
  ...

# Test new version
dig @localhost -p 5353 myapp.gslb.example.com

# Switch traffic
docker stop opengslb-overwatch
docker rename opengslb-overwatch opengslb-overwatch-old
docker rename opengslb-overwatch-new opengslb-overwatch
docker start opengslb-overwatch

# Cleanup
docker rm opengslb-overwatch-old
```

## Troubleshooting

### Port 53 Conflict

```bash
# Check what's using port 53
sudo lsof -i :53

# Common culprit: systemd-resolved
sudo systemctl stop systemd-resolved
sudo systemctl disable systemd-resolved

# Or change DNS stub listener
sudo sed -i 's/#DNSStubListener=yes/DNSStubListener=no/' /etc/systemd/resolved.conf
sudo systemctl restart systemd-resolved
```

### Container Won't Start

```bash
# Check logs
docker logs opengslb-overwatch

# Common issues:
# - Config file not mounted correctly
# - Invalid YAML syntax
# - Port already in use
# - Permission denied on data volume
```

### Cannot Reach Backends

```bash
# Enter container and test
docker exec -it opengslb-overwatch sh
wget -O- http://10.0.1.100:8080/health

# Check DNS resolution
docker exec -it opengslb-overwatch nslookup backend
```

## Building Custom Image

```bash
# Clone repository
git clone https://github.com/loganrossus/OpenGSLB.git
cd OpenGSLB

# Build image
docker build -t opengslb:custom .

# Run custom image
docker run -d --name opengslb opengslb:custom
```

## Related Documentation

- [Overwatch Deployment](./overwatch.md)
- [Agent Deployment](./agent.md)
- [HA Setup Guide](./ha-setup.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
