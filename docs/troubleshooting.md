# OpenGSLB Troubleshooting Guide

This guide covers common issues and their solutions for both standalone and cluster deployments.

## Table of Contents

- [DNS Issues](#dns-issues)
- [Health Check Issues](#health-check-issues)
- [Configuration Issues](#configuration-issues)
- [Cluster Mode Issues](#cluster-mode-issues)
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

### DNS Queries Return REFUSED (Cluster Mode)

**Symptoms:**
- `dig` returns `status: REFUSED`
- Queries work on some nodes but not others

**Cause:** In cluster mode, only the Raft leader serves DNS queries. Non-leaders return REFUSED.

**Solution:**
1. Check which node is leader:
   ```bash
   curl http://localhost:8080/api/v1/cluster/status | jq '.is_leader'
   ```

2. Query the leader node directly, or configure clients to retry with multiple servers.

3. For anycast deployments, ensure only the leader is advertising the VIP.

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
   - `cluster.mode`
   - `cluster.bind_address`

3. **Verify reload was successful:**
   ```bash
   curl http://localhost:9090/metrics | grep opengslb_config_reload
   ```

---

## Cluster Mode Issues

### Leader Election Fails

**Symptoms:**
- No leader elected
- All nodes report `state: candidate` or `state: follower`
- DNS queries return REFUSED on all nodes

**Possible Causes:**

1. **Insufficient nodes for quorum**
   - 3-node cluster requires 2 nodes for quorum
   - 5-node cluster requires 3 nodes for quorum
   
   ```bash
   # Check cluster members
   curl http://localhost:8080/api/v1/cluster/members | jq
   ```

2. **Network partition** - Nodes cannot communicate
   ```bash
   # From node-1, test connectivity to node-2
   nc -zv node-2-ip 7000  # Raft port
   ```

3. **Clock skew** - Raft is sensitive to time differences
   ```bash
   # Check time sync status
   timedatectl status
   chronyc tracking
   ```

**Solutions:**
- Ensure majority of nodes are running and reachable
- Verify firewall allows Raft port traffic (default: 7000)
- Sync clocks with NTP

### Node Fails to Join Cluster

**Symptoms:**
- New node starts but doesn't appear in cluster members
- Logs show "failed to join cluster" errors

**Possible Causes:**

1. **Join address incorrect**
   ```bash
   # Verify join address is reachable
   curl http://join-address:8080/api/v1/cluster/status
   ```

2. **Node name conflict**
   - Each node must have unique `node_name`
   
3. **Data directory contains stale state**
   ```bash
   # Remove stale Raft data (WARNING: loses cluster state)
   rm -rf /var/lib/opengslb/raft/*
   ```

4. **Cluster not accepting new voters**
   - Leader must explicitly add voter (handled automatically by join)
   
   ```bash
   # Manual voter addition (from leader)
   curl -X POST http://localhost:8080/api/v1/cluster/join \
     -d '{"node_id": "new-node", "address": "new-node-ip:7000"}'
   ```

### Split-Brain Scenario

**Symptoms:**
- Multiple nodes claim to be leader
- Inconsistent DNS responses

**Cause:** Network partition caused cluster to split into isolated groups.

**Solution:**
1. **Identify the partition:**
   ```bash
   # On each node
   curl http://localhost:8080/api/v1/cluster/members
   ```

2. **Minority partition will lose quorum:**
   - Nodes in minority will demote to follower
   - DNS will return REFUSED on minority nodes

3. **Restore network connectivity:**
   - Once network heals, cluster will reconcile automatically

4. **Monitor for resolution:**
   ```bash
   watch -n 1 'curl -s http://localhost:8080/api/v1/cluster/status | jq'
   ```

### Gossip Communication Issues

**Symptoms:**
- Health updates not propagating between nodes
- `opengslb_gossip_members` metric lower than expected

**Possible Causes:**

1. **Gossip port blocked**
   ```bash
   # Test gossip connectivity (default port: 7946)
   nc -zvu other-node-ip 7946
   ```

2. **Encryption key mismatch**
   - All nodes must use identical `gossip.encryption_key`

3. **Bind address issues**
   - Ensure gossip binds to correct interface for multi-homed hosts

**Solutions:**
```bash
# Check gossip members
curl http://localhost:9090/metrics | grep opengslb_gossip

# Review gossip logs
journalctl -u opengslb | grep -i gossip
```

### Overwatch Veto Not Working

**Symptoms:**
- Agent claims server healthy
- External checks failing
- Server still receiving traffic

**Possible Causes:**

1. **Veto mode set to permissive**
   ```yaml
   cluster:
     overwatch:
       veto_mode: balanced  # or "strict"
   ```

2. **Veto threshold too high**
   ```yaml
   cluster:
     overwatch:
       veto_threshold: 3  # External failures before veto
   ```

3. **Not running as leader**
   - Overwatch only runs on Raft leader

**Solutions:**
```bash
# Check veto metrics
curl http://localhost:9090/metrics | grep opengslb_overwatch_veto

# Review overwatch logs
journalctl -u opengslb | grep -i overwatch
```

### Predictive Health Not Triggering

**Symptoms:**
- High CPU/memory but no bleed signal
- No predictive health metrics

**Possible Causes:**

1. **Predictive health disabled**
   ```yaml
   cluster:
     predictive_health:
       enabled: true
   ```

2. **Thresholds not reached**
   ```yaml
   cluster:
     predictive_health:
       cpu:
         threshold: 80  # Current CPU below this?
   ```

3. **Running in standalone mode**
   - Predictive health only works in cluster mode

**Solutions:**
```bash
# Check predictive metrics
curl http://localhost:9090/metrics | grep opengslb_predictive

# Review agent logs
journalctl -u opengslb | grep -i predictive
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
cat /etc/opengslb/config.yaml | grep -v key

# Metrics snapshot
curl http://localhost:9090/metrics > metrics.txt

# Recent logs
journalctl -u opengslb --since "1 hour ago" > logs.txt

# Cluster status (if applicable)
curl http://localhost:8080/api/v1/cluster/status > cluster-status.json
curl http://localhost:8080/api/v1/cluster/members > cluster-members.json
curl http://localhost:8080/api/v1/health/servers > health-status.json
```

---

## Getting Help

If you cannot resolve an issue:

1. **Search existing issues:** https://github.com/LoganRossUS/OpenGSLB/issues
2. **Create a new issue** with diagnostic information above
3. **Community support:** [Discussions](https://github.com/LoganRossUS/OpenGSLB/discussions)

For commercial support: licensing@opengslb.org