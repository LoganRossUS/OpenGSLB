#!/bin/bash
# OpenGSLB Cluster Validation Script
# This script validates that an OpenGSLB cluster is functioning correctly.
# Designed to produce verbose output for debugging with Claude Code.
#
# Usage:
#   ./validate-cluster.sh --overwatch-ip IP [OPTIONS]
#
# Required Arguments:
#   --overwatch-ip      IP address of the Overwatch node
#
# Optional Arguments:
#   --expected-agents   Number of expected agents (for health check)
#   --expected-backends Number of expected backends (for health check)
#   --dns-zone          DNS zone to test (default: test.opengslb.local)
#   --service-name      Service name to test (default: web)
#   --timeout           Timeout for each test in seconds (default: 10)
#   --verbose           Enable extra verbose output
#   --json              Output results as JSON
#
# Exit codes:
#   0 - All tests passed
#   1 - One or more tests failed
#   2 - Invalid arguments

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
OVERWATCH_IP=""
EXPECTED_AGENTS=0
EXPECTED_BACKENDS=0
DNS_ZONE="test.opengslb.local"
SERVICE_NAME="web"
TIMEOUT=10
VERBOSE=false
JSON_OUTPUT=false

# Test results
declare -a TEST_RESULTS=()
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_WARNED=0

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --overwatch-ip)
                OVERWATCH_IP="$2"
                shift 2
                ;;
            --expected-agents)
                EXPECTED_AGENTS="$2"
                shift 2
                ;;
            --expected-backends)
                EXPECTED_BACKENDS="$2"
                shift 2
                ;;
            --dns-zone)
                DNS_ZONE="$2"
                shift 2
                ;;
            --service-name)
                SERVICE_NAME="$2"
                shift 2
                ;;
            --timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            --json)
                JSON_OUTPUT=true
                shift
                ;;
            --help|-h)
                echo "Usage: $0 --overwatch-ip IP [OPTIONS]"
                echo ""
                echo "Validates an OpenGSLB cluster deployment."
                echo ""
                echo "Required:"
                echo "  --overwatch-ip      Overwatch node IP address"
                echo ""
                echo "Optional:"
                echo "  --expected-agents   Expected number of agents"
                echo "  --expected-backends Expected number of backends"
                echo "  --dns-zone          DNS zone (default: test.opengslb.local)"
                echo "  --service-name      Service to test (default: web)"
                echo "  --timeout           Test timeout in seconds (default: 10)"
                echo "  --verbose           Extra verbose output"
                echo "  --json              Output results as JSON"
                exit 0
                ;;
            *)
                echo "Unknown argument: $1"
                exit 2
                ;;
        esac
    done

    if [[ -z "$OVERWATCH_IP" ]]; then
        echo "Error: --overwatch-ip is required"
        exit 2
    fi
}

# Print section header
print_header() {
    echo ""
    echo "================================================================================"
    echo "OpenGSLB Cluster Validation"
    echo "Started: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "================================================================================"
    echo ""
    echo "Configuration:"
    echo "  Overwatch IP:     $OVERWATCH_IP"
    echo "  DNS Zone:         $DNS_ZONE"
    echo "  Service Name:     $SERVICE_NAME"
    echo "  Expected Agents:  ${EXPECTED_AGENTS:-'(not specified)'}"
    echo "  Expected Backends: ${EXPECTED_BACKENDS:-'(not specified)'}"
    echo ""
}

# Print test result
test_pass() {
    local test_name="$1"
    local message="$2"
    echo -e "  ${GREEN}✓ PASS${NC} - $message"
    TEST_RESULTS+=("PASS|$test_name|$message")
    ((TESTS_PASSED++))
}

test_fail() {
    local test_name="$1"
    local message="$2"
    echo -e "  ${RED}✗ FAIL${NC} - $message"
    TEST_RESULTS+=("FAIL|$test_name|$message")
    ((TESTS_FAILED++))
}

test_warn() {
    local test_name="$1"
    local message="$2"
    echo -e "  ${YELLOW}⚠ WARN${NC} - $message"
    TEST_RESULTS+=("WARN|$test_name|$message")
    ((TESTS_WARNED++))
}

# Print debug info for failures
print_debug() {
    local title="$1"
    shift
    echo ""
    echo -e "  ${CYAN}DEBUG INFORMATION:${NC}"
    echo "  $title"
    echo "  ----------------------------------------"
    while IFS= read -r line; do
        echo "  $line"
    done
}

