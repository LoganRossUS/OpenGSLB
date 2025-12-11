# Rollback Procedures

This document describes procedures for rolling back OpenGSLB to a previous version when issues occur after an upgrade.

## When to Rollback

Consider rollback when:

- Service degradation after upgrade
- Critical functionality broken
- Unexpected errors in logs
- Performance regression
- Breaking changes not anticipated

## Rollback Decision Tree

```
Issue Detected After Upgrade
            │
            ▼
    Is service functional?
            │
     ┌──────┴──────┐
     │ Yes         │ No
     ▼             ▼
  Monitor      Immediate
  Closely      Rollback
     │
     ▼
  Improves within 15min?
     │
  ┌──┴──┐
  │Yes  │No
  ▼     ▼
 Keep  Rollback
```

## Pre-Rollback Checklist

Before rolling back:

- [ ] Document the issue (logs, metrics, symptoms)
- [ ] Verify backup of current state exists
- [ ] Locate previous version binary
- [ ] Notify team/stakeholders
- [ ] Confirm previous configuration is compatible

## Rollback Procedures

### Overwatch Rollback (Single Node)

```bash
# 1. Stop current service
sudo systemctl stop opengslb-overwatch

# 2. Restore previous binary
# Option A: From backup
sudo cp /usr/local/bin/opengslb.backup /usr/local/bin/opengslb

# Option B: Download previous version
PREVIOUS_VERSION="0.5.0"
curl -Lo /tmp/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${PREVIOUS_VERSION}/opengslb-linux-amd64
chmod +x /tmp/opengslb
sudo mv /tmp/opengslb /usr/local/bin/opengslb

# 3. Restore capabilities
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/opengslb

# 4. Restore configuration if needed
sudo cp /etc/opengslb/overwatch.yaml.backup /etc/opengslb/overwatch.yaml

# 5. Restore data directory if needed
sudo rm -rf /var/lib/opengslb
sudo cp -r /var/lib/opengslb.backup /var/lib/opengslb
sudo chown -R opengslb:opengslb /var/lib/opengslb

# 6. Start service
sudo systemctl start opengslb-overwatch

# 7. Verify
sudo systemctl status opengslb-overwatch
opengslb --version
curl http://localhost:9090/api/v1/ready
```

### Overwatch Rollback (HA - Rolling)

```bash
#!/bin/bash
# rolling-rollback.sh

PREVIOUS_VERSION="0.5.0"
OVERWATCHES="overwatch-1 overwatch-2 overwatch-3"
PAUSE_SECONDS=60

# Download previous binary
curl -Lo /tmp/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${PREVIOUS_VERSION}/opengslb-linux-amd64
chmod +x /tmp/opengslb

for host in $OVERWATCHES; do
    echo "=== Rolling back $host ==="

    # Copy binary
    scp /tmp/opengslb ${host}:/tmp/opengslb

    # Execute rollback
    ssh ${host} << 'ROLLBACK'
        sudo systemctl stop opengslb-overwatch
        sudo mv /tmp/opengslb /usr/local/bin/opengslb
        sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/opengslb

        # Restore config if backup exists
        if [ -f /etc/opengslb/overwatch.yaml.backup ]; then
            sudo cp /etc/opengslb/overwatch.yaml.backup /etc/opengslb/overwatch.yaml
        fi

        sudo systemctl start opengslb-overwatch
ROLLBACK

    # Wait for stabilization
    echo "Waiting ${PAUSE_SECONDS}s..."
    sleep $PAUSE_SECONDS

    # Verify
    ssh ${host} "curl -s http://localhost:9090/api/v1/ready"
    ssh ${host} "opengslb --version"

    echo "=== $host rollback complete ==="
done

echo "Rolling rollback complete!"
```

### Emergency Rollback (All Nodes Simultaneously)

Use only when service is completely broken:

```bash
#!/bin/bash
# emergency-rollback.sh

PREVIOUS_VERSION="0.5.0"
OVERWATCHES="overwatch-1 overwatch-2 overwatch-3"

# Download binary
curl -Lo /tmp/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${PREVIOUS_VERSION}/opengslb-linux-amd64
chmod +x /tmp/opengslb

# Stop all simultaneously
echo "Stopping all Overwatches..."
for host in $OVERWATCHES; do
    ssh ${host} "sudo systemctl stop opengslb-overwatch" &
done
wait

# Update all
echo "Updating all binaries..."
for host in $OVERWATCHES; do
    scp /tmp/opengslb ${host}:/tmp/opengslb
    ssh ${host} "sudo mv /tmp/opengslb /usr/local/bin/opengslb && sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/opengslb" &
done
wait

# Start all
echo "Starting all Overwatches..."
for host in $OVERWATCHES; do
    ssh ${host} "sudo systemctl start opengslb-overwatch" &
done
wait

echo "Emergency rollback complete. Verify all nodes!"
```

### Docker Rollback

