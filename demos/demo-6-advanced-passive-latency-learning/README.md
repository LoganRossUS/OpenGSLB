# ADR-017 Azure Test Environment

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

### Overwatch Configuration (vm-overwatch-eastus)

```yaml
# /etc/opengslb/overwatch.yaml
mode: overwatch

identity:
  node_id: overwatch-eastus
  region: us-east

dns:
  listen_address: "0.0.0.0:53"
  zones:
    - test.opengslb.local
  default_ttl: 30

agent_tokens:
  test-backend: "test-token-for-latency-testing"

gossip:
  bind_address: "0.0.0.0:7946"
  encryption_key: "dGVzdC1lbmNyeXB0aW9uLWtleS0zMi1ieXRlcw=="  # 32 bytes base64

validation:
  enabled: true
  check_interval: 30s
  check_timeout: 5s

routing:
  default_algorithm: latency
  
  latency:
    method: learned
    cold_start_fallback: geo
    stale_threshold: 1h

api:
  address: "0.0.0.0:9090"
  allowed_networks:
    - 10.0.0.0/8

metrics:
  enabled: true
  address: "0.0.0.0:9091"

logging:
  level: debug
  format: json
```

### Agent Configuration (Linux Backends)

```yaml
# /etc/opengslb/agent.yaml
mode: agent

identity:
  service_token: "test-token-for-latency-testing"
  region: eu-west  # or ap-southeast for Singapore

backends:
  - service: web
    address: 0.0.0.0
    port: 80
    weight: 100
    health_check:
      type: http
      path: /
      interval: 5s
      timeout: 2s

latency_learning:
  enabled: true
  poll_interval: 10s
  ipv4_prefix: 24
  ipv6_prefix: 48
  min_connection_age: 5s
  max_subnets: 10000
  subnet_ttl: 24h
  min_samples: 3
  report_interval: 30s
  ewma_alpha: 0.3

gossip:
  encryption_key: "dGVzdC1lbmNyeXB0aW9uLWtleS0zMi1ieXRlcw=="
  overwatch_nodes:
    - 10.1.1.10:7946  # Overwatch private IP

heartbeat:
  interval: 10s

metrics:
  enabled: true
  address: ":9100"

logging:
  level: debug
  format: json
```

### Agent Configuration (Windows Backend)

```yaml
# C:\opengslb\agent.yaml
mode: agent

identity:
  service_token: "test-token-for-latency-testing"
  region: eu-west

backends:
  - service: web
    address: 0.0.0.0
    port: 80
    weight: 100
    health_check:
      type: http
      path: /
      interval: 5s
      timeout: 2s

latency_learning:
  enabled: true
  poll_interval: 10s
  # ... same as Linux

gossip:
  encryption_key: "dGVzdC1lbmNyeXB0aW9uLWtleS0zMi1ieXRlcw=="
  overwatch_nodes:
    - 10.1.1.10:7946

logging:
  level: debug
  format: json
```

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

