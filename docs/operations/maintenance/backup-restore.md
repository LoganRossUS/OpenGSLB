# Backup and Restore Procedures

This document describes backup and restore procedures for OpenGSLB data, configuration, and certificates.

## What to Backup

### Critical Data

| Component | Location | Priority | Backup Frequency |
|-----------|----------|----------|------------------|
| Configuration | `/etc/opengslb/` | Critical | On change |
| DNSSEC keys | `/var/lib/opengslb/dnssec/` | Critical | Weekly + on rotation |
| Agent certificates | `/var/lib/opengslb/` | Important | Weekly |
| KV store (bbolt) | `/var/lib/opengslb/opengslb.db` | Important | Daily |
| GeoIP database | `/var/lib/opengslb/geoip/` | Low | On update |

### Data Recovery Priority

1. **Configuration**: Required to start - restore first
2. **DNSSEC keys**: Required for DNSSEC validation chain
3. **KV store**: Contains agent pins, custom geo mappings, overrides
4. **Agent certificates**: Agents can re-register if lost

## Backup Procedures

### Manual Backup Script

```bash
#!/bin/bash
# backup-opengslb.sh

BACKUP_DIR="/backup/opengslb"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_PATH="${BACKUP_DIR}/${TIMESTAMP}"

# Create backup directory
mkdir -p "${BACKUP_PATH}"

# Stop service for consistent backup (optional)
# sudo systemctl stop opengslb-overwatch

# Backup configuration
echo "Backing up configuration..."
cp -r /etc/opengslb "${BACKUP_PATH}/config"

# Backup data directory
echo "Backing up data directory..."
cp -r /var/lib/opengslb "${BACKUP_PATH}/data"

# Start service if stopped
# sudo systemctl start opengslb-overwatch

# Create tarball
echo "Creating archive..."
cd "${BACKUP_DIR}"
tar -czf "opengslb-backup-${TIMESTAMP}.tar.gz" "${TIMESTAMP}"
rm -rf "${TIMESTAMP}"

# Cleanup old backups (keep last 7 days)
find "${BACKUP_DIR}" -name "opengslb-backup-*.tar.gz" -mtime +7 -delete

echo "Backup complete: ${BACKUP_DIR}/opengslb-backup-${TIMESTAMP}.tar.gz"

# Verify backup
tar -tzf "${BACKUP_DIR}/opengslb-backup-${TIMESTAMP}.tar.gz" | head -20
```

### Automated Backup with systemd Timer

```bash
# Create backup script
sudo tee /usr/local/bin/opengslb-backup.sh << 'EOF'
#!/bin/bash
BACKUP_DIR="/backup/opengslb"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "${BACKUP_DIR}"
tar -czf "${BACKUP_DIR}/opengslb-backup-${TIMESTAMP}.tar.gz" \
    /etc/opengslb \
    /var/lib/opengslb

# Cleanup old backups
find "${BACKUP_DIR}" -name "opengslb-backup-*.tar.gz" -mtime +7 -delete

# Log success
logger "OpenGSLB backup completed: opengslb-backup-${TIMESTAMP}.tar.gz"
EOF

chmod +x /usr/local/bin/opengslb-backup.sh
```

```bash
# Create systemd service
sudo tee /etc/systemd/system/opengslb-backup.service << 'EOF'
[Unit]
Description=OpenGSLB Backup
After=opengslb-overwatch.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/opengslb-backup.sh
User=root
EOF
```

```bash
# Create systemd timer (daily at 2 AM)
sudo tee /etc/systemd/system/opengslb-backup.timer << 'EOF'
[Unit]
Description=Daily OpenGSLB Backup

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
EOF

# Enable timer
sudo systemctl daemon-reload
sudo systemctl enable opengslb-backup.timer
sudo systemctl start opengslb-backup.timer

# Verify timer is scheduled
systemctl list-timers | grep opengslb
```

### Configuration-Only Backup

For frequent configuration backups:

```bash
#!/bin/bash
# config-backup.sh

CONFIG_BACKUP="/backup/opengslb/configs"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "${CONFIG_BACKUP}"
cp /etc/opengslb/overwatch.yaml "${CONFIG_BACKUP}/overwatch-${TIMESTAMP}.yaml"

# Keep last 30 config versions
ls -t "${CONFIG_BACKUP}"/overwatch-*.yaml | tail -n +31 | xargs -r rm
```

### DNSSEC Key Backup

DNSSEC keys are critical for validation chain:

```bash
#!/bin/bash
# dnssec-backup.sh

DNSSEC_BACKUP="/backup/opengslb/dnssec"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "${DNSSEC_BACKUP}"

# Backup DNSSEC directory
cp -r /var/lib/opengslb/dnssec "${DNSSEC_BACKUP}/dnssec-${TIMESTAMP}"

# Create encrypted archive (recommended for key material)
tar -czf - /var/lib/opengslb/dnssec | \
    gpg --symmetric --cipher-algo AES256 \
    > "${DNSSEC_BACKUP}/dnssec-${TIMESTAMP}.tar.gz.gpg"

echo "DNSSEC backup: ${DNSSEC_BACKUP}/dnssec-${TIMESTAMP}.tar.gz.gpg"
```

