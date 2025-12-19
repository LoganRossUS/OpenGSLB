# Copyright (C) 2025 Logan Ross
#
# This file is part of OpenGSLB – https://opengslb.org
#
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

# Virtual Networks

resource "azurerm_virtual_network" "eastus" {
  name                = "vnet-eastus"
  address_space       = ["10.1.0.0/16"]
  location            = "East US"
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags
}

resource "azurerm_virtual_network" "westeurope" {
  name                = "vnet-westeurope"
  address_space       = ["10.2.0.0/16"]
  location            = "West Europe"
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags
}

resource "azurerm_virtual_network" "southeastasia" {
  name                = "vnet-southeastasia"
  address_space       = ["10.3.0.0/16"]
  location            = "Southeast Asia"
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags
}

# Subnets - East US

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

# Subnets - West Europe

resource "azurerm_subnet" "backends_westeurope" {
  name                 = "subnet-backends"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.westeurope.name
  address_prefixes     = ["10.2.1.0/24"]
}

# Subnets - Southeast Asia

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

# VNet Peering - East US <-> West Europe

resource "azurerm_virtual_network_peering" "eastus_to_westeurope" {
  name                      = "eastus-to-westeurope"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.eastus.name
  remote_virtual_network_id = azurerm_virtual_network.westeurope.id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = false
}

resource "azurerm_virtual_network_peering" "westeurope_to_eastus" {
  name                      = "westeurope-to-eastus"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.westeurope.name
  remote_virtual_network_id = azurerm_virtual_network.eastus.id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = false
}

# VNet Peering - East US <-> Southeast Asia

resource "azurerm_virtual_network_peering" "eastus_to_southeastasia" {
  name                      = "eastus-to-southeastasia"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.eastus.name
  remote_virtual_network_id = azurerm_virtual_network.southeastasia.id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = false
}

resource "azurerm_virtual_network_peering" "southeastasia_to_eastus" {
  name                      = "southeastasia-to-eastus"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.southeastasia.name
  remote_virtual_network_id = azurerm_virtual_network.eastus.id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = false
}

# VNet Peering - West Europe <-> Southeast Asia

resource "azurerm_virtual_network_peering" "westeurope_to_southeastasia" {
  name                      = "westeurope-to-southeastasia"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.westeurope.name
  remote_virtual_network_id = azurerm_virtual_network.southeastasia.id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = false
}

resource "azurerm_virtual_network_peering" "southeastasia_to_westeurope" {
  name                      = "southeastasia-to-westeurope"
  resource_group_name       = azurerm_resource_group.main.name
  virtual_network_name      = azurerm_virtual_network.southeastasia.name
  remote_virtual_network_id = azurerm_virtual_network.westeurope.id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = false
}

# Public IPs

resource "azurerm_public_ip" "overwatch" {
  name                = "pip-overwatch"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = var.tags
}

resource "azurerm_public_ip" "traffic_eastus" {
  name                = "pip-traffic-eastus"
  resource_group_name = azurerm_resource_group.main.name
  location            = "East US"
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = var.tags
}

resource "azurerm_public_ip" "traffic_southeastasia" {
  name                = "pip-traffic-southeastasia"
  resource_group_name = azurerm_resource_group.main.name
  location            = "Southeast Asia"
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = var.tags
}

resource "azurerm_public_ip" "backend_westeurope" {
  name                = "pip-backend-westeurope"
  resource_group_name = azurerm_resource_group.main.name
  location            = "West Europe"
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = var.tags
}

resource "azurerm_public_ip" "backend_southeastasia" {
  name                = "pip-backend-southeastasia"
  resource_group_name = azurerm_resource_group.main.name
  location            = "Southeast Asia"
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = var.tags
}

# Network Interfaces

resource "azurerm_network_interface" "overwatch" {
  name                = "nic-overwatch"
  location            = "East US"
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags

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
  tags                = var.tags

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
  tags                = var.tags

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.backends_westeurope.id
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.2.1.10"
    public_ip_address_id          = azurerm_public_ip.backend_westeurope.id
  }
}

resource "azurerm_network_interface" "backend_westeurope_win" {
  name                = "nic-backend-westeurope-win"
  location            = "West Europe"
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags

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
  tags                = var.tags

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.backends_southeastasia.id
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.3.1.10"
    public_ip_address_id          = azurerm_public_ip.backend_southeastasia.id
  }
}

resource "azurerm_network_interface" "traffic_southeastasia" {
  name                = "nic-traffic-southeastasia"
  location            = "Southeast Asia"
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.traffic_southeastasia.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.traffic_southeastasia.id
  }
}