```hcl
# main.tf - Azure infrastructure for ADR-017 testing

terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
}

# Resource Group
resource "azurerm_resource_group" "main" {
  name     = "rg-opengslb-latency-test"
  location = "East US"
  
  tags = {
    project = "OpenGSLB"
    purpose = "ADR-017 Testing"
  }
}

# Virtual Networks
resource "azurerm_virtual_network" "eastus" {
  name                = "vnet-eastus"
  address_space       = ["10.1.0.0/16"]
  location            = "East US"
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_virtual_network" "westeurope" {
  name                = "vnet-westeurope"
  address_space       = ["10.2.0.0/16"]
  location            = "West Europe"
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_virtual_network" "southeastasia" {
  name                = "vnet-southeastasia"
  address_space       = ["10.3.0.0/16"]
  location            = "Southeast Asia"
  resource_group_name = azurerm_resource_group.main.name
}

# Subnets
resource "azurerm_subnet" "overwatch" {
  name                 = "subnet-overwatch"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.eastus.name
  address_prefixes     = ["10.1.1.0/24"]
}

resource "azurerm_subnet" "traffic_eastus" {
  name                 = "subnet-traffic"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.eastus.name
  address_prefixes     = ["10.1.2.0/24"]
}

resource "azurerm_subnet" "backends_westeurope" {
  name                 = "subnet-backends"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.westeurope.name
  address_prefixes     = ["10.2.1.0/24"]
}

resource "azurerm_subnet" "backends_southeastasia" {
  name                 = "subnet-backends"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.southeastasia.name
  address_prefixes     = ["10.3.1.0/24"]
}

resource "azurerm_subnet" "traffic_southeastasia" {
  name                 = "subnet-traffic"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.southeastasia.name
  address_prefixes     = ["10.3.2.0/24"]
}

# VNet Peering
resource "azurerm_virtual_network_peering" "eastus_to_westeurope" {
  name                      = "eastus-to-westeurope"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.eastus.name
  remote_virtual_network_id = azurerm_virtual_network.westeurope.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "westeurope_to_eastus" {
  name                      = "westeurope-to-eastus"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.westeurope.name
  remote_virtual_network_id = azurerm_virtual_network.eastus.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "eastus_to_southeastasia" {
  name                      = "eastus-to-southeastasia"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.eastus.name
  remote_virtual_network_id = azurerm_virtual_network.southeastasia.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "southeastasia_to_eastus" {
  name                      = "southeastasia-to-eastus"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.southeastasia.name
  remote_virtual_network_id = azurerm_virtual_network.eastus.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "westeurope_to_southeastasia" {
  name                      = "westeurope-to-southeastasia"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.westeurope.name
  remote_virtual_network_id = azurerm_virtual_network.southeastasia.id
  allow_forwarded_traffic   = true
}

resource "azurerm_virtual_network_peering" "southeastasia_to_westeurope" {
  name                      = "southeastasia-to-westeurope"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.southeastasia.name
  remote_virtual_network_id = azurerm_virtual_network.westeurope.id
  allow_forwarded_traffic   = true
}

# Network Security Groups
resource "azurerm_network_security_group" "overwatch" {
  name                = "nsg-overwatch"
  location            = "East US"
  resource_group_name = azurerm_resource_group.main.name

  security_rule {
    name                       = "SSH"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "DNS-UDP"
    priority                   = 110
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Udp"
    source_port_range          = "*"
    destination_port_range     = "53"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "DNS-TCP"
    priority                   = 111
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "53"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "API"
    priority                   = 120
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "9090"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "Metrics"
    priority                   = 130
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "9091"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "Gossip"
    priority                   = 140
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "7946"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }
}

resource "azurerm_network_security_group" "backend" {
  name                = "nsg-backend"
  location            = "East US"
  resource_group_name = azurerm_resource_group.main.name

  security_rule {
    name                       = "SSH"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "RDP"
    priority                   = 101
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "3389"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "HTTP"
    priority                   = 110
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "80"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "Gossip"
    priority                   = 120
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "7946"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "AgentMetrics"
    priority                   = 130
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "9100"
    source_address_prefix      = "10.0.0.0/8"
    destination_address_prefix = "*"
  }
}

# Public IPs
resource "azurerm_public_ip" "overwatch" {
  name                = "pip-overwatch"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_public_ip" "traffic_eastus" {
  name                = "pip-traffic-eastus"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_public_ip" "traffic_southeastasia" {
  name                = "pip-traffic-southeastasia"
  resource_group_name = azurerm_resource_group.main.name
  location            = "Southeast Asia"
  allocation_method   = "Static"
  sku                 = "Standard"
}

# Network Interfaces
resource "azurerm_network_interface" "overwatch" {
  name                = "nic-overwatch"
  location            = "East US"
  resource_group_name = azurerm_resource_group.main.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.overwatch.id
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.1.1.10"
    public_ip_address_id          = azurerm_public_ip.overwatch.id
  }
}

resource "azurerm_network_interface" "traffic_eastus" {
  name                = "nic-traffic-eastus"
  location            = "East US"
  resource_group_name = azurerm_resource_group.main.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.traffic_eastus.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.traffic_eastus.id
  }
}

resource "azurerm_network_interface" "backend_westeurope" {
  name                = "nic-backend-westeurope"
  location            = "West Europe"
  resource_group_name = azurerm_resource_group.main.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.backends_westeurope.id
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.2.1.10"
  }
}

resource "azurerm_network_interface" "backend_westeurope_win" {
  name                = "nic-backend-westeurope-win"
  location            = "West Europe"
  resource_group_name = azurerm_resource_group.main.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.backends_westeurope.id
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.2.1.11"
  }
}

resource "azurerm_network_interface" "backend_southeastasia" {
  name                = "nic-backend-southeastasia"
  location            = "Southeast Asia"
  resource_group_name = azurerm_resource_group.main.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.backends_southeastasia.id
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.3.1.10"
  }
}

resource "azurerm_network_interface" "traffic_southeastasia" {
  name                = "nic-traffic-southeastasia"
  location            = "Southeast Asia"
  resource_group_name = azurerm_resource_group.main.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.traffic_southeastasia.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.traffic_southeastasia.id
  }
}

# NSG Associations
resource "azurerm_network_interface_security_group_association" "overwatch" {
  network_interface_id      = azurerm_network_interface.overwatch.id
  network_security_group_id = azurerm_network_security_group.overwatch.id
}

resource "azurerm_network_interface_security_group_association" "backend_westeurope" {
  network_interface_id      = azurerm_network_interface.backend_westeurope.id
  network_security_group_id = azurerm_network_security_group.backend.id
}

resource "azurerm_network_interface_security_group_association" "backend_westeurope_win" {
  network_interface_id      = azurerm_network_interface.backend_westeurope_win.id
  network_security_group_id = azurerm_network_security_group.backend.id
}

resource "azurerm_network_interface_security_group_association" "backend_southeastasia" {
  network_interface_id      = azurerm_network_interface.backend_southeastasia.id
  network_security_group_id = azurerm_network_security_group.backend.id
}

# Virtual Machines - Overwatch
resource "azurerm_linux_virtual_machine" "overwatch" {
  name                = "vm-overwatch-eastus"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  size                = "Standard_B2s"
  admin_username      = "azureuser"
  
  network_interface_ids = [
    azurerm_network_interface.overwatch.id,
  ]

  admin_ssh_key {
    username   = "azureuser"
    public_key = file("~/.ssh/id_rsa.pub")
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Premium_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }

  custom_data = base64encode(<<-EOF
    #!/bin/bash
    apt-get update
    apt-get install -y nginx dnsutils curl jq
    # OpenGSLB will be installed manually for testing
    mkdir -p /etc/opengslb /var/log/opengslb
  EOF
  )
}

# Virtual Machines - Traffic Generator East US
resource "azurerm_linux_virtual_machine" "traffic_eastus" {
  name                = "vm-traffic-eastus"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  size                = "Standard_B2s"
  admin_username      = "azureuser"
  
  network_interface_ids = [
    azurerm_network_interface.traffic_eastus.id,
  ]

  admin_ssh_key {
    username   = "azureuser"
    public_key = file("~/.ssh/id_rsa.pub")
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }

  custom_data = base64encode(<<-EOF
    #!/bin/bash
    apt-get update
    apt-get install -y curl dnsutils wrk
    # Install hey
    wget https://hey-release.s3.us-east-2.amazonaws.com/hey_linux_amd64 -O /usr/local/bin/hey
    chmod +x /usr/local/bin/hey
  EOF
  )
}

# Virtual Machines - Backend West Europe (Linux)
resource "azurerm_linux_virtual_machine" "backend_westeurope" {
  name                = "vm-backend-westeurope"
  resource_group_name = azurerm_resource_group.main.name
  location            = "West Europe"
  size                = "Standard_B2s"
  admin_username      = "azureuser"
  
  network_interface_ids = [
    azurerm_network_interface.backend_westeurope.id,
  ]

  admin_ssh_key {
    username   = "azureuser"
    public_key = file("~/.ssh/id_rsa.pub")
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }

  custom_data = base64encode(<<-EOF
    #!/bin/bash
    apt-get update
    apt-get install -y nginx
    systemctl enable nginx
    systemctl start nginx
    mkdir -p /etc/opengslb /var/log/opengslb
    # Agent will be installed manually
  EOF
  )
}

# Virtual Machines - Backend West Europe (Windows)
resource "azurerm_windows_virtual_machine" "backend_westeurope_win" {
  name                = "vm-backend-win"
  resource_group_name = azurerm_resource_group.main.name
  location            = "West Europe"
  size                = "Standard_B2s"
  admin_username      = "azureuser"
  admin_password      = "P@ssw0rd1234!"  # Change in production!
  
  network_interface_ids = [
    azurerm_network_interface.backend_westeurope_win.id,
  ]

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "MicrosoftWindowsServer"
    offer     = "WindowsServer"
    sku       = "2022-datacenter-azure-edition"
    version   = "latest"
  }
}

# Virtual Machines - Backend Southeast Asia
resource "azurerm_linux_virtual_machine" "backend_southeastasia" {
  name                = "vm-backend-southeastasia"
  resource_group_name = azurerm_resource_group.main.name
  location            = "Southeast Asia"
  size                = "Standard_B2s"
  admin_username      = "azureuser"
  
  network_interface_ids = [
    azurerm_network_interface.backend_southeastasia.id,
  ]

  admin_ssh_key {
    username   = "azureuser"
    public_key = file("~/.ssh/id_rsa.pub")
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }

  custom_data = base64encode(<<-EOF
    #!/bin/bash
    apt-get update
    apt-get install -y nginx
    systemctl enable nginx
    systemctl start nginx
    mkdir -p /etc/opengslb /var/log/opengslb
  EOF
  )
}

# Virtual Machines - Traffic Generator Southeast Asia
resource "azurerm_linux_virtual_machine" "traffic_southeastasia" {
  name                = "vm-traffic-southeastasia"
  resource_group_name = azurerm_resource_group.main.name
  location            = "Southeast Asia"
  size                = "Standard_B2s"
  admin_username      = "azureuser"
  
  network_interface_ids = [
    azurerm_network_interface.traffic_southeastasia.id,
  ]

  admin_ssh_key {
    username   = "azureuser"
    public_key = file("~/.ssh/id_rsa.pub")
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts"
    version   = "latest"
  }

  custom_data = base64encode(<<-EOF
    #!/bin/bash
    apt-get update
    apt-get install -y curl dnsutils wrk
    wget https://hey-release.s3.us-east-2.amazonaws.com/hey_linux_amd64 -O /usr/local/bin/hey
    chmod +x /usr/local/bin/hey
  EOF
  )
}

# Outputs
output "overwatch_public_ip" {
  value = azurerm_public_ip.overwatch.ip_address
}

output "traffic_eastus_public_ip" {
  value = azurerm_public_ip.traffic_eastus.ip_address
}

output "traffic_southeastasia_public_ip" {
  value = azurerm_public_ip.traffic_southeastasia.ip_address
}

output "backend_westeurope_private_ip" {
  value = azurerm_network_interface.backend_westeurope.private_ip_address
}

output "backend_westeurope_win_private_ip" {
  value = azurerm_network_interface.backend_westeurope_win.private_ip_address
}

output "backend_southeastasia_private_ip" {
  value = azurerm_network_interface.backend_southeastasia.private_ip_address
}
```

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