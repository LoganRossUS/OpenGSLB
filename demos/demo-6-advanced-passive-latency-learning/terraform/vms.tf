# Copyright (C) 2025 Logan Ross
#
# This file is part of OpenGSLB – https://opengslb.org
#
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

# Virtual Machines

# Overwatch VM (East US)
resource "azurerm_linux_virtual_machine" "overwatch" {
  name                = "vm-overwatch-eastus"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  size                = var.vm_size
  admin_username      = var.admin_username
  tags                = var.tags

  network_interface_ids = [
    azurerm_network_interface.overwatch.id,
  ]

  admin_ssh_key {
    username   = var.admin_username
    public_key = file(var.ssh_public_key_path)
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

  custom_data = base64encode(templatefile("${path.module}/scripts/cloud-init-overwatch.yaml", {
    git_repo              = var.opengslb_git_repo
    git_branch            = var.opengslb_git_branch
    gossip_encryption_key = var.gossip_encryption_key
    service_token         = var.service_token
  }))
}

# Traffic Generator VM (East US)
resource "azurerm_linux_virtual_machine" "traffic_eastus" {
  name                = "vm-traffic-eastus"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  size                = var.vm_size
  admin_username      = var.admin_username
  tags                = var.tags

  network_interface_ids = [
    azurerm_network_interface.traffic_eastus.id,
  ]

  admin_ssh_key {
    username   = var.admin_username
    public_key = file(var.ssh_public_key_path)
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

  custom_data = base64encode(templatefile("${path.module}/scripts/cloud-init-traffic.yaml", {
    region = "us-east"
  }))
}

# Backend VM (West Europe - Linux)
resource "azurerm_linux_virtual_machine" "backend_westeurope" {
  name                = "vm-backend-westeurope"
  resource_group_name = azurerm_resource_group.main.name
  location            = "West Europe"
  size                = var.vm_size
  admin_username      = var.admin_username
  tags                = var.tags

  network_interface_ids = [
    azurerm_network_interface.backend_westeurope.id,
  ]

  admin_ssh_key {
    username   = var.admin_username
    public_key = file(var.ssh_public_key_path)
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

  custom_data = base64encode(templatefile("${path.module}/scripts/cloud-init-agent.yaml", {
    git_repo              = var.opengslb_git_repo
    git_branch            = var.opengslb_git_branch
    gossip_encryption_key = var.gossip_encryption_key
    service_token         = var.service_token
    region                = "eu-west"
    hostname              = "backend-westeurope"
  }))
}

# Backend VM (West Europe - Windows)
resource "azurerm_windows_virtual_machine" "backend_westeurope_win" {
  name                = "vm-backend-win"
  resource_group_name = azurerm_resource_group.main.name
  location            = "West Europe"
  size                = var.vm_size
  admin_username      = var.admin_username
  admin_password      = var.windows_admin_password
  tags                = var.tags

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

  # Windows setup requires manual or custom script extension
  # IIS and OpenGSLB agent need to be installed via PowerShell
}

# Custom Script Extension for Windows VM to install IIS and OpenGSLB
resource "azurerm_virtual_machine_extension" "backend_win_setup" {
  name                 = "setup-opengslb"
  virtual_machine_id   = azurerm_windows_virtual_machine.backend_westeurope_win.id
  publisher            = "Microsoft.Compute"
  type                 = "CustomScriptExtension"
  type_handler_version = "1.10"

  protected_settings = jsonencode({
    commandToExecute = <<-EOT
      powershell -ExecutionPolicy Bypass -Command "
        # Install IIS
        Install-WindowsFeature -Name Web-Server -IncludeManagementTools

        # Create OpenGSLB directory
        New-Item -Path 'C:\opengslb' -ItemType Directory -Force
        New-Item -Path 'C:\opengslb\logs' -ItemType Directory -Force

        # Download Go
        Invoke-WebRequest -Uri 'https://go.dev/dl/go1.23.4.windows-amd64.msi' -OutFile 'C:\temp\go.msi'
        Start-Process msiexec.exe -Wait -ArgumentList '/I C:\temp\go.msi /quiet'

        # Clone and build OpenGSLB
        cd C:\opengslb
        git clone --depth 1 --branch ${var.opengslb_git_branch} ${var.opengslb_git_repo} src
        cd src
        & 'C:\Program Files\Go\bin\go.exe' build -o C:\opengslb\opengslb.exe ./cmd/opengslb

        # Create config file
        @'
mode: agent

identity:
  service_token: ${var.service_token}
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
  ipv4_prefix: 24
  ipv6_prefix: 48
  min_connection_age: 5s
  max_subnets: 10000
  subnet_ttl: 24h
  min_samples: 3
  report_interval: 30s
  ewma_alpha: 0.3

gossip:
  encryption_key: ${var.gossip_encryption_key}
  overwatch_nodes:
    - 10.1.1.10:7946

heartbeat:
  interval: 10s

metrics:
  enabled: true
  address: ':9090'

logging:
  level: debug
  format: json
'@ | Out-File -FilePath 'C:\opengslb\agent.yaml' -Encoding utf8

        # Create Windows service (requires NSSM or similar)
        # For demo, run as scheduled task at startup
        $$action = New-ScheduledTaskAction -Execute 'C:\opengslb\opengslb.exe' -Argument 'agent --config C:\opengslb\agent.yaml'
        $$trigger = New-ScheduledTaskTrigger -AtStartup
        $$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
        Register-ScheduledTask -TaskName 'OpenGSLB Agent' -Action $$action -Trigger $$trigger -Principal $$principal

        # Start the agent
        Start-ScheduledTask -TaskName 'OpenGSLB Agent'
      "
    EOT
  })
}

# Backend VM (Southeast Asia)
resource "azurerm_linux_virtual_machine" "backend_southeastasia" {
  name                = "vm-backend-southeastasia"
  resource_group_name = azurerm_resource_group.main.name
  location            = "Southeast Asia"
  size                = var.vm_size
  admin_username      = var.admin_username
  tags                = var.tags

  network_interface_ids = [
    azurerm_network_interface.backend_southeastasia.id,
  ]

  admin_ssh_key {
    username   = var.admin_username
    public_key = file(var.ssh_public_key_path)
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

  custom_data = base64encode(templatefile("${path.module}/scripts/cloud-init-agent.yaml", {
    git_repo              = var.opengslb_git_repo
    git_branch            = var.opengslb_git_branch
    gossip_encryption_key = var.gossip_encryption_key
    service_token         = var.service_token
    region                = "ap-southeast"
    hostname              = "backend-southeastasia"
  }))
}

# Traffic Generator VM (Southeast Asia)
resource "azurerm_linux_virtual_machine" "traffic_southeastasia" {
  name                = "vm-traffic-southeastasia"
  resource_group_name = azurerm_resource_group.main.name
  location            = "Southeast Asia"
  size                = var.vm_size
  admin_username      = var.admin_username
  tags                = var.tags

  network_interface_ids = [
    azurerm_network_interface.traffic_southeastasia.id,
  ]

  admin_ssh_key {
    username   = var.admin_username
    public_key = file(var.ssh_public_key_path)
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

  custom_data = base64encode(templatefile("${path.module}/scripts/cloud-init-traffic.yaml", {
    region = "ap-southeast"
  }))
}