### Docker Volume Backup

```bash
#!/bin/bash
# docker-backup.sh

BACKUP_DIR="/backup/opengslb"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Backup data volume
docker run --rm \
    -v opengslb-data:/data:ro \
    -v "${BACKUP_DIR}":/backup \
    alpine tar -czf "/backup/opengslb-data-${TIMESTAMP}.tar.gz" -C /data .

# Backup config
cp ./config/overwatch.yaml "${BACKUP_DIR}/overwatch-${TIMESTAMP}.yaml"

echo "Docker backup complete"
```

## Remote Backup

### S3 Backup

```bash
#!/bin/bash
# s3-backup.sh

BUCKET="s3://your-backup-bucket/opengslb"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
TMP_FILE="/tmp/opengslb-backup-${TIMESTAMP}.tar.gz"

# Create local backup
tar -czf "${TMP_FILE}" /etc/opengslb /var/lib/opengslb

# Upload to S3
aws s3 cp "${TMP_FILE}" "${BUCKET}/opengslb-backup-${TIMESTAMP}.tar.gz"

# Cleanup local temp file
rm "${TMP_FILE}"

# Cleanup old S3 backups (using lifecycle policy is recommended instead)
```

### rsync to Remote Server

```bash
#!/bin/bash
# rsync-backup.sh

REMOTE="backup@backup-server.internal"
REMOTE_PATH="/backup/opengslb"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Sync with versioned directory
rsync -avz --delete \
    /etc/opengslb \
    /var/lib/opengslb \
    "${REMOTE}:${REMOTE_PATH}/current/"

# Create snapshot
ssh "${REMOTE}" "cp -al ${REMOTE_PATH}/current ${REMOTE_PATH}/snapshot-${TIMESTAMP}"
```

## Restore Procedures

### Full Restore

```bash
#!/bin/bash
# restore-opengslb.sh

BACKUP_FILE=$1

if [ -z "$BACKUP_FILE" ]; then
    echo "Usage: $0 <backup-file.tar.gz>"
    exit 1
fi

# Stop service
echo "Stopping OpenGSLB..."
sudo systemctl stop opengslb-overwatch

# Extract backup
echo "Extracting backup..."
TMP_DIR=$(mktemp -d)
tar -xzf "${BACKUP_FILE}" -C "${TMP_DIR}"

# Restore configuration
echo "Restoring configuration..."
sudo cp -r "${TMP_DIR}/*/config/"* /etc/opengslb/
sudo chown -R root:opengslb /etc/opengslb
sudo chmod 750 /etc/opengslb
sudo chmod 640 /etc/opengslb/*.yaml

# Restore data
echo "Restoring data..."
sudo cp -r "${TMP_DIR}/*/data/"* /var/lib/opengslb/
sudo chown -R opengslb:opengslb /var/lib/opengslb
sudo chmod 700 /var/lib/opengslb

# Cleanup
rm -rf "${TMP_DIR}"

# Start service
echo "Starting OpenGSLB..."
sudo systemctl start opengslb-overwatch

# Verify
echo "Verifying..."
sleep 5
sudo systemctl status opengslb-overwatch
curl http://localhost:9090/api/v1/ready

echo "Restore complete!"
```

### Configuration-Only Restore

```bash
# Stop service
sudo systemctl stop opengslb-overwatch

# Restore configuration file
sudo cp /backup/opengslb/configs/overwatch-20250101_120000.yaml /etc/opengslb/overwatch.yaml
sudo chown root:opengslb /etc/opengslb/overwatch.yaml
sudo chmod 640 /etc/opengslb/overwatch.yaml

# Start service
sudo systemctl start opengslb-overwatch
```

### DNSSEC Key Restore

```bash
# Stop service
sudo systemctl stop opengslb-overwatch

# Decrypt and extract (if encrypted)
gpg --decrypt /backup/opengslb/dnssec/dnssec-20250101.tar.gz.gpg | \
    tar -xzf - -C /

# Or restore unencrypted
sudo cp -r /backup/opengslb/dnssec/dnssec-20250101/* /var/lib/opengslb/dnssec/
sudo chown -R opengslb:opengslb /var/lib/opengslb/dnssec

# Start service
sudo systemctl start opengslb-overwatch

# Trigger key sync to peers
curl -X POST http://localhost:9090/api/v1/dnssec/sync
```

### Docker Volume Restore

```bash
# Stop container
docker stop opengslb-overwatch

# Restore data volume
docker run --rm \
    -v opengslb-data:/data \
    -v /backup/opengslb:/backup:ro \
    alpine tar -xzf /backup/opengslb-data-20250101_120000.tar.gz -C /data

# Start container
docker start opengslb-overwatch
```

### S3 Restore

