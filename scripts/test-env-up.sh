#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "Starting integration test environment..."
docker compose -f docker-compose.test.yml up -d

echo "Waiting for services to be healthy..."
sleep 5

# Verify services are up
docker compose -f docker-compose.test.yml ps

echo "Test environment ready."
