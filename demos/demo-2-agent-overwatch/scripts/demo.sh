#!/bin/bash
# OpenGSLB Demo 2: Interactive Guided Demo
#
# This script walks you through the agent-overwatch architecture demo.
# Run this from inside the client container.

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

pause() {
    echo ""
    read -p "Press Enter to continue..."
    echo ""
}

header() {
    echo ""
    echo -e "${BLUE}================================================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}================================================================${NC}"
    echo ""
}

run_cmd() {
    echo -e "${CYAN}\$ $1${NC}"
    eval $1
    echo ""
}

clear
header "OpenGSLB Demo 2: Agent-Overwatch Architecture"
echo "This demo shows how backends can PROACTIVELY signal they're unhealthy"
echo "before external health checks would normally detect a problem."
echo ""
echo -e "${BOLD}Key insight:${NC} The drain mechanism lets you remove a backend from DNS"
echo "while it's still serving traffic - enabling graceful maintenance."
echo ""
echo "Architecture:"
echo "  - webapp1, webapp2, webapp3: nginx servers with health endpoints"
echo "  - overwatch: external health checks + DNS server"
echo "  - client: where you're running this demo"
pause

header "Step 1: Verify All Backends Are Healthy"
echo "Let's query DNS for app.demo.local multiple times."
echo "You should see all 3 backend IPs rotating (round-robin):"
echo "  - 10.20.0.21 (webapp1)"
echo "  - 10.20.0.22 (webapp2)"
echo "  - 10.20.0.23 (webapp3)"
echo ""
for i in {1..6}; do
    run_cmd "dig @10.20.0.10 app.demo.local +short"
    sleep 0.3
done
echo -e "${GREEN}All 3 backends are in rotation!${NC}"
pause

header "Step 2: Check Health via API"
echo "The Overwatch API shows health status of all backends:"
echo ""
run_cmd "curl -s http://10.20.0.10:8080/api/v1/health/servers | jq '.servers[] | {address, healthy}'"
pause

header "Step 3: Verify nginx Servers Are Responding"
echo "Let's hit each nginx server directly:"
echo ""
echo -e "${CYAN}webapp1 (10.20.0.21):${NC}"
curl -s http://10.20.0.21/health 2>/dev/null | jq . 2>/dev/null || echo "Connection failed"
echo ""
echo -e "${CYAN}webapp2 (10.20.0.22):${NC}"
curl -s http://10.20.0.22/health 2>/dev/null | jq . 2>/dev/null || echo "Connection failed"
echo ""
echo -e "${CYAN}webapp3 (10.20.0.23):${NC}"
curl -s http://10.20.0.23/health 2>/dev/null | jq . 2>/dev/null || echo "Connection failed"
echo ""
echo -e "${GREEN}All nginx servers healthy!${NC}"
pause

header "Step 4: PROACTIVE Drain (THE KEY DEMO)"
echo -e "${BOLD}This is the main feature we're demonstrating!${NC}"
echo ""
echo "We'll drain webapp2 by creating a drain file. This makes"
echo "the /health endpoint return 503 (unhealthy), but the main"
echo "page continues serving traffic!"
echo ""
echo -e "${YELLOW}>>> Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}    docker exec webapp2 touch /tmp/drain${NC}"
echo ""
echo "Then come back here and press Enter."
pause

echo "First, let's verify the /health endpoint now returns 503:"
echo ""
echo -e "${CYAN}webapp2 /health:${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://10.20.0.22/health)
if [ "$HTTP_CODE" = "503" ]; then
    echo -e "${RED}HTTP $HTTP_CODE - Health check returns UNHEALTHY${NC}"
    curl -s http://10.20.0.22/health | jq . 2>/dev/null
else
    echo "HTTP $HTTP_CODE"
    curl -s http://10.20.0.22/health | jq . 2>/dev/null
fi
echo ""

echo "But the main page STILL WORKS:"
echo -e "${CYAN}webapp2 main page:${NC}"
MAIN_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://10.20.0.22/)
echo -e "${GREEN}HTTP $MAIN_CODE - Main page responds!${NC}"
pause

echo "Now let's watch DNS. Overwatch will detect the unhealthy /health"
echo "response and remove webapp2 from rotation (~5-10 seconds)."
echo ""
echo -e "${YELLOW}Watching DNS (will check every 2s for 30s max)...${NC}"
echo ""
for i in {1..15}; do
    RESULT=$(dig @10.20.0.10 app.demo.local +short | tr '\n' ' ')
    echo "$(date +%H:%M:%S) - $RESULT"
    if ! echo "$RESULT" | grep -q "10.20.0.22"; then
        echo ""
        echo -e "${RED}webapp2 (10.20.0.22) has been REMOVED from DNS!${NC}"
        break
    fi
    sleep 2
done
echo ""
echo -e "${GREEN}The backend was removed while nginx is still running!${NC}"
pause

header "Step 5: Verify Traffic Still Works"
echo "Even though webapp2 is out of DNS, it's still serving traffic."
echo "This is key for graceful drains - existing connections complete."
echo ""
echo -e "${CYAN}Direct request to webapp2:${NC}"
curl -s http://10.20.0.22/ | head -5
echo "..."
echo ""
echo -e "${GREEN}Traffic still works! Perfect for graceful maintenance.${NC}"
pause

header "Step 6: Check Health API"
echo "Let's see what Overwatch knows about the backends:"
echo ""
run_cmd "curl -s http://10.20.0.10:8080/api/v1/health/servers | jq '.servers[] | {address, healthy}'"
echo ""
echo "Notice webapp2 (10.20.0.22) shows as unhealthy."
pause

header "Step 7: Recovery"
echo "Let's recover webapp2 by removing the drain file."
echo ""
echo -e "${YELLOW}>>> Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}    docker exec webapp2 rm /tmp/drain${NC}"
echo ""
echo "Then come back here and press Enter."
pause

echo "Watching for recovery (~5-10 seconds)..."
echo ""
for i in {1..15}; do
    RESULT=$(dig @10.20.0.10 app.demo.local +short | tr '\n' ' ')
    echo "$(date +%H:%M:%S) - $RESULT"
    if echo "$RESULT" | grep -q "10.20.0.22"; then
        echo ""
        echo -e "${GREEN}webapp2 (10.20.0.22) is BACK in DNS rotation!${NC}"
        break
    fi
    sleep 2
done
pause

header "Demo Complete!"
echo ""
echo -e "${BOLD}Key Takeaways:${NC}"
echo ""
echo "  1. Drain files let backends signal 'I'm going away'"
echo "  2. /health returns unhealthy, but main traffic continues"
echo "  3. Overwatch detects the unhealthy status and updates DNS"
echo "  4. Existing connections complete gracefully"
echo "  5. Perfect for maintenance, deployments, and scaling"
echo ""
echo -e "${BOLD}Try it yourself:${NC}"
echo ""
echo "  docker exec webapp1 touch /tmp/drain   # Drain webapp1"
echo "  docker exec webapp1 rm /tmp/drain      # Recover webapp1"
echo "  dig @10.20.0.10 app.demo.local +short  # Check DNS"
echo ""
