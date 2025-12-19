# ADR-017 Terraform Deployment

This Terraform configuration deploys a complete multi-region Azure environment for testing the Passive Latency Learning feature (ADR-017).

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

3. **SSH Key Pair** for Linux VM access:
   ```bash
   ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa
   ```

## Quick Start

```bash
# Initialize Terraform
terraform init

# Copy and edit variables
cp terraform.tfvars.example terraform.tfvars
vim terraform.tfvars  # Set your SSH key path and Windows password

# Preview the deployment
terraform plan

# Deploy (takes ~10-15 minutes)
terraform apply

# View outputs with connection info
terraform output
```

## Configuration

### Required Variables

| Variable | Description |
|----------|-------------|
| `windows_admin_password` | Password for Windows VM (must be complex) |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ssh_public_key_path` | `~/.ssh/id_rsa.pub` | Path to SSH public key |
| `admin_username` | `azureuser` | Admin username for VMs |
| `vm_size` | `Standard_B2s` | Azure VM size |
| `opengslb_git_repo` | GitHub repo | Git repository URL |
| `opengslb_git_branch` | `main` | Branch to build from |

## What Gets Deployed

### VMs (6 total)

| VM | Region | Role |
|----|--------|------|
| vm-overwatch-eastus | East US | Overwatch DNS server |
| vm-traffic-eastus | East US | Traffic generator |
| vm-backend-westeurope | West Europe | Linux backend + agent |
| vm-backend-win | West Europe | Windows backend + agent |
| vm-backend-southeastasia | Southeast Asia | Linux backend + agent |
| vm-traffic-southeastasia | Southeast Asia | Traffic generator |

### Networking

- 3 VNets across 3 regions
- Full mesh VNet peering (all regions can communicate)
- NSGs for security

### Auto-Configuration (via cloud-init)

All VMs automatically:
1. Install Go 1.23
2. Clone OpenGSLB from GitHub
3. Build the binary
4. Create configuration files
5. Start the service (systemd on Linux)

## Testing the Deployment

### 1. Wait for Cloud-Init

VMs need ~10 minutes to complete setup. Check progress:

```bash
# SSH to Overwatch
ssh azureuser@$(terraform output -raw overwatch_public_ip)

# Check cloud-init status
cloud-init status --wait

# Check service
sudo systemctl status opengslb-overwatch
sudo journalctl -u opengslb-overwatch -f
```

### 2. Generate Traffic

From the traffic generator:

```bash
# SSH to traffic generator
ssh azureuser@$(terraform output -raw traffic_eastus_public_ip)

# Generate sustained traffic (1 req/s for 5 minutes)
generate-traffic.sh 1 300

# Run load test
load-test.sh 1000 10
```

### 3. Verify Latency Learning

```bash
# On traffic generator
show-latency.sh

# Test DNS routing
query-dns.sh 5
```

### 4. Check Metrics

```bash
# From any VM with access to Overwatch
curl http://10.1.1.10:9091/metrics | grep opengslb_latency
```

## Troubleshooting

### Cloud-init failed
```bash
sudo cat /var/log/cloud-init-output.log
```

### Service won't start
```bash
sudo journalctl -u opengslb-overwatch --no-pager
sudo journalctl -u opengslb-agent --no-pager
```

### No latency data
1. Verify agents are running
2. Check gossip connectivity: `curl http://10.1.1.10:9090/api/v1/health`
3. Verify traffic is flowing

### Windows agent issues
```powershell
# RDP to Windows VM through traffic generator (bastion)
Get-Content C:\opengslb\logs\agent.log
Get-ScheduledTask -TaskName "OpenGSLB Agent"
```

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
| 1 Windows VM (Standard_B2s) | ~$25 |
| Managed Disks | ~$10 |
| Public IPs | ~$10 |
| VNet Peering (data) | ~$20 |
| **Total** | **~$140/month** |

**Cost Optimization Tips:**
- Deallocate VMs when not testing
- Use Azure Spot VMs for traffic generators
- Delete the resource group when done
