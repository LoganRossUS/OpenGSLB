#!/bin/bash
# set-latency.sh - Set network latency on webapp containers using tc
#
# Usage: ./set-latency.sh <container> <latency_ms>
# Example: ./set-latency.sh webapp2 100
#
# This script uses Linux traffic control (tc) with netem to add artificial
# network delay to container traffic, simulating different network conditions.

CONTAINER=$1
LATENCY=$2

show_current_latencies() {
    echo "Current latencies:"
    for c in webapp1 webapp2 webapp3; do
        delay=$(docker exec "$c" tc qdisc show dev eth0 2>/dev/null | grep -oP 'delay \K[0-9.]+ms' || echo "0ms")
        echo "  $c: $delay"
    done
}

if [ -z "$CONTAINER" ] || [ -z "$LATENCY" ]; then
    echo "Usage: $0 <container> <latency_ms>"
    echo "Example: $0 webapp2 100"
    echo ""
    echo "Sets network latency on a webapp container using tc netem."
    echo "Use latency_ms=0 to remove delay."
    echo ""
    show_current_latencies
    exit 1
fi

# Check if container exists
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
    echo "Error: Container '$CONTAINER' not found or not running"
    echo ""
    show_current_latencies
    exit 1
fi

# Check if qdisc exists
if docker exec "$CONTAINER" tc qdisc show dev eth0 | grep -q "netem"; then
    # Modify existing qdisc
    if [ "$LATENCY" = "0" ]; then
        docker exec "$CONTAINER" tc qdisc del dev eth0 root 2>/dev/null || true
        echo "Removed latency from $CONTAINER"
    else
        docker exec "$CONTAINER" tc qdisc change dev eth0 root netem delay "${LATENCY}ms"
        echo "Changed $CONTAINER latency to ${LATENCY}ms"
    fi
else
    # Add new qdisc
    if [ "$LATENCY" != "0" ]; then
        docker exec "$CONTAINER" tc qdisc add dev eth0 root netem delay "${LATENCY}ms"
        echo "Set $CONTAINER latency to ${LATENCY}ms"
    else
        echo "$CONTAINER already has no latency set"
    fi
fi
