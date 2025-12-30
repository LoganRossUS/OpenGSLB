# Demo 6: Passive Latency Learning (ADR-017)

**Level**: Advanced
**Time**: 30-45 minutes (Azure deployment)
**Prerequisites**: Azure subscription, Terraform, SSH key

## Overview

This demo showcases OpenGSLB's **passive latency learning** feature - a unique capability that learns real client-to-backend latency by reading TCP RTT (Round-Trip Time) data directly from the operating system.

Unlike active latency probing (which measures Overwatch-to-backend latency), passive learning captures the actual latency experienced by your clients. This data is aggregated by client subnet and gossiped to Overwatch nodes for intelligent routing decisions.

### What Makes This Different

| Approach | What It Measures | Accuracy |
|----------|-----------------|----------|
| Active Latency (Demo 3) | Overwatch → Backend | Proxy's perspective |
| **Passive Learning (This Demo)** | **Client → Backend** | **Client's actual experience** |

### Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│  Client (10.1.2.x)                                                  │
│       │                                                              │
│       │ HTTP Request                                                 │
│       ▼                                                              │
│  ┌─────────────┐         ┌─────────────┐         ┌─────────────┐   │
│  │ Backend EU  │◄────────│   Agent     │────────►│  Overwatch  │   │
│  │ (nginx)     │  TCP    │ (RTT: 85ms) │  Gossip │  (DNS)      │   │
│  └─────────────┘  conn   └─────────────┘         └─────────────┘   │
│                                                          │          │
│  Agent reads TCP_INFO from kernel:                       │          │
│  - tcpi_rtt (smoothed RTT in microseconds)              │          │
│  - Aggregates by /24 subnet                             ▼          │
│  - Reports to Overwatch via gossip            DNS Response:        │
│                                               "Use EU backend       │
│                                                (lowest latency)"    │
└─────────────────────────────────────────────────────────────────────┘
```

## What You'll Learn

1. How agents collect TCP RTT data from the OS
2. How latency is aggregated by client subnet
3. How Overwatch uses learned latency for routing
4. Cold-start fallback to geolocation routing
5. Cross-platform support (Linux + Windows)

## Prerequisites

- **Azure Subscription** with permissions to create VMs
- **Terraform** >= 1.0 installed
- **SSH Key** for VM access
- **Azure CLI** authenticated (`az login`)

## Quick Start

```bash
# Clone the repository
git clone https://github.com/LoganRossUS/OpenGSLB.git
cd OpenGSLB/demos/demo-6-advanced-passive-latency-learning/terraform

# Configure deployment
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your SSH key path

# Deploy infrastructure (~2 minutes)
terraform init
terraform apply -var="windows_admin_password=YourComplexPassword123!"

# Wait for cloud-init to complete, then validate
ssh azureuser@$(terraform output -raw traffic_eastus_public_ip)
test-cluster
```

## Infrastructure Overview

The demo deploys across 3 Azure regions:

| Region | VMs | Purpose |
|--------|-----|---------|
| **East US** | Overwatch, Traffic Generator | DNS authority, simulated clients |
| **West Europe** | Linux Backend, Windows Backend | Application servers with agents |
| **Southeast Asia** | Linux Backend, Traffic Generator | APAC region backend + clients |

### Expected Latencies

| From | To | Expected RTT |
|------|-----|--------------|
| East US | West Europe | ~80-100ms |
| East US | Southeast Asia | ~200-250ms |
| Southeast Asia | Southeast Asia | ~1-5ms |

## Step-by-Step Walkthrough

### Step 1: Verify Deployment

SSH to the traffic generator and run validation:

```bash
ssh azureuser@<traffic_eastus_public_ip>

# Run cluster validation
test-cluster

# Expected output:
# ✓ Overwatch DNS responding
# ✓ All agents connected
# ✓ Health checks passing
```

### Step 2: Generate Traffic

Generate sustained traffic to populate the latency table:

```bash
# Generate 5 requests/second for 5 minutes
generate-traffic 5 300

# This creates TCP connections to all backends
# Agents read RTT from each connection
```

### Step 3: View Learned Latency Data

Query the Overwatch API to see collected latency data:

```bash
curl http://10.1.1.10:8080/api/v1/overwatch/latency | jq .
```

Expected output:
```json
{
  "entries": [
    {
      "subnet": "10.1.2.0/24",
      "domain": "app.demo.local",
      "region": "eu-west",
      "rtt_ms": 85,
      "samples": 150,
      "last_updated": "2025-12-19T10:05:00Z"
    },
    {
      "subnet": "10.1.2.0/24",
      "domain": "app.demo.local",
      "region": "ap-southeast",
      "rtt_ms": 220,
      "samples": 150,
      "last_updated": "2025-12-19T10:05:00Z"
    }
  ]
}
```

### Step 4: Test Latency-Based Routing

Query DNS and verify the lowest-latency backend is selected:

```bash
# From East US traffic generator
dig @10.1.1.10 app.demo.local A +short