```bash
# Stop current container
docker stop opengslb-overwatch
docker rm opengslb-overwatch

# Run previous version
docker run -d \
  --name opengslb-overwatch \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 7946:7946 \
  -p 9090:9090 \
  -p 9091:9091 \
  -v ./config/overwatch.yaml:/etc/opengslb/config.yaml:ro \
  -v opengslb-data:/var/lib/opengslb \
  ghcr.io/loganrossus/opengslb:v0.5.0

# Verify
docker logs opengslb-overwatch
```

### Docker Compose Rollback

```bash
# Edit docker-compose.yml to use previous version
# image: ghcr.io/loganrossus/opengslb:v0.5.0

# Or rollback via command line
docker-compose down
docker-compose pull  # With old tag in compose file
docker-compose up -d
```

### Agent Rollback

```bash
# 1. Stop agent
sudo systemctl stop opengslb-agent

# 2. Restore previous binary
PREVIOUS_VERSION="0.5.0"
curl -Lo /tmp/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v${PREVIOUS_VERSION}/opengslb-linux-amd64
chmod +x /tmp/opengslb
sudo mv /tmp/opengslb /usr/local/bin/opengslb

# 3. Start agent
sudo systemctl start opengslb-agent

# 4. Verify
journalctl -u opengslb-agent -n 20
```

## Configuration Rollback

If configuration changes caused the issue:

```bash
# 1. View backup
cat /etc/opengslb/overwatch.yaml.backup

# 2. Compare with current
diff /etc/opengslb/overwatch.yaml.backup /etc/opengslb/overwatch.yaml

# 3. Restore backup
sudo cp /etc/opengslb/overwatch.yaml.backup /etc/opengslb/overwatch.yaml

# 4. Reload service
sudo systemctl reload opengslb-overwatch
# Or restart if reload doesn't apply changes
sudo systemctl restart opengslb-overwatch
```

## Data Rollback

For corrupted or incompatible data:

```bash
# 1. Stop service
sudo systemctl stop opengslb-overwatch

# 2. Backup current (corrupted) data
sudo mv /var/lib/opengslb /var/lib/opengslb.corrupted

# 3. Restore backup
sudo cp -r /var/lib/opengslb.backup /var/lib/opengslb
sudo chown -R opengslb:opengslb /var/lib/opengslb

# 4. Start service
sudo systemctl start opengslb-overwatch
```

**Note**: Rolling back data may lose:
- Agent certificate pins (agents need to re-register)
- Runtime custom geo mappings
- Override history

## Post-Rollback Verification

After rollback:

```bash
# 1. Verify version
opengslb --version
# Should show previous version

# 2. Check service status
sudo systemctl status opengslb-overwatch

# 3. Verify DNS is working
dig @localhost myapp.gslb.example.com

# 4. Check API
curl http://localhost:9090/api/v1/ready

# 5. Verify backends
opengslb-cli servers --api http://localhost:9090

# 6. Check logs for errors
journalctl -u opengslb-overwatch -n 100 --no-pager | grep -i error

# 7. Monitor metrics
curl http://localhost:9091/metrics | grep -E "(queries_total|backends_healthy)"
```

## Rollback Considerations

### DNSSEC After Rollback

If DNSSEC keys changed during upgrade:

1. Keys may need to be re-synchronized
2. DS records in parent zone may need update (unlikely for minor versions)
3. Force key sync after rollback:
   ```bash
   curl -X POST http://localhost:9090/api/v1/dnssec/sync
   ```

### Agent Re-Registration

If data was rolled back, agents may need to re-register:

1. Agents with TOFU certs may be rejected (cert mismatch)
2. Options:
   - Clear agent data and let them re-register
   - Delete pinned certs from Overwatch data
   - Restart agents to trigger re-registration

```bash
# On Overwatch, list agents
curl http://localhost:9090/api/v1/overwatch/agents

# Delete specific agent cert if needed
curl -X DELETE http://localhost:9090/api/v1/overwatch/agents/agent-123
```

### Custom Geo Mappings

Runtime geo mappings (added via API) are stored in KV:

- Rollback may lose these mappings
- Re-add via API or configuration file

## Rollback Recovery Time Objectives

| Scenario | Target RTO |
|----------|------------|
| Single node | < 5 minutes |
| HA rolling | < 30 minutes |
| Emergency all-nodes | < 10 minutes |
| Docker | < 3 minutes |

## Post-Incident Actions

After rollback:

1. **Document the incident**
   - What went wrong
   - Timeline of events
   - Root cause analysis

2. **File issue if needed**
   - Report to OpenGSLB GitHub if it's a bug

3. **Plan retry**
   - Fix configuration issues
   - Wait for patch release
   - Test more thoroughly in staging

## Related Documentation

- [Upgrade Procedures](./upgrades.md)
- [Backup and Restore](./backup-restore.md)
- [Incident Response](../incident-response/playbook.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
