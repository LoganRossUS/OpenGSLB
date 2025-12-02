# Docker Deployment Guide

This guide covers deploying OpenGSLB using Docker containers.

## Quick Start

```bash
# Pull the latest image
docker pull ghcr.io/loganrossus/opengslb:latest

# Run with default configuration
docker run -d \
  --name opengslb \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 9090:9090 \
  -v $(pwd)/config.yaml:/etc/opengslb/config.yaml:ro \
  ghcr.io/loganrossus/opengslb:latest
```

## Image Tags

| Tag | Description |
|-----|-------------|
| `latest` | Most recent build from main branch |
| `main` | Same as latest |
| `develop` | Latest development build |
| `v1.0.0` | Specific version (when available) |
| `sha-abc1234` | Specific commit build |

## Configuration

### Mounting Configuration File

Create your configuration file and mount it into the container:

```bash
# Create config directory
mkdir -p ./config

# Create configuration file
cat > ./config/config.yaml << 'EOF'
dns:
  listen_address: ":53"
  default_ttl: 60

logging:
  level: info
  format: json

metrics:
  enabled: true
  address: ":9090"

regions:
  - name: primary
    servers:
      - address: 10.0.1.10
        port: 80
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /health
      failure_threshold: 3
      success_threshold: 2

domains:
  - name: app.example.com
    routing_algorithm: round-robin
    regions:
      - primary
EOF

# Run with mounted config
docker run -d \
  --name opengslb \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 9090:9090 \
  -v $(pwd)/config:/etc/opengslb:ro \
  ghcr.io/loganrossus/opengslb:latest
```

**Note:** The configuration file inside the container must have secure permissions. When mounting, ensure the source file has appropriate permissions (600 or 640).

## Port Mappings

| Container Port | Protocol | Purpose |
|----------------|----------|---------|
| 53 | UDP | DNS queries (primary) |
| 53 | TCP | DNS queries (for large responses) |
| 9090 | TCP | Prometheus metrics |

### Using Non-Standard Ports

If port 53 is in use on the host:

```bash
docker run -d \
  --name opengslb \
  -p 5353:53/udp \
  -p 5353:53/tcp \
  -p 9090:9090 \
  -v $(pwd)/config:/etc/opengslb:ro \
  ghcr.io/loganrossus/opengslb:latest
```

Query using the mapped port:
```bash
dig @localhost -p 5353 app.example.com
```

## Networking

### Bridge Network (Default)

With the default bridge network, containers can reach external IPs but may have issues reaching host services.

```bash
# Backend on external network - works fine
servers:
  - address: 10.0.1.10
    port: 80
```

### Host Network Mode

For production deployments or when backends run on the same host:

```bash
docker run -d \
  --name opengslb \
  --network=host \
  -v $(pwd)/config:/etc/opengslb:ro \
  ghcr.io/loganrossus/opengslb:latest
```

**Notes:**
- Port mappings are ignored in host mode
- OpenGSLB binds directly to host network interfaces
- Better performance (no NAT overhead)

### Accessing Host Services

To reach services running on the Docker host:

**Docker Desktop (Mac/Windows):**
```yaml
servers:
  - address: host.docker.internal
    port: 8080
```

**Linux:**
```bash
# Get host IP from container's perspective
docker run --rm alpine ip route | grep default | awk '{print $3}'
# Use that IP in configuration
```

Or use host network mode.

## Docker Compose

### Basic Setup

```yaml
# docker-compose.yml
version: '3.8'

services:
  opengslb:
    image: ghcr.io/loganrossus/opengslb:latest
    container_name: opengslb
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "9090:9090"
    volumes:
      - ./config:/etc/opengslb:ro
    restart: unless-stopped
```

### With Backend Services

```yaml
# docker-compose.yml
version: '3.8'

services:
  opengslb:
    image: ghcr.io/loganrossus/opengslb:latest
    container_name: opengslb
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "9090:9090"
    volumes:
      - ./config:/etc/opengslb:ro
    depends_on:
      - backend1
      - backend2
    restart: unless-stopped

  backend1:
    image: nginx:alpine
    container_name: backend1
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 10s
      timeout: 5s

  backend2:
    image: nginx:alpine
    container_name: backend2
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 10s
      timeout: 5s
```

