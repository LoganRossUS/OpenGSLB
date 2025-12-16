#!/bin/bash
#
# chaos.sh - Chaos injection helper for OpenGSLB Demo 5
#
# Usage:
#   ./chaos.sh cpu [duration] [intensity]   - Trigger CPU spike
#   ./chaos.sh memory [duration] [amount]   - Trigger memory pressure
#   ./chaos.sh errors [duration] [rate]     - Trigger error injection
#   ./chaos.sh latency [duration] [ms]      - Trigger latency injection
#   ./chaos.sh stop                         - Stop all chaos
#   ./chaos.sh status                       - View chaos status

BACKEND3_URL="http://10.50.0.23:8080"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

case "$1" in
    cpu)
        DURATION="${2:-60s}"
        INTENSITY="${3:-85}"
        echo -e "${RED}Triggering CPU spike on backend-3...${NC}"
        echo -e "  Duration: ${DURATION}"
        echo -e "  Intensity: ${INTENSITY}%"
        curl -s -X POST "${BACKEND3_URL}/chaos/cpu?duration=${DURATION}&intensity=${INTENSITY}" | jq .
        echo -e "\n${YELLOW}Watch the Grafana dashboard to see the CPU spike and predictive signal!${NC}"
        ;;
    memory)
        DURATION="${2:-60s}"
        AMOUNT="${3:-500}"
        echo -e "${YELLOW}Triggering memory pressure on backend-3...${NC}"
        echo -e "  Duration: ${DURATION}"
        echo -e "  Amount: ${AMOUNT}MB"
        curl -s -X POST "${BACKEND3_URL}/chaos/memory?duration=${DURATION}&amount=${AMOUNT}" | jq .
        echo -e "\n${YELLOW}Watch the Grafana dashboard to see memory pressure!${NC}"
        ;;
    errors)
        DURATION="${2:-60s}"
        RATE="${3:-100}"
        echo -e "${RED}Triggering error injection on backend-3...${NC}"
        echo -e "  Duration: ${DURATION}"
        echo -e "  Error rate: ${RATE}%"
        curl -s -X POST "${BACKEND3_URL}/chaos/errors?duration=${DURATION}&rate=${RATE}" | jq .
        echo -e "\n${YELLOW}Health checks will now fail at ${RATE}% rate!${NC}"
        ;;
    latency)
        DURATION="${2:-60s}"
        LATENCY="${3:-500}"
        echo -e "${BLUE}Triggering latency injection on backend-3...${NC}"
        echo -e "  Duration: ${DURATION}"
        echo -e "  Latency: ${LATENCY}ms"
        curl -s -X POST "${BACKEND3_URL}/chaos/latency?duration=${DURATION}&latency=${LATENCY}" | jq .
        echo -e "\n${YELLOW}All requests to backend-3 will have ${LATENCY}ms added latency!${NC}"
        ;;
    stop)
        echo -e "${GREEN}Stopping all chaos on backend-3...${NC}"
        curl -s -X POST "${BACKEND3_URL}/chaos/stop" | jq .
        echo -e "\n${GREEN}Chaos cleared. Backend should recover shortly.${NC}"
        ;;
    status)
        echo -e "${BLUE}Chaos status on backend-3:${NC}"
        curl -s "${BACKEND3_URL}/chaos/status" | jq .
        ;;
    *)
        echo "OpenGSLB Demo 5: Chaos Injection Helper"
        echo ""
        echo "Usage: $0 <command> [options]"
        echo ""
        echo "Commands:"
        echo "  cpu [duration] [intensity]   Trigger CPU spike (default: 60s, 85%)"
        echo "  memory [duration] [amount]   Trigger memory pressure (default: 60s, 500MB)"
        echo "  errors [duration] [rate]     Trigger error injection (default: 60s, 100%)"
        echo "  latency [duration] [ms]      Trigger latency injection (default: 60s, 500ms)"
        echo "  stop                         Stop all chaos conditions"
        echo "  status                       View current chaos status"
        echo ""
        echo "Examples:"
        echo "  $0 cpu 60s 85       # 85% CPU for 60 seconds"
        echo "  $0 memory 30s 1000  # 1GB memory pressure for 30 seconds"
        echo "  $0 errors 45s 50    # 50% error rate for 45 seconds"
        echo "  $0 stop             # Clear all chaos"
        exit 1
        ;;
esac
