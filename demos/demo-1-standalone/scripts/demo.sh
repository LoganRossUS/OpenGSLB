#!/bin/bash
#
# OpenGSLB Demo 1: Standalone Overwatch
# Interactive demo script
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Configuration
DNS_PORT=15353  # Using 15353 to avoid mDNS/Avahi conflict on port 5353
API_PORT=8080
METRICS_PORT=9090
DOMAIN="app.demo.local"

print_header() {
    echo -e "\n${BOLD}${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${BLUE}  $1${NC}"
    echo -e "${BOLD}${BLUE}═══════════════════════════════════════════════════════════${NC}\n"
}

print_step() {
    echo -e "${CYAN}▶ $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

wait_for_enter() {
    echo -e "\n${YELLOW}Press Enter to continue...${NC}"
    read -r
}

check_prerequisites() {
    print_header "Checking Prerequisites"

    # Check for opengslb binary
    if [[ ! -f "./bin/opengslb" ]]; then
        print_error "opengslb binary not found at ./bin/opengslb"
        echo "Build it with: cd ../.. && make build && cp opengslb demos/demo-1-standalone/bin/"
        exit 1
    fi
    print_success "opengslb binary found"

    # Check for docker
    if ! command -v docker &> /dev/null; then
        print_error "docker not found"
        exit 1
    fi
    print_success "docker found"

    # Check for docker-compose
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        print_error "docker-compose not found"
        exit 1
    fi
    print_success "docker-compose found"

    # Check for dig
    if ! command -v dig &> /dev/null; then
        print_warning "dig not found - DNS queries won't work from host"
    else
        print_success "dig found"
    fi
}

start_demo() {
    print_header "Starting Demo Environment"

    print_step "Building and starting containers..."
    docker-compose up -d --build

    print_step "Waiting for services to be healthy..."
    sleep 5

    # Wait for overwatch to be ready
    local retries=30
    while ! curl -s http://localhost:${API_PORT}/api/v1/live > /dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ $retries -eq 0 ]]; then
            print_error "Overwatch failed to start"
            docker-compose logs overwatch
            exit 1
        fi
        sleep 1
    done

    print_success "Demo environment is running!"
    echo ""
    docker-compose ps
}

stop_demo() {
    print_header "Stopping Demo Environment"
    docker-compose down -v
    print_success "Demo environment stopped"
}

show_status() {
    print_header "Current Status"

    print_step "Container Status:"
    docker-compose ps

    echo ""
    print_step "Backend Health (via API):"
    curl -s http://localhost:${API_PORT}/api/v1/health/servers | jq '.' 2>/dev/null || echo "API not available"
}

demo_dns_queries() {
    print_header "Demo: DNS Round-Robin"

    echo "We'll query ${DOMAIN} multiple times to see round-robin in action."
    echo "Each query should return a different backend IP."
    echo ""

    wait_for_enter

    print_step "Querying DNS 6 times..."
    echo ""

    for i in {1..6}; do
        result=$(dig @localhost -p ${DNS_PORT} ${DOMAIN} +short 2>/dev/null || echo "DNS query failed")
        echo "  Query $i: $result"
        sleep 0.5
    done

    echo ""
    print_success "Notice how the IPs rotate through the backends!"
}

demo_failure_detection() {
    print_header "Demo: Failure Detection"

    echo "We'll stop webapp2 and watch it leave the DNS rotation."
    echo ""

    print_step "Current healthy backends:"
    curl -s http://localhost:${API_PORT}/api/v1/health/servers | jq -r '.servers[] | "\(.address) - \(.healthy)"' 2>/dev/null

    wait_for_enter

    print_step "Stopping webapp2..."
    docker stop webapp2

    echo ""
    print_warning "Waiting for health checks to detect failure (10-15 seconds)..."
    sleep 12

    print_step "Backend status after failure:"
    curl -s http://localhost:${API_PORT}/api/v1/health/servers | jq -r '.servers[] | "\(.address) - healthy: \(.healthy)"' 2>/dev/null

    echo ""
    print_step "DNS queries now (webapp2 should be gone):"
    for i in {1..4}; do
        result=$(dig @localhost -p ${DNS_PORT} ${DOMAIN} +short 2>/dev/null || echo "DNS query failed")
        echo "  Query $i: $result"
        sleep 0.5
    done

    echo ""
    print_success "webapp2 (10.10.0.22) is no longer in rotation!"
}

demo_recovery() {
    print_header "Demo: Recovery"

    echo "We'll restart webapp2 and watch it return to rotation."

    wait_for_enter

    print_step "Starting webapp2..."
    docker start webapp2

    echo ""
    print_warning "Waiting for health checks to detect recovery (5-10 seconds)..."
    sleep 8

    print_step "Backend status after recovery:"
    curl -s http://localhost:${API_PORT}/api/v1/health/servers | jq -r '.servers[] | "\(.address) - healthy: \(.healthy)"' 2>/dev/null

    echo ""
    print_step "DNS queries now (all 3 backends should be back):"
    for i in {1..6}; do
        result=$(dig @localhost -p ${DNS_PORT} ${DOMAIN} +short 2>/dev/null || echo "DNS query failed")
        echo "  Query $i: $result"
        sleep 0.5
    done

    echo ""
    print_success "webapp2 is back in rotation!"
}

demo_metrics() {
    print_header "Demo: Prometheus Metrics"

    echo "OpenGSLB exposes Prometheus metrics for monitoring."

    wait_for_enter

    print_step "Key metrics:"
    echo ""

    echo "DNS Queries:"
    curl -s http://localhost:${METRICS_PORT}/metrics | grep "opengslb_dns_queries_total" | head -5

    echo ""
    echo "Health Check Results:"
    curl -s http://localhost:${METRICS_PORT}/metrics | grep "opengslb_health_check" | head -5

    echo ""
    print_success "Metrics available at http://localhost:${METRICS_PORT}/metrics"
}

demo_api() {
    print_header "Demo: REST API"

    echo "OpenGSLB provides a REST API for management and monitoring."

    wait_for_enter

    print_step "API Endpoints:"
    echo ""

    echo "1. Liveness Check:"
    curl -s http://localhost:${API_PORT}/api/v1/live | jq '.'

    echo ""
    echo "2. Readiness Check:"
    curl -s http://localhost:${API_PORT}/api/v1/ready | jq '.'

    echo ""
    echo "3. Server Health:"
    curl -s http://localhost:${API_PORT}/api/v1/health/servers | jq '.'

    echo ""
    print_success "API available at http://localhost:${API_PORT}/api/v1/"
}

run_full_demo() {
    check_prerequisites
    start_demo

    wait_for_enter

    demo_dns_queries
    wait_for_enter

    demo_api
    wait_for_enter

    demo_failure_detection
    wait_for_enter

    demo_recovery
    wait_for_enter

    demo_metrics

    print_header "Demo Complete!"
    echo "The demo environment is still running. You can:"
    echo "  - Query DNS:    dig @localhost -p ${DNS_PORT} ${DOMAIN}"
    echo "  - Check API:    curl http://localhost:${API_PORT}/api/v1/health/servers"
    echo "  - View metrics: curl http://localhost:${METRICS_PORT}/metrics"
    echo "  - Stop demo:    $0 stop"
}

show_help() {
    echo "OpenGSLB Demo 1: Standalone Overwatch"
    echo ""
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  start     Start the demo environment"
    echo "  stop      Stop the demo environment"
    echo "  status    Show current status"
    echo "  demo      Run the full interactive demo"
    echo "  dns       Demo DNS round-robin"
    echo "  fail      Demo failure detection"
    echo "  recover   Demo recovery"
    echo "  metrics   Demo metrics"
    echo "  api       Demo REST API"
    echo "  help      Show this help"
}

# Main
cd "$(dirname "$0")/.."

case "${1:-}" in
    start)
        check_prerequisites
        start_demo
        ;;
    stop)
        stop_demo
        ;;
    status)
        show_status
        ;;
    demo)
        run_full_demo
        ;;
    dns)
        demo_dns_queries
        ;;
    fail)
        demo_failure_detection
        ;;
    recover)
        demo_recovery
        ;;
    metrics)
        demo_metrics
        ;;
    api)
        demo_api
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        show_help
        exit 1
        ;;
esac
