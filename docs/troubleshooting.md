# Troubleshooting Guide

This guide covers common issues when deploying and operating OpenGSLB.

## Startup Issues

### "config file has insecure permissions"

**Error:**
```
configuration file security check failed: config file /etc/opengslb/config.yaml has insecure permissions 0644 (world-readable)
```

**Cause:** OpenGSLB requires configuration files to not be world-readable for security.

**Solution:**
```bash
# Set secure permissions (owner + group read)
sudo chmod 640 /etc/opengslb/config.yaml

# Or owner-only
sudo chmod 600 /etc/opengslb/config.yaml
```

### "failed to load configuration"

**Error:**
```
failed to load configuration: yaml: unmarshal errors
```

**Cause:** Invalid YAML syntax in configuration file.

**Solution:**
1. Validate YAML syntax:
   ```bash
   python3 -c "import yaml; yaml.safe_load(open('/etc/opengslb/config.yaml'))"
   ```
2. Check for common issues:
   - Incorrect indentation (YAML requires consistent spaces, not tabs)
   - Missing colons after keys
   - Unquoted special characters

### "failed to stat config file"

**Error:**
```
configuration file security check failed: failed to stat config file: no such file or directory
```

**Cause:** Configuration file doesn't exist at the specified path.

**Solution:**
```bash
# Create config directory
sudo mkdir -p /etc/opengslb

# Copy example configuration
sudo cp config/example.yaml /etc/opengslb/config.yaml
sudo chmod 640 /etc/opengslb/config.yaml
```

### "listen udp :53: bind: permission denied"

**Error:**
```
DNS server error: listen udp :53: bind: permission denied
```

**Cause:** Port 53 requires root privileges.

**Solutions:**

1. **Run as root:**
   ```bash
   sudo ./opengslb --config /etc/opengslb/config.yaml
   ```

2. **Use a high port (development):**
   ```yaml
   dns:
     listen_address: ":5353"
   ```

3. **Use setcap (production):**
   ```bash
   sudo setcap 'cap_net_bind_service=+ep' ./opengslb
   ./opengslb --config /etc/opengslb/config.yaml
   ```

### "listen udp :53: bind: address already in use"

**Error:**
```
DNS server error: listen udp :53: bind: address already in use
```

**Cause:** Another process (often systemd-resolved) is using port 53.

**Solution:**

1. **Find the conflicting process:**
   ```bash
   sudo lsof -i :53
   sudo ss -tulpn | grep :53
   ```

2. **For systemd-resolved on Ubuntu/Debian:**
   ```bash
   # Option A: Disable stub listener
   sudo sed -i 's/#DNSStubListener=yes/DNSStubListener=no/' /etc/systemd/resolved.conf
   sudo systemctl restart systemd-resolved

   # Option B: Use a different port for OpenGSLB
   dns:
     listen_address: ":5353"
   ```

3. **For other DNS servers (dnsmasq, bind, etc.):**
   ```bash
   sudo systemctl stop dnsmasq  # or bind9, named, etc.
   sudo systemctl disable dnsmasq
   ```

## DNS Query Issues

### NXDOMAIN for Configured Domain

**Symptom:** `dig @localhost myapp.example.com` returns `NXDOMAIN`

**Possible causes:**

1. **Domain not configured:**
   ```bash
   # Check your configuration
   grep -A5 "domains:" /etc/opengslb/config.yaml
   ```

2. **Domain name mismatch:**
   - Domain names are case-insensitive but must match exactly
   - Check for typos or trailing dots

3. **OpenGSLB not running:**
   ```bash
   # Check if process is running
   pgrep -a opengslb
   
   # Check if listening
   sudo ss -tulpn | grep opengslb
   ```

### SERVFAIL Response

**Symptom:** `dig @localhost myapp.example.com` returns `SERVFAIL`

**Cause:** All backend servers for the domain are unhealthy.

**Diagnosis:**
```bash
# Check metrics for healthy server count
curl -s http://localhost:9090/metrics | grep opengslb_healthy_servers

# Check health check results
curl -s http://localhost:9090/metrics | grep opengslb_health_check_results_total
```

**Solutions:**

1. **Verify backends are reachable:**
   ```bash
   curl -v http://10.0.1.10:80/health
   ```

2. **Check health check configuration:**
   - Verify the path exists and returns 2xx
   - Ensure timeout < interval
   - Check firewall rules between OpenGSLB and backends

3. **Enable last-healthy fallback (if acceptable):**
   ```yaml
   dns:
     return_last_healthy: true
   ```

### Slow DNS Responses

**Symptom:** DNS queries take several seconds to respond.

**Possible causes:**

1. **Health checks timing out:**
   - Check if backends are slow to respond
   - Reduce health check timeout
   - Verify network connectivity

2. **Too many concurrent queries:**
   - Check query rate in metrics
   - Consider horizontal scaling

**Diagnosis:**
```bash
# Check query latency histogram
curl -s http://localhost:9090/metrics | grep opengslb_dns_query_duration_seconds

# Check health check duration
curl -s http://localhost:9090/metrics | grep opengslb_health_check_duration_seconds
```

### Wrong Server Returned

**Symptom:** DNS returns a server you didn't expect.

**Explanation:** With round-robin routing, different queries return different servers. This is expected behavior.

**Verification:**
```bash
# Query multiple times to see rotation
for i in {1..10}; do dig @localhost myapp.example.com +short; done
```

