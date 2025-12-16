#!/bin/bash
#
# query-dns.sh - DNS query helper for OpenGSLB Demo 5
#
# Shows DNS responses over time to visualize traffic shifting

OVERWATCH_IP="10.50.0.10"
DOMAIN="app.demo.local"

GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "Querying DNS for ${DOMAIN} every 2 seconds..."
echo "Press Ctrl+C to stop"
echo ""
echo "Backend IPs:"
echo "  10.50.0.21 = backend-1 (stable)"
echo "  10.50.0.22 = backend-2 (stable)"
echo "  10.50.0.23 = backend-3 (chaos target)"
echo ""
echo "-------------------------------------------"

while true; do
    TIMESTAMP=$(date '+%H:%M:%S')
    RESULT=$(dig @${OVERWATCH_IP} ${DOMAIN} +short 2>/dev/null | sort | tr '\n' ' ')

    # Color code based on what's returned
    if echo "$RESULT" | grep -q "10.50.0.23"; then
        # backend-3 is in rotation
        echo -e "${GREEN}[${TIMESTAMP}]${NC} ${RESULT}"
    else
        # backend-3 is NOT in rotation (likely draining)
        echo -e "${YELLOW}[${TIMESTAMP}]${NC} ${RESULT} ${RED}(backend-3 excluded!)${NC}"
    fi

    sleep 2
done
