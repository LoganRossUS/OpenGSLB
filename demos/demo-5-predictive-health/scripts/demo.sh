#!/bin/bash
#
# demo.sh - Guided walkthrough for OpenGSLB Demo 5: Predictive Health Detection
#
# This script walks through the key demo scenarios step by step.

OVERWATCH_IP="10.50.0.10"
BACKEND3_URL="http://10.50.0.23:8080"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

press_enter() {
    echo ""
    echo -e "${CYAN}Press Enter to continue...${NC}"
    read
}

clear
echo -e "${BOLD}${BLUE}"
cat << 'EOF'
  ╔══════════════════════════════════════════════════════════════════════╗
  ║                                                                      ║
  ║     OpenGSLB Demo 5: Predictive Health Detection                     ║
  ║                                                                      ║
  ║     "We knew it was going to fail before it did."                    ║
  ║                                                                      ║
  ╚══════════════════════════════════════════════════════════════════════╝
EOF
echo -e "${NC}"

echo -e "This demo shows how OpenGSLB's predictive health monitoring detects"
echo -e "problems ${BOLD}before${NC} they impact users."
echo ""
echo -e "Traditional GSLB:"
echo -e "  ${RED}App crashes → Health check fails → DNS updated → Users see errors${NC}"
echo ""
echo -e "OpenGSLB:"
echo -e "  ${GREEN}CPU spikes → Agent predicts failure → Traffic drains → App crashes → Zero user impact${NC}"

press_enter

# ============================================================================
# ACT 1: BASELINE
# ============================================================================
clear
echo -e "${BOLD}${BLUE}━━━ ACT 1: BASELINE ━━━${NC}"
echo ""
echo "Let's verify all backends are healthy and DNS is working."
echo ""

echo -e "${YELLOW}Checking backend health via Overwatch API:${NC}"
curl -s "http://${OVERWATCH_IP}:8080/api/v1/health/servers" | jq '.servers[] | {address, healthy, draining}'
echo ""

echo -e "${YELLOW}DNS resolution (should return all 3 backends):${NC}"
for i in 1 2 3; do
    echo -n "Query $i: "
    dig @${OVERWATCH_IP} app.demo.local +short | tr '\n' ' '
    echo ""
    sleep 1
done

echo ""
echo -e "${GREEN}All 3 backends are healthy and in rotation.${NC}"

press_enter

# ============================================================================
# ACT 2: TRIGGER CPU SPIKE
# ============================================================================
clear
echo -e "${BOLD}${BLUE}━━━ ACT 2: TRIGGER CPU SPIKE ━━━${NC}"
echo ""
echo "Now we'll simulate a problem: a runaway process causing high CPU on backend-3."
echo ""
echo -e "The app is ${GREEN}still responding${NC}, health checks ${GREEN}still pass${NC},"
echo -e "but ${RED}trouble is brewing${NC}..."
echo ""

echo -e "${YELLOW}Triggering CPU spike (85% for 60 seconds):${NC}"
curl -s -X POST "${BACKEND3_URL}/chaos/cpu?duration=60s&intensity=85" | jq .
echo ""

echo -e "${BOLD}What happens next:${NC}"
echo "1. The agent on backend-3 detects elevated CPU"
echo "2. Agent sends a 'bleed' signal via gossip to Overwatch"
echo "3. Overwatch starts draining traffic from backend-3"
echo "4. DNS queries exclude backend-3"
echo ""
echo -e "${CYAN}Watch the Grafana dashboard at http://localhost:3000${NC}"
echo -e "${CYAN}You'll see the CPU spike on backend-3!${NC}"

press_enter

# ============================================================================
# ACT 3: TRAFFIC SHIFTS
# ============================================================================
clear
echo -e "${BOLD}${BLUE}━━━ ACT 3: TRAFFIC SHIFTS ━━━${NC}"
echo ""
echo "Let's see how DNS responds now..."
echo ""

echo -e "${YELLOW}DNS queries (notice backend-3 may be excluded):${NC}"
for i in 1 2 3 4 5 6; do
    RESULT=$(dig @${OVERWATCH_IP} app.demo.local +short | sort | tr '\n' ' ')
    if echo "$RESULT" | grep -q "10.50.0.23"; then
        echo -e "Query $i: ${RESULT}"
    else
        echo -e "Query $i: ${RESULT} ${RED}(backend-3 excluded!)${NC}"
    fi
    sleep 2
