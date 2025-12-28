# ADR-017 Azure Test Environment

## Quick Start (Simplified Deployment)

**NEW: Deployment time reduced from ~15 minutes to ~2 minutes!**

VMs now download pre-built binaries from GitHub Releases instead of building from source.

```bash
cd terraform/

# Copy and configure variables
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your SSH key path and Windows password

# Deploy
terraform init
terraform apply -var="windows_admin_password=YourComplexPassword123!"

# Wait ~2 minutes for cloud-init to complete, then validate:
ssh azureuser@<traffic_eastus_public_ip>
test-cluster
```

### What Gets Deployed

All VMs automatically:
1. Download pre-built OpenGSLB binaries from GitHub Releases
2. Generate configuration with auto-discovered network settings
3. Start the appropriate service (Overwatch or Agent)
4. Run self-tests to verify health

### Validating the Deployment

SSH to the traffic generator and run the validation script:

```bash
ssh azureuser@<traffic_eastus_public_ip>

# Run full cluster validation (verbose output for debugging)
test-cluster

# If all tests pass, generate traffic:
generate-traffic 5 300    # 5 req/s for 5 minutes

# Check learned latency data:
curl http://10.1.1.10:8080/api/v1/overwatch/latency | jq .
```

### Troubleshooting

If validation fails, the output is designed to be pasted into Claude Code for analysis:

```bash
# Get bootstrap logs from any VM
ssh azureuser@<vm_public_ip>
cat /var/log/opengslb-bootstrap.log
```

See [Terraform README](terraform/README.md) for detailed deployment instructions.

## Overview

This document specifies an Azure infrastructure for testing the Passive Latency Learning feature (ADR-017). The goal is to validate that agents can collect real TCP RTT data and that Overwatches can use this data for intelligent routing decisions.

## Test Objectives

1. **Verify RTT Collection**: Agents correctly read TCP connection RTT from the OS
2. **Verify Aggregation**: RTT data is properly aggregated by client subnet
3. **Verify Gossip**: Aggregated data reaches Overwatch nodes
4. **Verify Routing**: Overwatch uses learned latency for routing decisions
5. **Verify Cold Start**: Fallback to geo-routing works when no data exists
6. **Verify Cross-Platform**: Both Linux and Windows agents work

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          AZURE TEST ENVIRONMENT                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  REGION: East US (eastus)                    REGION: West Europe (westeurope)
│  ─────────────────────────                   ─────────────────────────────── 
│                                                                              │
│  ┌─────────────────────────┐                ┌─────────────────────────────┐ │
│  │  Overwatch VM           │                │  Backend VM (Linux)         │ │
│  │  Standard_B2s           │   Gossip/DNS   │  Standard_B2s               │ │
│  │  Ubuntu 22.04           │◄──────────────►│  Ubuntu 22.04               │ │
│  │                         │                │                             │ │
│  │  - Overwatch process    │                │  - nginx (port 80)          │ │
│  │  - DNS on :53           │                │  - OpenGSLB Agent           │ │
│  │  - API on :9090         │                │  - Latency collector        │ │
│  │  - Metrics on :9091     │                │                             │ │
│  └─────────────────────────┘                └─────────────────────────────┘ │
│            │                                             ▲                   │
│            │                                             │                   │
│            │                                    TCP connections              │
│            │                                    (generates RTT data)         │
│            │                                             │                   │
│            ▼                                             │                   │
│  ┌─────────────────────────┐                ┌───────────┴─────────────────┐ │
│  │  Traffic Generator      │                │  Backend VM (Windows)       │ │
│  │  Standard_B2s           │                │  Standard_B2s               │ │
│  │  Ubuntu 22.04           │                │  Windows Server 2022        │ │
│  │                         │                │                             │ │
│  │  - hey/wrk/curl scripts │                │  - IIS (port 80)            │ │
│  │  - Simulates clients    │                │  - OpenGSLB Agent           │ │
│  │  - From East US subnet  │                │  - Latency collector        │ │
│  └─────────────────────────┘                └─────────────────────────────┘ │
│                                                                              │
│                                                                              │
│  REGION: Southeast Asia (southeastasia)                                     │
│  ───────────────────────────────────────                                    │
│                                                                              │
│  ┌─────────────────────────┐                ┌─────────────────────────────┐ │
│  │  Backend VM (Linux)     │                │  Traffic Generator          │ │
│  │  Standard_B2s           │                │  Standard_B2s               │ │
│  │  Ubuntu 22.04           │                │  Ubuntu 22.04               │ │
│  │                         │                │                             │ │
│  │  - nginx (port 80)      │                │  - Simulates APAC clients   │ │
│  │  - OpenGSLB Agent       │                │  - Connects to all backends │ │
│  │  - Latency collector    │                │                             │ │
│  └─────────────────────────┘                └─────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Resource Group Structure

