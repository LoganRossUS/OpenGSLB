# Manual Integration Testing - Lessons Learned

## Date: 2025-12-02
## Sprint: 2 (Story 5 - Component Integration)

## Context

After completing Story 5 (Component Integration), we performed manual testing to verify that the DNS server, health checks, and round-robin routing worked together as expected.

## Key Learnings

### 1. Health Check Path Must Return 2xx Status

**Problem**: Initial tests used Python's `http.server` module with `/health` path, which returned 404.

**Solution**: Either configure the health check path to match what the server provides (e.g., `/` for directory listing) or use a server that has a proper health endpoint.

**Takeaway**: When configuring health checks, verify the endpoint exists and returns a successful status code:
```bash
curl -I http://<server>:<port>/<health_path>
```

### 2. DNS A Records Don't Include Ports

**Problem**: When testing round-robin with multiple servers on the same IP (127.0.0.1) but different ports (8081, 8082, 8083), all DNS responses showed the same IP.

**Solution**: Use different IP addresses to visually verify round-robin rotation. On Linux, loopback aliases can be added:
```bash
sudo ip addr add 127.0.0.2/8 dev lo
sudo ip addr add 127.0.0.3/8 dev lo
```

**Takeaway**: For meaningful round-robin testing, servers need distinct IP addresses.

### 3. Port 5353 Conflicts with Avahi/mDNS

**Problem**: Default mDNS port (5353) is used by `avahi-daemon` on Linux systems.

**Solution**: Use an alternate port like 15353 for testing.

**Takeaway**: Avoid well-known service ports when testing. Use high ports (>10000) to avoid conflicts.

### 4. External IPs May Not Be Reachable

**Problem**: Public IPs like `93.184.216.34` (example.com) failed health checks due to network restrictions.

**Solution**: Use localhost servers for reliable testing.

**Takeaway**: Integration tests should use local mock servers, not external dependencies.

### 5. Health Check Timing Matters

**Problem**: Tests run immediately after startup fail because health checks haven't completed.

**Solution**: Wait for at least `interval * failure_threshold` seconds before testing. With `interval: 2s` and `failure_threshold: 2`, wait at least 5 seconds.

**Takeaway**: Allow health checks to stabilize before running DNS queries.

### 6. Duplicate Server Registration Not Allowed

**Problem**: Same server IP:port in multiple regions caused startup failure.

**Error**: `server 93.184.216.34:80 already registered`

**Solution**: Use unique server addresses across regions, or modify the health manager to handle duplicates.

**Takeaway**: Current design assumes each server is unique across all regions.

### 7. Config File Permissions Are Enforced

**Problem**: Config files with world-readable permissions (0644) are rejected.

**Solution**: Use secure permissions:
```bash
chmod 600 /path/to/config.yaml  # Owner read/write only
chmod 640 /path/to/config.yaml  # Owner read/write, group read
```

**Takeaway**: This is a security feature, not a bug.

## Test Environment Setup

### Prerequisites
- Go 1.23+
- Python 3 (for mock HTTP servers)
- `dig` command (dnsutils package)
- Linux with `ip` command for loopback aliases

### Quick Test Commands

```bash
# Add loopback aliases
sudo ip addr add 127.0.0.2/8 dev lo
sudo ip addr add 127.0.0.3/8 dev lo

# Start mock servers (separate terminals)
python3 -m http.server 8081 --bind 127.0.0.1
python3 -m http.server 8082 --bind 127.0.0.2
python3 -m http.server 8083 --bind 127.0.0.3

# Run OpenGSLB
./opengslb --config /path/to/config.yaml

# Test round-robin
for i in {1..6}; do
    dig @127.0.0.1 -p 15353 test.local A +short
    sleep 0.3
done
```

## Recommended Test Configuration

```yaml
dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 30

regions:
  - name: local-region
    servers:
      - address: "127.0.0.1"
        port: 8081
      - address: "127.0.0.2"
        port: 8082
      - address: "127.0.0.3"
        port: 8083
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

domains:
  - name: test.local
    routing_algorithm: round-robin
    regions:
      - local-region
    ttl: 10
```

## Future Improvements

1. **Add TCP health checks** - Not all services expose HTTP endpoints
2. **Support duplicate servers** - Allow same server in multiple regions
3. **Add metrics** - Expose health check status via Prometheus
4. **Debug logging** - Show routing decisions in logs
5. **Automated integration test** - Script to set up environment and run tests
