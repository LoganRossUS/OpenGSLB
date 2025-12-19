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
# Uses a multi-step approach: write script to file, then execute
resource "azurerm_virtual_machine_extension" "backend_win_setup" {
  name                 = "setup-opengslb"
  virtual_machine_id   = azurerm_windows_virtual_machine.backend_westeurope_win.id
  publisher            = "Microsoft.Compute"
  type                 = "CustomScriptExtension"
  type_handler_version = "1.10"

  settings = jsonencode({
    commandToExecute = join(" && ", [
      "mkdir C:\\opengslb 2>nul",
      "powershell -ExecutionPolicy Bypass -Command \"Start-Transcript -Path C:\\opengslb-setup.log; Write-Host 'Starting setup...'\"",
      "powershell -ExecutionPolicy Bypass -Command \"Install-WindowsFeature -Name Web-Server -IncludeManagementTools\"",
      "powershell -ExecutionPolicy Bypass -Command \"[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))\"",
      "powershell -ExecutionPolicy Bypass -Command \"& 'C:\\ProgramData\\chocolatey\\bin\\choco.exe' install git golang -y --no-progress\"",
      "powershell -ExecutionPolicy Bypass -Command \"& 'C:\\Program Files\\Git\\bin\\git.exe' clone --depth 1 --branch ${var.opengslb_git_branch} ${var.opengslb_git_repo} C:\\opengslb\\src\"",
      "powershell -ExecutionPolicy Bypass -Command \"$env:GOPATH='C:\\Users\\${var.admin_username}\\go'; $env:GOMODCACHE='C:\\Users\\${var.admin_username}\\go\\pkg\\mod'; New-Item -Path $env:GOPATH -ItemType Directory -Force; New-Item -Path $env:GOMODCACHE -ItemType Directory -Force; cd C:\\opengslb\\src; & 'C:\\Program Files\\Go\\bin\\go.exe' build -o C:\\opengslb\\opengslb.exe ./cmd/opengslb\"",
      "powershell -ExecutionPolicy Bypass -Command \"'mode: agent`nidentity:`n  service_token: ${var.service_token}`n  region: eu-west`nbackends:`n  - service: web`n    address: 0.0.0.0`n    port: 80`n    weight: 100`n    health_check:`n      type: http`n      path: /`n      interval: 5s`n      timeout: 2s`nlatency_learning:`n  enabled: true`n  poll_interval: 10s`n  ipv4_prefix: 24`n  ipv6_prefix: 48`n  min_connection_age: 5s`n  max_subnets: 10000`n  subnet_ttl: 24h`n  min_samples: 3`n  report_interval: 30s`n  ewma_alpha: 0.3`ngossip:`n  encryption_key: ${var.gossip_encryption_key}`n  overwatch_nodes:`n    - 10.1.1.10:7946`nheartbeat:`n  interval: 10s`nmetrics:`n  enabled: true`n  address: :9090`nlogging:`n  level: debug`n  format: json' | Out-File -Encoding utf8 C:\\opengslb\\agent.yaml\"",
      "powershell -ExecutionPolicy Bypass -Command \"$a = New-ScheduledTaskAction -Execute 'C:\\opengslb\\opengslb.exe' -Argument '--mode agent --config C:\\opengslb\\agent.yaml'; $t = New-ScheduledTaskTrigger -AtStartup; $p = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest; Register-ScheduledTask -TaskName 'OpenGSLB Agent' -Action $a -Trigger $t -Principal $p -Force; Start-ScheduledTask -TaskName 'OpenGSLB Agent'\"",
      "powershell -ExecutionPolicy Bypass -Command \"Write-Host 'Setup complete'; Stop-Transcript\""
    ])
  })

  timeouts {
    create = "60m"
  }
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