```
rg-opengslb-latency-test/
├── Virtual Networks
│   ├── vnet-eastus (10.1.0.0/16)
│   │   ├── subnet-overwatch (10.1.1.0/24)
│   │   └── subnet-traffic (10.1.2.0/24)
│   ├── vnet-westeurope (10.2.0.0/16)
│   │   └── subnet-backends (10.2.1.0/24)
│   └── vnet-southeastasia (10.3.0.0/16)
│       ├── subnet-backends (10.3.1.0/24)
│       └── subnet-traffic (10.3.2.0/24)
│
├── Virtual Network Peering
│   ├── eastus <-> westeurope
│   ├── eastus <-> southeastasia
│   └── westeurope <-> southeastasia
│
├── Virtual Machines
│   ├── vm-overwatch-eastus (Ubuntu 22.04, Standard_B2s)
│   ├── vm-backend-westeurope (Ubuntu 22.04, Standard_B2s)
│   ├── vm-backend-westeurope-win (Windows Server 2022, Standard_B2s)
│   ├── vm-backend-southeastasia (Ubuntu 22.04, Standard_B2s)
│   ├── vm-traffic-eastus (Ubuntu 22.04, Standard_B2s)
│   └── vm-traffic-southeastasia (Ubuntu 22.04, Standard_B2s)
│
├── Network Security Groups
│   ├── nsg-overwatch (allow 53/udp, 53/tcp, 9090, 9091, 7946)
│   ├── nsg-backend (allow 80, 7946)
│   └── nsg-traffic (allow 22 only)
│
└── Public IPs
    ├── pip-overwatch (for DNS testing from outside)
    └── pip-traffic-* (for SSH access)
```

## VM Specifications

### Overwatch VM (vm-overwatch-eastus)

| Property | Value |
|----------|-------|
| Region | East US |
| Size | Standard_B2s (2 vCPU, 4 GB RAM) |
| OS | Ubuntu 22.04 LTS |
| Disk | 30 GB Premium SSD |
| Public IP | Yes (for external DNS testing) |
| Ports Open | 53/udp, 53/tcp, 9090, 9091, 7946 |

**Software to Install**:
- OpenGSLB binary (overwatch mode)
- dig, nslookup for testing
- prometheus/grafana (optional, for visualization)

### Backend VMs (Linux)

| Property | Value |
|----------|-------|
| Regions | West Europe, Southeast Asia |
| Size | Standard_B2s |
| OS | Ubuntu 22.04 LTS |
| Disk | 30 GB Standard SSD |
| Public IP | No (accessed via VNet peering) |
| Ports Open | 80, 7946 |

**Software to Install**:
- nginx (simple web server)
- OpenGSLB binary (agent mode)
- CAP_NET_ADMIN capability for agent

### Backend VM (Windows)

| Property | Value |
|----------|-------|
| Region | West Europe |
| Size | Standard_B2s |
| OS | Windows Server 2022 |
| Disk | 128 GB Standard SSD |
| Public IP | No |
| Ports Open | 80, 7946, 3389 (RDP) |

