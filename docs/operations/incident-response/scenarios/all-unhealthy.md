# Scenario: All Backends Unhealthy

## Symptoms

- Alert: `OpenGSLBNoHealthyServers`
- DNS queries return SERVFAIL (if `return_last_healthy: false`)
- DNS queries return stale IPs (if `return_last_healthy: true`)
- Metric: `opengslb_overwatch_backends_healthy == 0`
- User reports: Cannot reach application

## Impact

- **Severity**: SEV1
- **User Impact**: Complete service unavailability (unless `return_last_healthy: true`)
- **Duration**: Until at least one backend recovers

## Diagnosis

### Step 1: Verify All Backends Are Indeed Unhealthy

```bash
# List all backends with health status
opengslb-cli servers --api http://localhost:9090

# Check API directly
curl http://localhost:9090/api/v1/overwatch/backends | jq '.backends[] | {service, address, effective_status}'
```

### Step 2: Determine Why Backends Are Unhealthy

Check the source of unhealthy status:

```bash
# Get detailed backend info
curl http://localhost:9090/api/v1/overwatch/backends | jq '.backends[] | {
    service,
    address,
    agent_healthy,
    validation_healthy,
    validation_error,
    override_status
}'
```

Possible causes:

| Source | Indicator | Likely Cause |
|--------|-----------|--------------|
| `agent_healthy: false` | Agent reporting unhealthy | Backend service down |
| `validation_healthy: false` | Overwatch validation failed | Network issue or service down |
| `override_status: false` | Manual override | Someone marked unhealthy |
| All stale | `effective_status: stale` | All agents disconnected |

### Step 3: Check Backend Services Directly

```bash
# Test backends directly (bypass Overwatch)
for ip in 10.0.1.10 10.0.1.11 10.0.1.12; do
    echo "=== $ip ==="
    curl -s -o /dev/null -w "%{http_code}" http://${ip}:8080/health
done
```

### Step 4: Check Network Connectivity

```bash
# From Overwatch to backends
for ip in 10.0.1.10 10.0.1.11 10.0.1.12; do
    echo "=== $ip ==="
    nc -zv $ip 8080
done
```

### Step 5: Check Agent Status

```bash
# Are agents sending heartbeats?
curl http://localhost:9090/api/v1/overwatch/stats | jq '{
    active_agents,
    stale_backends
}'

# List agents
curl http://localhost:9090/api/v1/overwatch/agents | jq '.agents[].agent_id'
```

## Resolution

### If Backends Are Actually Down

**This is expected behavior** - Overwatch correctly reports unhealthy.

1. Fix the backend services (application team responsibility)
2. Once backends are healthy, Overwatch will automatically detect

### If Network Partition (Overwatch Can't Reach Backends)

```bash
# Check firewall rules
sudo iptables -L -n

# Check routing
ip route get 10.0.1.10

# Check from Overwatch directly
curl http://10.0.1.10:8080/health
```

Fix network issue to restore connectivity.

### If Overrides Are Blocking

```bash
# List active overrides
opengslb-cli overrides list --api http://localhost:9090

# Clear overrides if appropriate
opengslb-cli overrides clear myapp 10.0.1.10:8080 --api http://localhost:9090
```

### If All Agents Disconnected

```bash
# Check gossip connectivity
nc -zv overwatch 7946

# On agent servers, check agent status
journalctl -u opengslb-agent -n 50

# Check encryption key matches
# (Compare between agent and overwatch configs)
```

### Emergency: Force Traffic to Specific Backend

If you know a backend is healthy but validation fails:

```bash
# Override to force healthy
opengslb-cli overrides set myapp 10.0.1.10:8080 \
    --healthy=true \
    --reason="Emergency override - backend verified healthy manually" \
    --api http://localhost:9090
```

**Warning**: This bypasses health checking. Use only in emergencies.

### Emergency: Enable return_last_healthy

If DNS must return something:

```yaml
# In config
dns:
  return_last_healthy: true  # Return last known healthy IPs when all unhealthy
```

```bash
sudo systemctl reload opengslb-overwatch
```

## Prevention

1. **Multiple backends**: Always have N+1 capacity
2. **Multi-region**: Distribute backends across regions
3. **Health check tuning**: Ensure thresholds aren't too aggressive
4. **Monitoring**: Alert before all backends are unhealthy
5. **Graceful degradation**: Use `return_last_healthy: true` if appropriate

## Alerts to Add

```yaml
- alert: OpenGSLBFewHealthyBackends
  expr: opengslb_overwatch_backends_healthy < 2
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "Only {{ $value }} healthy backends remaining"

- alert: OpenGSLBNoHealthyBackends
  expr: opengslb_overwatch_backends_healthy == 0
  for: 30s
  labels:
    severity: critical
  annotations:
    summary: "No healthy backends - service unavailable"
```

## Related

- [Incident Response Playbook](../playbook.md)
- [Agent Disconnection](./agent-disconnect.md)
- [Overwatch Down](./overwatch-down.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
