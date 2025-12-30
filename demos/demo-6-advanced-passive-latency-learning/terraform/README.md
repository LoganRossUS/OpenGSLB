# ADR-017 Terraform Deployment

This Terraform configuration deploys a complete multi-region Azure environment for testing the Passive Latency Learning feature (ADR-017).

## Key Features

- **Auto-Registration**: Agent nodes automatically register with the Overwatch cluster
- **Latency-Based Routing**: DNS responses are optimized based on learned client latency data
- **Multi-Region**: Demonstrates routing across 3 Azure regions

## Prerequisites

1. **Azure CLI** installed and authenticated:
   ```bash
   az login
   az account set --subscription "Your Subscription Name"
   ```

2. **Terraform** >= 1.0 installed:
   ```bash
   # macOS
   brew install terraform

   # Linux
   curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
   echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
   sudo apt update && sudo apt install terraform
   ```

3. **RSA SSH Key Pair** for Linux VM access (Azure does NOT support ed25519):
   ```bash
   # Generate an RSA key (Azure requires RSA, not ed25519)
   ssh-keygen -t rsa -b 4096 -f ~/.ssh/azure_rsa

   # Then set in terraform.tfvars:
   # ssh_public_key_path = "~/.ssh/azure_rsa.pub"
   ```

## Quick Start

```bash
# Initialize Terraform
terraform init

# Copy and edit variables
cp terraform.tfvars.example terraform.tfvars
vim terraform.tfvars  # Set your SSH key path

# Preview the deployment
terraform plan

# Deploy (takes ~2 minutes)
terraform apply

# View outputs with connection info
terraform output
```

## Configuration

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ssh_public_key_path` | `~/.ssh/id_rsa.pub` | Path to SSH public key |
| `admin_username` | `azureuser` | Admin username for VMs |
| `vm_size` | `Standard_B2s` | Azure VM size |
| `opengslb_version` | `latest` | OpenGSLB version to deploy |
| `opengslb_github_repo` | `LoganRossUS/OpenGSLB` | GitHub repository |
| `enable_bastion` | `false` | Enable Azure Bastion for secure access |

## What Gets Deployed

### VMs (5 total)

| VM | Region | Role |
|----|--------|------|
| vm-overwatch-eastus | East US | Overwatch DNS server |
| vm-traffic-eastus | East US | Traffic generator |
| vm-backend-westeurope | West Europe | Linux backend + agent |
| vm-backend-southeastasia | Southeast Asia | Linux backend + agent |
| vm-traffic-southeastasia | Southeast Asia | Traffic generator |

### Networking

- 3 VNets across 3 regions
- Full mesh VNet peering (all regions can communicate)
- NSGs for security

### Auto-Configuration (via bootstrap scripts)

All VMs automatically:
1. Download pre-built OpenGSLB binary from GitHub Releases
2. Generate configuration files
3. Start the service (systemd)
4. Register with the cluster (agents)

## Testing the Deployment

### 1. Validate the Cluster

From the traffic generator:

```bash
# SSH to traffic generator
ssh azureuser@$(terraform output -raw traffic_eastus_public_ip)

# Run validation (waits for all agents to register)
test-cluster
```

### 2. Generate Traffic

```bash
# Generate sustained traffic (5 req/s for 5 minutes)
generate-traffic 5 300
```

### 3. Check Latency Learning

```bash
# View learned latency data
curl http://10.1.1.10:8080/api/v1/overwatch/latency | jq .

# Check cluster status
curl http://10.1.1.10:8080/api/v1/cluster/status | jq .
```

### 4. Test DNS Routing

```bash
# Query DNS multiple times - should return optimal backend
for i in {1..10}; do
  dig @10.1.1.10 web.test.opengslb.local +short
done
```

## Troubleshooting

### Bootstrap failed
```bash
cat /var/log/opengslb-bootstrap.log
```

### Service won't start
```bash
sudo journalctl -u opengslb-overwatch --no-pager
sudo journalctl -u opengslb-agent --no-pager
```

### No latency data
1. Verify agents are running: `curl http://10.1.1.10:8080/api/v1/overwatch/agents`
2. Check gossip connectivity
3. Generate traffic first - latency is learned from actual requests

## Adding Windows Agents

For Windows Server support, see the [Windows Setup Guide](../../../docs/windows-agent-setup.md).

## Cleanup

```bash
# Destroy all resources
terraform destroy

# Or via Azure CLI
az group delete --name rg-opengslb-latency-test --yes --no-wait
```

## Estimated Costs

| Resource | Monthly Cost |
|----------|--------------|
| 5 Linux VMs (Standard_B2s) | ~$75 |
| Managed Disks | ~$10 |
| Public IPs | ~$10 |
| VNet Peering (data) | ~$20 |
| **Total** | **~$115/month** |

**Cost Optimization Tips:**
- Deallocate VMs when not testing
- Use Azure Spot VMs for traffic generators
- Delete the resource group when done
