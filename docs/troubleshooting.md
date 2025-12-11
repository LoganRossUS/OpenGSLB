# OpenGSLB Troubleshooting Guide

This guide covers common issues and their solutions for Agent and Overwatch mode deployments.

## Table of Contents

- [DNS Issues](#dns-issues)
- [Health Check Issues](#health-check-issues)
- [Configuration Issues](#configuration-issues)
- [Agent Mode Issues](#agent-mode-issues)
- [Overwatch Mode Issues](#overwatch-mode-issues)
- [Performance Issues](#performance-issues)
- [Logging and Debugging](#logging-and-debugging)

---

## DNS Issues

### DNS Queries Return SERVFAIL

**Symptoms:**
- `dig` returns `status: SERVFAIL`
- No IP addresses in DNS response

**Possible Causes:**

1. **All backend servers unhealthy**
   ```bash
   # Check server health status
   curl http://localhost:8080/api/v1/health/servers | jq '.servers[] | select(.healthy == false)'
   ```

   **Solution:** Verify backend servers are running and health check endpoints are accessible.

2. **Health checks not yet completed**
   ```bash
   # Check readiness
   curl http://localhost:8080/api/v1/ready
   ```

   **Solution:** Wait for initial health checks to complete (typically 5-10 seconds after startup).

3. **Configuration validation error**
   ```bash
   # Check logs for validation errors
   journalctl -u opengslb | grep -i "validation\|error"
   ```

### DNS Queries Return NXDOMAIN

**Symptoms:**
- `dig` returns `status: NXDOMAIN`
- Domain exists in configuration but not resolved

**Possible Causes:**

1. **Domain not configured**
   ```bash
   # List configured domains
   curl http://localhost:8080/api/v1/health/servers | jq '.servers[].domain' | sort -u
   ```

2. **Domain name mismatch** (trailing dot, case sensitivity)
   - DNS queries include trailing dot: `app.example.com.`
   - Configuration should not include trailing dot: `app.example.com`

3. **No servers in domain's regions**
   - Check that the domain references valid regions with servers

### High DNS Query Latency

**Symptoms:**
- DNS responses take >100ms
- Timeouts on client side

**Possible Causes:**

1. **Health check blocking** - DNS handler waiting for health status
2. **Router algorithm inefficiency** - Complex routing taking too long
3. **Resource exhaustion** - CPU/memory issues

**Solutions:**
```bash
# Check DNS query duration metrics
curl http://localhost:9090/metrics | grep opengslb_dns_query_duration

# Check system resources
top -p $(pgrep opengslb)
```

---

## Health Check Issues

### All Servers Marked Unhealthy

**Symptoms:**
- All servers show `healthy: false`
- DNS returns SERVFAIL

**Diagnostic Steps:**

1. **Verify backend connectivity:**
   ```bash
   # From OpenGSLB host
   curl -v http://backend-ip:port/health
   ```

2. **Check health check configuration:**
   ```yaml
   health_check:
     type: http
     path: /health  # Correct path?
     timeout: 5s    # Sufficient timeout?
   ```

3. **Review health check logs:**
   ```bash
   journalctl -u opengslb | grep -i "health check failed"
   ```

### Health Check Flapping

**Symptoms:**
- Server rapidly alternates between healthy/unhealthy
- Frequent log entries about state changes

**Possible Causes:**

1. **Timeout too aggressive**
   ```yaml
   health_check:
     timeout: 10s  # Increase from default
   ```

2. **Thresholds too low**
   ```yaml
   health_check:
     failure_threshold: 3  # Require 3 failures before marking unhealthy
     success_threshold: 2  # Require 2 successes before marking healthy
   ```

3. **Backend intermittently failing** - Check backend logs

### TCP Health Checks Failing

**Symptoms:**
- TCP health checks report connection refused
- Backend service is running

**Possible Causes:**

1. **Firewall blocking connections**
   ```bash
   # Test connectivity
   nc -zv backend-ip port
   ```

2. **Service not listening on expected interface**
   ```bash
   # On backend server
   ss -tlnp | grep :port
   ```

3. **Connection limit reached on backend**

---

## Configuration Issues

### Configuration Reload Failed

**Symptoms:**
- SIGHUP sent but configuration unchanged
- Log shows "configuration reload failed"

**Solutions:**

1. **Validate configuration before reload:**
   ```bash
   opengslb --config /etc/opengslb/config.yaml --validate
   ```

2. **Check for syntax errors:**
   ```bash
   # YAML syntax validation
   python3 -c "import yaml; yaml.safe_load(open('/etc/opengslb/config.yaml'))"
   ```

3. **Review reload logs:**
   ```bash
   journalctl -u opengslb | grep -i reload
   ```

### Configuration Changes Not Taking Effect

**Symptoms:**
- Configuration file updated but behavior unchanged

**Solutions:**

1. **Send SIGHUP to reload:**
   ```bash
   sudo systemctl reload opengslb
   # or
   sudo kill -SIGHUP $(pgrep opengslb)
   ```

2. **Some changes require restart:**
   - `dns.listen_address`
   - `mode` (agent/overwatch)
   - `overwatch.gossip.bind_address`
   - `agent.gossip.overwatch_nodes`

3. **Verify reload was successful:**
   ```bash
   curl http://localhost:9090/metrics | grep opengslb_config_reload
   ```

---

## Agent Mode Issues

### Agent Not Sending Heartbeats

**Symptoms:**
- Agent logs show no heartbeat activity
- Overwatch shows backends as stale

**Possible Causes:**

1. **Gossip not configured**
   ```yaml
   agent:
     gossip:
       encryption_key: "your-32-byte-base64-key"
       overwatch_nodes:
         - "overwatch-1.internal:7946"
   ```

2. **Network connectivity to Overwatch**
   ```bash
   # Test gossip port connectivity
   nc -zv overwatch-ip 7946
   ```

3. **Encryption key mismatch**
   - Agent and Overwatch must use identical `encryption_key`

**Solutions:**
```bash
# Check agent gossip metrics
curl http://localhost:9090/metrics | grep opengslb_gossip

# Review agent logs
journalctl -u opengslb | grep -i "gossip\|heartbeat"
```

### Agent Health Checks Not Running

**Symptoms:**
- No health check metrics for agent backends
- Backend status unknown

**Possible Causes:**

1. **Backends not configured**
   ```yaml
   agent:
     backends:
       - service: "my-service"
         address: "127.0.0.1"
         port: 8080
         health_check:
           type: http
           path: /health
   ```

2. **Health check interval too long**
   ```yaml
   agent:
     backends:
       - service: "my-service"
         health_check:
           interval: 10s  # Default may be longer
   ```

**Solutions:**
```bash
# Check health check status
curl http://localhost:9090/metrics | grep opengslb_health_check

# Review agent backend status
journalctl -u opengslb | grep -i "backend\|health"
```

### Agent Certificate Issues

**Symptoms:**
- Agent fails to start with certificate errors
- Gossip connection rejected

**Possible Causes:**

1. **Certificate paths incorrect**
   ```yaml
   agent:
     identity:
       cert_path: /var/lib/opengslb/agent.crt
       key_path: /var/lib/opengslb/agent.key
   ```

2. **Certificate permissions**
   ```bash
   # Check file permissions
   ls -la /var/lib/opengslb/agent.*

   # Fix permissions
   chmod 600 /var/lib/opengslb/agent.key
   chmod 644 /var/lib/opengslb/agent.crt
   ```

3. **Certificate not trusted by Overwatch**
   - Ensure agent tokens are configured on Overwatch

---

## Overwatch Mode Issues

### Backends Not Registering

**Symptoms:**
- Overwatch shows no backends
- Agent heartbeats not received

**Possible Causes:**

1. **Gossip port not accessible**
   ```bash
   # Check gossip listener
   ss -tlnp | grep 7946

   # Test from agent
   nc -zv overwatch-ip 7946
   ```

2. **Agent tokens not configured**
   ```yaml
   overwatch:
     agent_tokens:
       my-service: "service-token-here"
   ```

3. **Encryption key mismatch**
   - All agents and Overwatch nodes must use the same key

**Solutions:**
```bash
# Check Overwatch backend registry
curl http://localhost:8080/api/v1/overwatch/backends | jq

# Review gossip logs
journalctl -u opengslb | grep -i gossip
```

### External Validation Disagreements

**Symptoms:**
- Overwatch validation disagrees with agent claims
- Backend marked unhealthy despite agent claiming healthy

**This is expected behavior.** Per ADR-015, Overwatch validation ALWAYS wins over agent claims.

**Investigate disagreements:**
```bash
# Check validation status
curl http://localhost:8080/api/v1/overwatch/backends | jq '.backends[] | select(.validation_healthy != .agent_healthy)'

# Check validation metrics
curl http://localhost:9090/metrics | grep opengslb_overwatch_validation

# Review disagreement logs
journalctl -u opengslb | grep -i "disagrees with agent"
```

**Possible causes of disagreement:**
1. **Network path differences** - Overwatch can't reach backend that agent can
2. **Different health check configuration** - Agent and Overwatch using different paths/ports
3. **Intermittent failures** - Backend flapping, caught at different times

### Backends Going Stale

**Symptoms:**
- Backends marked as `stale` status
- `agent_last_seen` timestamp is old

**Possible Causes:**

1. **Agent stopped or crashed**
   ```bash
   # Check agent status on backend server
   systemctl status opengslb
   ```

2. **Network partition between agent and Overwatch**
   ```bash
   # Test connectivity from agent to Overwatch
   nc -zv overwatch-ip 7946
   ```

3. **Stale threshold too aggressive**
   ```yaml
   overwatch:
     stale:
       threshold: 30s      # Time before marking stale
       remove_after: 5m    # Time before removing
   ```

**Solutions:**
```bash
# Check stale backends
curl http://localhost:8080/api/v1/overwatch/backends?status=stale | jq

# Check stale metrics
curl http://localhost:9090/metrics | grep opengslb_overwatch_stale
```

### Manual Override Not Working

**Symptoms:**
- Override API returns success but backend status unchanged
- Override not persisting

**Diagnostic Steps:**

1. **Verify override was set:**
   ```bash
   curl http://localhost:8080/api/v1/overwatch/backends | jq '.backends[] | select(.override_status != null)'
   ```

2. **Check override takes precedence:**
   - Override > Validation > Staleness > Agent claim
   - Backend must not be stale for override to show in effective status

3. **Review persistence:**
   ```bash
   # Check if bbolt store is working
   journalctl -u opengslb | grep -i "store\|persist"
   ```

**Setting an override:**
```bash
# Force backend healthy
curl -X POST http://localhost:8080/api/v1/overwatch/backends/my-service/10.0.1.10/80/override \
  -H "Content-Type: application/json" \
  -H "X-User: admin" \
  -d '{"healthy": true, "reason": "maintenance bypass"}'

# Clear override
curl -X DELETE http://localhost:8080/api/v1/overwatch/backends/my-service/10.0.1.10/80/override
```

---

## Performance Issues

### High Memory Usage

**Symptoms:**
- Memory usage >500MB for small configurations
- OOM kills

**Solutions:**

1. **Check for goroutine leaks:**
   ```bash
   curl http://localhost:9090/debug/pprof/goroutine?debug=2
   ```

2. **Reduce health check frequency:**
   ```yaml
   health_check:
     interval: 30s  # Increase from default
   ```

3. **Limit concurrent checks:**
   - Consider reducing number of monitored servers

### High CPU Usage

**Symptoms:**
- CPU >50% under normal load
- Slow DNS responses

**Solutions:**

1. **Profile the application:**
   ```bash
   curl http://localhost:9090/debug/pprof/profile?seconds=30 > cpu.prof
   go tool pprof cpu.prof
   ```

2. **Check for excessive logging:**
   ```yaml
   logging:
     level: info  # Avoid "debug" in production
   ```

3. **Review routing algorithm:**
   - Weighted routing is slightly more CPU-intensive than round-robin

---

## Logging and Debugging

### Enable Debug Logging

Temporarily increase log verbosity:

```yaml
logging:
  level: debug
  format: json
```

Or via environment variable:
```bash
OPENGSLB_LOG_LEVEL=debug opengslb --config config.yaml
```

### View Real-Time Logs

```bash
# systemd
journalctl -u opengslb -f

# Docker
docker logs -f opengslb

# Direct
./opengslb --config config.yaml 2>&1 | tee opengslb.log
```

### Export Diagnostic Information

For support requests, gather:

```bash
# System info
uname -a
cat /etc/os-release

# OpenGSLB version
opengslb --version

# Configuration (sanitize secrets)
cat /etc/opengslb/config.yaml | grep -v key | grep -v token

# Metrics snapshot
curl http://localhost:9090/metrics > metrics.txt

# Recent logs
journalctl -u opengslb --since "1 hour ago" > logs.txt

# Overwatch status (if applicable)
curl http://localhost:8080/api/v1/overwatch/backends > backends.json
curl http://localhost:8080/api/v1/overwatch/stats > stats.json
curl http://localhost:8080/api/v1/health/servers > health-status.json
```

---

## Getting Help

If you cannot resolve an issue:

1. **Search existing issues:** https://github.com/LoganRossUS/OpenGSLB/issues
2. **Create a new issue** with diagnostic information above
3. **Community support:** [Discussions](https://github.com/LoganRossUS/OpenGSLB/discussions)

For commercial support: licensing@opengslb.org
