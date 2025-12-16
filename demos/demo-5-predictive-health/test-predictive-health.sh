#!/bin/bash
# Demo 5 Predictive Health Test Script
#
# This script:
# 1. Rebuilds the OpenGSLB binary
# 2. Restarts all containers fresh
# 3. Waits for startup
# 4. Triggers CPU chaos on backend-3
# 5. Monitors logs and DNS responses to verify predictive health works

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${CYAN}=== $1 ===${NC}\n"; }

# Cleanup function
cleanup() {
    if [ -n "$TAIL_PID" ]; then
        kill $TAIL_PID 2>/dev/null || true
    fi
}
trap cleanup EXIT

cd "$SCRIPT_DIR"

log_header "DEMO 5: PREDICTIVE HEALTH TEST"

# Step 1: Rebuild binary
log_header "Step 1: Rebuilding OpenGSLB binary"
cd "$PROJECT_ROOT"
if go build -o "$SCRIPT_DIR/bin/opengslb" ./cmd/opengslb; then
    log_success "Binary rebuilt successfully"
else
    log_error "Failed to build binary"
    exit 1
fi
cd "$SCRIPT_DIR"

# Step 2: Stop any existing containers
log_header "Step 2: Stopping existing containers"
docker-compose down --remove-orphans 2>/dev/null || true
log_success "Containers stopped"

# Step 3: Start fresh containers
log_header "Step 3: Starting fresh containers"
docker-compose up -d --build
log_success "Containers started"

# Step 4: Wait for startup
log_header "Step 4: Waiting for services to initialize (15 seconds)"
for i in {15..1}; do
    echo -ne "\r  Waiting... ${i}s remaining  "
    sleep 1
done
echo -e "\r  Waiting... done!              "

# Step 5: Verify services are healthy
log_header "Step 5: Verifying services"

# Check DNS
if dig @localhost -p 5354 app.demo.local A +short +time=2 > /dev/null 2>&1; then
    log_success "DNS server responding"
else
    log_error "DNS server not responding"
    docker logs overwatch 2>&1 | tail -20
    exit 1
fi

# Check backend health endpoints
for i in 1 2 3; do
    port=$((8080 + i - 1))
    if [ $i -eq 3 ]; then port=8083; fi
    if curl -s "http://localhost:$port/health" > /dev/null 2>&1 || curl -s "http://10.50.0.2$i:8080/health" > /dev/null 2>&1; then
        log_success "Backend-$i health endpoint responding"
    else
        log_warn "Backend-$i health endpoint not accessible from host (may be internal only)"
    fi
done

# Step 6: Show initial DNS state
log_header "Step 6: Initial DNS state (before chaos)"
echo "DNS responses for app.demo.local:"
for i in 1 2 3; do
    result=$(dig @localhost -p 5354 app.demo.local A +short +time=2 2>/dev/null || echo "ERROR")
    echo "  Query $i: $result"
done

# Step 7: Show current registry state
log_header "Step 7: Current backend registry state"
echo "Registered backends:"
docker logs overwatch 2>&1 | grep -E "backend registered|backend draining" | tail -10 || echo "  (no registration logs yet)"

# Step 8: Trigger chaos
log_header "Step 8: Triggering CPU chaos on backend-3 (10.50.0.23)"
if curl -s http://localhost:8083/chaos/cpu > /dev/null 2>&1; then
    log_success "CPU chaos triggered on backend-3"
else
    log_error "Failed to trigger chaos - is backend-3 running?"
    exit 1
fi

# Step 9: Monitor for predictive health detection
log_header "Step 9: Monitoring for predictive health signals (30 seconds)"
echo ""
echo "Watching for:"
echo "  - 'backend draining started' - agent detected high CPU"
echo "  - 'DNS excluding draining backend' - DNS excluding the backend"
echo "  - 'bleed' - agent sending bleed signal"
echo ""
echo "Live logs (filtered):"
echo "---"

# Start tailing logs in background
docker logs -f overwatch 2>&1 | grep --line-buffered -E "(draining|excluding|bleed|Predictive|cpu_percent)" &
TAIL_PID=$!

# Monitor for 30 seconds while also checking DNS
for i in {1..6}; do
    sleep 5
    echo ""
    echo -e "${YELLOW}[DNS CHECK @ ${i}x5s]${NC} $(dig @localhost -p 5354 app.demo.local A +short +time=2 2>/dev/null | tr '\n' ' ')"
done

# Stop log tailing
kill $TAIL_PID 2>/dev/null || true
TAIL_PID=""

echo "---"
echo ""

# Step 10: Final analysis
log_header "Step 10: Final Analysis"

echo "=== All draining/bleed related logs ==="
docker logs overwatch 2>&1 | grep -E "(draining|excluding|bleed)" || echo "(none found)"

echo ""
echo "=== Current DNS responses (5 queries) ==="
declare -A ip_counts
for i in {1..5}; do
    ip=$(dig @localhost -p 5354 app.demo.local A +short +time=2 2>/dev/null)
    echo "  Query $i: $ip"
    if [ -n "$ip" ]; then
        ip_counts[$ip]=$((${ip_counts[$ip]:-0} + 1))
    fi
done

echo ""
echo "=== IP Distribution ==="
for ip in "${!ip_counts[@]}"; do
    echo "  $ip: ${ip_counts[$ip]} times"
done

echo ""
echo "=== Backend CPU/Memory from agent logs ==="
for backend in backend-1 backend-2 backend-3; do
    echo "$backend:"
    docker logs $backend 2>&1 | grep -E "(cpu|CPU|memory|Memory|bleed|Bleeding)" | tail -3 || echo "  (no metrics logs)"
done

# Determine if test passed
echo ""
log_header "Test Result"

draining_detected=$(docker logs overwatch 2>&1 | grep -c "draining started" || echo "0")
excluding_detected=$(docker logs overwatch 2>&1 | grep -c "excluding draining" || echo "0")

if [ "$draining_detected" -gt 0 ]; then
    log_success "Draining signal received from agent ($draining_detected occurrences)"
else
    log_error "No draining signals received - agent may not be detecting CPU stress"
fi

if [ "$excluding_detected" -gt 0 ]; then
    log_success "DNS is excluding draining backends ($excluding_detected occurrences)"
else
    log_warn "DNS not excluding backends - check combinedHealthProvider logic"
fi

# Check if draining backend is still being returned
draining_ip=$(docker logs overwatch 2>&1 | grep "draining started" | tail -1 | grep -oE "address=10\.[0-9]+\.[0-9]+\.[0-9]+" | cut -d= -f2)
if [ -n "$draining_ip" ]; then
    echo ""
    echo "Draining backend: $draining_ip"
    still_returned=$(dig @localhost -p 5354 app.demo.local A +short +time=2 2>/dev/null | grep -c "$draining_ip" || echo "0")
    if [ "$still_returned" -gt 0 ]; then
        log_error "FAIL: Draining backend $draining_ip is STILL being returned by DNS!"
    else
        log_success "PASS: Draining backend $draining_ip is correctly excluded from DNS"
    fi
fi

echo ""
echo "For more details, run:"
echo "  docker logs overwatch 2>&1 | less"
echo "  docker logs backend-3 2>&1 | less"
