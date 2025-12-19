# Copyright (C) 2025 Logan Ross
#
# This file is part of OpenGSLB – https://opengslb.org
#
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

variable "resource_group_name" {
  description = "Name of the Azure resource group"
  type        = string
  default     = "rg-opengslb-latency-test"
}

variable "admin_username" {
  description = "Admin username for VMs"
  type        = string
  default     = "azureuser"
}

variable "ssh_public_key_path" {
  description = "Path to SSH public key for Linux VM authentication"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "windows_admin_password" {
  description = "Admin password for Windows VM (must be complex)"
  type        = string
  sensitive   = true
}

variable "vm_size" {
  description = "Azure VM size for all VMs"
  type        = string
  default     = "Standard_B2s"
}

variable "opengslb_git_repo" {
  description = "Git repository URL for OpenGSLB"
  type        = string
  default     = "https://github.com/LoganRossUS/OpenGSLB.git"
}

variable "opengslb_git_branch" {
  description = "Git branch to build from"
  type        = string
  default     = "main"
}

variable "gossip_encryption_key" {
  description = "Base64-encoded 32-byte encryption key for gossip"
  type        = string
  default     = "dGVzdC1lbmNyeXB0aW9uLWtleS0zMi1ieXRlcw=="
}

variable "service_token" {
  description = "Token for agent authentication to Overwatch"
  type        = string
  default     = "test-token-for-latency-testing"
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default = {
    project = "OpenGSLB"
    purpose = "ADR-017-Testing"
    demo    = "passive-latency-learning"
  }
}
