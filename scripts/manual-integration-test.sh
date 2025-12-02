#!/bin/bash
# =============================================================================
# OpenGSLB Manual Integration Test
# =============================================================================
# This script sets up a local test environment and verifies:
#   - DNS server functionality
#   - Health check integration
#   - Round-robin load balancing
#   - NXDOMAIN for unknown domains
#   - SERVFAIL when no healthy servers
#
# Prerequisites:
#   - OpenGSLB binary built (./opengslb)
#   - Python 3 installed
#   - dig command available (dnsutils)
#   - Run as user with sudo access (for loopback aliases)
#
# Usage:
#   ./scripts/manual-integration-test.sh
# =============================================================================

set -e

# Configuration
DNS_PORT="15353"
DNS_SERVER="127.0.0.1"
HTTP_PORTS=(8081 8082 8083)
LOOPBACK_IPS=("127.0.0.1" "127.0.0.2" "127.0.0.3")
CONFIG_FILE="/tmp/opengslb-integration-test/config.yaml"
PID_DIR="/tmp/opengslb-integration-test/pids"
HEALTH_STABILIZE_TIME=5

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track PIDs for cleanup
declare -a PYTHON_PIDS
OPENGSLB_PID=""

# =============================================================================
# Helper Functions
# =============================================================================

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_test() {
    echo -e "\n${YELLOW}=== $1 ===${NC}"
}

cleanup() {
    log_info "Cleaning up..."
    
    # Kill Python servers
    for pid in "${PYTHON_PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done
    
    # Kill OpenGSLB
    if [ -n "$OPENGSLB_PID" ] && kill -0 "$OPENGSLB_PID" 2>/dev/null; then
        kill "$OPENGSLB_PID" 2>/dev/null || true
        wait "$OPENGSLB_PID" 2>/dev/null || true
    fi
    
    # Remove loopback aliases (ignore errors if they don't exist)
    sudo ip addr del 127.0.0.2/8 dev lo 2>/dev/null || true
    sudo ip addr del 127.0.0.3/8 dev lo 2>/dev/null || true
    
    # Clean up temp files
    rm -rf /tmp/opengslb-integration-test
    
    log_info "Cleanup complete"
}

# Set up trap for cleanup on exit
trap cleanup EXIT INT TERM

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check for OpenGSLB binary
    if [ ! -f "./opengslb" ]; then
        log_error "OpenGSLB binary not found. Run 'go build -o opengslb ./cmd/opengslb' first."
        exit 1
    fi
    
    # Check for Python 3
    if ! command -v python3 &> /dev/null; then
        log_error "Python 3 is required but not installed."
        exit 1
    fi
    
    # Check for dig
    if ! command -v dig &> /dev/null; then
        log_error "dig command not found. Install dnsutils package."
        exit 1
    fi
    
    # Check for sudo access
    if ! sudo -n true 2>/dev/null; then
        log_warn "This script requires sudo for loopback aliases. You may be prompted for password."
    fi
    
    log_info "Prerequisites check passed"
}

setup_loopback_aliases() {
    log_info "Setting up loopback aliases..."
    
    # Add aliases if they don't exist
    if ! ip addr show lo | grep -q "127.0.0.2"; then
        sudo ip addr add 127.0.0.2/8 dev lo
    fi
    if ! ip addr show lo | grep -q "127.0.0.3"; then
        sudo ip addr add 127.0.0.3/8 dev lo
    fi
    
    log_info "Loopback aliases configured"
}

create_config() {
    log_info "Creating test configuration..."
    
    mkdir -p "$(dirname "$CONFIG_FILE")"
    mkdir -p "$PID_DIR"
    
    cat > "$CONFIG_FILE" << 'EOF'
dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 30

regions:
  # Region with all healthy servers
  - name: healthy-region
    servers:
      - address: "127.0.0.1"
        port: 8081
        weight: 100
      - address: "127.0.0.2"
        port: 8082
        weight: 100
      - address: "127.0.0.3"
        port: 8083
        weight: 100
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

  # Region with no servers running (will fail health checks)
  - name: unhealthy-region
    servers:
      - address: "127.0.0.1"
        port: 9991
        weight: 100
      - address: "127.0.0.1"
        port: 9992
        weight: 100
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

domains:
  - name: healthy.test
    routing_algorithm: round-robin
    regions:
      - healthy-region
    ttl: 10

  - name: unhealthy.test
    routing_algorithm: round-robin
    regions:
      - unhealthy-region
    ttl: 10

logging:
  level: info
  format: text
EOF
    
    chmod 600 "$CONFIG_FILE"
    log_info "Configuration created at $CONFIG_FILE"
}

