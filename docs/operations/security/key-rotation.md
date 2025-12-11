# DNSSEC Key Rotation Guide

This document describes procedures for rotating DNSSEC keys in OpenGSLB.

## Overview

DNSSEC uses cryptographic keys to sign DNS records. Key rotation is necessary for:

- Security best practices (limit key exposure time)
- Algorithm upgrades
- Compromised key response
- Compliance requirements

## Key Types

OpenGSLB uses a single key signing model:

| Key | Purpose | Algorithm | Recommended Rotation |
|-----|---------|-----------|---------------------|
| ZSK (Zone Signing Key) | Signs DNS records | ECDSAP256SHA256 | Annually |

**Note**: OpenGSLB currently uses a combined KSK/ZSK model with a single key.

## Pre-Rotation Checklist

- [ ] Notify stakeholders (DNS admins, security team)
- [ ] Verify access to parent zone for DS record update
- [ ] Backup current DNSSEC keys
- [ ] Schedule maintenance window
- [ ] Test procedure in staging environment
- [ ] Ensure all Overwatches are healthy

## Standard Key Rotation Procedure

### Step 1: Backup Current Keys

```bash
# Backup DNSSEC keys
sudo cp -r /var/lib/opengslb/dnssec /var/lib/opengslb/dnssec.backup.$(date +%Y%m%d)

# Or encrypted backup
tar -czf - /var/lib/opengslb/dnssec | \
    gpg --symmetric --cipher-algo AES256 \
    > /backup/dnssec-backup-$(date +%Y%m%d).tar.gz.gpg
```

### Step 2: Document Current Key Tag

```bash
# Get current key tag
curl http://localhost:9090/api/v1/dnssec/status | jq '.keys[0].key_tag'

# Save for reference
OLD_KEY_TAG=$(curl -s http://localhost:9090/api/v1/dnssec/status | jq -r '.keys[0].key_tag')
echo "Old key tag: $OLD_KEY_TAG"
```

### Step 3: Generate New Keys (Primary Overwatch)

```bash
# Stop Overwatch
sudo systemctl stop opengslb-overwatch

# Remove old keys (new keys generated on start)
sudo rm -rf /var/lib/opengslb/dnssec/*

# Start Overwatch (generates new keys)
sudo systemctl start opengslb-overwatch

# Wait for startup
sleep 10
```

### Step 4: Verify New Keys Generated

```bash
# Get new key tag
curl http://localhost:9090/api/v1/dnssec/status | jq '.keys[0].key_tag'

# Get new DS record
curl http://localhost:9090/api/v1/dnssec/ds | jq '.ds_records[0]'
```

### Step 5: Sync Keys to Other Overwatches

Force immediate sync:

```bash
# On each secondary Overwatch
curl -X POST http://overwatch-2:9090/api/v1/dnssec/sync
curl -X POST http://overwatch-3:9090/api/v1/dnssec/sync
```

Verify sync completed:

```bash
# Check all Overwatches have same key
for ow in overwatch-{1,2,3}; do
    echo "=== $ow ==="
    curl -s http://${ow}:9090/api/v1/dnssec/status | jq '.keys[0].key_tag'
done
```

### Step 6: Update DS Record in Parent Zone

This is the critical step - update depends on your DNS infrastructure:

#### Option A: Contact Domain Registrar

1. Log in to registrar
2. Navigate to DNSSEC settings
3. Add new DS record (keeping old one temporarily)
4. Wait for propagation

#### Option B: Update Parent Zone Directly

If you control the parent zone:

```bash
# Get DS record
DS_RECORD=$(curl -s http://localhost:9090/api/v1/dnssec/ds | jq -r '.ds_records[0].ds_record')

# Add to parent zone file
# gslb.example.com. IN DS 12345 13 2 abc123...
```

#### DS Record Double-Signing Period

Keep both old and new DS records in parent zone for propagation:

```
; Parent zone during transition
gslb    IN  DS  OLD_KEY_TAG 13 2 old_digest...   ; Old key
gslb    IN  DS  NEW_KEY_TAG 13 2 new_digest...   ; New key
```

### Step 7: Verify DNSSEC Chain