```bash
# Download from S3
aws s3 cp s3://your-backup-bucket/opengslb/opengslb-backup-20250101_120000.tar.gz /tmp/

# Use full restore procedure
./restore-opengslb.sh /tmp/opengslb-backup-20250101_120000.tar.gz
```

## Disaster Recovery

### Complete System Loss

1. **Provision new server** with same OS

2. **Install OpenGSLB**:
   ```bash
   curl -Lo /usr/local/bin/opengslb https://github.com/loganrossus/OpenGSLB/releases/download/v0.6.0/opengslb-linux-amd64
   chmod +x /usr/local/bin/opengslb
   ```

3. **Create system user and directories**:
   ```bash
   useradd --system --no-create-home opengslb
   mkdir -p /etc/opengslb /var/lib/opengslb
   ```

4. **Restore from backup**:
   ```bash
   ./restore-opengslb.sh /path/to/backup.tar.gz
   ```

5. **Install systemd service**:
   ```bash
   # Copy service file from backup or create new
   sudo systemctl daemon-reload
   sudo systemctl enable opengslb-overwatch
   ```

6. **Start and verify**:
   ```bash
   sudo systemctl start opengslb-overwatch
   dig @localhost myapp.gslb.example.com
   ```

### HA Recovery (Lost All Nodes)

1. **Deploy single Overwatch** from backup

2. **Verify service restored**:
   ```bash
   dig @new-overwatch myapp.gslb.example.com
   ```

3. **Deploy additional Overwatches** per HA guide

4. **Re-configure agents** to point to new Overwatches:
   ```yaml
   gossip:
     overwatch_nodes:
       - new-overwatch-1:7946
       - new-overwatch-2:7946
   ```

5. **Update DNS configuration** with new Overwatch IPs

## Backup Verification

### Regular Verification Schedule

| Test | Frequency | Procedure |
|------|-----------|-----------|
| Backup file integrity | Daily | Verify tar can extract |
| Configuration restore | Weekly | Test restore to staging |
| Full restore | Monthly | DR test to staging |
| DNSSEC key restore | Quarterly | Verify key validity |

### Verification Script

```bash
#!/bin/bash
# verify-backup.sh

BACKUP_FILE=$1

if [ -z "$BACKUP_FILE" ]; then
    echo "Usage: $0 <backup-file.tar.gz>"
    exit 1
fi

echo "=== Backup Verification ==="

# Check file exists and is readable
if [ ! -r "$BACKUP_FILE" ]; then
    echo "FAIL: Cannot read backup file"
    exit 1
fi
echo "PASS: Backup file readable"

# Verify tar integrity
if ! tar -tzf "$BACKUP_FILE" > /dev/null 2>&1; then
    echo "FAIL: Tar archive corrupted"
    exit 1
fi
echo "PASS: Tar archive valid"

# Check required files present
REQUIRED_FILES=(
    "config/overwatch.yaml"
    "data/opengslb.db"
)

TMP_DIR=$(mktemp -d)
tar -xzf "$BACKUP_FILE" -C "$TMP_DIR"

for file in "${REQUIRED_FILES[@]}"; do
    if ! find "$TMP_DIR" -name "$(basename $file)" | grep -q .; then
        echo "WARN: Missing file: $file"
    else
        echo "PASS: Found $file"
    fi
done

# Validate configuration
CONFIG_FILE=$(find "$TMP_DIR" -name "overwatch.yaml" | head -1)
if [ -n "$CONFIG_FILE" ]; then
    if opengslb --config="$CONFIG_FILE" --validate 2>/dev/null; then
        echo "PASS: Configuration valid"
    else
        echo "FAIL: Configuration invalid"
    fi
fi

rm -rf "$TMP_DIR"

echo "=== Verification Complete ==="
```

## Retention Policy

### Recommended Retention

| Backup Type | Retention | Storage Location |
|-------------|-----------|------------------|
| Daily | 7 days | Local |
| Weekly | 4 weeks | Remote |
| Monthly | 12 months | Archive |
| DNSSEC keys | Indefinite | Secure vault |

### Cleanup Script

```bash
#!/bin/bash
# cleanup-backups.sh

BACKUP_DIR="/backup/opengslb"

# Keep daily backups for 7 days
find "${BACKUP_DIR}" -name "opengslb-backup-*.tar.gz" -mtime +7 -delete

# Keep weekly backups for 4 weeks
# (Assumes weekly backups in subdirectory)
find "${BACKUP_DIR}/weekly" -name "*.tar.gz" -mtime +28 -delete

# Log cleanup
logger "OpenGSLB backup cleanup completed"
```

## Security Considerations

- Store backups in encrypted form for sensitive data (DNSSEC keys)
- Use separate credentials for backup access
- Test restore procedures regularly
- Monitor backup job success/failure
- Protect backup storage with appropriate access controls

## Related Documentation

- [Upgrade Procedures](./upgrades.md)
- [Rollback Procedures](./rollback.md)
- [DNSSEC Key Rotation](../security/key-rotation.md)
- [Disaster Recovery Scenarios](../incident-response/scenarios/overwatch-down.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