done

echo ""
echo -e "${YELLOW}But the health check still passes!${NC}"
echo -n "Health check: "
curl -s "${BACKEND3_URL}/health" | jq -c .
echo ""

echo -e "${YELLOW}Overwatch API shows backend-3 draining:${NC}"
curl -s "http://${OVERWATCH_IP}:8080/api/v1/health/servers" | jq '.servers[] | select(.address | contains("10.50.0.23"))'

echo ""
echo -e "${GREEN}Key insight:${NC} The backend's health check is still passing!"
echo "But we're proactively draining because the agent predicted trouble."
echo ""
echo "This is 'predictive from the inside, reactive from the outside.'"

press_enter

# ============================================================================
# ACT 4: OVERWATCH VALIDATES (OPTIONAL)
# ============================================================================
clear
echo -e "${BOLD}${BLUE}━━━ ACT 4: OVERWATCH VALIDATES ━━━${NC}"
echo ""
echo "What if the agent was wrong? Or lying?"
echo "Overwatch doesn't just trust - it validates."
echo ""
echo -e "${RED}Let's make the prediction come true - trigger actual errors:${NC}"
echo ""

curl -s -X POST "${BACKEND3_URL}/chaos/errors?duration=30s&rate=100" | jq .
echo ""

echo "Now Overwatch's own health check will see the failures."
echo "The agent's prediction was correct!"
echo ""

sleep 5

echo -e "${YELLOW}After a few seconds, Overwatch confirms:${NC}"
curl -s "http://${OVERWATCH_IP}:8080/api/v1/health/servers" | jq '.servers[] | select(.address | contains("10.50.0.23"))'

press_enter

# ============================================================================
# ACT 5: RECOVERY
# ============================================================================
clear
echo -e "${BOLD}${BLUE}━━━ ACT 5: RECOVERY ━━━${NC}"
echo ""
echo "Let's clear all chaos and watch the backend recover."
echo ""

echo -e "${GREEN}Stopping all chaos:${NC}"
curl -s -X POST "${BACKEND3_URL}/chaos/stop" | jq .
echo ""

echo "Waiting for recovery (15 seconds)..."
sleep 15

echo ""
echo -e "${YELLOW}Checking health:${NC}"
curl -s "http://${OVERWATCH_IP}:8080/api/v1/health/servers" | jq '.servers[] | {address, healthy, draining}'
echo ""

echo -e "${YELLOW}DNS resolution (should return all 3 backends again):${NC}"
for i in 1 2 3; do
    echo -n "Query $i: "
    dig @${OVERWATCH_IP} app.demo.local +short | tr '\n' ' '
    echo ""
    sleep 1
done

echo ""
echo -e "${GREEN}Backend-3 is back in rotation!${NC}"

press_enter

# ============================================================================
# CONCLUSION
# ============================================================================
clear
echo -e "${BOLD}${BLUE}━━━ DEMO COMPLETE ━━━${NC}"
echo ""
cat << 'EOF'
Key Takeaways:

1. PREDICTIVE FROM THE INSIDE
   - Agents monitor local metrics (CPU, memory, error rate)
   - They signal "bleed" BEFORE health checks fail
   - Traffic drains proactively

2. REACTIVE FROM THE OUTSIDE
   - Overwatch validates agent claims externally
   - If agent says "healthy" but checks fail, Overwatch wins
   - Trust but verify

3. ZERO USER IMPACT
   - Traditional GSLB: 30-60 seconds of failed requests
   - OpenGSLB: Traffic drained before crash

EOF

echo -e "${CYAN}Useful commands for further exploration:${NC}"
echo "  ./chaos.sh cpu 60s 90    # Trigger CPU spike"
echo "  ./chaos.sh memory 30s 1000  # Memory pressure"
echo "  ./chaos.sh errors 30s 50    # 50% error rate"
echo "  ./chaos.sh stop           # Clear all chaos"
echo "  ./query-dns.sh            # Watch DNS responses"
echo ""
echo "  Grafana: http://localhost:3000"
echo "  Prometheus: http://localhost:9092"
echo "  Overwatch API: http://localhost:8080/api/v1/health/servers"
echo ""