print_suggestion() {
    echo ""
    echo -e "  ${YELLOW}Possible causes:${NC}"
    while [[ $# -gt 0 ]]; do
        echo "    - $1"
        shift
    done
}

print_commands() {
    echo ""
    echo -e "  ${CYAN}Suggested commands to run on the failing node:${NC}"
    while [[ $# -gt 0 ]]; do
        echo "    $1"
        shift
    done
}

# Test 1: Network Connectivity
test_network_connectivity() {
    echo ""
    echo "[1/7] CHECKING NETWORK CONNECTIVITY"
    echo "--------------------------------------------------------------------------------"

    local ping_success=false
    local port_8080_success=false
    local port_53_success=false
    local port_7946_success=false

    # Ping test
    echo "  Testing ping to $OVERWATCH_IP..."
    if ping -c 1 -W 2 "$OVERWATCH_IP" >/dev/null 2>&1; then
        test_pass "network_ping" "Overwatch host is reachable via ICMP"
        ping_success=true
    else
        test_warn "network_ping" "Overwatch host does not respond to ICMP (may be blocked by firewall)"
    fi

    # Helper function to test port connectivity
    test_port() {
        local port=$1
        if command -v nc >/dev/null 2>&1; then
            nc -z -w 2 "$OVERWATCH_IP" "$port" 2>/dev/null
        else
            timeout 2 bash -c "echo >/dev/tcp/$OVERWATCH_IP/$port" 2>/dev/null
        fi
    }

    # Port 8080 (API)
    echo "  Testing port 8080 (API)..."
    if test_port 8080; then
        test_pass "network_port_8080" "API port 8080 is reachable"
        port_8080_success=true
    else
        test_fail "network_port_8080" "API port 8080 is not reachable"
        print_suggestion \
            "Firewall blocking port 8080" \
            "Overwatch API server not running" \
            "Wrong IP address specified"
        print_commands \
            "sudo ss -tlnp | grep 8080" \
            "sudo systemctl status opengslb-overwatch" \
            "sudo firewall-cmd --list-ports  # or check Azure NSG"
    fi

    # Port 53 (DNS)
    echo "  Testing port 53 (DNS)..."
    if test_port 53; then
        test_pass "network_port_53" "DNS port 53 is reachable"
        port_53_success=true
    else
        test_fail "network_port_53" "DNS port 53 is not reachable"
        print_suggestion \
            "Firewall blocking port 53" \
            "DNS server not running" \
            "systemd-resolved still holding port 53"
        print_commands \
            "sudo ss -tlnp | grep :53" \
            "sudo systemctl status systemd-resolved" \
            "sudo journalctl -u opengslb-overwatch | grep -i dns"
    fi

    # Port 7946 (Gossip)
    echo "  Testing port 7946 (Gossip)..."
    if test_port 7946; then
        test_pass "network_port_7946" "Gossip port 7946 is reachable"
        port_7946_success=true
    else
        test_warn "network_port_7946" "Gossip port 7946 is not reachable (may affect agent registration)"
    fi
}

# Test 2: Overwatch Health
test_overwatch_health() {
    echo ""
    echo "[2/7] CHECKING OVERWATCH HEALTH"
    echo "--------------------------------------------------------------------------------"

    local health_url="http://${OVERWATCH_IP}:8080/api/v1/live"
    echo "  Endpoint: $health_url"

    local response
    local http_code

    response=$(curl -s -w "\n%{http_code}" --connect-timeout "$TIMEOUT" "$health_url" 2>&1) || true
    http_code=$(echo "$response" | tail -1)
    response=$(echo "$response" | sed '$d')

    if [[ "$http_code" == "200" ]]; then
        test_pass "overwatch_health" "Overwatch health endpoint responding (HTTP 200)"
        if [[ "$VERBOSE" == "true" ]]; then
            echo "  Response: $response"
        fi
    else
        test_fail "overwatch_health" "Overwatch health endpoint failed (HTTP $http_code)"
        echo "  Response: $response"
        print_suggestion \
            "Overwatch service not running" \
            "API server failed to start" \
            "Port 8080 bound by another process"
        print_commands \
            "sudo systemctl status opengslb-overwatch" \
            "sudo journalctl -u opengslb-overwatch --no-pager -n 50" \
            "sudo ss -tlnp | grep 8080"
    fi
}

# Test 3: Cluster Status
test_cluster_status() {
    echo ""
    echo "[3/7] CHECKING CLUSTER STATUS"
    echo "--------------------------------------------------------------------------------"

    local status_url="http://${OVERWATCH_IP}:8080/api/v1/cluster/status"
    if [[ $EXPECTED_AGENTS -gt 0 ]]; then
        status_url="${status_url}?expected_agents=${EXPECTED_AGENTS}"
    fi
    echo "  Endpoint: $status_url"

    local response
    local http_code

    response=$(curl -s -w "\n%{http_code}" --connect-timeout "$TIMEOUT" "$status_url" 2>&1) || true
    http_code=$(echo "$response" | tail -1)
    response=$(echo "$response" | sed '$d')

    if [[ "$http_code" != "200" ]]; then
        test_fail "cluster_status" "Cluster status endpoint failed (HTTP $http_code)"
        echo "  Response: $response"
        return
    fi

    # Parse response
    local cluster_healthy
    local healthy_agents
    local gossip_members
    local backend_total
    local backend_healthy

    cluster_healthy=$(echo "$response" | grep -o '"cluster_healthy":[^,}]*' | cut -d: -f2 | tr -d ' ')
    healthy_agents=$(echo "$response" | grep -o '"healthy_agents":[0-9]*' | cut -d: -f2)
    gossip_members=$(echo "$response" | grep -o '"gossip_members":[0-9]*' | cut -d: -f2)
    backend_total=$(echo "$response" | grep -o '"total":[0-9]*' | head -1 | cut -d: -f2)
    backend_healthy=$(echo "$response" | grep -o '"healthy":[0-9]*' | head -1 | cut -d: -f2)

    echo "  Cluster Healthy:  $cluster_healthy"
    echo "  Healthy Agents:   ${healthy_agents:-0}"
    echo "  Gossip Members:   ${gossip_members:-0}"
    echo "  Total Backends:   ${backend_total:-0}"
    echo "  Healthy Backends: ${backend_healthy:-0}"

    if [[ "$cluster_healthy" == "true" ]]; then
        test_pass "cluster_healthy" "Cluster reports healthy status"
    else
        test_fail "cluster_healthy" "Cluster reports unhealthy status"
        print_suggestion \
            "Not all expected agents have registered" \
            "Agents cannot reach Overwatch gossip port" \
            "Gossip encryption key mismatch"
    fi

    # Check agent count
    if [[ $EXPECTED_AGENTS -gt 0 ]]; then
        if [[ ${healthy_agents:-0} -ge $EXPECTED_AGENTS ]]; then
            test_pass "agent_count" "Expected $EXPECTED_AGENTS agents, found ${healthy_agents:-0}"
        else
            test_fail "agent_count" "Expected $EXPECTED_AGENTS agents, found ${healthy_agents:-0}"
            print_suggestion \
                "Agents still starting up" \
                "Agent bootstrap failed" \
                "Network connectivity issues between agents and overwatch"
            print_commands \
                "# On each agent VM:" \
                "sudo systemctl status opengslb-agent" \
                "sudo journalctl -u opengslb-agent --no-pager -n 50" \
                "nc -zv $OVERWATCH_IP 7946"
        fi
    fi

    # Check backend count
    if [[ $EXPECTED_BACKENDS -gt 0 ]]; then
        if [[ ${backend_healthy:-0} -ge $EXPECTED_BACKENDS ]]; then
            test_pass "backend_count" "Expected $EXPECTED_BACKENDS backends, found ${backend_healthy:-0} healthy"
        else
            test_fail "backend_count" "Expected $EXPECTED_BACKENDS healthy backends, found ${backend_healthy:-0}"
        fi
    fi

    if [[ "$VERBOSE" == "true" ]]; then
        echo ""
        echo "  Full response:"
        echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
    fi
}

# Test 4: DNS Resolution
test_dns_resolution() {
    echo ""
    echo "[4/7] TESTING DNS RESOLUTION"
    echo "--------------------------------------------------------------------------------"

    local query="${SERVICE_NAME}.${DNS_ZONE}"
    echo "  Query: dig @${OVERWATCH_IP} ${query} A +short"

    local response
    response=$(dig @"$OVERWATCH_IP" "$query" A +short +time=5 2>&1) || true

    if [[ -z "$response" ]]; then
        test_fail "dns_resolution" "DNS query returned empty response"
        print_suggestion \
            "No backends registered for service '$SERVICE_NAME'" \
            "DNS zone '$DNS_ZONE' not configured" \
            "DNS server not running"
        print_commands \
            "dig @${OVERWATCH_IP} ${query} A +trace" \
            "curl -s http://${OVERWATCH_IP}:8080/api/v1/overwatch/backends | jq ." \
            "sudo journalctl -u opengslb-overwatch | grep -i dns"
        return
    fi

    if [[ "$response" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || [[ "$response" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+[[:space:]] ]]; then
        test_pass "dns_resolution" "DNS resolved to: $response"

        # Store the IP for later tests
        DNS_RESOLVED_IP=$(echo "$response" | head -1 | tr -d '[:space:]')
    else
        test_fail "dns_resolution" "DNS returned unexpected response: $response"
        print_suggestion \
            "DNS server returned error" \
            "Query format incorrect" \
            "Zone not configured properly"

        echo ""
        echo "  Full dig output:"
        dig @"$OVERWATCH_IP" "$query" A +noall +answer +comments 2>&1 | while IFS= read -r line; do
            echo "    $line"
        done
    fi
}

# Test 5: HTTP Backend Connectivity
test_http_backends() {
    echo ""
    echo "[5/7] TESTING HTTP BACKEND CONNECTIVITY"
    echo "--------------------------------------------------------------------------------"

    # Get list of backends from API
    local backends_url="http://${OVERWATCH_IP}:8080/api/v1/overwatch/backends"
    echo "  Fetching backends from: $backends_url"

    local response
    response=$(curl -s --connect-timeout "$TIMEOUT" "$backends_url" 2>&1) || true

    if [[ -z "$response" ]] || [[ "$response" == *"error"* && "$response" != *"backends"* ]]; then
        test_warn "http_backends" "Could not fetch backend list from API"
        return
    fi

    # Parse backend addresses (simple extraction)
    local addresses
    addresses=$(echo "$response" | grep -oE '"address":"[^"]*"' | cut -d'"' -f4 | sort -u)

    if [[ -z "$addresses" ]]; then
        test_warn "http_backends" "No backends found in API response"
        if [[ "$VERBOSE" == "true" ]]; then
            echo "  Response: $response"
        fi
        return
    fi

    local backend_count=0
    local backend_success=0

    for addr in $addresses; do
        ((backend_count++))

        # Get port for this backend
        local port
        port=$(echo "$response" | grep -oE "\"address\":\"$addr\",\"port\":[0-9]+" | grep -oE "[0-9]+$" | head -1)
        port=${port:-80}

        echo "  Testing backend $addr:$port..."

        local http_response
        local http_code

        http_response=$(curl -s -w "\n%{http_code}" --connect-timeout 5 "http://${addr}:${port}/" 2>&1) || true
        http_code=$(echo "$http_response" | tail -1)
        http_response=$(echo "$http_response" | sed '$d' | head -c 200)

        if [[ "$http_code" == "200" ]]; then
            test_pass "backend_$addr" "Backend $addr:$port responding (HTTP 200)"
            ((backend_success++))
            if [[ "$VERBOSE" == "true" ]]; then
                echo "    Body (truncated): $http_response"
            fi
        else
            test_fail "backend_$addr" "Backend $addr:$port failed (HTTP $http_code)"
            echo "    Response: $http_response"
        fi
    done

    if [[ $backend_count -gt 0 ]]; then
        if [[ $backend_success -eq $backend_count ]]; then
            test_pass "all_backends" "All $backend_count backends are responding"
        else
            test_fail "all_backends" "Only $backend_success of $backend_count backends responding"
        fi
    fi
}

# Test 6: Latency API
test_latency_api() {
    echo ""
    echo "[6/7] TESTING LATENCY API"
    echo "--------------------------------------------------------------------------------"

    local latency_url="http://${OVERWATCH_IP}:8080/api/v1/overwatch/latency"
    echo "  Endpoint: $latency_url"

    local response
    local http_code

    response=$(curl -s -w "\n%{http_code}" --connect-timeout "$TIMEOUT" "$latency_url" 2>&1) || true
    http_code=$(echo "$response" | tail -1)
    response=$(echo "$response" | sed '$d')

    if [[ "$http_code" != "200" ]]; then
        test_fail "latency_api" "Latency API failed (HTTP $http_code)"
        return
    fi

    local entry_count
    local subnet_count

    entry_count=$(echo "$response" | grep -o '"count":[0-9]*' | cut -d: -f2)
    subnet_count=$(echo "$response" | grep -o '"subnet_count":[0-9]*' | cut -d: -f2)

    echo "  Entry Count:  ${entry_count:-0}"
    echo "  Subnet Count: ${subnet_count:-0}"

    test_pass "latency_api" "Latency API responding"

    if [[ ${entry_count:-0} -eq 0 ]]; then
        test_warn "latency_data" "No latency data collected yet (expected before traffic generation)"
    else
        test_pass "latency_data" "Latency data present: ${entry_count} entries across ${subnet_count} subnets"
    fi

    if [[ "$VERBOSE" == "true" ]]; then
        echo ""
        echo "  Full response:"
        echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
    fi
}

# Test 7: End-to-End Routing
test_end_to_end() {
    echo ""
    echo "[7/7] TESTING END-TO-END ROUTING"
    echo "--------------------------------------------------------------------------------"

    local query="${SERVICE_NAME}.${DNS_ZONE}"
    local test_count=5
    local success_count=0
    local failed_count=0

    echo "  Generating $test_count requests through DNS..."

    for i in $(seq 1 $test_count); do
        # Resolve DNS
        local ip
        ip=$(dig @"$OVERWATCH_IP" "$query" A +short +time=2 2>/dev/null | head -1)

        if [[ -z "$ip" ]] || [[ ! "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "  Request $i: DNS failed"
            ((failed_count++))
            continue
        fi

        # Make HTTP request
        local start_time
        local end_time
        local http_code
        local body

        start_time=$(date +%s%3N)
        response=$(curl -s -w "\n%{http_code}" --connect-timeout 5 "http://${ip}/" 2>&1) || true
        end_time=$(date +%s%3N)

        http_code=$(echo "$response" | tail -1)
        body=$(echo "$response" | sed '$d' | head -c 100)

        local rtt=$((end_time - start_time))

        # Extract region from body if present
        local region="unknown"
        if [[ "$body" =~ [Rr]egion[:\s]*([a-zA-Z0-9-]+) ]]; then
            region="${BASH_REMATCH[1]}"
        fi

        if [[ "$http_code" == "200" ]]; then
            echo "  Request $i: DNS=$ip, HTTP=200, Region=$region, RTT=${rtt}ms"
            ((success_count++))
        else
            echo "  Request $i: DNS=$ip, HTTP=$http_code (failed)"
            ((failed_count++))
        fi

        # Small delay between requests
        sleep 0.5
    done

    echo ""
    if [[ $success_count -eq $test_count ]]; then
        test_pass "end_to_end" "All $test_count requests successful"
    elif [[ $success_count -gt 0 ]]; then
        test_warn "end_to_end" "$success_count of $test_count requests successful"
    else
        test_fail "end_to_end" "All $test_count requests failed"
        print_suggestion \
            "Backends not responding" \
            "DNS returning wrong IPs" \
            "Network connectivity issues"
    fi
}

# Print summary
print_summary() {
    echo ""
    echo "================================================================================"
    echo "VALIDATION COMPLETE"
    echo "================================================================================"
    echo "  Total tests:  $((TESTS_PASSED + TESTS_FAILED + TESTS_WARNED))"
    echo "  Passed:       ${TESTS_PASSED}"
    echo "  Failed:       ${TESTS_FAILED}"
    echo "  Warnings:     ${TESTS_WARNED}"
    echo ""

    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "  Status: ${GREEN}✓ ALL TESTS PASSED${NC}"
        echo ""
        echo "  Cluster is ready for use."
    else
        echo -e "  Status: ${RED}✗ SOME TESTS FAILED${NC}"
        echo ""
        echo "  Review the failures above and check the suggested commands."
        echo "  For detailed debugging, run with --verbose flag."
    fi

    echo "================================================================================"
}

# Output JSON results
output_json() {
    echo "{"
    echo "  \"timestamp\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\","
    echo "  \"overwatch_ip\": \"$OVERWATCH_IP\","
    echo "  \"tests_passed\": $TESTS_PASSED,"
    echo "  \"tests_failed\": $TESTS_FAILED,"
    echo "  \"tests_warned\": $TESTS_WARNED,"
    echo "  \"success\": $([ $TESTS_FAILED -eq 0 ] && echo "true" || echo "false"),"
    echo "  \"results\": ["

    local first=true
    for result in "${TEST_RESULTS[@]}"; do
        IFS='|' read -r status name message <<< "$result"
        if [[ "$first" == "true" ]]; then
            first=false
        else
            echo ","
        fi
        echo -n "    {\"status\": \"$status\", \"test\": \"$name\", \"message\": \"$message\"}"
    done

    echo ""
    echo "  ]"
    echo "}"
}

# Main
main() {
    parse_args "$@"

    if [[ "$JSON_OUTPUT" != "true" ]]; then
        print_header
    fi

    test_network_connectivity
    test_overwatch_health
    test_cluster_status
    test_dns_resolution
    test_http_backends
    test_latency_api
    test_end_to_end

    if [[ "$JSON_OUTPUT" == "true" ]]; then
        output_json
    else
        print_summary
    fi

    if [[ $TESTS_FAILED -gt 0 ]]; then
        exit 1
    fi
    exit 0
}

main "$@"