**Software to Install**:
- IIS with default website
- OpenGSLB agent (Windows binary)
- Run as Administrator (for GetPerTcpConnectionEStats)

### Traffic Generator VMs

| Property | Value |
|----------|-------|
| Regions | East US, Southeast Asia |
| Size | Standard_B2s |
| OS | Ubuntu 22.04 LTS |
| Disk | 30 GB Standard SSD |
| Public IP | Yes (for SSH access) |
| Ports Open | 22 |

**Software to Install**:
- curl, wget
- hey (HTTP load generator)
- wrk (HTTP benchmarking tool)
- Custom test scripts

## Expected Latencies

Based on Azure region distances:

| From | To | Expected RTT |
|------|-----|--------------|
| East US | West Europe | ~80-100ms |
| East US | Southeast Asia | ~200-250ms |
| West Europe | Southeast Asia | ~150-180ms |
| Within same region | Same region | ~1-5ms |

## Configuration Files

**Note**: With the simplified deployment, configuration files are auto-generated by the bootstrap scripts. The bootstrap scripts:
- Generate unique gossip encryption keys (via Terraform `random_bytes`)
- Generate service tokens (via Terraform `random_password`)
- Auto-discover the local IP address from cloud metadata
- Configure appropriate settings based on the role (overwatch/agent)

### Example Overwatch Configuration (auto-generated)

```yaml
# /etc/opengslb/config.yaml (generated by bootstrap-linux.sh)
mode: overwatch
node_id: overwatch-eastus
bind_address: "10.1.1.10"  # auto-discovered
gossip_port: 7946
api_port: 8080
dns_port: 53
gossip_key: "<randomly-generated-32-bytes-base64>"  # from Terraform
service_token: "<randomly-generated-token>"         # from Terraform
```

### Example Agent Configuration (auto-generated)

```yaml
# /etc/opengslb/config.yaml (generated by bootstrap-linux.sh)
mode: agent
node_id: backend-westeurope
bind_address: "10.2.1.10"  # auto-discovered
gossip_port: 7946
api_port: 8080
overwatch_address: "10.1.1.10:7946"  # passed via Terraform
gossip_key: "<randomly-generated-32-bytes-base64>"
service_token: "<randomly-generated-token>"
```

### Windows Configuration (auto-generated)

```yaml
# C:\opengslb\config.yaml (generated by bootstrap-windows.ps1)
mode: agent
node_id: backend-westeurope-win
bind_address: "10.2.1.11"  # auto-discovered
# ... same structure as Linux
```

For full configuration options, see the bootstrap scripts in `/scripts/`.

## Test Scenarios

### Test 1: Basic RTT Collection (Linux)

**Objective**: Verify agent collects TCP RTT data from active connections

**Steps**:
1. Start nginx on backend VM (West Europe)
2. Start agent on backend VM
3. From traffic generator (East US), run: `curl http://10.2.1.10/`
4. Repeat curl 10 times with 1-second intervals
5. Check agent logs for RTT observations

**Expected Results**:
```json
{
  "level": "DEBUG",
  "msg": "tcp rtt observation",
  "remote_addr": "10.1.2.x",
  "rtt_us": 85000,
  "connection_age_s": 2
}
```

**Verification**:
```bash
# On agent VM
curl http://localhost:9100/metrics | grep opengslb_agent_latency
```

Expected metrics:
```
opengslb_agent_latency_observations_total{backend="web"} 10
opengslb_agent_latency_subnets_tracked{backend="web"} 1
```

### Test 2: Subnet Aggregation

**Objective**: Verify RTT is aggregated per /24 subnet

**Steps**:
1. Generate traffic from multiple source IPs in same /24
2. Verify agent reports single aggregated value

**Verification**:
```bash
# Check gossip message in agent debug logs
grep "latency report" /var/log/opengslb/agent.log
```

