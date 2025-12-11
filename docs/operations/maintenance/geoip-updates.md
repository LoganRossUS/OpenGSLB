# GeoIP Database Maintenance Guide

This document describes procedures for maintaining and updating the MaxMind GeoIP database used for geolocation-based routing.

## Overview

OpenGSLB uses the MaxMind GeoLite2-Country database for geolocation routing. This database requires:

- Initial setup and registration
- Regular updates (recommended: weekly)
- License compliance

## MaxMind Registration

### Step 1: Create Account

1. Visit [MaxMind GeoLite2 Signup](https://www.maxmind.com/en/geolite2/signup)
2. Create a free account
3. Accept the license agreement

### Step 2: Generate License Key

1. Log in to MaxMind account
2. Navigate to "Manage License Keys"
3. Click "Generate New License Key"
4. Save the license key securely

### Step 3: Download Database

```bash
# Set your license key
export MAXMIND_LICENSE_KEY="your-license-key-here"

# Download GeoLite2-Country database
curl -o /tmp/GeoLite2-Country.tar.gz \
    "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz"

# Extract
tar -xzf /tmp/GeoLite2-Country.tar.gz -C /tmp/
cp /tmp/GeoLite2-Country_*/GeoLite2-Country.mmdb /var/lib/opengslb/geoip/

# Set permissions
chown opengslb:opengslb /var/lib/opengslb/geoip/GeoLite2-Country.mmdb
chmod 644 /var/lib/opengslb/geoip/GeoLite2-Country.mmdb

# Cleanup
rm -rf /tmp/GeoLite2-Country*
```

## Automatic Updates

### Using geoipupdate Tool (Recommended)

MaxMind provides `geoipupdate` tool for automatic updates.

#### Installation

```bash
# Ubuntu/Debian
sudo add-apt-repository ppa:maxmind/ppa
sudo apt update
sudo apt install geoipupdate

# RHEL/CentOS
sudo yum install https://github.com/maxmind/geoipupdate/releases/download/v4.10.0/geoipupdate_4.10.0_linux_amd64.rpm

# From source
curl -Lo /tmp/geoipupdate.tar.gz https://github.com/maxmind/geoipupdate/releases/download/v4.10.0/geoipupdate_4.10.0_linux_amd64.tar.gz
tar -xzf /tmp/geoipupdate.tar.gz -C /tmp/
sudo mv /tmp/geoipupdate_*/geoipupdate /usr/local/bin/
```

#### Configuration

```bash
# Create configuration file
sudo tee /etc/GeoIP.conf << EOF
# MaxMind GeoIP Update Configuration
AccountID YOUR_ACCOUNT_ID
LicenseKey YOUR_LICENSE_KEY
EditionIDs GeoLite2-Country
DatabaseDirectory /var/lib/opengslb/geoip
EOF

sudo chmod 600 /etc/GeoIP.conf
```

#### Manual Update

```bash
# Run update
sudo geoipupdate -v

# Verify update
ls -la /var/lib/opengslb/geoip/
```

#### Automated Updates with systemd

```bash
# Create systemd service
sudo tee /etc/systemd/system/geoipupdate.service << 'EOF'
[Unit]
Description=GeoIP Database Update
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/geoipupdate -v
ExecStartPost=/bin/systemctl reload opengslb-overwatch
User=root
EOF
```

```bash
# Create systemd timer (weekly on Sunday at 3 AM)
sudo tee /etc/systemd/system/geoipupdate.timer << 'EOF'
[Unit]
Description=Weekly GeoIP Database Update

[Timer]
OnCalendar=Sun *-*-* 03:00:00
Persistent=true
RandomizedDelaySec=3600

[Install]
WantedBy=timers.target
EOF

# Enable timer
sudo systemctl daemon-reload
sudo systemctl enable geoipupdate.timer
sudo systemctl start geoipupdate.timer

# Verify timer
systemctl list-timers | grep geoip
```

### Custom Update Script

If not using `geoipupdate`:

```bash
#!/bin/bash
# update-geoip.sh

set -e

GEOIP_DIR="/var/lib/opengslb/geoip"
LICENSE_KEY="${MAXMIND_LICENSE_KEY}"
TMP_DIR="/tmp/geoip-update"

if [ -z "$LICENSE_KEY" ]; then
    echo "Error: MAXMIND_LICENSE_KEY not set"
    exit 1
fi

mkdir -p "$TMP_DIR"

# Download database
echo "Downloading GeoLite2-Country database..."
curl -sL -o "${TMP_DIR}/GeoLite2-Country.tar.gz" \
    "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country&license_key=${LICENSE_KEY}&suffix=tar.gz"

# Extract
tar -xzf "${TMP_DIR}/GeoLite2-Country.tar.gz" -C "$TMP_DIR"

# Find the mmdb file
MMDB_FILE=$(find "$TMP_DIR" -name "*.mmdb" | head -1)

if [ -z "$MMDB_FILE" ]; then
    echo "Error: No .mmdb file found in download"
    rm -rf "$TMP_DIR"
    exit 1
fi

# Backup current database
if [ -f "${GEOIP_DIR}/GeoLite2-Country.mmdb" ]; then
    cp "${GEOIP_DIR}/GeoLite2-Country.mmdb" "${GEOIP_DIR}/GeoLite2-Country.mmdb.backup"
fi

# Install new database
cp "$MMDB_FILE" "${GEOIP_DIR}/GeoLite2-Country.mmdb"
chown opengslb:opengslb "${GEOIP_DIR}/GeoLite2-Country.mmdb"
chmod 644 "${GEOIP_DIR}/GeoLite2-Country.mmdb"

# Cleanup
rm -rf "$TMP_DIR"

# Trigger hot-reload
echo "Triggering hot-reload..."
pkill -HUP -f "opengslb.*overwatch" || true

# Verify
echo "GeoIP database updated successfully"
ls -la "${GEOIP_DIR}/GeoLite2-Country.mmdb"
```

## Hot-Reload

OpenGSLB supports hot-reload of the GeoIP database without restart.

### Triggering Reload

```bash
# Option 1: Send SIGHUP to process
sudo systemctl reload opengslb-overwatch

# Option 2: Via kill command
sudo pkill -HUP -f "opengslb.*overwatch"
```

### Verifying Reload

```bash
# Check logs for reload message
journalctl -u opengslb-overwatch | grep -i "geoip\|reload"

# Test geolocation
opengslb-cli geo test 8.8.8.8 --api http://localhost:9090
```

## Docker Updates

### Volume-Based Updates

```bash
# Update database on host
./update-geoip.sh

# Database is automatically available in container via volume mount
# Trigger reload
docker exec opengslb-overwatch kill -HUP 1
```

### Sidecar Container Approach

```yaml
# docker-compose.yml with geoipupdate sidecar
version: '3.8'

services:
  overwatch:
    image: ghcr.io/loganrossus/opengslb:latest
    volumes:
      - geoip-data:/var/lib/opengslb/geoip

  geoipupdate:
    image: maxmindinc/geoipupdate
    environment:
      - GEOIPUPDATE_ACCOUNT_ID=${MAXMIND_ACCOUNT_ID}
      - GEOIPUPDATE_LICENSE_KEY=${MAXMIND_LICENSE_KEY}
      - GEOIPUPDATE_EDITION_IDS=GeoLite2-Country
      - GEOIPUPDATE_FREQUENCY=168  # Weekly
    volumes:
      - geoip-data:/usr/share/GeoIP

volumes:
  geoip-data:
```

## Database Information

### Check Database Version

```bash
# Using mmdblookup (part of libmaxminddb)
mmdblookup --file /var/lib/opengslb/geoip/GeoLite2-Country.mmdb --info

# Output includes:
# - Database type
# - Build time
# - Description
```

### Check File Modification Time

```bash
ls -la /var/lib/opengslb/geoip/GeoLite2-Country.mmdb
stat /var/lib/opengslb/geoip/GeoLite2-Country.mmdb
```

### Metrics

Monitor database age via metrics:

```promql
# If custom metric exposed
opengslb_geoip_database_age_seconds
```

## Troubleshooting

### Download Failures

```bash
# Check license key is valid
curl -v "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz"

# Common issues:
# - 401: Invalid license key
# - 403: Account issue or exceeded download limit
# - Network connectivity issues
```

### Database Not Loading

```bash
# Check file exists and permissions
ls -la /var/lib/opengslb/geoip/

# Check file is valid mmdb
file /var/lib/opengslb/geoip/GeoLite2-Country.mmdb
# Should show: MaxMind DB database

# Check logs
journalctl -u opengslb-overwatch | grep -i geoip
```

### Incorrect Geolocation Results

```bash
# Test specific IP
opengslb-cli geo test 8.8.8.8 --api http://localhost:9090

# Compare with MaxMind demo
# https://www.maxmind.com/en/geoip2-precision-demo

# GeoLite2 is less accurate than commercial GeoIP2
# Consider upgrading for better accuracy
```

## License Compliance

### GeoLite2 Attribution Requirements

When using GeoLite2 data, you must:

1. **Include attribution** in your application:
   ```
   This product includes GeoLite2 data created by MaxMind,
   available from https://www.maxmind.com.
   ```

2. **Link to MaxMind** if displaying geolocation in UI

3. **Update regularly** (MaxMind updates weekly on Tuesdays)

4. **Don't redistribute** the database

### Commercial Alternative

For production use with higher accuracy needs, consider:

- **GeoIP2 Country**: More accurate, commercial license
- **GeoIP2 City**: Includes city-level data
- **GeoIP2 Precision**: Web service with real-time updates

## Database Comparison

| Database | Accuracy | Update Frequency | Price |
|----------|----------|------------------|-------|
| GeoLite2-Country | ~95% | Weekly | Free |
| GeoIP2-Country | ~99% | Weekly | Paid |
| GeoIP2-City | ~80% city | Weekly | Paid |

## Monitoring Updates

### Alert on Stale Database

```yaml
# Prometheus alert
groups:
  - name: opengslb-geoip
    rules:
      - alert: GeoIPDatabaseStale
        # Alert if database older than 14 days
        expr: (time() - opengslb_geoip_database_timestamp) > 1209600
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "GeoIP database is older than 14 days"
```

### Manual Check Script

```bash
#!/bin/bash
# check-geoip-age.sh

DB_FILE="/var/lib/opengslb/geoip/GeoLite2-Country.mmdb"
MAX_AGE_DAYS=14

if [ ! -f "$DB_FILE" ]; then
    echo "CRITICAL: GeoIP database not found"
    exit 2
fi

AGE_SECONDS=$(($(date +%s) - $(stat -c %Y "$DB_FILE")))
AGE_DAYS=$((AGE_SECONDS / 86400))

if [ $AGE_DAYS -gt $MAX_AGE_DAYS ]; then
    echo "WARNING: GeoIP database is ${AGE_DAYS} days old"
    exit 1
else
    echo "OK: GeoIP database is ${AGE_DAYS} days old"
    exit 0
fi
```

## Related Documentation

- [Geolocation Routing Configuration](../../configuration.md)
- [Custom CIDR Mappings](../../cli.md#geo-commands)
- [Upgrade Procedures](./upgrades.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