start_mock_servers() {
    log_info "Starting mock HTTP servers..."
    
    for i in "${!HTTP_PORTS[@]}"; do
        port="${HTTP_PORTS[$i]}"
        ip="${LOOPBACK_IPS[$i]}"
        
        # Create a simple directory to serve
        server_dir="/tmp/opengslb-integration-test/server-$port"
        mkdir -p "$server_dir"
        echo "OK" > "$server_dir/index.html"
        
        # Start Python HTTP server in background
        cd "$server_dir"
        python3 -m http.server "$port" --bind "$ip" > /dev/null 2>&1 &
        PYTHON_PIDS+=($!)
        cd - > /dev/null
        
        log_info "Started HTTP server on $ip:$port (PID: ${PYTHON_PIDS[-1]})"
    done
    
    # Give servers time to start
    sleep 1
}

start_opengslb() {
    log_info "Starting OpenGSLB..."
    
    ./opengslb --config "$CONFIG_FILE" > /tmp/opengslb-integration-test/opengslb.log 2>&1 &
    OPENGSLB_PID=$!
    
    # Wait for startup
    sleep 2
    
    # Verify it's running
    if ! kill -0 "$OPENGSLB_PID" 2>/dev/null; then
        log_error "OpenGSLB failed to start. Check /tmp/opengslb-integration-test/opengslb.log"
        cat /tmp/opengslb-integration-test/opengslb.log
        exit 1
    fi
    
    log_info "OpenGSLB started (PID: $OPENGSLB_PID)"
    log_info "Waiting ${HEALTH_STABILIZE_TIME}s for health checks to stabilize..."
    sleep "$HEALTH_STABILIZE_TIME"
}

query_dns() {
    local domain=$1
    dig @${DNS_SERVER} -p ${DNS_PORT} ${domain} A +short +tries=1 +time=2 2>/dev/null
}

query_dns_status() {
    local domain=$1
    dig @${DNS_SERVER} -p ${DNS_PORT} ${domain} A +tries=1 +time=2 2>/dev/null | grep "status:" | head -1
}

# =============================================================================
# Test Functions
# =============================================================================

test_round_robin() {
    log_test "TEST 1: Round-Robin Load Balancing"
    echo "Domain: healthy.test"
    echo "Expected: Rotating through 127.0.0.1, 127.0.0.2, 127.0.0.3"
    echo ""
    
    local results=()
    local pass=true
    
    for i in {1..6}; do
        result=$(query_dns "healthy.test")
        results+=("$result")
        echo "  Query $i: $result"
        sleep 0.2
    done
    
    # Verify we got different IPs
    local unique_ips=$(printf '%s\n' "${results[@]}" | sort -u | wc -l)
    
    if [ "$unique_ips" -ge 3 ]; then
        log_info "PASSED: Round-robin returned $unique_ips unique IPs"
    else
        log_error "FAILED: Expected 3 unique IPs, got $unique_ips"
        pass=false
    fi
    
    # Verify rotation pattern
    if [ "${results[0]}" == "${results[3]}" ] && [ "${results[1]}" == "${results[4]}" ]; then
        log_info "PASSED: Rotation pattern is consistent"
    else
        log_warn "Rotation pattern may not be perfectly sequential (not a failure)"
    fi
    
    $pass
}

test_unhealthy_servfail() {
    log_test "TEST 2: SERVFAIL for Unhealthy Servers"
    echo "Domain: unhealthy.test"
    echo "Expected: SERVFAIL status"
    echo ""
    
    local status=$(query_dns_status "unhealthy.test")
    echo "  Response: $status"
    
    if echo "$status" | grep -q "SERVFAIL"; then
        log_info "PASSED: Got SERVFAIL for unhealthy domain"
        return 0
    else
        log_error "FAILED: Expected SERVFAIL, got: $status"
        return 1
    fi
}

test_nxdomain() {
    log_test "TEST 3: NXDOMAIN for Unknown Domain"
    echo "Domain: nonexistent.test"
    echo "Expected: NXDOMAIN status"
    echo ""
    
    local status=$(query_dns_status "nonexistent.test")
    echo "  Response: $status"
    
    if echo "$status" | grep -q "NXDOMAIN"; then
        log_info "PASSED: Got NXDOMAIN for unknown domain"
        return 0
    else
        log_error "FAILED: Expected NXDOMAIN, got: $status"
        return 1
    fi
}

