#!/bin/sh
# download-geoip.sh
# Downloads GeoIP database for Demo 4
#
# Priority:
# 1. If MAXMIND_LICENSE_KEY is set, downloads official MaxMind GeoLite2-Country
# 2. Otherwise, downloads free DB-IP database (compatible format)
# 3. If all downloads fail, creates a marker file and demo uses fallback routing

set -e

GEOIP_DIR="/data"
GEOIP_FILE="$GEOIP_DIR/GeoLite2-Country.mmdb"

echo "=========================================="
echo "  OpenGSLB Demo 4: GeoIP Database Setup"
echo "=========================================="

# Check if already downloaded
if [ -f "$GEOIP_FILE" ]; then
    echo "GeoIP database already exists at $GEOIP_FILE"
    echo "Size: $(ls -lh "$GEOIP_FILE" | awk '{print $5}')"
    exit 0
fi

mkdir -p "$GEOIP_DIR"

# Option 1: MaxMind GeoLite2 (requires license key)
if [ -n "$MAXMIND_LICENSE_KEY" ]; then
    echo "Downloading MaxMind GeoLite2-Country database..."
    DOWNLOAD_URL="https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz"

    if curl -sL "$DOWNLOAD_URL" -o /tmp/geoip.tar.gz && [ -s /tmp/geoip.tar.gz ]; then
        tar -xzf /tmp/geoip.tar.gz -C /tmp
        mv /tmp/GeoLite2-Country_*/GeoLite2-Country.mmdb "$GEOIP_FILE"
        rm -rf /tmp/GeoLite2-Country_* /tmp/geoip.tar.gz
        echo "Successfully downloaded MaxMind GeoLite2-Country database"
        echo "Size: $(ls -lh "$GEOIP_FILE" | awk '{print $5}')"
        exit 0
    else
        echo "WARNING: Failed to download from MaxMind (check license key)"
    fi
fi

# Option 2: DB-IP free database (no registration required)
echo "Downloading DB-IP Country Lite database (free alternative)..."
CURRENT_MONTH=$(date +%Y-%m)
DBIP_URL="https://download.db-ip.com/free/dbip-country-lite-${CURRENT_MONTH}.mmdb.gz"

if curl -sL "$DBIP_URL" -o /tmp/dbip.mmdb.gz && [ -s /tmp/dbip.mmdb.gz ]; then
    gunzip -c /tmp/dbip.mmdb.gz > "$GEOIP_FILE"
    rm -f /tmp/dbip.mmdb.gz
    echo "Successfully downloaded DB-IP Country Lite database"
    echo "Size: $(ls -lh "$GEOIP_FILE" | awk '{print $5}')"
    exit 0
else
    echo "WARNING: Failed to download DB-IP database for $CURRENT_MONTH"

    # Try previous month as fallback
    PREV_MONTH=$(date -d "last month" +%Y-%m 2>/dev/null || date -v-1m +%Y-%m 2>/dev/null || echo "")
    if [ -n "$PREV_MONTH" ]; then
        DBIP_URL="https://download.db-ip.com/free/dbip-country-lite-${PREV_MONTH}.mmdb.gz"
        echo "Trying previous month ($PREV_MONTH)..."

        if curl -sL "$DBIP_URL" -o /tmp/dbip.mmdb.gz && [ -s /tmp/dbip.mmdb.gz ]; then
            gunzip -c /tmp/dbip.mmdb.gz > "$GEOIP_FILE"
            rm -f /tmp/dbip.mmdb.gz
            echo "Successfully downloaded DB-IP Country Lite database (previous month)"
            echo "Size: $(ls -lh "$GEOIP_FILE" | awk '{print $5}')"
            exit 0
        fi
    fi
fi

# Option 3: Fallback - create marker file
echo ""
echo "=========================================="
echo "  WARNING: Could not download GeoIP database"
echo "=========================================="
echo ""
echo "The demo will still work using:"
echo "  - Custom CIDR mappings (always work)"
echo "  - Default region fallback"
echo ""
echo "To enable full GeoIP functionality:"
echo "  1. Register at https://dev.maxmind.com/geoip/geolite2-free-geolocation-data"
echo "  2. Set MAXMIND_LICENSE_KEY environment variable"
echo "  3. Re-run: docker-compose up geoip-init"
echo ""

# Create an empty marker file so overwatch knows to use fallback
touch "$GEOIP_FILE.missing"

# Create a minimal placeholder (Overwatch will handle missing DB gracefully)
echo "Creating placeholder file..."
touch "$GEOIP_FILE"

exit 0
