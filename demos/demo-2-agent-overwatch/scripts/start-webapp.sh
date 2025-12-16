#!/bin/sh
set -e

echo "Starting nginx..."
nginx

echo "Starting OpenGSLB agent..."

# Drain file check loop
# If /tmp/drain exists, we kill the agent to make it report unhealthy
# This simulates the drain_file feature for the demo

while true; do
    if [ -f /tmp/drain ]; then
        # Drain mode active - kill agent if running
        if pgrep -f opengslb > /dev/null 2>&1; then
            echo "Drain file detected - stopping agent to trigger unhealthy status"
            pkill -f opengslb || true
        fi
        sleep 1
    else
        # Normal mode - ensure agent is running
        if ! pgrep -f opengslb > /dev/null 2>&1; then
            echo "Starting OpenGSLB agent..."
            /opt/opengslb/opengslb --config /etc/opengslb/config.yaml &
        fi
        sleep 2
    fi
done
