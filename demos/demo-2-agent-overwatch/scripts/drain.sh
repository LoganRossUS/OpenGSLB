#!/bin/bash
# Drain helper script for OpenGSLB Demo 2
#
# Usage: ./drain.sh <container> [on|off]
# Example: ./drain.sh webapp2 on
#
# This creates/removes /tmp/drain on the specified container.
# When drain is ON:
#   - nginx's /health endpoint returns 503 (unhealthy)
#   - Overwatch's external validation sees the failure
#   - Backend is removed from DNS rotation
#   - BUT nginx still serves traffic on / (demonstrates graceful drain!)

CONTAINER=${1:?Usage: ./drain.sh <container> [on|off]}
ACTION=${2:-on}

if [ "$ACTION" = "on" ]; then
    # Create drain file - nginx /health will return 503
    docker exec $CONTAINER touch /tmp/drain
    echo ""
    echo "  DRAIN ENABLED on $CONTAINER"
    echo "  ================================"
    echo "  - /health now returns 503 (unhealthy)"
    echo "  - Overwatch will detect failure via external validation"
    echo "  - Backend will be removed from DNS (~5-10s)"
    echo "  - Main page (/) STILL WORKS - try: curl http://$CONTAINER/"
    echo ""
    echo "  This demonstrates PROACTIVE health signaling!"
    echo ""
else
    # Remove drain file - nginx /health returns 200 again
    docker exec $CONTAINER rm -f /tmp/drain
    echo ""
    echo "  DRAIN DISABLED on $CONTAINER"
    echo "  ================================"
    echo "  - /health now returns 200 (healthy)"
    echo "  - Overwatch will see recovery via external validation"
    echo "  - Backend will return to DNS rotation (~5-10s)"
    echo ""
fi
