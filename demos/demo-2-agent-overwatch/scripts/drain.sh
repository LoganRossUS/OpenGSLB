#!/bin/bash
# Drain helper script for OpenGSLB Demo 2
#
# Usage: ./drain.sh <container> [on|off]
# Example: ./drain.sh webapp2 on
#
# This creates/removes /tmp/drain on the specified container.
# When drain is ON, the agent stops reporting and the backend
# is removed from DNS - even though nginx is still running!

CONTAINER=${1:?Usage: ./drain.sh <container> [on|off]}
ACTION=${2:-on}

# Check if we're running inside Docker or from host
if [ -f /.dockerenv ]; then
    # Running inside container - use docker command
    DOCKER_CMD="docker"
else
    # Running from host
    DOCKER_CMD="docker"
fi

if [ "$ACTION" = "on" ]; then
    # Create drain file - agent will stop, backend removed from DNS
    $DOCKER_CMD exec $CONTAINER touch /tmp/drain
    echo ""
    echo "  DRAIN ENABLED on $CONTAINER"
    echo "  ================================"
    echo "  - Agent will stop reporting to Overwatch"
    echo "  - Backend will be removed from DNS rotation"
    echo "  - nginx is STILL RUNNING and serving requests!"
    echo ""
    echo "  This demonstrates PROACTIVE health signaling."
    echo "  The backend removed itself BEFORE any requests failed."
    echo ""
else
    # Remove drain file - agent will restart, backend returns to rotation
    $DOCKER_CMD exec $CONTAINER rm -f /tmp/drain
    echo ""
    echo "  DRAIN DISABLED on $CONTAINER"
    echo "  ================================"
    echo "  - Agent will restart and report healthy"
    echo "  - Backend will return to DNS rotation"
    echo "  - Wait ~5-10 seconds for gossip propagation"
    echo ""
fi