With this setup, use container names in configuration:

```yaml
# config/config.yaml
regions:
  - name: local
    servers:
      - address: backend1
        port: 80
      - address: backend2
        port: 80
    health_check:
      type: http
      interval: 30s
      timeout: 5s
      path: /
```

### With Prometheus Stack

```yaml
# docker-compose.yml
version: '3.8'

services:
  opengslb:
    image: ghcr.io/loganrossus/opengslb:latest
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "9090:9090"
    volumes:
      - ./config:/etc/opengslb:ro
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9091:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    restart: unless-stopped
```

Prometheus configuration:
```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'opengslb'
    static_configs:
      - targets: ['opengslb:9090']
```

## Health Checks

Add Docker health checks to monitor OpenGSLB:

```yaml
services:
  opengslb:
    image: ghcr.io/loganrossus/opengslb:latest
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

Or using the DNS itself:
```yaml
healthcheck:
  test: ["CMD", "dig", "@localhost", "configured-domain.example.com", "+short", "+time=2"]
  interval: 30s
  timeout: 10s
  retries: 3
```

## Resource Limits

Set resource constraints for production:

```yaml
services:
  opengslb:
    image: ghcr.io/loganrossus/opengslb:latest
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 256M
        reservations:
          cpus: '0.25'
          memory: 64M
```

## Logging

### View Logs

```bash
# Follow logs
docker logs -f opengslb

# Last 100 lines
docker logs --tail 100 opengslb

# Since timestamp
docker logs --since 2024-01-01T00:00:00 opengslb
```

### Log Drivers

For production, configure a log driver:

```yaml
services:
  opengslb:
    image: ghcr.io/loganrossus/opengslb:latest
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

Or send to external logging:

```yaml
services:
  opengslb:
    image: ghcr.io/loganrossus/opengslb:latest
    logging:
      driver: "syslog"
      options:
        syslog-address: "tcp://logserver:514"
        tag: "opengslb"
```

## Updating

### Pull New Image

```bash
# Pull latest
docker pull ghcr.io/loganrossus/opengslb:latest

# Recreate container
docker stop opengslb
docker rm opengslb
docker run -d ... # same run command as before
```

### With Docker Compose

```bash
docker-compose pull
docker-compose up -d
```

## Building from Source

If you need to build the image locally:

```bash
git clone https://github.com/loganrossus/OpenGSLB.git
cd OpenGSLB

# Build image
docker build -t opengslb:local .

# Run local build
docker run -d \
  --name opengslb \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 9090:9090 \
  -v $(pwd)/config:/etc/opengslb:ro \
  opengslb:local
```

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker logs opengslb

# Common issues:
# - Config file not mounted correctly
# - Invalid YAML in config
# - Port already in use
```

### Cannot Reach Backends

```bash
# Enter container and test connectivity
docker exec -it opengslb sh
wget -O- http://10.0.1.10:80/health
```

### Port 53 Conflict

On Linux, systemd-resolved often uses port 53:

```bash
# Check what's using port 53
sudo lsof -i :53

# Option 1: Disable systemd-resolved stub listener
sudo sed -i 's/#DNSStubListener=yes/DNSStubListener=no/' /etc/systemd/resolved.conf
sudo systemctl restart systemd-resolved

# Option 2: Use a different host port
docker run -p 5353:53/udp ...
```

### Permission Denied on Config

Ensure the config file has proper permissions:

```bash
chmod 640 ./config/config.yaml
```

If using SELinux:
```bash
chcon -Rt svirt_sandbox_file_t ./config/
```

## Security Considerations

1. **Run as non-root** (default in the image)
2. **Read-only config mount**: Use `:ro` flag
3. **No privileged mode**: Don't use `--privileged`
4. **Network policies**: Restrict which networks can query DNS
5. **Resource limits**: Prevent resource exhaustion attacks

## Production Checklist

- [ ] Configuration file has secure permissions (640 or 600)
- [ ] Health checks configured
- [ ] Resource limits set
- [ ] Log rotation configured
- [ ] Monitoring/alerting on metrics endpoint
- [ ] Backup strategy for configuration
- [ ] Update procedure documented
- [ ] Network security (firewall rules, network policies)