# Upgrade Procedures

This document describes procedures for upgrading OpenGSLB components with minimal service disruption.

## Upgrade Strategy Overview

OpenGSLB supports zero-downtime upgrades through:

1. **Rolling upgrades**: Update nodes one at a time
2. **DNS client retry**: Automatic failover during node updates
3. **Backwards compatibility**: New versions work with existing agents (within major version)
4. **Configuration compatibility**: Existing configs work with new versions

## Pre-Upgrade Checklist

Before any upgrade:

- [ ] Review [CHANGELOG](https://github.com/loganrossus/OpenGSLB/blob/main/CHANGELOG.md) for breaking changes
- [ ] Backup current configuration files
- [ ] Backup KV store data (`/var/lib/opengslb/`)
- [ ] Verify current version: `opengslb --version`
- [ ] Test new version in staging environment
- [ ] Ensure monitoring is active
- [ ] Schedule maintenance window (if required)
- [ ] Notify stakeholders

## Version Compatibility

| Component | Compatible Versions | Notes |
|-----------|--------------------|----- |
| Overwatch ↔ Agent | Same major version | v0.6.x works with v0.6.y |
| DNSSEC Keys | All versions | Keys are version-independent |
| Configuration | See CHANGELOG | New options may be added |

## Upgrading Overwatch Nodes

### Single Node Upgrade

For non-HA deployments:

```bash
# 1. Backup current state
sudo cp -r /var/lib/opengslb /var/lib/opengslb.backup
sudo cp /etc/opengslb/overwatch.yaml /etc/opengslb/overwatch.yaml.backup

# 2. Download new version
VERSION="1.0.0"
curl -Lo /tmp/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${VERSION}/opengslb-linux-amd64
chmod +x /tmp/opengslb

# 3. Verify download
/tmp/opengslb --version

# 4. Stop service
sudo systemctl stop opengslb-overwatch

# 5. Replace binary
sudo mv /tmp/opengslb /usr/local/bin/opengslb

# 6. Restore capabilities
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/opengslb

# 7. Start service
sudo systemctl start opengslb-overwatch

# 8. Verify
sudo systemctl status opengslb-overwatch
opengslb-cli status --api http://localhost:9090
```

### Rolling Upgrade (HA Deployment)

For multiple Overwatches:

```bash
#!/bin/bash
# rolling-upgrade.sh

VERSION="1.0.0"
OVERWATCHES="overwatch-1 overwatch-2 overwatch-3"
PAUSE_SECONDS=60

# Download new binary
curl -Lo /tmp/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${VERSION}/opengslb-linux-amd64
chmod +x /tmp/opengslb

for host in $OVERWATCHES; do
    echo "=== Upgrading $host ==="

    # Copy binary
    scp /tmp/opengslb ${host}:/tmp/opengslb

    # Execute upgrade
    ssh ${host} << 'UPGRADE'
        sudo systemctl stop opengslb-overwatch
        sudo mv /tmp/opengslb /usr/local/bin/opengslb
        sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/opengslb
        sudo systemctl start opengslb-overwatch
UPGRADE

    # Wait for node to stabilize
    echo "Waiting ${PAUSE_SECONDS}s for stabilization..."
    sleep $PAUSE_SECONDS

    # Verify
    ssh ${host} "curl -s http://localhost:9090/api/v1/ready | jq .ready"

    echo "=== $host upgrade complete ==="
done

echo "Rolling upgrade complete!"
```

### Docker Upgrade

```bash
# Pull new image
docker pull ghcr.io/loganrossus/opengslb:v0.6.0

# Stop and remove old container
docker stop opengslb-overwatch
docker rm opengslb-overwatch

# Start with new image
docker run -d \
  --name opengslb-overwatch \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 7946:7946 \
  -p 9090:9090 \
  -p 9091:9091 \
  -v ./config/overwatch.yaml:/etc/opengslb/config.yaml:ro \
  -v opengslb-data:/var/lib/opengslb \
  ghcr.io/loganrossus/opengslb:v0.6.0
```

### Docker Compose Upgrade

```bash
# Update image tag in docker-compose.yml, then:
docker-compose pull
docker-compose up -d
```

## Upgrading Agents

Agents can be upgraded independently of Overwatches (within same major version).

### Single Agent Upgrade

```bash
# 1. Download new version
VERSION="1.0.0"
curl -Lo /tmp/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${VERSION}/opengslb-linux-amd64
chmod +x /tmp/opengslb

# 2. Stop service
sudo systemctl stop opengslb-agent

# 3. Replace binary
sudo mv /tmp/opengslb /usr/local/bin/opengslb

# 4. Start service
sudo systemctl start opengslb-agent

# 5. Verify
journalctl -u opengslb-agent -n 20
```

### Fleet Agent Upgrade

For many agents, use automation tools:

**Ansible Example:**

```yaml
# upgrade-agents.yml
- hosts: opengslb_agents
  become: yes
  serial: 5  # Upgrade 5 at a time
  vars:
    opengslb_version: "0.6.0"
  tasks:
    - name: Download new binary
      get_url:
        url: "https://github.com/loganrossus/OpenGSLB/releases/download/v{{ opengslb_version }}/opengslb-linux-amd64"
        dest: /tmp/opengslb
        mode: '0755'

    - name: Stop agent
      systemd:
        name: opengslb-agent
        state: stopped

    - name: Install new binary
      copy:
        src: /tmp/opengslb
        dest: /usr/local/bin/opengslb
        remote_src: yes
        mode: '0755'

    - name: Start agent
      systemd:
        name: opengslb-agent
        state: started

    - name: Wait for agent to register
      wait_for:
        timeout: 30
```

## Upgrading CLI Tool

```bash
VERSION="1.0.0"
curl -Lo /tmp/opengslb-cli https://github.com/loganrossus/OpenGSLB/releases/download/v${VERSION}/opengslb-cli-linux-amd64
chmod +x /tmp/opengslb-cli
sudo mv /tmp/opengslb-cli /usr/local/bin/opengslb-cli

# Verify
opengslb-cli --version
```

## Configuration Migration

### Checking for Deprecated Options

```bash
# Validate configuration with new version
/tmp/opengslb --config=/etc/opengslb/overwatch.yaml --validate

# Check for deprecation warnings in logs
journalctl -u opengslb-overwatch | grep -i deprecat
```

### Adding New Configuration Options

New versions may add optional configuration. Example migration:

```yaml
# Old config (v0.5.x)
overwatch:
  validation:
    enabled: true
    check_interval: 30s

# New config (v0.6.x) with new options
overwatch:
  validation:
    enabled: true
    check_interval: 30s
    check_timeout: 5s  # New in v0.6

  # New section in v0.6
  geolocation:
    database_path: "/var/lib/opengslb/geoip/GeoLite2-Country.mmdb"
    default_region: us-east
```

## Post-Upgrade Verification

### Functional Tests

```bash
# 1. Check version
opengslb --version

# 2. Check service status
sudo systemctl status opengslb-overwatch

# 3. Check API
curl http://localhost:9090/api/v1/ready

# 4. Test DNS
dig @localhost myapp.gslb.example.com

# 5. Check metrics
curl http://localhost:9091/metrics | head -20

# 6. Verify agents registered
opengslb-cli servers --api http://localhost:9090
```

### Monitoring Checks

After upgrade, verify:

- [ ] DNS query rate returned to normal
- [ ] No increase in error rate
- [ ] All backends showing healthy
- [ ] No stale agents
- [ ] DNSSEC validation working

```promql
# Check for anomalies
rate(opengslb_dns_queries_total[5m])
rate(opengslb_dns_queries_total{status!="success"}[5m])
opengslb_overwatch_backends_healthy
```

## Rollback Procedures

If issues occur after upgrade, see [Rollback Procedures](./rollback.md).

## Upgrade Path Matrix

| From Version | To Version | Direct Upgrade | Notes |
|--------------|------------|----------------|-------|
| 0.5.x | 0.6.x | Yes | Add geolocation config for new features |
| 0.4.x | 0.6.x | Yes | Review config changes |
| 0.3.x | 0.6.x | Test first | Legacy mode deprecated |

## Troubleshooting Upgrades

### Service Won't Start After Upgrade

```bash
# Check logs
journalctl -u opengslb-overwatch -n 100 --no-pager

# Common issues:
# - Config syntax errors (validate with --validate)
# - Missing capabilities (re-run setcap)
# - Permission issues on data directory
```

### Agents Not Reconnecting

```bash
# On agent, check logs
journalctl -u opengslb-agent -n 50

# Verify connectivity
nc -zv overwatch-1:7946

# Check gossip encryption key matches
```

### DNSSEC Issues After Upgrade

```bash
# Verify keys
curl http://localhost:9090/api/v1/dnssec/status | jq .

# Force key sync
curl -X POST http://localhost:9090/api/v1/dnssec/sync

# Check DNSSEC in DNS response
dig @localhost myapp.gslb.example.com +dnssec
```

## Related Documentation

- [Rollback Procedures](./rollback.md)
- [Backup and Restore](./backup-restore.md)
- [HA Setup Guide](../deployment/ha-setup.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
