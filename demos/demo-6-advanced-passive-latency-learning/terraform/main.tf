# Copyright (C) 2025 Logan Ross
#
# This file is part of OpenGSLB – https://opengslb.org
#
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

# ADR-017 Azure Test Environment for Passive Latency Learning
#
# This Terraform configuration deploys a multi-region Azure environment
# for testing the Passive Latency Learning feature.
#
# SIMPLIFIED DEPLOYMENT (v2):
# - Downloads pre-built binaries from GitHub Releases
# - Uses bootstrap scripts for configuration
# - Generates random secrets automatically
# - Validates cluster health after deployment
#
# Usage:
#   terraform init
#   terraform apply
#
# After deployment (~2 minutes instead of ~15 minutes):
#   1. Overwatch and agents start automatically
#   2. Run validate-cluster.sh to verify deployment
#   3. Generate traffic to test latency learning

terraform {
  required_version = ">= 1.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
    random = {
      source  = "hashicorp/random"
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

# Generate random gossip encryption key (32 bytes, base64 encoded)
resource "random_bytes" "gossip_key" {
  length = 32
}

# Generate random service token for agent authentication
resource "random_password" "service_token" {
  length  = 32
  special = false
}

# Resource Group
resource "azurerm_resource_group" "main" {
  name     = var.resource_group_name
  location = "East US"
  tags     = var.tags
}

# Local values for bootstrap parameters
locals {
  # Bootstrap script URL from main branch
  bootstrap_linux_url = "https://raw.githubusercontent.com/${var.opengslb_github_repo}/main/scripts/bootstrap-linux.sh"

  # Overwatch IP (first IP in overwatch subnet)
  overwatch_ip = "10.1.1.10"

  # Common bootstrap parameters
  gossip_key    = random_bytes.gossip_key.base64
  service_token = random_password.service_token.result
  version       = var.opengslb_version
  github_repo   = var.opengslb_github_repo
}