test_ttl() {
    log_test "TEST 4: TTL Value Verification"
    echo "Domain: healthy.test (configured TTL: 10)"
    echo ""
    
    local response=$(dig @${DNS_SERVER} -p ${DNS_PORT} healthy.test A +tries=1 +time=2 2>/dev/null)
    local ttl=$(echo "$response" | grep -A1 "ANSWER SECTION" | tail -1 | awk '{print $2}')
    
    echo "  TTL in response: $ttl"
    
    if [ "$ttl" -le 10 ] && [ "$ttl" -gt 0 ]; then
        log_info "PASSED: TTL is correct (≤10)"
        return 0
    else
        log_error "FAILED: Expected TTL ≤10, got: $ttl"
        return 1
    fi
}

test_tcp_query() {
    log_test "TEST 5: TCP Query Support"
    echo "Domain: healthy.test via TCP"
    echo ""
    
    local result=$(dig @${DNS_SERVER} -p ${DNS_PORT} healthy.test A +short +tcp +tries=1 +time=2 2>/dev/null)
    echo "  Response: $result"
    
    if [ -n "$result" ] && [[ "$result" =~ ^127\.0\.0\.[1-3]$ ]]; then
        log_info "PASSED: TCP query returned valid IP"
        return 0
    else
        log_error "FAILED: TCP query failed or returned invalid result"
        return 1
    fi
}

test_health_recovery() {
    log_test "TEST 6: Health Check Recovery"
    echo "Stopping one mock server, then restarting it"
    echo ""
    
    # Kill server on port 8082
    local pid_to_kill="${PYTHON_PIDS[1]}"
    log_info "Stopping server on 127.0.0.2:8082 (PID: $pid_to_kill)"
    kill "$pid_to_kill" 2>/dev/null || true
    
    # Wait for health check to mark it unhealthy
    log_info "Waiting for health check to detect failure..."
    sleep 5
    
    # Query should now only return 2 IPs
    echo "Queries after stopping one server:"
    local results_down=()
    for i in {1..4}; do
        result=$(query_dns "healthy.test")
        results_down+=("$result")
        echo "  Query $i: $result"
        sleep 0.2
    done
    
    # Check that 127.0.0.2 is not in results
    if printf '%s\n' "${results_down[@]}" | grep -q "127.0.0.2"; then
        log_error "FAILED: 127.0.0.2 still being returned after server stopped"
        return 1
    else
        log_info "PASSED: Unhealthy server excluded from rotation"
    fi
    
    # Restart the server
    log_info "Restarting server on 127.0.0.2:8082"
    server_dir="/tmp/opengslb-integration-test/server-8082"
    cd "$server_dir"
    python3 -m http.server 8082 --bind 127.0.0.2 > /dev/null 2>&1 &
    PYTHON_PIDS[1]=$!
    cd - > /dev/null
    
    # Wait for health check to mark it healthy again
    log_info "Waiting for health check to detect recovery..."
    sleep 5
    
    # Query should now return all 3 IPs again
    echo "Queries after restarting server:"
    local results_up=()
    for i in {1..6}; do
        result=$(query_dns "healthy.test")
        results_up+=("$result")
        echo "  Query $i: $result"
        sleep 0.2
    done
    
    # Check that 127.0.0.2 is back in results
    if printf '%s\n' "${results_up[@]}" | grep -q "127.0.0.2"; then
        log_info "PASSED: Server recovered and back in rotation"
        return 0
    else
        log_error "FAILED: 127.0.0.2 not returned after recovery"
        return 1
    fi
}

# =============================================================================
# Main
# =============================================================================

main() {
    echo "=============================================="
    echo "OpenGSLB Manual Integration Test"
    echo "=============================================="
    echo ""
    
    check_prerequisites
    setup_loopback_aliases
    create_config
    start_mock_servers
    start_opengslb
    
    echo ""
    echo "=============================================="
    echo "Running Tests"
    echo "=============================================="
    
    local passed=0
    local failed=0
    
    if test_round_robin; then ((passed++)); else ((failed++)); fi
    if test_unhealthy_servfail; then ((passed++)); else ((failed++)); fi
    if test_nxdomain; then ((passed++)); else ((failed++)); fi
    if test_ttl; then ((passed++)); else ((failed++)); fi
    if test_tcp_query; then ((passed++)); else ((failed++)); fi
    if test_health_recovery; then ((passed++)); else ((failed++)); fi
    
    echo ""
    echo "=============================================="
    echo "Test Summary"
    echo "=============================================="
    echo -e "Passed: ${GREEN}$passed${NC}"
    echo -e "Failed: ${RED}$failed${NC}"
    echo ""
    
    if [ "$failed" -eq 0 ]; then
        log_info "All tests passed!"
        exit 0
    else
        log_error "Some tests failed!"
        exit 1
    fi
}

main "$@"
