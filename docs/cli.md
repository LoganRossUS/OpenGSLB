# OpenGSLB CLI Reference

The `opengslb-cli` is a command-line tool for managing and debugging OpenGSLB Overwatch. It provides operational commands for viewing status, managing backends, handling overrides, and more.

## Installation

### From Source

```bash
# Build the CLI
make build-cli

# The binary will be at ./opengslb-cli
./opengslb-cli --help
```

### From Release

Download the pre-built binary from the releases page and add it to your PATH.

## Global Flags

All commands support these global flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--api` | Overwatch API endpoint | `http://localhost:9090` |
| `--timeout` | API request timeout in seconds | `10` |
| `--json` | Output in JSON format | `false` |
| `-h, --help` | Show help | |
| `-v, --version` | Show version | |

You can also set the API endpoint via environment variable:

```bash
export OPENGSLB_API=http://overwatch:9090
```

## Commands

### status

Show overall Overwatch status.

```bash
opengslb-cli status
```

**Example Output:**

```
OpenGSLB Overwatch Status: Healthy
  Mode:      overwatch
  Uptime:    3d 4h 12m
  Agents:    5 connected
  Backends:  12 total, 11 healthy, 0 unhealthy, 1 stale
  Domains:   5 configured
  DNSSEC:    Enabled (zones: [gslb.example.com])
```

**JSON Output:**

```bash
opengslb-cli status --json
```

### servers

List backend servers with health status.

```bash
opengslb-cli servers
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--service` | Filter by service name |
| `--region` | Filter by region |
| `--status` | Filter by status (healthy, unhealthy, stale) |

**Example Output:**

```
SERVICE    ADDRESS           REGION      STATUS     LATENCY   AUTHORITY
myapp      10.0.1.10:8080    us-east-1   healthy    45ms      overwatch
myapp      10.0.1.11:8080    us-east-1   healthy    52ms      agent
myapp      10.0.2.10:8080    us-west-2   unhealthy  -         overwatch (veto)
otherapp   10.0.3.10:9090    eu-west-1   healthy    120ms     overwatch
```

**Filter Examples:**

```bash
# Show only healthy servers
opengslb-cli servers --status healthy

# Show servers for a specific service
opengslb-cli servers --service myapp

# Show servers in a specific region
opengslb-cli servers --region us-east-1

# Combine filters
opengslb-cli servers --service myapp --status unhealthy
```

### domains

List configured domains. This reads from the local configuration file.

```bash
opengslb-cli domains --config /etc/opengslb/overwatch.yaml
```

**Flags:**

| Flag | Description |
|------|-------------|
| `-c, --config` | Path to configuration file |

**Example Output:**

```
DOMAIN                  ALGORITHM     REGIONS              TTL
global.example.com      geolocation   us-east-1, eu-west-1 30
api.example.com         latency       us-east-1, us-west-2 15
internal.corp           failover      us-east-1, dr-site   60
```

### overrides

Manage health overrides for backend servers.

#### overrides list

List all active overrides.

```bash
opengslb-cli overrides list
```

**Example Output:**

```
SERVICE   ADDRESS          HEALTHY   REASON                 CREATED
myapp     10.0.1.10:8080   false     Maintenance window     2025-12-11T10:00:00Z
```

#### overrides set

Set a health override for a backend.

```bash
opengslb-cli overrides set <service> <address> --healthy=<bool> --reason="<reason>"
```

**Examples:**

```bash
# Mark server as unhealthy for maintenance
opengslb-cli overrides set myapp 10.0.1.10:8080 --healthy=false --reason="Maintenance window"

# Mark server as healthy (force)
opengslb-cli overrides set myapp 10.0.1.10:8080 --healthy=true --reason="Restored after maintenance"
```

#### overrides clear

Remove a health override.

```bash
opengslb-cli overrides clear <service> <address>
```

**Example:**

```bash
opengslb-cli overrides clear myapp 10.0.1.10:8080
```

### geo

Manage geolocation custom CIDR mappings.

#### geo mappings

List all custom CIDR-to-region mappings.

```bash
opengslb-cli geo mappings
```

**Example Output:**

```
CIDR              REGION        SOURCE   COMMENT
10.1.0.0/16       us-chicago    config   KY office - route to Chicago DC
10.2.0.0/16       us-dallas     config   TX office - route to Dallas DC
10.4.0.0/16       us-dallas     api      New Denver office - route to Dallas
172.16.0.0/12     us-east-1     config   VPN users default to us-east
```

#### geo add

Add a custom CIDR-to-region mapping.

```bash
opengslb-cli geo add <cidr> <region> [--comment="<comment>"]
```

**Example:**

```bash
opengslb-cli geo add 10.5.0.0/16 us-west-2 --comment "Seattle office"
```

