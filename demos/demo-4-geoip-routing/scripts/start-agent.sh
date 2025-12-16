#!/bin/sh
# start-agent.sh
# Startup script for OpenGSLB agent in Demo 4

set -e

# Copy config with secure permissions (volume mounts preserve host perms)
# The config is mounted to /tmp/agent-config.yaml, we copy it with chmod 600
if [ -f /tmp/agent-config.yaml ]; then
    cp /tmp/agent-config.yaml /etc/opengslb/config.yaml
    chmod 600 /etc/opengslb/config.yaml
    echo "Config copied with secure permissions"
fi

echo "Starting OpenGSLB agent..."
# Agent runs continuously, reporting health to Overwatch via gossip
exec /opt/opengslb/opengslb --config /etc/opengslb/config.yaml
