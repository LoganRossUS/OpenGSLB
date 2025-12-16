#!/bin/bash
# Interactive demonstration of OpenGSLB latency-based routing

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

header() {
    echo ""
    echo -e "${CYAN}=================================================================${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}=================================================================${NC}"
    echo ""
}

run_cmd() {
    echo -e "${YELLOW}\$ $1${NC}"
    eval "$1"
    echo ""
}

pause() {
    echo -e "${GREEN}Press Enter to continue...${NC}"
    read -r
}

header "OpenGSLB Demo 3: Latency-Based Routing"
echo "This demo shows how OpenGSLB automatically routes to the fastest backend."
echo ""
echo "We use Linux 'tc' (traffic control) to simulate different network latencies:"
echo "  - webapp1: 5ms   (FAST)"
echo "  - webapp2: 50ms  (MEDIUM)"
echo "  - webapp3: 150ms (SLOW)"
echo ""
echo "OpenGSLB measures health check response times and routes to the lowest latency."
pause

header "Step 1: Set Initial Latencies"
echo "Setting up simulated network conditions..."
echo ""
echo -e "${YELLOW}Run these commands in a HOST terminal:${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp1 5${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp2 50${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp3 150${NC}"
echo ""
echo "Wait a moment for the commands to be executed..."
pause

header "Step 2: Check Health API (View Latencies)"
echo "The API shows measured latency for each backend:"
echo ""
run_cmd "curl -s http://10.30.0.10:8080/api/v1/health/servers | jq '.servers[] | {address, latency_ms: .latency_ms, healthy}'"
echo ""
echo "Notice the 'latency_ms' field shows the measured response time for each server!"
pause

header "Step 3: Query DNS (Latency-Based Selection)"
echo "With latency routing, we should consistently get the FASTEST server (webapp1):"
echo ""
for i in {1..6}; do
    run_cmd "dig @10.30.0.10 app.demo.local +short"
    sleep 0.5
done
echo ""
echo -e "${GREEN}Notice: All responses should be 10.30.0.21 (webapp1 - fastest at 5ms)${NC}"
pause

header "Step 4: Change the Game - Make webapp2 Fastest"
echo "Now let's make webapp2 the fastest by reducing its latency..."
echo ""
echo -e "${YELLOW}Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp2 2${NC}"
echo ""
echo "Wait for the command to be executed..."
pause

echo "Waiting for health checks to detect the change (~5 seconds)..."
sleep 6

header "Step 5: Verify Routing Changed"
echo "Now DNS should return webapp2 (10.30.0.22) since it's fastest at 2ms:"
echo ""
for i in {1..6}; do
    run_cmd "dig @10.30.0.10 app.demo.local +short"
    sleep 0.5
done
echo ""
echo -e "${GREEN}Notice: Responses should now be 10.30.0.22 (webapp2 - now fastest)${NC}"
pause

header "Step 6: API Shows Updated Latencies"
run_cmd "curl -s http://10.30.0.10:8080/api/v1/health/servers | jq '.servers[] | {address, latency_ms: .latency_ms, healthy}'"
pause

header "Step 7: Extreme Scenario - All Latencies Equal"
echo "What happens when all servers have similar latency?"
echo ""
echo -e "${YELLOW}Run these commands in a HOST terminal:${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp1 10${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp2 10${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp3 10${NC}"
echo ""
echo "Wait for the commands to be executed..."
pause

echo "Waiting for health checks to stabilize..."
sleep 6

echo "With equal latencies, OpenGSLB may distribute traffic more evenly:"
for i in {1..9}; do
    run_cmd "dig @10.30.0.10 app.demo.local +short"
    sleep 0.3
done
pause

header "Step 8: Simulate Network Degradation"
echo "Simulating network problems on webapp1 (increase latency dramatically)..."
echo ""
echo -e "${YELLOW}Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp1 500${NC}"
echo ""
echo "Wait for the command to be executed..."
pause

echo "Waiting for detection..."
sleep 6

echo "DNS should now avoid webapp1 (too slow):"
for i in {1..6}; do
    run_cmd "dig @10.30.0.10 app.demo.local +short"
    sleep 0.5
done
pause

header "Step 9: Recovery"
echo "Restoring webapp1 to fast performance..."
echo ""
echo -e "${YELLOW}Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}  ./scripts/set-latency.sh webapp1 5${NC}"
echo ""
echo "Wait for the command to be executed..."
pause

echo "Waiting for recovery detection..."
sleep 6

echo "webapp1 should return to rotation (and be preferred if fastest):"
for i in {1..6}; do
    run_cmd "dig @10.30.0.10 app.demo.local +short"
    sleep 0.5
done
pause

header "Demo Complete!"
echo "Key takeaways:"
echo ""
echo "  1. OpenGSLB measures actual health check response times"
echo "  2. Latency-based routing automatically selects the fastest backend"
echo "  3. Changes are detected within seconds (configurable)"
echo "  4. When latencies are equal, traffic is distributed more evenly"
echo "  5. No configuration changes needed - routing adapts automatically"
echo ""
echo "Real-world applications:"
echo "  - Route to the closest datacenter"
echo "  - Automatically shift traffic during network congestion"
echo "  - Performance-aware load balancing"
echo ""
echo "Try experimenting with different latency values!"
echo ""