#### geo remove

Remove a custom CIDR mapping.

```bash
opengslb-cli geo remove <cidr>
```

**Example:**

```bash
opengslb-cli geo remove 10.5.0.0/16
```

#### geo test

Test which region an IP address would route to.

```bash
opengslb-cli geo test <ip>
```

**Example:**

```bash
$ opengslb-cli geo test 10.1.50.100
  IP:          10.1.50.100
  Region:      us-chicago
  Match Type:  custom_mapping
  Matched:     10.1.0.0/16
  Comment:     KY office - route to Chicago DC

$ opengslb-cli geo test 8.8.8.8
  IP:          8.8.8.8
  Region:      us-east-1
  Match Type:  geoip
  Country:     US
  Continent:   NA
```

### config

Configuration management commands.

#### config validate

Validate a configuration file for syntax and semantic errors.

```bash
opengslb-cli config validate --config <path>
```

**Example:**

```bash
$ opengslb-cli config validate --config /etc/opengslb/overwatch.yaml
Configuration valid.
  Mode:          overwatch
  Zones:         [gslb.example.com]
  Regions:       3
  Domains:       5
  Agent tokens:  2
```

**Error Example:**

```bash
$ opengslb-cli config validate --config broken.yaml
Configuration invalid: domains[0]: region "missing-region" not found
Error: configuration invalid
```

### dnssec

DNSSEC management commands.

#### dnssec ds

Show DS records that should be added to the parent zone.

```bash
opengslb-cli dnssec ds [--zone <zone>]
```

**Example:**

```bash
$ opengslb-cli dnssec ds --zone gslb.example.com
Zone: gslb.example.com
DS Record: gslb.example.com. IN DS 12345 13 2 abc123def456...

  Key Tag:     12345
  Algorithm:   13
  Digest Type: 2
  Created At:  2025-12-01T00:00:00Z

Add the DS Record above to your parent zone.
```

#### dnssec status

Show DNSSEC status including key information and sync status.

```bash
opengslb-cli dnssec status
```

### completion

Generate shell completion scripts.

```bash
# Bash
opengslb-cli completion bash > /etc/bash_completion.d/opengslb-cli

# Zsh
opengslb-cli completion zsh > "${fpath[1]}/_opengslb-cli"

# Fish
opengslb-cli completion fish > ~/.config/fish/completions/opengslb-cli.fish

# PowerShell
opengslb-cli completion powershell > opengslb-cli.ps1
```

## JSON Output

All commands support `--json` for machine-readable output:

```bash
# Status as JSON
opengslb-cli status --json

# Servers as JSON
opengslb-cli servers --json

# Geo mappings as JSON
opengslb-cli geo mappings --json
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (API error, validation failure, etc.) |

## Common Use Cases

### Pre-deployment Health Check

```bash
#!/bin/bash
# Check if all servers in a region are healthy before deployment
unhealthy=$(opengslb-cli servers --region us-east-1 --status unhealthy --json | jq 'length')
if [ "$unhealthy" -gt 0 ]; then
    echo "Warning: $unhealthy unhealthy servers in us-east-1"
    exit 1
fi
```

### Maintenance Mode Script

```bash
#!/bin/bash
# Put a server into maintenance mode
SERVER=$1
opengslb-cli overrides set myapp "$SERVER:8080" \
    --healthy=false \
    --reason="Maintenance: deploying new version"

# ... do deployment ...

# Restore server
opengslb-cli overrides clear myapp "$SERVER:8080"
```

### Export Configuration for Backup

```bash
# Export current geo mappings
opengslb-cli geo mappings --json > geo-mappings-backup.json

# Export current overrides
opengslb-cli overrides list --json > overrides-backup.json
```

### Integration with Monitoring

```bash
# Prometheus-style check
healthy=$(opengslb-cli servers --json | jq '[.[] | select(.effective_status == "healthy")] | length')
total=$(opengslb-cli servers --json | jq 'length')
echo "opengslb_backends_healthy $healthy"
echo "opengslb_backends_total $total"
```

## Troubleshooting

### Cannot Connect to API

```
Error: failed to connect to API: dial tcp 127.0.0.1:9090: connect: connection refused
```

Check that:
1. Overwatch is running
2. The API endpoint is correct (`--api` flag)
3. Network connectivity exists
4. API is enabled in Overwatch configuration

### API Access Denied

```
Error: API error (403): Forbidden
```

Check that:
1. Your IP is in the `allowed_networks` configuration
2. If behind a proxy, `trust_proxy_headers` is enabled

### Configuration Validation Errors

The CLI provides detailed error messages including file and line information:

```
Configuration invalid: regions[2].servers[0].port must be between 1 and 65535
```

Fix the indicated issue and re-run validation.