Expected:
```json
{
  "subnet": "10.1.2.0/24",
  "rtt_ms": 85,
  "sample_count": 10,
  "last_updated": "2025-12-19T10:00:00Z"
}
```

### Test 3: Gossip to Overwatch

**Objective**: Verify latency data reaches Overwatch via gossip

**Steps**:
1. Generate sustained traffic (5 minutes)
2. Query Overwatch API for latency data

**Verification**:
```bash
# On any machine with access to Overwatch API
curl http://10.1.1.10:9090/api/v1/latency/table
```

Expected:
```json
{
  "entries": [
    {
      "subnet": "10.1.2.0/24",
      "backend": "eu-west/web/10.2.1.10:80",
      "rtt_ms": 85,
      "source": "agent",
      "samples": 50,
      "last_updated": "2025-12-19T10:05:00Z"
    }
  ]
}
```

### Test 4: Latency-Based Routing

**Objective**: Verify Overwatch routes to lowest-latency backend

**Steps**:
1. Generate traffic from East US to all backends (establish RTT data)
2. Wait for data to propagate (2-3 minutes)
3. Query DNS for `web.test.opengslb.local` from East US
4. Verify response is West Europe backend (lower latency than Singapore)

**Verification**:
```bash
# From East US traffic generator
dig @10.1.1.10 web.test.opengslb.local A +short
```

Expected: IP of West Europe backend (not Singapore)

Check Overwatch logs for routing decision:
```json
{
  "level": "DEBUG",
  "msg": "routing decision",
  "domain": "web.test.opengslb.local",
  "client_subnet": "10.1.2.0/24",
  "algorithm": "latency",
  "method": "learned",
  "selected": "eu-west/10.2.1.10",
  "rtt_ms": 85,
  "alternatives": [
    {"backend": "ap-southeast/10.3.1.10", "rtt_ms": 220}
  ]
}
```

### Test 5: Cold Start Fallback

**Objective**: Verify fallback to geo-routing when no latency data exists

**Steps**:
1. Restart Overwatch (clears latency table)
2. Immediately query DNS from new subnet (no learned data)
3. Verify geo-fallback is used

**Verification**:
```bash
# Immediately after Overwatch restart
dig @10.1.1.10 web.test.opengslb.local A +short
```

Check logs:
```json
{
  "level": "DEBUG",
  "msg": "routing decision",
  "algorithm": "latency",
  "method": "geo_fallback",
  "reason": "no learned data for subnet"
}
```

### Test 6: Windows Agent

**Objective**: Verify Windows agent collects RTT via GetPerTcpConnectionEStats

**Steps**:
1. Start IIS on Windows backend
2. Start agent as Administrator
3. Generate traffic from East US
4. Verify RTT collection in agent logs

**Verification**:
```powershell
# On Windows backend
Get-Content C:\opengslb\logs\agent.log | Select-String "tcp rtt"
```

### Test 7: Stale Data Handling

**Objective**: Verify stale latency data is not used for routing

**Steps**:
1. Establish baseline latency data
2. Stop traffic generation for >1 hour (stale_threshold)
3. Query DNS and verify fallback is used

**Verification**:
Check Overwatch logs for stale data detection:
```json
{
  "level": "DEBUG",
  "msg": "latency data stale",
  "subnet": "10.1.2.0/24",
  "age_minutes": 65,
  "threshold_minutes": 60,
  "action": "using fallback"
}
```

### Test 8: Multi-Region Comparison

**Objective**: Verify routing changes based on client location

**Steps**:
1. From East US traffic generator, query DNS
2. From Southeast Asia traffic generator, query DNS
3. Verify different backends selected based on learned latency

**Expected Results**:
- East US client → West Europe backend (closer)
- Southeast Asia client → Southeast Asia backend (closest)

## Traffic Generation Scripts

### Sustained HTTP Traffic