## Health Check Issues

### All Servers Show Unhealthy

**Symptom:** Metrics show all servers as unhealthy.

**Diagnosis checklist:**

1. **Verify backend is running:**
   ```bash
   curl -v http://<backend-ip>:<port>/health
   ```

2. **Check health check path:**
   ```yaml
   # Ensure this path exists and returns 2xx
   health_check:
     path: /health
   ```

3. **Check network connectivity:**
   ```bash
   # From OpenGSLB host
   telnet <backend-ip> <port>
   ```

4. **Check firewall rules:**
   ```bash
   # On backend server
   sudo iptables -L -n | grep <opengslb-ip>
   ```

5. **Verify timeout settings:**
   ```yaml
   health_check:
     timeout: 5s   # Must be less than interval
     interval: 30s
   ```

### Health Checks Flapping

**Symptom:** Servers frequently toggle between healthy/unhealthy.

**Causes:**
- Network instability
- Backend under heavy load
- Timeout too aggressive

**Solutions:**

1. **Increase failure threshold:**
   ```yaml
   health_check:
     failure_threshold: 5  # Require 5 failures before unhealthy
   ```

2. **Increase timeout:**
   ```yaml
   health_check:
     timeout: 10s
   ```

3. **Check backend health endpoint performance:**
   ```bash
   time curl http://<backend-ip>:<port>/health
   ```

### Health Checks Not Running

**Symptom:** Health check metrics not incrementing.

**Diagnosis:**
```bash
# Check if health manager started
grep "health manager" /var/log/opengslb.log

# Verify servers are registered
grep "registered server" /var/log/opengslb.log
```

**Solution:** Ensure regions have servers configured:
```yaml
regions:
  - name: my-region
    servers:        # Must have at least one server
      - address: 10.0.1.10
        port: 80
```

## Metrics Issues

### Metrics Endpoint Not Responding

**Symptom:** `curl http://localhost:9090/metrics` fails.

**Checklist:**

1. **Metrics enabled?**
   ```yaml
   metrics:
     enabled: true  # Must be true
     address: ":9090"
   ```

2. **Correct port?**
   ```bash
   sudo ss -tulpn | grep 9090
   ```

3. **Firewall blocking?**
   ```bash
   curl -v http://localhost:9090/metrics
   ```

### Missing Metrics

**Symptom:** Expected metrics not present.

**Cause:** Metrics appear after relevant operations occur. For example:
- `opengslb_dns_queries_total` appears after first DNS query
- `opengslb_routing_decisions_total` appears after first successful routing

**Solution:** Send test queries to populate metrics:
```bash
dig @localhost configured-domain.example.com
```

## Logging Issues

### No Log Output

**Symptom:** No logs appearing in stdout.

**Check log level:**
```yaml
logging:
  level: info  # Try "debug" for more output
```

### JSON Logs Hard to Read

**Solution:** Use text format for development:
```yaml
logging:
  format: text  # Human-readable
```

Or pipe JSON to jq:
```bash
./opengslb 2>&1 | jq .
```

## Docker Issues

### Container Exits Immediately

**Diagnosis:**
```bash
docker logs opengslb
```

**Common causes:**
- Configuration file not mounted
- Invalid configuration
- Port already in use on host

**Solution:**
```bash
# Ensure config is mounted correctly
docker run -d \
  -v $(pwd)/config:/etc/opengslb:ro \
  ghcr.io/loganrossus/opengslb:latest
```

### Cannot Reach Backends from Container

**Cause:** Docker network isolation.

**Solutions:**

1. **Use host network mode:**
   ```bash
   docker run --network=host ...
   ```

2. **Use backend container names (Docker Compose):**
   ```yaml
   # In OpenGSLB config
   servers:
     - address: backend-container-name
       port: 80
   ```

3. **Use host.docker.internal (Docker Desktop):**
   ```yaml
   servers:
     - address: host.docker.internal
       port: 8080
   ```

### DNS Port Conflict in Docker

**Solution:** Map to different host port:
```bash
docker run -d \
  -p 5353:53/udp \
  -p 5353:53/tcp \
  ...
```

Then query on the mapped port:
```bash
dig @localhost -p 5353 myapp.example.com
```

## Performance Issues

### High Memory Usage

**Possible causes:**
- Many domains/servers configured
- High query volume with debug logging

**Solutions:**
- Reduce log level to `info` or `warn`
- Monitor with `docker stats` or `top`

### High CPU Usage

**Diagnosis:**
```bash
# Check query rate
curl -s http://localhost:9090/metrics | grep opengslb_dns_queries_total
```

**If query rate is very high:**
- Consider rate limiting at network level
- Check for DNS amplification attack

## Getting Help

If you've exhausted this guide:

1. **Enable debug logging:**
   ```yaml
   logging:
     level: debug
     format: text
   ```

2. **Collect diagnostics:**
   ```bash
   # Configuration (redact sensitive data)
   cat /etc/opengslb/config.yaml
   
   # Metrics snapshot
   curl http://localhost:9090/metrics > metrics.txt
   
   # Recent logs
   journalctl -u opengslb -n 100  # if using systemd
   # or
   docker logs opengslb --tail 100
   ```

3. **Open an issue:** https://github.com/loganrossus/OpenGSLB/issues
   - Include Go version, OS, and OpenGSLB version
   - Describe expected vs. actual behavior
   - Include relevant logs and configuration