# Scenario: DNSSEC Issues

## Symptoms

- DNS queries return SERVFAIL when validating resolver is used
- `dig +dnssec` shows no RRSIG records
- DS record mismatch errors in validator logs
- Alert: DNSSEC validation failing
- Clients using validating resolvers cannot resolve

## Impact

- **Severity**: SEV2
- **User Impact**:
  - Clients using DNSSEC-validating resolvers: Cannot resolve
  - Clients without validation: No impact
- **Note**: Most end-user resolvers don't validate DNSSEC, but security-conscious organizations do

## Diagnosis

### Step 1: Check DNSSEC Status

```bash
# Check if DNSSEC is enabled
curl http://localhost:9090/api/v1/dnssec/status | jq .

# Expected output
{
  "enabled": true,
  "keys": [...],
  "sync": {
    "enabled": true,
    "last_sync": "...",
    "last_sync_error": null
  }
}
```

### Step 2: Verify DNS Responses Include Signatures

```bash
# Query with DNSSEC
dig @localhost myapp.gslb.example.com +dnssec

# Should include RRSIG records
# Look for: flags: qr aa rd

# Query DNSKEY
dig @localhost gslb.example.com DNSKEY +dnssec
```

### Step 3: Check DS Record in Parent Zone

```bash
# Query parent zone for DS record
dig DS gslb.example.com

# Or trace the delegation
dig +trace myapp.gslb.example.com
```

### Step 4: Compare DS and DNSKEY

```bash
# Get DS from Overwatch
curl http://localhost:9090/api/v1/dnssec/ds | jq '.ds_records[0].key_tag'

# Get DNSKEY from DNS
dig @localhost gslb.example.com DNSKEY | grep -oP '\d+ IN DNSKEY \d+ \d+ \d+ \K\S+'
```

### Step 5: Validate Chain

```bash
# Using delv (from bind-utils)
delv @localhost myapp.gslb.example.com

# Should say "fully validated" or show error

# Using drill
drill -DS myapp.gslb.example.com @localhost
```

### Step 6: Check Key Sync (HA Deployment)

```bash
# Check sync status
curl http://localhost:9090/api/v1/dnssec/status | jq '.sync'

# Compare key tags across Overwatches
for ow in overwatch-{1,2,3}; do
    echo "=== $ow ==="
    curl -s http://${ow}:9090/api/v1/dnssec/status | jq '.keys[].key_tag'
done
```

## Common Causes and Solutions

### Cause 1: DS Record Not in Parent Zone

The DS record was never added to parent zone, or was removed.

**Diagnosis:**
```bash
dig DS gslb.example.com +short
# Empty = no DS record
```

**Solution:**
1. Get DS record from Overwatch:
   ```bash
   curl http://localhost:9090/api/v1/dnssec/ds | jq '.ds_records[0].ds_record'
   ```

2. Add to parent zone (via registrar or parent zone admin)

3. Wait for propagation (minutes to hours)

### Cause 2: DS Record Mismatch (Key Rotation)

Keys were rotated but DS record not updated in parent.

**Diagnosis:**
```bash
# Compare key tag in DS vs DNSKEY
dig DS gslb.example.com +short  # Shows parent's DS key_tag
curl http://localhost:9090/api/v1/dnssec/ds | jq '.ds_records[].key_tag'  # Current key_tag
```

**Solution:**
1. Update DS record in parent zone with new value
2. Wait for propagation

### Cause 3: Keys Not Synced Between Overwatches

Different Overwatches have different keys.

**Diagnosis:**
```bash
for ow in overwatch-{1,2,3}; do
    echo "=== $ow ==="
    curl -s http://${ow}:9090/api/v1/dnssec/status | jq '.keys[].key_tag'
done
# Should all show same key_tag
```

**Solution:**
```bash
# Force key sync
curl -X POST http://localhost:9090/api/v1/dnssec/sync

# Check sync completed
curl http://localhost:9090/api/v1/dnssec/status | jq '.sync.last_sync_error'
```

### Cause 4: DNSSEC Disabled in Config

**Diagnosis:**
```bash
grep -A5 dnssec /etc/opengslb/overwatch.yaml
```

**Solution:**
Enable DNSSEC in config:
```yaml
dnssec:
  enabled: true
```

Reload:
```bash
sudo systemctl reload opengslb-overwatch
```

### Cause 5: Key Generation Failed

Keys may not have been generated on first start.

**Diagnosis:**
```bash
ls -la /var/lib/opengslb/dnssec/
# Should contain key files

curl http://localhost:9090/api/v1/dnssec/status | jq '.keys'
# Should not be empty
```

**Solution:**
```bash
# Restart to regenerate keys
sudo systemctl restart opengslb-overwatch

# Get new DS record and update parent zone
curl http://localhost:9090/api/v1/dnssec/ds
```

### Cause 6: Clock Skew

DNSSEC signatures have validity periods. Clock skew can cause issues.

**Diagnosis:**
```bash
# Check system time
date
timedatectl status

# Compare with NTP
ntpq -p
```

**Solution:**
```bash
# Sync time
sudo systemctl restart systemd-timesyncd
# or
sudo ntpdate pool.ntp.org
```

## Emergency: Disable DNSSEC

If DNSSEC issues are causing outages and can't be quickly fixed:

```yaml
# WARNING: This removes DNSSEC security
dnssec:
  enabled: false
  security_acknowledgment: "I understand that disabling DNSSEC allows DNS spoofing attacks"
```

```bash
sudo systemctl reload opengslb-overwatch
```

**Note**: You should also remove the DS record from parent zone to avoid validation failures.

## Recovery Verification

After fixing, verify:

```bash
# 1. Check DNSSEC status
curl http://localhost:9090/api/v1/dnssec/status | jq .

# 2. Query with DNSSEC
dig @localhost myapp.gslb.example.com +dnssec | grep RRSIG

# 3. Validate chain
delv @localhost myapp.gslb.example.com

# 4. Test from external validating resolver
dig @8.8.8.8 myapp.gslb.example.com
# (Assuming DS is correctly set up)
```

## Prevention

1. **Monitor DNSSEC status**: Alert on sync failures
2. **Document DS record process**: Know how to update parent zone
3. **Key rotation procedure**: Follow [DNSSEC Key Rotation](../../security/key-rotation.md)
4. **Multiple Overwatches**: Ensure key sync is configured
5. **Time sync**: Use NTP on all servers

## Alerts

```yaml
- alert: DNSSECSyncFailed
  expr: time() - opengslb_dnssec_last_sync_timestamp > 7200
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "DNSSEC key sync hasn't succeeded in 2 hours"

- alert: DNSSECDisabled
  expr: opengslb_dnssec_enabled == 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "DNSSEC is disabled"
```

## Useful Tools

- `dig`: DNS queries with DNSSEC options
- `delv`: DNSSEC validation debugger (bind-utils)
- `drill`: DNS query tool with DNSSEC support
- `dnsviz.net`: Online DNSSEC visualization
- `verisignlabs.com/dnssec-debugger`: Online debugger

## Related

- [Incident Response Playbook](../playbook.md)
- [DNSSEC Key Rotation](../../security/key-rotation.md)
- [HA Setup Guide](../../deployment/ha-setup.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
