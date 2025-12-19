# Copyright (C) 2025 Logan Ross
#
# This file is part of OpenGSLB – https://opengslb.org
#
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

# ADR-017 Azure Test Environment for Passive Latency Learning
#
# This Terraform configuration deploys a multi-region Azure environment
# for testing the Passive Latency Learning feature. All VMs are provisioned
# with cloud-init scripts that automatically build and start OpenGSLB.
#
# Usage:
#   terraform init
#   terraform apply -var="windows_admin_password=YourComplexPassword123!"
#
# After deployment, VMs will automatically:
#   1. Install Go and build dependencies
#   2. Clone and build OpenGSLB from source
#   3. Configure and start the appropriate service (Overwatch or Agent)

terraform {
  required_version = ">= 1.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {
    virtual_machine {
      delete_os_disk_on_deletion     = true
      graceful_shutdown              = false
      skip_shutdown_and_force_delete = false
    }
  }
}

# Resource Group
resource "azurerm_resource_group" "main" {
  name     = var.resource_group_name
  location = "East US"
  tags     = var.tags
}
