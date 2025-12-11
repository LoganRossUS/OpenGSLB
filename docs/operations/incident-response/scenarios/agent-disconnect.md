# Scenario: Agent Disconnection

## Symptoms

- Alert: `OpenGSLBStaleAgents`
- Backends showing as "stale" in status
- Metric: `opengslb_overwatch_stale_agents > 0`
- Agent logs show connection failures
- Heartbeat metrics not incrementing

## Impact

- **Severity**: SEV2-SEV3 (depends on number of agents)
- **User Impact**: Potentially stale health data, may route to unhealthy backends
- **Note**: Overwatch validation can compensate if enabled

## Diagnosis

### Step 1: Identify Stale Agents

```bash
# List all backends, filter for stale
opengslb-cli servers --api http://localhost:9090 | grep stale

# Or via API
curl http://localhost:9090/api/v1/overwatch/backends?status=stale | jq '.backends[] | {service, address, agent_id}'
```

### Step 2: Check Overwatch Stats

```bash
curl http://localhost:9090/api/v1/overwatch/stats | jq '{
    active_agents,
    stale_backends,
    total_backends
}'
```

### Step 3: Check Agent Health

On the agent server:

```bash
# Check service status
sudo systemctl status opengslb-agent

# Check recent logs
journalctl -u opengslb-agent -n 100 --no-pager

# Check for errors
journalctl -u opengslb-agent | grep -E "(error|fail|timeout|refused)"
```

### Step 4: Verify Network Connectivity

From agent to Overwatch:

```bash
# Test gossip port
nc -zv overwatch-1 7946
nc -zv overwatch-2 7946
nc -zv overwatch-3 7946

# Test with specific protocol
nc -zuv overwatch-1 7946  # UDP
```

### Step 5: Check Configuration

```bash
# Verify encryption key matches
# On agent
grep encryption_key /etc/opengslb/agent.yaml

# On Overwatch
grep encryption_key /etc/opengslb/overwatch.yaml

# Must be identical
```

```bash
# Verify Overwatch addresses in agent config
grep -A5 overwatch_nodes /etc/opengslb/agent.yaml
```

## Common Causes and Solutions

### Cause 1: Agent Service Stopped

```bash
# Check and restart
sudo systemctl status opengslb-agent
sudo systemctl restart opengslb-agent
```

### Cause 2: Network Connectivity Lost

Check firewall rules:

```bash
# On agent server
sudo iptables -L -n | grep 7946

# On Overwatch server
sudo iptables -L -n | grep 7946
```

Check security groups (cloud environments):
- Ensure port 7946 TCP/UDP is allowed between agent and Overwatch

### Cause 3: Gossip Encryption Key Mismatch

If key was recently rotated:

```bash
# Update agent config with new key
sudo vi /etc/opengslb/agent.yaml

# Restart agent
sudo systemctl restart opengslb-agent
```

### Cause 4: Certificate Revoked or Expired

```bash
# Check certificate on agent
openssl x509 -in /var/lib/opengslb/agent.crt -noout -dates

# If expired or revoked, remove and restart to regenerate
sudo systemctl stop opengslb-agent
sudo rm /var/lib/opengslb/agent.crt /var/lib/opengslb/agent.key
sudo systemctl start opengslb-agent
```

On Overwatch, you may need to delete the old certificate pin:

```bash
curl -X DELETE http://localhost:9090/api/v1/overwatch/agents/agent-id-here
```

### Cause 5: Overwatch Not Accepting Gossip

Check Overwatch gossip is listening:

```bash
ss -tulnp | grep 7946
```

Check Overwatch logs for gossip errors:

```bash
journalctl -u opengslb-overwatch | grep -i gossip
```

### Cause 6: Agent Token Invalid

Verify service token matches:

```bash
# On agent
grep service_token /etc/opengslb/agent.yaml

# On Overwatch (check agent_tokens section)
grep -A10 agent_tokens /etc/opengslb/overwatch.yaml
```

## Recovery Steps

### Step 1: Fix the Underlying Issue

Apply appropriate solution from above.

### Step 2: Verify Agent Reconnects

```bash
# Watch agent logs
journalctl -u opengslb-agent -f

# Should see successful registration messages
```

### Step 3: Verify on Overwatch

```bash
# Check stale count decreasing
watch -n5 'curl -s http://localhost:9090/api/v1/overwatch/stats | jq .stale_backends'
```

### Step 4: Verify Backend Health

```bash
opengslb-cli servers --api http://localhost:9090
```

## Temporary Mitigation

### If Overwatch Validation is Enabled

Overwatch will continue health checking even if agents are stale:

```yaml
# In Overwatch config
overwatch:
  validation:
    enabled: true  # External validation continues
```

Check validation is working:

```bash
curl http://localhost:9090/api/v1/overwatch/backends | jq '.backends[] | {address, validation_healthy, validation_last_check}'
```

### If You Know Backends Are Healthy

Set manual override to force healthy:

```bash
opengslb-cli overrides set myapp 10.0.1.10:8080 \
    --healthy=true \
    --reason="Agent disconnected but backend verified healthy" \
    --api http://localhost:9090
```

## Prevention

1. **Monitor agent heartbeats**: Alert early on missed heartbeats
2. **Redundant network paths**: Multiple routes between agents and Overwatches
3. **All Overwatches in gossip config**: Agent gossips to all Overwatches
4. **Enable Overwatch validation**: Secondary health checking path
5. **Certificate expiration monitoring**: Alert before agent certs expire

## Alerts

```yaml
- alert: OpenGSLBAgentStale
  expr: opengslb_overwatch_stale_agents > 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "{{ $value }} agents are stale"

- alert: OpenGSLBManyAgentsStale
  expr: opengslb_overwatch_stale_agents / opengslb_overwatch_agents_registered > 0.3
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "More than 30% of agents are stale"
```

## Related

- [Incident Response Playbook](../playbook.md)
- [All Backends Unhealthy](./all-unhealthy.md)
- [Certificate Rotation](../../security/certificate-rotation.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
