#!/bin/sh
set -e

# Copy config with secure permissions (volume mounts preserve host perms)
# The config is mounted to /tmp/agent-config.yaml, we copy it with chmod 600
if [ -f /tmp/agent-config.yaml ]; then
    cp /tmp/agent-config.yaml /etc/opengslb/config.yaml
    chmod 600 /etc/opengslb/config.yaml
    echo "Config copied with secure permissions"
fi

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
