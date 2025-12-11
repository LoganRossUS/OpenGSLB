# Incident Response Playbook

This document provides a general framework for responding to OpenGSLB incidents.

## Incident Severity Levels

| Level | Description | Response Time | Examples |
|-------|-------------|---------------|----------|
| SEV1 | Complete service outage | Immediate | All Overwatches down, DNS not resolving |
| SEV2 | Partial outage or degraded service | 15 minutes | Single region down, high error rate |
| SEV3 | Minor issues, limited impact | 1 hour | Single backend unhealthy, config warning |
| SEV4 | Informational, no user impact | Next business day | Metric anomaly, minor alert |

## General Response Framework

### Phase 1: Detection and Assessment (0-5 minutes)

1. **Acknowledge the alert**
   - Note alert time and source
   - Assign incident owner

2. **Initial assessment**
   - What is the user impact?
   - How many users/services affected?
   - Is this a known issue?

3. **Gather basic information**
   ```bash
   # Quick health check
   opengslb-cli status --api http://overwatch:9090

   # Check all Overwatches
   for host in overwatch-{1,2,3}; do
       echo "=== $host ==="
       curl -s http://${host}:9090/api/v1/ready
   done

   # Test DNS
   dig @overwatch-1 myapp.gslb.example.com
   ```

### Phase 2: Triage and Communication (5-15 minutes)

1. **Determine severity level**
   - Use criteria above
   - Escalate if SEV1/SEV2

2. **Notify stakeholders**
   - Internal status channel
   - On-call escalation if needed
   - Customer communication for SEV1/SEV2

3. **Document initial findings**
   - Symptoms observed
   - Timeline of events
   - Initial hypothesis

### Phase 3: Mitigation (15-60 minutes)

1. **Apply temporary fix if available**
   - Traffic redirect
   - Override unhealthy backends
   - Rollback if recent change caused issue

2. **Monitor mitigation effectiveness**
   ```bash
   # Watch key metrics
   watch -n5 'opengslb-cli servers --api http://overwatch:9090'
   ```

3. **Continue investigation**
   - Root cause analysis
   - Collect logs and metrics

### Phase 4: Resolution (Variable)

1. **Implement permanent fix**
   - Configuration change
   - Code fix deployment
   - Infrastructure repair

2. **Verify fix**
   - Functional testing
   - Monitor for recurrence

3. **Stand down**
   - Clear incident status
   - Notify stakeholders

### Phase 5: Post-Incident (Within 48 hours)

1. **Conduct post-mortem**
   - What happened?
   - Why did it happen?
   - How was it detected?
   - How was it resolved?
   - How do we prevent it?

2. **Create action items**
   - Immediate fixes
   - Long-term improvements
   - Monitoring enhancements

3. **Update documentation**
   - Runbooks
   - Alert thresholds
   - Architecture documentation

## Quick Reference Commands

### Health Assessment

```bash
# Overall status
opengslb-cli status --api http://localhost:9090

# Backend health
opengslb-cli servers --api http://localhost:9090

# Domain configuration
opengslb-cli domains --api http://localhost:9090

# Check overrides
opengslb-cli overrides list --api http://localhost:9090
```

### DNS Testing

```bash
# Query all Overwatches
for ow in 10.0.1.{53,54,55}; do
    echo "=== $ow ==="
    dig @$ow myapp.gslb.example.com +short
done

# DNSSEC validation
dig @overwatch myapp.gslb.example.com +dnssec

# Query with specific client IP (for geo testing)
dig @overwatch myapp.gslb.example.com +subnet=8.8.8.8/32
```

### Log Analysis

```bash
# Recent errors
journalctl -u opengslb-overwatch -p err --since "1 hour ago"

# Follow logs
journalctl -u opengslb-overwatch -f

# Search for specific patterns
journalctl -u opengslb-overwatch | grep -E "(error|fail|timeout)"
```

### Emergency Actions

```bash
# Mark backend unhealthy (immediate traffic diversion)
opengslb-cli overrides set myapp 10.0.1.10:8080 \
    --healthy=false \
    --reason="Emergency override during incident" \
    --api http://localhost:9090

# Clear override (restore traffic)
opengslb-cli overrides clear myapp 10.0.1.10:8080 \
    --api http://localhost:9090

# Force validation
curl -X POST http://localhost:9090/api/v1/overwatch/validate

# Reload configuration
sudo systemctl reload opengslb-overwatch
```

### Metrics Queries

```promql
# Current query rate
sum(rate(opengslb_dns_queries_total[5m]))

# Error rate
sum(rate(opengslb_dns_queries_total{status!="success"}[5m])) / sum(rate(opengslb_dns_queries_total[5m]))

# Healthy backends
opengslb_overwatch_backends_healthy

# Stale agents
opengslb_overwatch_stale_agents
```

## Incident Response Contacts

| Role | Contact | Escalation Path |
|------|---------|-----------------|
| On-Call Engineer | [Your contact info] | Page via PagerDuty |
| Platform Lead | [Contact] | Phone for SEV1 |
| Security | [Contact] | For security incidents |

## Common Scenarios

For detailed procedures on specific incidents, see:

- [All Backends Unhealthy](./scenarios/all-unhealthy.md)
- [Agent Disconnection](./scenarios/agent-disconnect.md)
- [Overwatch Down](./scenarios/overwatch-down.md)
- [DNSSEC Issues](./scenarios/dnssec-issues.md)

## Runbook Template

Use this template for documenting new scenarios:

```markdown
# [Scenario Name]

## Symptoms
- What alerts fire?
- What do users experience?

## Impact
- Severity level
- Affected services/users

## Diagnosis
- Commands to run
- What to look for

## Resolution
1. Step-by-step fix

## Prevention
- How to avoid recurrence

## Related
- Links to relevant docs
```

## Post-Incident Review Template

```markdown
# Incident Review: [Title]

**Date**: YYYY-MM-DD
**Duration**: X hours
**Severity**: SEVN
**Owner**: [Name]

## Summary
Brief description of what happened.

## Timeline
- HH:MM - Alert fired
- HH:MM - Acknowledged
- HH:MM - Root cause identified
- HH:MM - Fix applied
- HH:MM - Incident resolved

## Root Cause
Detailed explanation of why it happened.

## Impact
- Services affected
- Users affected
- Duration

## Resolution
What was done to fix it.

## Action Items
| Item | Owner | Due Date | Status |
|------|-------|----------|--------|
| Fix X | Person | Date | Open |

## Lessons Learned
- What went well?
- What could be improved?
```

## Related Documentation

- [Upgrade Procedures](../maintenance/upgrades.md)
- [Rollback Procedures](../maintenance/rollback.md)
- [Backup and Restore](../maintenance/backup-restore.md)
- [HA Setup Guide](../deployment/ha-setup.md)

---

**Document Version**: 1.0
**Last Updated**: December 2025
