#!/bin/bash
#
# cleanup.sh - Stop and clean up the OpenGSLB Demo 5 environment

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="$(dirname "$SCRIPT_DIR")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

cd "$DEMO_DIR"

echo -e "${YELLOW}Stopping Demo 5 containers...${NC}"
docker-compose down -v --remove-orphans

echo -e "${YELLOW}Removing built images...${NC}"
docker-compose down --rmi local 2>/dev/null || true

echo -e "${GREEN}Cleanup complete.${NC}"
