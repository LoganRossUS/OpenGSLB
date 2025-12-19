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
  description = "SSH commands for all VMs"
  value       = <<-EOT

    ============================================
    SSH Commands for All VMs
    ============================================

    # Overwatch (East US) - DNS Server
    ssh ${var.admin_username}@${azurerm_public_ip.overwatch.ip_address}

    # Traffic Generator (East US)
    ssh ${var.admin_username}@${azurerm_public_ip.traffic_eastus.ip_address}

    # Traffic Generator (Southeast Asia)
    ssh ${var.admin_username}@${azurerm_public_ip.traffic_southeastasia.ip_address}

    # Backend (West Europe - Linux)
    ssh ${var.admin_username}@${azurerm_public_ip.backend_westeurope.ip_address}

    # Backend (Southeast Asia)
    ssh ${var.admin_username}@${azurerm_public_ip.backend_southeastasia.ip_address}

    ============================================
    Check Cloud-Init Logs
    ============================================

    # On Overwatch or Backend VMs:
    cat /var/log/opengslb/cloud-init.log

    # On Traffic Generator VMs:
    cat /var/log/traffic-setup.log

    # Also check system cloud-init logs:
    sudo cat /var/log/cloud-init-output.log

    ============================================
    Check Service Status
    ============================================

    # On Overwatch:
    sudo systemctl status opengslb-overwatch
    sudo journalctl -u opengslb-overwatch -f

    # On Backends:
    sudo systemctl status opengslb-agent
    sudo journalctl -u opengslb-agent -f

  EOT
}

output "quick_start" {
  description = "Quick start instructions"
  value       = <<-EOT

    ============================================
    OpenGSLB ADR-017 Demo Environment Deployed!
    ============================================

    The VMs are now provisioning and will automatically:
    1. Install Go and build dependencies
    2. Clone and build OpenGSLB from source
    3. Start the appropriate service

    Wait ~5-10 minutes for cloud-init to complete, then:

    1. SSH to Overwatch and check logs:
       ssh ${var.admin_username}@${azurerm_public_ip.overwatch.ip_address}
       cat /var/log/opengslb/cloud-init.log
       sudo journalctl -u opengslb-overwatch -f

    2. SSH to traffic generator:
       ssh ${var.admin_username}@${azurerm_public_ip.traffic_eastus.ip_address}

    3. Generate traffic to backends:
       generate-traffic.sh 1 300   # 1 req/s for 5 minutes

    4. Check learned latency data:
       show-latency.sh

    5. Test DNS routing:
       query-dns.sh 5

    Cleanup: terraform destroy

  EOT
}
