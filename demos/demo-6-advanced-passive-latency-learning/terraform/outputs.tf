# Copyright (C) 2025 Logan Ross
#
# This file is part of OpenGSLB – https://opengslb.org
#
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

# Outputs

output "overwatch_public_ip" {
  description = "Public IP of the Overwatch VM (for DNS queries)"
  value       = azurerm_public_ip.overwatch.ip_address
}

output "traffic_eastus_public_ip" {
  description = "Public IP of East US traffic generator (for SSH)"
  value       = azurerm_public_ip.traffic_eastus.ip_address
}

output "traffic_southeastasia_public_ip" {
  description = "Public IP of Southeast Asia traffic generator (for SSH)"
  value       = azurerm_public_ip.traffic_southeastasia.ip_address
}

output "backend_westeurope_public_ip" {
  description = "Public IP of West Europe Linux backend (for SSH)"
  value       = azurerm_public_ip.backend_westeurope.ip_address
}

output "backend_southeastasia_public_ip" {
  description = "Public IP of Southeast Asia backend (for SSH)"
  value       = azurerm_public_ip.backend_southeastasia.ip_address
}

output "backend_westeurope_win_public_ip" {
  description = "Public IP of West Europe Windows backend (for RDP)"
  value       = azurerm_public_ip.backend_westeurope_win.ip_address
}

output "backend_westeurope_private_ip" {
  description = "Private IP of West Europe Linux backend"
  value       = azurerm_network_interface.backend_westeurope.private_ip_address
}

output "backend_westeurope_win_private_ip" {
  description = "Private IP of West Europe Windows backend"
  value       = azurerm_network_interface.backend_westeurope_win.private_ip_address
}

output "backend_southeastasia_private_ip" {
  description = "Private IP of Southeast Asia backend"
  value       = azurerm_network_interface.backend_southeastasia.private_ip_address
}

output "overwatch_private_ip" {
  description = "Private IP of Overwatch (for internal gossip)"
  value       = azurerm_network_interface.overwatch.private_ip_address
}

output "dns_test_command" {
  description = "Command to test DNS routing"
  value       = "dig @${azurerm_public_ip.overwatch.ip_address} web.test.opengslb.local A +short"
}

output "ssh_commands" {
  description = "SSH/RDP commands for all VMs"
  value       = <<-EOT

    ============================================
    SSH Commands for Linux VMs
    ============================================

    # Overwatch (East US) - DNS Server
    ssh ${var.admin_username}@${azurerm_public_ip.overwatch.ip_address}

    # Traffic Generator (East US) - Run tests from here
    ssh ${var.admin_username}@${azurerm_public_ip.traffic_eastus.ip_address}

    # Traffic Generator (Southeast Asia)
    ssh ${var.admin_username}@${azurerm_public_ip.traffic_southeastasia.ip_address}

    # Backend (West Europe - Linux)
    ssh ${var.admin_username}@${azurerm_public_ip.backend_westeurope.ip_address}

    # Backend (Southeast Asia)
    ssh ${var.admin_username}@${azurerm_public_ip.backend_southeastasia.ip_address}

    ============================================
    RDP Command for Windows VM
    ============================================

    # Backend (West Europe - Windows)
    mstsc /v:${azurerm_public_ip.backend_westeurope_win.ip_address}
    # Username: ${var.admin_username}
    # Password: (from terraform.tfvars windows_admin_password)

    ============================================
    Check Bootstrap Logs
    ============================================

    # On any Linux VM:
    cat /var/log/opengslb-bootstrap.log
    sudo journalctl -u opengslb-overwatch -f  # or opengslb-agent

    # On Windows VM:
    type C:\opengslb-bootstrap.log

    ============================================
    Troubleshooting
    ============================================

    # Check service status:
    sudo systemctl status opengslb-overwatch  # on overwatch
    sudo systemctl status opengslb-agent      # on agents

    # View recent logs:
    sudo journalctl -u opengslb-overwatch --no-pager -n 100

    # Check ports:
    sudo ss -tlnp | grep -E '(53|8080|7946|9090)'

  EOT
}

output "quick_start" {
  description = "Quick start instructions"
  value       = <<-EOT

    ============================================
    OpenGSLB Demo Environment Deployed!
    ============================================

    SIMPLIFIED DEPLOYMENT: VMs download pre-built binaries
    Deployment time: ~2 minutes (vs ~15 minutes building from source)

    Wait ~2 minutes for cloud-init to complete, then:

    1. SSH to traffic generator and validate cluster:
       ssh ${var.admin_username}@${azurerm_public_ip.traffic_eastus.ip_address}
       test-cluster

    2. If validation passes, generate traffic:
       generate-traffic 5 300    # 5 req/s for 5 minutes

    3. Check learned latency data:
       curl http://${azurerm_network_interface.overwatch.private_ip_address}:8080/api/v1/overwatch/latency | jq .

    4. Check cluster status:
       curl http://${azurerm_network_interface.overwatch.private_ip_address}:8080/api/v1/cluster/status | jq .

    ============================================
    If Tests Fail
    ============================================

    The test-cluster command produces verbose output for debugging.
    Copy the output and paste it into Claude Code for analysis.

    Check bootstrap logs:
    ssh ${var.admin_username}@${azurerm_public_ip.overwatch.ip_address}
    cat /var/log/opengslb-bootstrap.log

    ============================================
    Cleanup
    ============================================

    terraform destroy

  EOT
}

output "validation_command" {
  description = "Command to validate the cluster from traffic generator"
  value       = "ssh ${var.admin_username}@${azurerm_public_ip.traffic_eastus.ip_address} test-cluster"
}

output "cluster_status_url" {
  description = "URL to check cluster status (from within VNet)"
  value       = "http://${azurerm_network_interface.overwatch.private_ip_address}:8080/api/v1/cluster/status"
}
