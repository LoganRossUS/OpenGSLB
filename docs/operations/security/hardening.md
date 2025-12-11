# Security Hardening Checklist

This document provides a comprehensive security checklist for hardening OpenGSLB deployments.

## Overview

OpenGSLB is designed with security in mind:
- Mandatory gossip encryption
- TOFU (Trust On First Use) agent authentication
- DNSSEC enabled by default
- API access controls

This checklist ensures you've configured all security features properly.

## Pre-Deployment Checklist

### Secrets Management

- [ ] **Gossip encryption key generated securely**
  ```bash
  openssl rand -base64 32  # Generate 256-bit key
  ```

- [ ] **Service tokens are unique and strong**
  - Minimum 32 characters
  - Generated randomly, not manually created
  - Different token per service

- [ ] **Secrets stored securely**
  - Use secrets manager (Vault, AWS Secrets Manager, etc.)
  - Not stored in version control
  - Environment variables or mounted secrets files

- [ ] **Secret rotation plan documented**
  - Schedule for rotation
  - Procedure documented
  - Tested in staging

### Configuration Security

- [ ] **Configuration file permissions**
  ```bash
  # Overwatch config
  chown root:opengslb /etc/opengslb/overwatch.yaml
  chmod 640 /etc/opengslb/overwatch.yaml

  # Agent config
  chown root:opengslb /etc/opengslb/agent.yaml
  chmod 640 /etc/opengslb/agent.yaml
  ```

- [ ] **Data directory permissions**
  ```bash
  chown opengslb:opengslb /var/lib/opengslb
  chmod 700 /var/lib/opengslb
  ```

- [ ] **No secrets in plain text logs**
  - Log level set appropriately (info or warn for production)
  - Sensitive data redacted

## Network Security

### Firewall Configuration

- [ ] **Overwatch firewall rules**
  ```bash
  # Allow DNS from authorized networks
  iptables -A INPUT -p udp --dport 53 -s TRUSTED_NETWORK -j ACCEPT
  iptables -A INPUT -p tcp --dport 53 -s TRUSTED_NETWORK -j ACCEPT

  # Allow gossip from agents
  iptables -A INPUT -p tcp --dport 7946 -s AGENT_NETWORK -j ACCEPT
  iptables -A INPUT -p udp --dport 7946 -s AGENT_NETWORK -j ACCEPT

  # Allow API from management network only
  iptables -A INPUT -p tcp --dport 9090 -s MGMT_NETWORK -j ACCEPT

  # Allow metrics from monitoring network
  iptables -A INPUT -p tcp --dport 9091 -s MONITORING_NETWORK -j ACCEPT

  # Drop all other traffic to these ports
  iptables -A INPUT -p tcp --dport 53 -j DROP
  # ... etc
  ```

- [ ] **Agent firewall rules**
  ```bash
  # Allow outbound gossip to Overwatches
  iptables -A OUTPUT -p tcp --dport 7946 -d OVERWATCH_NETWORK -j ACCEPT
  iptables -A OUTPUT -p udp --dport 7946 -d OVERWATCH_NETWORK -j ACCEPT

  # Allow metrics from localhost or monitoring
  iptables -A INPUT -p tcp --dport 9100 -s 127.0.0.1 -j ACCEPT
  iptables -A INPUT -p tcp --dport 9100 -s MONITORING_NETWORK -j ACCEPT
  ```

### Network Segmentation

- [ ] **Overwatches in private network**
  - Not directly exposed to internet (use load balancer or internal DNS)

- [ ] **API not publicly accessible**
  - Bind to internal interface only
  - Or use VPN/bastion for access

- [ ] **Metrics endpoint restricted**
  - Only accessible from monitoring infrastructure

### TLS/SSL

- [ ] **DNSSEC enabled**
  ```yaml
  dnssec:
    enabled: true
  ```

- [ ] **HTTPS for API** (if using reverse proxy)
  - Terminate TLS at load balancer
  - Use valid certificates

- [ ] **Gossip encryption enabled** (mandatory)
  ```yaml
  gossip:
    encryption_key: "base64-encoded-32-byte-key"
  ```

## API Security

### Access Control

- [ ] **API restricted to authorized networks**
  ```yaml
  api:
    enabled: true
    address: "127.0.0.1:9090"  # Localhost only
    allowed_networks:
      - 10.0.0.0/8           # Internal network
      - 127.0.0.1/32         # Localhost
    trust_proxy_headers: false
  ```

- [ ] **Don't trust proxy headers in production** (unless behind trusted proxy)
  ```yaml
  api:
    trust_proxy_headers: false  # Prevent IP spoofing
  ```

- [ ] **Audit API access**
  - Enable access logging
  - Monitor for unusual patterns

### Override API

- [ ] **Override API restricted**
  - Only management systems should have access
  - Monitor override changes

- [ ] **Override reasons required**
  - Configure external systems to provide `source` field
  - Review override audit trail

## Authentication

### Agent Authentication

- [ ] **Unique service tokens per application**
  ```yaml
  # In Overwatch config
  agent_tokens:
    webapp: "unique-token-for-webapp"
    api: "different-token-for-api"
    # NOT: all_services: "shared-token"
  ```

- [ ] **TOFU certificates protected**
  - Certificate directory not world-readable
  - Backup certificates securely

- [ ] **Certificate expiration monitored**
  ```bash
  curl http://localhost:9090/api/v1/overwatch/agents/expiring?threshold_days=30
  ```

