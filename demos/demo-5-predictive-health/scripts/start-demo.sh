#!/bin/bash
#
# start-demo.sh - Start the OpenGSLB Demo 5 environment
#
# This script builds the OpenGSLB binary and starts all containers.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$(dirname "$DEMO_DIR")")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

cd "$DEMO_DIR"

echo -e "${BLUE}━━━ OpenGSLB Demo 5: Predictive Health Detection ━━━${NC}"
echo ""

# Step 1: Build OpenGSLB binary
echo -e "${YELLOW}Step 1: Building OpenGSLB binary...${NC}"
mkdir -p bin
cd "$PROJECT_ROOT"
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o "$DEMO_DIR/bin/opengslb" ./cmd/opengslb
cd "$DEMO_DIR"
echo -e "${GREEN}Binary built successfully.${NC}"
echo ""

# Step 2: Build containers
echo -e "${YELLOW}Step 2: Building Docker images...${NC}"
docker-compose build --quiet
echo -e "${GREEN}Images built successfully.${NC}"
echo ""

# Step 3: Start containers
echo -e "${YELLOW}Step 3: Starting containers...${NC}"
docker-compose up -d
echo -e "${GREEN}Containers started.${NC}"
echo ""

# Step 4: Wait for services to be healthy
echo -e "${YELLOW}Step 4: Waiting for services to stabilize...${NC}"
sleep 10

# Check overwatch health
echo -n "  Overwatch: "
for i in {1..30}; do
    if curl -s "http://localhost:8080/api/v1/health/servers" > /dev/null 2>&1; then
        echo -e "${GREEN}ready${NC}"
        break
    fi
    sleep 1
    echo -n "."
done

# Check backends
echo -n "  Backends: "
for i in {1..30}; do
    COUNT=$(curl -s "http://localhost:8080/api/v1/health/servers" 2>/dev/null | jq '.servers | length' 2>/dev/null || echo 0)
    if [ "$COUNT" = "3" ]; then
        echo -e "${GREEN}all 3 registered${NC}"
        break
    fi
    sleep 1
    echo -n "."
done

# Check Grafana
echo -n "  Grafana: "
for i in {1..30}; do
    if curl -s "http://localhost:3000/api/health" > /dev/null 2>&1; then
        echo -e "${GREEN}ready${NC}"
        break
    fi
    sleep 1
    echo -n "."
done

echo ""

# Step 5: Show access info
echo -e "${BLUE}━━━ Demo Environment Ready ━━━${NC}"
echo ""
echo "Access points:"
echo "  Grafana Dashboard:  http://localhost:3000"
echo "  Prometheus:         http://localhost:9092"
echo "  Overwatch API:      http://localhost:8080/api/v1/health/servers"
echo "  DNS (via dig):      dig @localhost -p 5353 app.demo.local +short"
echo ""
echo "Client SSH access:"
echo "  ssh root@localhost -p 2222   (password: demo)"
echo ""
echo "Quick chaos commands (from host):"
echo "  curl -X POST 'http://localhost:8083/chaos/cpu?duration=60s&intensity=85'"
echo "  curl -X POST 'http://localhost:8083/chaos/stop'"
echo ""
echo -e "${GREEN}Run the guided demo: ssh to client and run ./demo.sh${NC}"
echo ""
