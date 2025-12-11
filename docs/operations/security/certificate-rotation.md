# Agent Certificate Rotation Guide

This document describes procedures for rotating agent certificates in OpenGSLB.

## Overview

OpenGSLB uses Trust On First Use (TOFU) authentication for agents:

1. Agent generates self-signed certificate on first start
2. Agent presents service token + certificate to Overwatch
3. Overwatch pins the certificate fingerprint
4. Subsequent connections authenticated by certificate

Certificate rotation is needed for:
- Certificate expiration
- Compromised certificate
- Security policy compliance
- Key algorithm upgrades

## Certificate Lifecycle

| Event | Action |
|-------|--------|
| Agent first start | Certificate generated, fingerprint pinned |
| Normal operation | Certificate used for authentication |
| Certificate expiring | Proactive rotation needed |
| Certificate expired | Agent cannot connect, immediate rotation |
| Certificate compromised | Emergency revocation and rotation |

## Checking Certificate Status

### View All Agent Certificates

```bash
opengslb-cli servers --api http://localhost:9090
# Or
curl http://localhost:9090/api/v1/overwatch/agents | jq '.agents[]'
```

### Check Expiring Certificates

```bash
# Certificates expiring within 30 days
curl http://localhost:9090/api/v1/overwatch/agents/expiring?threshold_days=30 | jq .

# Certificates expiring within 90 days
curl http://localhost:9090/api/v1/overwatch/agents/expiring?threshold_days=90 | jq .
```

### Check Specific Agent Certificate

On the agent server:

```bash
# View certificate details
openssl x509 -in /var/lib/opengslb/agent.crt -noout -text

# Check expiration date
openssl x509 -in /var/lib/opengslb/agent.crt -noout -dates

# Get fingerprint
openssl x509 -in /var/lib/opengslb/agent.crt -noout -fingerprint -sha256
```

## Proactive Certificate Rotation

Use this procedure for scheduled rotation before expiration.

### Step 1: Identify Agent to Rotate

```bash
# List expiring certificates
curl http://localhost:9090/api/v1/overwatch/agents/expiring?threshold_days=60 | jq '.expiring[] | {agent_id, expires_in_hours}'
```

### Step 2: Prepare Agent for Rotation

On the agent server:

```bash
# Note current certificate fingerprint
openssl x509 -in /var/lib/opengslb/agent.crt -noout -fingerprint -sha256
```

### Step 3: Delete Old Certificate Pin on Overwatch

```bash
# Delete the agent's certificate pin
curl -X DELETE http://overwatch:9090/api/v1/overwatch/agents/AGENT_ID

# Verify deletion
curl http://overwatch:9090/api/v1/overwatch/agents/AGENT_ID
# Should return 404
```

### Step 4: Regenerate Certificate on Agent

```bash
# Stop agent
sudo systemctl stop opengslb-agent

# Remove old certificate and key
sudo rm /var/lib/opengslb/agent.crt /var/lib/opengslb/agent.key

# Start agent (generates new certificate)
sudo systemctl start opengslb-agent

# Wait for connection
sleep 10
```

### Step 5: Verify New Certificate

On agent:
```bash
openssl x509 -in /var/lib/opengslb/agent.crt -noout -dates
# Should show new validity period
```

On Overwatch:
```bash
# Verify agent re-registered
curl http://overwatch:9090/api/v1/overwatch/agents | jq '.agents[] | select(.agent_id == "AGENT_ID")'
```

### Step 6: Verify Backend Health

```bash
opengslb-cli servers --api http://overwatch:9090 | grep AGENT_SERVICE
# Should show healthy
```

## Bulk Certificate Rotation

For rotating multiple agents:

```bash
#!/bin/bash
# bulk-cert-rotation.sh

OVERWATCH="overwatch-1.internal:9090"
AGENTS="agent-1 agent-2 agent-3"

for agent_host in $AGENTS; do
    echo "=== Rotating certificate for $agent_host ==="

    # Get agent ID from hostname/service
    AGENT_ID=$(ssh $agent_host "hostname")

    # Delete old pin on Overwatch
    curl -X DELETE http://${OVERWATCH}/api/v1/overwatch/agents/${AGENT_ID}

    # Regenerate certificate on agent
    ssh $agent_host << 'ROTATE'
        sudo systemctl stop opengslb-agent
        sudo rm -f /var/lib/opengslb/agent.crt /var/lib/opengslb/agent.key
        sudo systemctl start opengslb-agent
ROTATE

    # Wait for re-registration
    sleep 15

    # Verify
    curl -s http://${OVERWATCH}/api/v1/overwatch/agents | jq ".agents[] | select(.agent_id == \"${AGENT_ID}\")"

    echo "=== $agent_host rotation complete ==="
done
```

## Emergency Certificate Revocation

If an agent certificate is compromised:

### Step 1: Revoke Certificate Immediately

```bash
# Revoke on Overwatch
curl -X POST http://overwatch:9090/api/v1/overwatch/agents/AGENT_ID/revoke \
    -H "Content-Type: application/json" \
    -d '{"reason": "Certificate compromised - security incident"}'
```

This immediately prevents the agent from authenticating.

