# Scenario: Overwatch Down

## Symptoms

- Alert: `OverwatchDown`
- DNS queries to specific Overwatch timeout
- Metrics endpoint not responding
- systemd service failed/stopped
- Container not running (Docker deployments)

## Impact

- **Severity**: SEV2 (single node), SEV1 (all nodes)
- **User Impact**:
  - Single Overwatch: Clients retry to other Overwatches
  - All Overwatches: DNS service unavailable
- **HA Note**: With proper client configuration, single Overwatch failure has minimal impact

## Diagnosis

### Step 1: Check Overwatch Status

```bash
# Check service status
sudo systemctl status opengslb-overwatch

# Check if process is running
ps aux | grep opengslb

# Check listening ports
ss -tulnp | grep -E "(53|7946|9090|9091)"
```

### Step 2: Check System Resources

```bash
# Disk space
df -h

# Memory
free -m

# CPU load
uptime

# System logs
dmesg | tail -50
```

### Step 3: Check Overwatch Logs

```bash
# Recent logs
journalctl -u opengslb-overwatch -n 200 --no-pager

# Errors only
journalctl -u opengslb-overwatch -p err --since "1 hour ago"

# Follow logs during restart
journalctl -u opengslb-overwatch -f
```

### Step 4: Test DNS Manually

```bash
# On the Overwatch server
dig @127.0.0.1 myapp.gslb.example.com

# From remote client
dig @overwatch-ip myapp.gslb.example.com
```

### Step 5: Check Other Overwatches (HA)

```bash
for ow in overwatch-{1,2,3}; do
    echo "=== $ow ==="
    ssh $ow "sudo systemctl status opengslb-overwatch --no-pager"
done
```

## Common Causes and Solutions

### Cause 1: Service Crashed

Check for crash:

```bash
journalctl -u opengslb-overwatch | grep -E "(panic|fatal|killed)"
```

Restart service:

```bash
sudo systemctl restart opengslb-overwatch
```

If recurring, check logs for root cause and file issue.

### Cause 2: Configuration Error

After config change, service may fail to start:

```bash
# Validate configuration
opengslb --config=/etc/opengslb/overwatch.yaml --validate

# Check last config change
ls -la /etc/opengslb/overwatch.yaml
git -C /etc/opengslb log --oneline -5  # If version controlled
```

Fix configuration and restart.

### Cause 3: Port Conflict

Another process using port 53:

```bash
sudo lsof -i :53
sudo ss -tulnp | grep :53
```

Stop conflicting service (often systemd-resolved):

```bash
sudo systemctl stop systemd-resolved
sudo systemctl disable systemd-resolved
```

### Cause 4: Out of Memory

```bash
dmesg | grep -i "out of memory"
journalctl -k | grep -i oom
```

Solutions:
- Increase server memory
- Increase container memory limits
- Check for memory leaks (file issue if suspected)

### Cause 5: Disk Full

```bash
df -h /var/lib/opengslb
```

Clean up:

```bash
# Remove old backups
find /var/lib/opengslb -name "*.backup" -mtime +7 -delete

# Clear old logs
journalctl --vacuum-time=7d
```

### Cause 6: Certificate/Key Issues

DNSSEC or TLS issues:

```bash
journalctl -u opengslb-overwatch | grep -i "certificate\|key\|tls"

# Check DNSSEC keys
ls -la /var/lib/opengslb/dnssec/
```

### Cause 7: Network Interface Down

```bash
ip addr show
ip link show
```

Bring interface up:

```bash
sudo ip link set eth0 up
```

## Recovery Steps

### Step 1: Restart the Service

```bash
sudo systemctl restart opengslb-overwatch
```

### Step 2: Verify Service Started

```bash
sudo systemctl status opengslb-overwatch
curl http://localhost:9090/api/v1/ready
```

### Step 3: Verify DNS Working

```bash
dig @localhost myapp.gslb.example.com +short
```

### Step 4: Verify Gossip Receiving

```bash
# Check for agent connections
curl http://localhost:9090/api/v1/overwatch/stats | jq .active_agents
```

### Step 5: Verify DNSSEC (if enabled)

```bash
curl http://localhost:9090/api/v1/dnssec/status | jq .enabled
dig @localhost myapp.gslb.example.com +dnssec
```

## Docker Recovery

### Check Container Status

```bash
docker ps -a | grep opengslb
docker logs opengslb-overwatch
```

### Restart Container

```bash
docker restart opengslb-overwatch
```

### Recreate Container

```bash
docker stop opengslb-overwatch
docker rm opengslb-overwatch
docker run -d \
    --name opengslb-overwatch \
    -p 53:53/udp -p 53:53/tcp \
    -p 7946:7946 \
    -p 9090:9090 -p 9091:9091 \
    -v ./config/overwatch.yaml:/etc/opengslb/config.yaml:ro \
    -v opengslb-data:/var/lib/opengslb \
    ghcr.io/loganrossus/opengslb:latest
```

## All Overwatches Down

### Emergency Procedure

1. **Start any single Overwatch first**
   ```bash
   sudo systemctl start opengslb-overwatch
   ```

2. **Update DNS clients to point to working Overwatch**
   - Update resolv.conf
   - Update DNS forwarding
   - Temporary: hardcode IP

3. **Bring up remaining Overwatches**
   ```bash
   for host in overwatch-{2,3}; do
       ssh $host "sudo systemctl start opengslb-overwatch"
   done
   ```

4. **Restore normal DNS configuration**

### Disaster Recovery

If all Overwatches are lost, see [Backup and Restore](../../maintenance/backup-restore.md).

## Prevention

1. **High Availability**: Deploy multiple Overwatches
2. **Resource Monitoring**: Alert on disk, memory, CPU
3. **Health Checks**: Monitor liveness and readiness endpoints
4. **Graceful Degradation**: Configure `return_last_healthy`
5. **Automated Recovery**: Use systemd restart policies

## Systemd Restart Policy

```ini
# In /etc/systemd/system/opengslb-overwatch.service
[Service]
Restart=on-failure
RestartSec=5
```

## Alerts

```yaml
- alert: OverwatchDown
  expr: up{job="opengslb-overwatch"} == 0
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "Overwatch {{ $labels.instance }} is down"

- alert: AllOverwatchesDown
  expr: sum(up{job="opengslb-overwatch"}) == 0
  for: 30s
  labels:
    severity: critical
  annotations:
    summary: "All Overwatch nodes are down - DNS unavailable"

- alert: OverwatchHighMemory
  expr: process_resident_memory_bytes{job="opengslb-overwatch"} > 1e9
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Overwatch using more than 1GB memory"
```

## Related

- [Incident Response Playbook](../playbook.md)
- [HA Setup Guide](../../deployment/ha-setup.md)
- [Backup and Restore](../../maintenance/backup-restore.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
