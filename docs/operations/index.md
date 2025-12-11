# Operations Documentation

This section contains operational runbooks, procedures, and guides for deploying and maintaining OpenGSLB in production environments.

## Quick Links

| Need to... | Go to... |
|------------|----------|
| Deploy an agent | [Agent Deployment](deployment/agent.md) |
| Deploy Overwatch | [Overwatch Deployment](deployment/overwatch.md) |
| Set up HA | [HA Setup Guide](deployment/ha-setup.md) |
| Use Docker | [Docker Deployment](deployment/docker.md) |
| Upgrade OpenGSLB | [Upgrade Procedures](maintenance/upgrades.md) |
| Respond to incident | [Incident Playbook](incident-response/playbook.md) |
| Secure deployment | [Security Hardening](security/hardening.md) |
| Plan capacity | [Capacity Planning](capacity/planning.md) |

## Documentation Structure

### Deployment Guides

Step-by-step instructions for initial deployment:

- **[Agent Deployment](deployment/agent.md)** - Deploy agents on application servers
- **[Overwatch Deployment](deployment/overwatch.md)** - Deploy DNS-serving Overwatch nodes
- **[HA Setup Guide](deployment/ha-setup.md)** - Multi-Overwatch high availability
- **[Docker Deployment](deployment/docker.md)** - Container-based deployments

### Maintenance Procedures

Day-to-day operations and lifecycle management:

- **[Upgrade Procedures](maintenance/upgrades.md)** - Upgrading to new versions
- **[Rollback Procedures](maintenance/rollback.md)** - Rolling back failed upgrades
- **[Backup and Restore](maintenance/backup-restore.md)** - Data protection
- **[GeoIP Updates](maintenance/geoip-updates.md)** - Maintaining GeoIP database

### Incident Response

Troubleshooting and incident management:

- **[Incident Response Playbook](incident-response/playbook.md)** - General response framework
- **Specific Scenarios:**
  - [All Backends Unhealthy](incident-response/scenarios/all-unhealthy.md)
  - [Agent Disconnection](incident-response/scenarios/agent-disconnect.md)
  - [Overwatch Down](incident-response/scenarios/overwatch-down.md)
  - [DNSSEC Issues](incident-response/scenarios/dnssec-issues.md)

### Security

Security configuration and procedures:

- **[Security Hardening](security/hardening.md)** - Comprehensive security checklist
- **[DNSSEC Key Rotation](security/key-rotation.md)** - Rotating DNSSEC keys
- **[Certificate Rotation](security/certificate-rotation.md)** - Agent certificate management

### Capacity Planning

Sizing and performance guidance:

- **[Capacity Planning](capacity/planning.md)** - Sizing guidelines
- **[Benchmarks](capacity/benchmarks.md)** - Performance benchmarks

## Runbook Conventions

All runbooks in this documentation follow these conventions:

### Severity Levels

| Level | Description | Response Time |
|-------|-------------|---------------|
| SEV1 | Complete service outage | Immediate |
| SEV2 | Partial outage or degraded | 15 minutes |
| SEV3 | Minor issues, limited impact | 1 hour |
| SEV4 | Informational | Next business day |

### Command Notation

```bash
# Commands with sudo require root privileges
sudo systemctl restart opengslb-overwatch

# Variables in UPPERCASE should be replaced
curl http://OVERWATCH_IP:9090/api/v1/ready

# Optional parameters in [brackets]
opengslb-cli status [--api http://localhost:9090]
```

### Verification Steps

Each procedure includes verification steps marked with checkboxes:

- [ ] Step completed successfully
- [ ] Metric within expected range
- [ ] No errors in logs

## Prerequisites

Before using these runbooks, ensure you have:

1. **Access credentials**
   - SSH access to OpenGSLB servers
   - API endpoint access
   - Secrets (gossip key, tokens)

2. **Tools installed**
   - `opengslb-cli` - CLI management tool
   - `dig` - DNS query tool
   - `curl` - HTTP client
   - `jq` - JSON processor

3. **Monitoring access**
   - Prometheus/Grafana dashboards
   - Alerting system access
   - Log aggregation system

## Getting Help

- **Documentation Issues**: File issues at [GitHub](https://github.com/loganrossus/OpenGSLB/issues)
- **Community Support**: [Discussions](https://github.com/loganrossus/OpenGSLB/discussions)
- **Security Issues**: security@opengslb.org

## Contributing

Found an issue with these runbooks? Contributions welcome:

1. Fork the repository
2. Edit files in `docs/operations/`
3. Submit a pull request

---

**Document Version**: 1.0
**Last Updated**: December 2025