# Expected: West Europe backend IP (lower latency than Singapore)
```

Check Overwatch logs for routing decision:

```bash
ssh azureuser@<overwatch_public_ip>
journalctl -u opengslb | grep "routing decision"
```

### Step 5: Test Cold-Start Fallback

When no latency data exists, Overwatch falls back to geolocation:

```bash
# Restart Overwatch (clears latency table)
sudo systemctl restart opengslb

# Immediately query DNS
dig @10.1.1.10 app.demo.local A +short

# Check logs - should show geo_fallback
journalctl -u opengslb | grep "geo_fallback"
```

### Step 6: Compare Regions

Traffic from different regions should route to different backends:

```bash
# From East US (connects to EU - lower latency)
ssh azureuser@<traffic_eastus_public_ip>
dig @10.1.1.10 app.demo.local +short

# From Singapore (connects to Singapore - local)
ssh azureuser@<traffic_singapore_public_ip>
dig @10.1.1.10 app.demo.local +short
```

## Configuration Deep-Dive

### Agent Latency Learning Config

```yaml
agent:
  latency_learning:
    enabled: true
    poll_interval: 10s        # How often to read TCP stats
    min_connection_age: 5s    # Ignore new connections
    ipv4_prefix: 24           # Aggregate by /24
    ipv6_prefix: 48           # Aggregate by /48
    ewma_alpha: 0.3           # Smoothing factor
    max_subnets: 100000       # Memory limit
    subnet_ttl: 168h          # 7-day retention
    min_samples: 5            # Min samples before reporting
    report_interval: 30s      # Gossip frequency
```

### Domain Configuration for Learned Latency

```yaml
domains:
  - name: app.demo.local
    routing_algorithm: learned_latency  # Use passive learning
    regions:
      - eu-west
      - ap-southeast
    latency_config:
      max_latency_ms: 300     # Exclude high-latency backends
      min_samples: 5          # Require sufficient data
```

## How It Works

### 1. TCP RTT Collection (Agent)

On Linux, agents read `/proc/net/tcp` and use `getsockopt(TCP_INFO)`:

```c
struct tcp_info info;
getsockopt(sock, IPPROTO_TCP, TCP_INFO, &info, &len);
// info.tcpi_rtt contains smoothed RTT in microseconds
```

On Windows, agents use `GetPerTcpConnectionEStats()`:

```powershell
# Requires Administrator privileges
GetPerTcpConnectionEStats -State EstablishedConnections
```

### 2. Subnet Aggregation

RTT samples are aggregated by client subnet using EWMA:

```
new_rtt = α × sample + (1 - α) × old_rtt
```

With α = 0.3, recent samples have moderate influence while maintaining stability.

### 3. Gossip to Overwatch

Agents periodically send latency reports:

```json
{
  "type": "latency_report",
  "agent_id": "backend-eu-west",
  "entries": [
    {"subnet": "10.1.2.0/24", "rtt_ms": 85, "samples": 50}
  ]
}
```

### 4. Routing Decision

When a DNS query arrives, Overwatch:

1. Extracts client IP (or ECS subnet)
2. Looks up learned latency for each backend
3. Selects the backend with lowest RTT
4. Falls back to geolocation if no data exists

## Troubleshooting

### No Latency Data Appearing

```bash
# Check agent logs
journalctl -u opengslb | grep "latency"

# Verify CAP_NET_ADMIN on Linux
getcap /usr/local/bin/opengslb
# Should show: cap_net_admin+ep

# Verify connections exist
ss -tn | grep ESTAB
```

### Unexpected Routing

```bash
# Check what Overwatch sees
curl http://10.1.1.10:8080/api/v1/overwatch/latency | jq .

# Verify domain config
curl http://10.1.1.10:8080/api/v1/domains | jq .
```

### Windows Agent Issues

```powershell
# Check if running as Administrator
([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")

# Check agent logs
Get-Content C:\opengslb\logs\agent.log | Select-String "latency"
```

## Cleanup

```bash
# Destroy all Azure resources
cd terraform
terraform destroy

# Or delete the resource group directly
az group delete --name rg-opengslb-latency-test --yes
```

## Cost Estimate

| Resource | Monthly Cost |
|----------|-------------|
| 5x Linux VMs (B2s) | ~$75 |
| 1x Windows VM (B2s) | ~$25 |
| VNet Peering | ~$20 |
| **Total** | **~$120** |

**Tip**: Deallocate VMs when not testing to reduce costs.

## Next Steps

- Review [ADR-017: Passive Latency Learning](../ARCHITECTURE_DECISIONS.md#adr-017-passive-latency-learning-via-os-tcp-statistics) for design rationale
- Explore the [Configuration Reference](../configuration.md) for all latency_learning options
- Try [Demo 3: Latency Routing](demo-3-latency-routing.md) for comparison with active probing

## Key Takeaways

1. **Passive learning captures real client latency** - not just Overwatch-to-backend
2. **Subnet aggregation** prevents unbounded memory growth
3. **Cold-start fallback** ensures routing works before data is collected
4. **Cross-platform support** - Linux (netlink) and Windows (GetPerTcpConnectionEStats)
5. **No client changes required** - works with existing TCP connections
