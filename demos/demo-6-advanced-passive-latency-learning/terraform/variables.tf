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
  description = "Path to SSH public key for Linux VM authentication (Azure only supports RSA keys, not ed25519)"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "vm_size" {
  description = "Azure VM size for all VMs"
  type        = string
  default     = "Standard_B2s"
}

variable "opengslb_version" {
  description = "OpenGSLB version to deploy (e.g., 'v0.1.0' or 'latest')"
  type        = string
  default     = "latest"
}

variable "opengslb_github_repo" {
  description = "GitHub repository for OpenGSLB (owner/repo format)"
  type        = string
  default     = "LoganRossUS/OpenGSLB"
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

variable "enable_bastion" {
  description = "Enable Azure Bastion for secure VM access without public SSH/RDP"
  type        = bool
  default     = false
}

variable "bastion_sku" {
  description = "Azure Bastion SKU (Basic or Standard)"
  type        = string
  default     = "Basic"
}