- [ ] **Revocation procedure documented**
  - How to revoke compromised agent certificates

### Admin Access

- [ ] **CLI access controlled**
  - CLI installed only on admin workstations
  - API endpoint not exposed publicly

- [ ] **SSH access to servers hardened**
  - Key-based authentication only
  - No root login
  - Fail2ban or similar

## Runtime Security

### Process Isolation

- [ ] **Run as non-root user**
  ```bash
  User=opengslb
  Group=opengslb
  ```

- [ ] **Use capabilities instead of root** (for port 53)
  ```bash
  setcap 'cap_net_bind_service=+ep' /usr/local/bin/opengslb
  ```

- [ ] **systemd hardening options enabled**
  ```ini
  [Service]
  NoNewPrivileges=yes
  ProtectSystem=strict
  ProtectHome=yes
  PrivateTmp=yes
  ReadWritePaths=/var/lib/opengslb
  ```

### Resource Limits

- [ ] **File descriptor limits set**
  ```ini
  LimitNOFILE=65536
  ```

- [ ] **Memory limits (containers)**
  ```yaml
  deploy:
    resources:
      limits:
        memory: 1G
  ```

### Container Security (Docker)

- [ ] **Run as non-root in container**
  - OpenGSLB image runs as non-root by default

- [ ] **Read-only root filesystem**
  ```yaml
  read_only: true
  tmpfs:
    - /tmp
  ```

- [ ] **No privileged mode**
  - Never use `--privileged`

- [ ] **Drop all capabilities, add only needed**
  ```yaml
  cap_drop:
    - ALL
  cap_add:
    - NET_BIND_SERVICE
  ```

## Monitoring and Auditing

### Logging

- [ ] **Log level appropriate for production**
  ```yaml
  logging:
    level: info  # Not debug in production
    format: json  # Structured logging for analysis
  ```

- [ ] **Logs shipped to central system**
  - SIEM integration
  - Log retention policy

- [ ] **Sensitive data not logged**
  - Review logs for secrets
  - Encryption keys should never appear

### Metrics

- [ ] **Metrics endpoint secured**
  - Not publicly accessible
  - Authentication if required

- [ ] **Security-relevant metrics monitored**
  ```promql
  # Authentication failures
  opengslb_tofu_authentication_failures_total

  # Gossip decryption failures (possible attack)
  opengslb_gossip_messages_decryption_failures_total

  # Override changes
  opengslb_gossip_override_operations_total
  ```

### Alerting

- [ ] **Security alerts configured**
  ```yaml
  - alert: AuthenticationFailures
    expr: rate(opengslb_tofu_authentication_failures_total[5m]) > 0.1
    labels:
      severity: warning
    annotations:
      summary: "High rate of authentication failures"

  - alert: GossipDecryptionFailures
    expr: rate(opengslb_gossip_messages_decryption_failures_total[5m]) > 0
    labels:
      severity: warning
    annotations:
      summary: "Gossip decryption failures - possible attack or misconfiguration"
  ```

## DNSSEC Security

- [ ] **DNSSEC enabled**
  - Not disabled without documented reason

- [ ] **DS records in parent zone**
  - Chain of trust established

- [ ] **Key rotation schedule**
  - Keys rotated periodically
  - Procedure documented

- [ ] **Key sync between Overwatches**
  - All Overwatches have same keys

## Operational Security

### Change Management

- [ ] **Configuration changes version controlled**
  - Git or similar for /etc/opengslb

- [ ] **Changes tested in staging first**

- [ ] **Rollback procedure ready**
  - Backups available
  - Procedure tested

### Incident Response

- [ ] **Security incident procedure documented**
  - Who to contact
  - Containment steps
  - Evidence preservation

- [ ] **Certificate revocation procedure ready**
  - How to revoke compromised agent
  - How to rotate gossip key

### Backup Security

- [ ] **Backups encrypted**
  - Especially DNSSEC keys

- [ ] **Backup access restricted**
  - Separate credentials
  - Audit access

## Compliance Checklist

### For Sensitive Environments

- [ ] **Encryption at rest** (disk encryption)
- [ ] **Encryption in transit** (TLS everywhere feasible)
- [ ] **Access logging** (who did what when)
- [ ] **Regular security audits**
- [ ] **Penetration testing**
- [ ] **Vulnerability scanning**

## Quick Security Audit Commands

```bash
# Check file permissions
ls -la /etc/opengslb/
ls -la /var/lib/opengslb/

# Check running user
ps aux | grep opengslb

# Check listening ports
ss -tulnp | grep opengslb

# Check API allowed networks
grep -A5 "allowed_networks" /etc/opengslb/overwatch.yaml

# Check DNSSEC status
curl http://localhost:9090/api/v1/dnssec/status | jq .enabled

# Check for expiring certs
curl http://localhost:9090/api/v1/overwatch/agents/expiring?threshold_days=30

# Check active overrides
curl http://localhost:9090/api/v1/overrides

# Check auth failures in logs
journalctl -u opengslb-overwatch | grep -i "auth\|fail\|denied"
```

## Related Documentation

- [Agent Deployment](../deployment/agent.md)
- [Overwatch Deployment](../deployment/overwatch.md)
- [DNSSEC Key Rotation](./key-rotation.md)
- [Certificate Rotation](./certificate-rotation.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