### Step 2: Mark Associated Backends Unhealthy

```bash
# Set override to prevent traffic to compromised backends
opengslb-cli overrides set SERVICE_NAME BACKEND_ADDRESS \
    --healthy=false \
    --reason="Agent certificate compromised" \
    --api http://overwatch:9090
```

### Step 3: Investigate and Remediate

- Determine scope of compromise
- Secure the agent server
- Rotate service token if needed

### Step 4: Issue New Certificate

After remediation:

```bash
# Delete revoked certificate entry
curl -X DELETE http://overwatch:9090/api/v1/overwatch/agents/AGENT_ID

# On agent server, regenerate
sudo systemctl stop opengslb-agent
sudo rm /var/lib/opengslb/agent.crt /var/lib/opengslb/agent.key
sudo systemctl start opengslb-agent
```

### Step 5: Clear Override

```bash
opengslb-cli overrides clear SERVICE_NAME BACKEND_ADDRESS --api http://overwatch:9090
```

## Service Token Rotation

If the service token is compromised, rotate it:

### Step 1: Generate New Token

```bash
NEW_TOKEN=$(openssl rand -base64 32)
echo "New token: $NEW_TOKEN"
```

### Step 2: Update Overwatch Configuration

```yaml
# In overwatch.yaml
agent_tokens:
  myservice: "NEW_TOKEN_HERE"
```

```bash
sudo systemctl reload opengslb-overwatch
```

### Step 3: Delete Existing Certificate Pins for Service

```bash
# List agents for service
curl http://overwatch:9090/api/v1/overwatch/backends?service=myservice | jq '.backends[].agent_id'

# Delete each agent's pin
curl -X DELETE http://overwatch:9090/api/v1/overwatch/agents/AGENT_ID
```

### Step 4: Update Agent Configurations

On each agent:

```yaml
# In agent.yaml
agent:
  identity:
    service_token: "NEW_TOKEN_HERE"
```

```bash
# Restart agent (will regenerate cert and re-register)
sudo systemctl stop opengslb-agent
sudo rm /var/lib/opengslb/agent.crt /var/lib/opengslb/agent.key
sudo systemctl restart opengslb-agent
```

## Gossip Encryption Key Rotation

The gossip encryption key is separate from certificates but may need rotation:

### Step 1: Generate New Key

```bash
NEW_GOSSIP_KEY=$(openssl rand -base64 32)
echo "New gossip key: $NEW_GOSSIP_KEY"
```

### Step 2: Update All Overwatches

```yaml
# In each overwatch.yaml
gossip:
  encryption_key: "NEW_KEY_HERE"
```

```bash
# Rolling restart
for ow in overwatch-{1,2,3}; do
    ssh $ow "sudo sed -i 's/OLD_KEY/NEW_KEY/' /etc/opengslb/overwatch.yaml"
    ssh $ow "sudo systemctl restart opengslb-overwatch"
    sleep 30
done
```

### Step 3: Update All Agents

```bash
# Update each agent config
for agent in agent-{1,2,3,4,5}; do
    ssh $agent "sudo sed -i 's/OLD_KEY/NEW_KEY/' /etc/opengslb/agent.yaml"
    ssh $agent "sudo systemctl restart opengslb-agent"
done
```

**Note**: During key transition, agents with old key cannot communicate with Overwatches with new key. Plan for brief disconnection.

## Monitoring Certificate Expiration

### Prometheus Alert

```yaml
- alert: AgentCertificateExpiringSoon
  expr: opengslb_agent_certificate_expiry_days < 30
  for: 1d
  labels:
    severity: warning
  annotations:
    summary: "Agent certificate expiring within 30 days"

- alert: AgentCertificateExpired
  expr: opengslb_agent_certificate_expiry_days < 0
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Agent certificate has expired"
```

### Monitoring Script

```bash
#!/bin/bash
# check-cert-expiry.sh

THRESHOLD_DAYS=30
OVERWATCH="http://localhost:9090"

expiring=$(curl -s "${OVERWATCH}/api/v1/overwatch/agents/expiring?threshold_days=${THRESHOLD_DAYS}" | jq '.count')

if [ "$expiring" -gt 0 ]; then
    echo "WARNING: $expiring agent certificates expiring within ${THRESHOLD_DAYS} days"
    curl -s "${OVERWATCH}/api/v1/overwatch/agents/expiring?threshold_days=${THRESHOLD_DAYS}" | \
        jq '.expiring[] | "\(.agent_id): expires in \(.expires_in_hours) hours"'
    exit 1
fi

echo "OK: No certificates expiring within ${THRESHOLD_DAYS} days"
exit 0
```

## Certificate Validity Period

Default certificate validity is 1 year. This is hardcoded in the agent but can be changed by modifying source and rebuilding.

### Recommended Schedule

| Environment | Validity | Rotation |
|-------------|----------|----------|
| Development | 1 year | As needed |
| Production | 1 year | 60 days before expiry |
| High-security | 90 days | 30 days before expiry |

## Related Documentation

- [Security Hardening](./hardening.md)
- [DNSSEC Key Rotation](./key-rotation.md)
- [Agent Disconnection](../incident-response/scenarios/agent-disconnect.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
