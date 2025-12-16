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
echo "This demo shows how agents can PROACTIVELY report health issues"
echo "before external health checks detect them."
echo ""
echo -e "${BOLD}Key insight:${NC} Agents have INSIDE KNOWLEDGE that external checks can't see."
echo ""
echo "Architecture:"
echo "  - webapp1, webapp2, webapp3: nginx + opengslb agent (sidecar)"
echo "  - overwatch: receives gossip from agents, serves DNS"
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
echo "The Overwatch API shows health status of all registered backends:"
echo ""
run_cmd "curl -s http://10.20.0.10:8080/api/v1/health/servers | jq '.servers[] | {address, healthy, last_seen}'"
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

header "Step 4: Traditional Failure (REACTIVE)"
echo "First, let's see what happens with a TRADITIONAL failure."
echo "We'll stop nginx on webapp2 and watch the external health check catch it."
echo ""
echo -e "${YELLOW}>>> Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}    docker exec webapp2 nginx -s stop${NC}"
echo ""
echo "Then come back here and press Enter."
pause

echo "Let's watch DNS. The backend will disappear after health checks fail (~5-10s)."
echo "Press Ctrl+C when you see webapp2 (10.20.0.22) removed, then press Enter."
echo ""
echo -e "${YELLOW}Watching DNS...${NC}"
echo ""
for i in {1..20}; do
    echo -n "$(date +%H:%M:%S) - "
    dig @10.20.0.10 app.demo.local +short | tr '\n' ' '
    echo ""
    sleep 2
done
pause

echo "Now let's restart nginx on webapp2."
echo ""
echo -e "${YELLOW}>>> Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}    docker exec webapp2 nginx${NC}"
echo ""
echo "Then come back here and press Enter."
pause

echo "Waiting for recovery..."
sleep 5
echo ""
run_cmd "dig @10.20.0.10 app.demo.local +short"
echo -e "${GREEN}webapp2 is back in rotation!${NC}"
pause

header "Step 5: PROACTIVE Drain (THE MAGIC)"
echo -e "${BOLD}This is the key demo feature!${NC}"
echo ""
echo "Now we'll drain webapp2 WITHOUT stopping nginx."
echo "The agent will report unhealthy, but nginx keeps serving!"
echo ""
echo -e "${YELLOW}>>> Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}    cd demos/demo-2-agent-overwatch${NC}"
echo -e "${CYAN}    ./scripts/drain.sh webapp2 on${NC}"
echo ""
echo "Then come back here and press Enter."
pause

echo "Let's verify nginx is STILL responding on webapp2:"
echo ""
echo -e "${CYAN}webapp2 (10.20.0.22):${NC}"
HTTP_RESULT=$(curl -s -o /dev/null -w "%{http_code}" http://10.20.0.22/health 2>/dev/null)
if [ "$HTTP_RESULT" = "200" ]; then
    curl -s http://10.20.0.22/health | jq .
    echo ""
    echo -e "${GREEN}nginx is healthy and responding with HTTP 200!${NC}"
else
    echo "HTTP Status: $HTTP_RESULT"
    echo -e "${YELLOW}Note: nginx might need a moment...${NC}"
fi
pause

echo "But check DNS - webapp2 should NOT be in rotation:"
echo ""
for i in {1..6}; do
    run_cmd "dig @10.20.0.10 app.demo.local +short"
    sleep 0.3
done
echo ""
if dig @10.20.0.10 app.demo.local +short | grep -q "10.20.0.22"; then
    echo -e "${YELLOW}webapp2 still in DNS - wait a few seconds for gossip propagation${NC}"
else
    echo -e "${RED}webapp2 (10.20.0.22) is NOT in DNS rotation!${NC}"
fi
echo ""
echo -e "${GREEN}The agent PROACTIVELY removed the backend before external checks failed.${NC}"
echo -e "${GREEN}This is the power of inside knowledge.${NC}"
pause

header "Step 6: Verify via API"
echo "Let's check the health API to see webapp2's status:"
echo ""
run_cmd "curl -s http://10.20.0.10:8080/api/v1/health/servers | jq '.servers[] | {address, healthy, last_seen}'"
echo ""
echo "Notice webapp2 shows as unhealthy due to agent drain signal."
pause

header "Step 7: Recovery"
echo "Let's recover webapp2 by removing the drain file."
echo ""
echo -e "${YELLOW}>>> Run this command in a HOST terminal:${NC}"
echo -e "${CYAN}    ./scripts/drain.sh webapp2 off${NC}"
echo ""
echo "Then come back here and press Enter."
pause

echo "Waiting for agent to restart and report healthy..."
sleep 8
echo ""
echo "Checking DNS..."
for i in {1..6}; do
    run_cmd "dig @10.20.0.10 app.demo.local +short"
    sleep 0.3
done
echo ""
if dig @10.20.0.10 app.demo.local +short | grep -q "10.20.0.22"; then
    echo -e "${GREEN}webapp2 is back in rotation!${NC}"
else
    echo -e "${YELLOW}webapp2 not yet visible - gossip may need more time${NC}"
fi
pause

header "Demo Complete!"
echo ""
echo -e "${BOLD}Key Takeaways:${NC}"
echo ""
echo "  1. Agents run alongside your apps with INSIDE knowledge"
echo "  2. Agents can signal problems BEFORE external checks fail"
echo "  3. This enables zero-failed-request maintenance drains"
echo "  4. External checks still provide a safety net"
echo ""
echo -e "${BOLD}Comparison:${NC}"
echo ""
echo "  Traditional GSLB (Demo 1):"
echo "    External check fails -> some requests fail -> DNS updates"
echo ""
echo "  Agent-based GSLB (Demo 2):"
echo "    Agent signals early -> DNS updates -> zero failed requests"
echo ""
echo -e "${GREEN}Try ./drain.sh on other nodes to experiment!${NC}"
echo ""