```bash
#!/bin/bash
# generate-traffic.sh
# Run on traffic generator VMs

BACKENDS=(
  "10.2.1.10"   # West Europe Linux
  "10.2.1.11"   # West Europe Windows
  "10.3.1.10"   # Southeast Asia
)

while true; do
  for backend in "${BACKENDS[@]}"; do
    # Make HTTP request (creates TCP connection for RTT measurement)
    curl -s -o /dev/null -w "%{time_connect}" "http://${backend}/" &
  done
  sleep 1
done
```

### Load Test with hey

```bash
#!/bin/bash
# load-test.sh
# Generates sustained load for latency learning

# 10 requests/second for 5 minutes to each backend
for backend in 10.2.1.10 10.2.1.11 10.3.1.10; do
  hey -n 3000 -c 10 -q 10 "http://${backend}/" &
done
wait
```

## Terraform Deployment

The infrastructure is defined in the `terraform/` directory with a simplified deployment model:

```
terraform/
├── main.tf          # Provider config, random secrets, locals
├── variables.tf     # Input variables (version, SSH key, etc.)
├── outputs.tf       # Deployment outputs and validation commands
├── vms.tf           # All VM definitions with cloud-init
├── network.tf       # VNets, subnets, NSGs, peering
└── terraform.tfvars.example  # Example configuration
```

### Key Features

1. **Pre-built Binaries**: Downloads from GitHub Releases instead of building from source
2. **Auto-generated Secrets**: Terraform generates gossip keys and service tokens
3. **Bootstrap Scripts**: Cloud-init runs bootstrap scripts that handle all configuration
4. **Built-in Validation**: Traffic generator VMs include the `test-cluster` command

### Deployment Commands

```bash
cd terraform/

# Initialize and deploy
terraform init
terraform apply -var="windows_admin_password=YourComplexPassword123!"

# View outputs
terraform output

# Validate cluster (after ~2 min for cloud-init)
ssh azureuser@$(terraform output -raw traffic_eastus_public_ip)
test-cluster
```

### Configuration Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `opengslb_version` | `v1.0.0` | Version tag to download from GitHub |
| `opengslb_github_repo` | `LoganRossUS/OpenGSLB` | GitHub repo for releases |
| `resource_group_name` | `rg-opengslb-latency-test` | Azure resource group name |
| `ssh_public_key_path` | `~/.ssh/id_rsa.pub` | Path to SSH public key |
| `windows_admin_password` | (required) | Windows VM password |

See `terraform/variables.tf` for all available options.

## Estimated Costs

| Resource | Count | Size | Est. Monthly Cost |
|----------|-------|------|-------------------|
| Linux VMs | 5 | Standard_B2s | ~$75 ($15 each) |
| Windows VM | 1 | Standard_B2s | ~$25 |
| Managed Disks | 6 | 30GB Standard | ~$10 |
| Public IPs | 3 | Static | ~$10 |
| VNet Peering | 6 | Cross-region | ~$20 (data transfer) |
| **Total** | | | **~$140/month** |

**Cost Optimization**:
- Use Azure Spot VMs for traffic generators (up to 90% savings)
- Deallocate VMs when not testing
- Delete resource group when testing complete

## Cleanup

```bash
# Delete all resources when done
terraform destroy

# Or via Azure CLI
az group delete --name rg-opengslb-latency-test --yes --no-wait
```

## Success Criteria

The test environment is successful if:

1. ✅ Agents collect TCP RTT data from real connections
2. ✅ RTT values match expected latencies (±20%)
3. ✅ Data aggregates correctly by /24 subnet
4. ✅ Gossip delivers data to Overwatch
5. ✅ Overwatch uses learned data for routing
6. ✅ Cold start fallback works correctly
7. ✅ Windows agent collects RTT via GetPerTcpConnectionEStats
8. ✅ Stale data detection works
9. ✅ Multi-region routing selects optimal backend

## Next Steps After Testing

1. Document observed latencies vs expected
2. Tune EWMA alpha based on real-world behavior
3. Validate poll interval doesn't impact performance
4. Create production deployment guide based on learnings
5. Consider adding more regions for broader testing