```bash
# Wait for propagation (can take hours)
# Then verify
dig +dnssec myapp.gslb.example.com @8.8.8.8

# Use DNSSEC debugger
# https://dnsviz.net/d/myapp.gslb.example.com/dnssec/
```

### Step 8: Remove Old DS Record

After successful propagation (24-48 hours):

1. Remove old DS record from parent zone
2. Verify resolution still works:
   ```bash
   delv myapp.gslb.example.com
   ```

### Step 9: Cleanup

```bash
# Remove old key backups after successful rotation (keep one recent backup)
find /backup -name "dnssec-backup-*.tar.gz.gpg" -mtime +30 -delete
```

## Emergency Key Rotation (Compromised Key)

If keys are compromised, immediate rotation is required:

### Step 1: Generate New Keys Immediately

```bash
# On primary Overwatch
sudo systemctl stop opengslb-overwatch
sudo rm -rf /var/lib/opengslb/dnssec/*
sudo systemctl start opengslb-overwatch
```

### Step 2: Force Sync to All Overwatches

```bash
for ow in overwatch-{1,2,3}; do
    ssh $ow "sudo systemctl stop opengslb-overwatch && sudo rm -rf /var/lib/opengslb/dnssec/*"
done

# Start primary first
sudo systemctl start opengslb-overwatch

# Then secondaries (they'll sync from primary)
for ow in overwatch-{2,3}; do
    ssh $ow "sudo systemctl start opengslb-overwatch"
done

# Force sync
for ow in overwatch-{2,3}; do
    curl -X POST http://${ow}:9090/api/v1/dnssec/sync
done
```

### Step 3: Update DS Record Urgently

Contact registrar/parent zone admin immediately.

**Note**: During DS update propagation, DNSSEC validation may fail for some resolvers. This is unavoidable in emergency rotation.

### Step 4: Document Incident

- Time of compromise detection
- Actions taken
- Impact assessment

## Scheduled Key Rotation

### Recommended Schedule

| Scenario | Rotation Frequency |
|----------|-------------------|
| Normal operations | Annually |
| High-security environment | Quarterly |
| After security incident | Immediately |

### Automation

Create a rotation reminder:

```bash
# Add to cron or calendar
# Annual reminder on January 1
0 9 1 1 * /usr/local/bin/notify-team.sh "DNSSEC key rotation due"
```

## Troubleshooting

### DS Record Update Not Propagating

```bash
# Check DS record at authoritative servers
dig DS gslb.example.com @ns1.parent-zone.com

# Check propagation
dig +trace DS gslb.example.com
```

### DNSSEC Validation Failures After Rotation

Common causes:

1. **DS record not updated**: Update parent zone
2. **Propagation delay**: Wait 24-48 hours
3. **Wrong DS record**: Verify key tag and digest match
4. **Key sync failed**: Force sync between Overwatches

```bash
# Verify key tags match across Overwatches
for ow in overwatch-{1,2,3}; do
    curl -s http://${ow}:9090/api/v1/dnssec/status | jq '.keys[0].key_tag'
done
```

### Rollback to Previous Keys

If rotation fails:

```bash
# Stop all Overwatches
for ow in overwatch-{1,2,3}; do
    ssh $ow "sudo systemctl stop opengslb-overwatch"
done

# Restore backup on primary
sudo rm -rf /var/lib/opengslb/dnssec
sudo tar -xzf /backup/dnssec-backup-YYYYMMDD.tar.gz -C /

# Start primary
sudo systemctl start opengslb-overwatch

# Start secondaries (they'll sync)
for ow in overwatch-{2,3}; do
    ssh $ow "sudo systemctl start opengslb-overwatch"
done
```

## Monitoring

### Key Age Metric

```promql
# Alert if key older than 400 days
opengslb_dnssec_key_age_seconds > 34560000
```

### Alert Example

```yaml
- alert: DNSSECKeyOld
  expr: opengslb_dnssec_key_age_seconds > 31536000  # 1 year
  for: 1d
  labels:
    severity: warning
  annotations:
    summary: "DNSSEC key is older than 1 year - consider rotation"
```

## Related Documentation

- [Security Hardening](./hardening.md)
- [Backup and Restore](../maintenance/backup-restore.md)
- [DNSSEC Issues](../incident-response/scenarios/dnssec-issues.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
