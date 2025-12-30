# Copyright (C) 2025 Logan Ross
#
# This file is part of OpenGSLB – https://opengslb.org
#
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

# Virtual Machines - Simplified Deployment
#
# Uses bootstrap scripts from GitHub Releases instead of building from source.
# Deployment time reduced from ~15 minutes to ~2 minutes.

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

  custom_data = base64encode(<<-EOF
    #cloud-config
    runcmd:
      - curl -fsSL "${local.bootstrap_linux_url}" -o /tmp/bootstrap.sh
      - chmod +x /tmp/bootstrap.sh
      - /tmp/bootstrap.sh --role overwatch --region us-east --gossip-key "${local.gossip_key}" --service-token "${local.service_token}" --version "${local.version}" --github-repo "${local.github_repo}" --verbose 2>&1 | tee /var/log/opengslb-bootstrap.log
  EOF
  )
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

  custom_data = base64encode(<<-EOF
    #cloud-config
    packages:
      - curl
      - dnsutils
      - jq
      - tmux
      - bc
      - netcat-openbsd
    runcmd:
      # Download hey for load testing
      - curl -fsSL https://hey-release.s3.us-east-2.amazonaws.com/hey_linux_amd64 -o /usr/local/bin/hey
      - chmod +x /usr/local/bin/hey
      # Download validation script
      - curl -fsSL "https://github.com/${local.github_repo}/releases/download/${local.version}/validate-cluster.sh" -o /usr/local/bin/validate-cluster.sh || curl -fsSL "https://raw.githubusercontent.com/${local.github_repo}/main/scripts/validate-cluster.sh" -o /usr/local/bin/validate-cluster.sh
      - chmod +x /usr/local/bin/validate-cluster.sh
      # Create helper scripts
      - |
        cat > /usr/local/bin/test-cluster << 'SCRIPT'
        #!/bin/bash
        /usr/local/bin/validate-cluster.sh --overwatch-ip ${local.overwatch_ip} --expected-agents 2 "$@"
        SCRIPT
      - chmod +x /usr/local/bin/test-cluster
      - |
        cat > /usr/local/bin/generate-traffic << 'SCRIPT'
        #!/bin/bash
        # Generate traffic using hey with persistent connections for latency learning
        # Usage: generate-traffic [rate_per_second] [duration_seconds]
        RATE=$${1:-5}
        DURATION=$${2:-60}
        OVERWATCH_IP="${local.overwatch_ip}"

        # Resolve backend IPs via DNS
        echo "Resolving backends via DNS..."
        BACKEND1=$(dig @$OVERWATCH_IP web.test.opengslb.local +short | head -1)
        BACKEND2=$(dig @$OVERWATCH_IP web.test.opengslb.local +short | tail -1)

        if [ -z "$BACKEND1" ]; then
          echo "ERROR: Could not resolve any backends from DNS"
          exit 1
        fi

        echo "Generating traffic at $RATE req/s for $DURATION seconds..."
        echo "Using backends: $BACKEND1 $BACKEND2"
        echo ""

        # Use hey with persistent connections (keep-alive enabled by default)
        # -z duration, -q rate limit, -c concurrent connections
        for IP in $BACKEND1 $BACKEND2; do
          if [ -n "$IP" ]; then
            echo "Starting load to $IP..."
            hey -z $${DURATION}s -q $RATE -c 5 -disable-compression "http://$IP/" &
          fi
        done

        wait
        echo ""
        echo "Done. Check latency data with: curl http://$OVERWATCH_IP:8080/api/v1/overwatch/latency | jq ."
        SCRIPT
      - chmod +x /usr/local/bin/generate-traffic
      - |
        cat > /usr/local/bin/show-latency << 'SCRIPT'
        #!/bin/bash
        curl -s "http://${local.overwatch_ip}:8080/api/v1/overwatch/latency" | jq .
        SCRIPT
      - chmod +x /usr/local/bin/show-latency
      - |
        cat > /usr/local/bin/show-backends << 'SCRIPT'
        #!/bin/bash
        curl -s "http://${local.overwatch_ip}:8080/api/v1/overwatch/backends" | jq .
        SCRIPT
      - chmod +x /usr/local/bin/show-backends
      - echo "Traffic generator ready. Commands: generate-traffic [rate] [duration], show-latency, show-backends, test-cluster" > /etc/motd
  EOF
  )
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

  custom_data = base64encode(<<-EOF
    #cloud-config
    packages:
      - nginx
    runcmd:
      # Configure nginx with region identifier
      - |
        cat > /var/www/html/index.html << 'HTML'
        <!DOCTYPE html>
        <html>
        <head><title>OpenGSLB Backend</title></head>
        <body>
        <h1>OpenGSLB Backend</h1>
        <p>Region: eu-west</p>
        <p>Hostname: backend-westeurope</p>
        </body>
        </html>
        HTML
      - systemctl enable nginx
      - systemctl start nginx
      # Download and run bootstrap script
      - curl -fsSL "${local.bootstrap_linux_url}" -o /tmp/bootstrap.sh
      - chmod +x /tmp/bootstrap.sh
      - /tmp/bootstrap.sh --role agent --overwatch-ip ${local.overwatch_ip} --region eu-west --gossip-key "${local.gossip_key}" --service-token "${local.service_token}" --service-name web --backend-port 80 --version "${local.version}" --github-repo "${local.github_repo}" --verbose 2>&1 | tee /var/log/opengslb-bootstrap.log
  EOF
  )
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

  custom_data = base64encode(<<-EOF
    #cloud-config
    packages:
      - nginx
    runcmd:
      # Configure nginx with region identifier
      - |
        cat > /var/www/html/index.html << 'HTML'
        <!DOCTYPE html>
        <html>
        <head><title>OpenGSLB Backend</title></head>
        <body>
        <h1>OpenGSLB Backend</h1>
        <p>Region: ap-southeast</p>
        <p>Hostname: backend-southeastasia</p>
        </body>
        </html>
        HTML
      - systemctl enable nginx
      - systemctl start nginx
      # Download and run bootstrap script
      - curl -fsSL "${local.bootstrap_linux_url}" -o /tmp/bootstrap.sh
      - chmod +x /tmp/bootstrap.sh
      - /tmp/bootstrap.sh --role agent --overwatch-ip ${local.overwatch_ip} --region ap-southeast --gossip-key "${local.gossip_key}" --service-token "${local.service_token}" --service-name web --backend-port 80 --version "${local.version}" --github-repo "${local.github_repo}" --verbose 2>&1 | tee /var/log/opengslb-bootstrap.log
  EOF
  )
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

  custom_data = base64encode(<<-EOF
    #cloud-config
    packages:
      - curl
      - dnsutils
      - jq
      - tmux
      - bc
      - netcat-openbsd
    runcmd:
      # Download hey for load testing
      - curl -fsSL https://hey-release.s3.us-east-2.amazonaws.com/hey_linux_amd64 -o /usr/local/bin/hey
      - chmod +x /usr/local/bin/hey
      # Download validation script
      - curl -fsSL "https://github.com/${local.github_repo}/releases/download/${local.version}/validate-cluster.sh" -o /usr/local/bin/validate-cluster.sh || curl -fsSL "https://raw.githubusercontent.com/${local.github_repo}/main/scripts/validate-cluster.sh" -o /usr/local/bin/validate-cluster.sh
      - chmod +x /usr/local/bin/validate-cluster.sh
      # Create helper scripts
      - |
        cat > /usr/local/bin/test-cluster << 'SCRIPT'
        #!/bin/bash
        /usr/local/bin/validate-cluster.sh --overwatch-ip ${local.overwatch_ip} --expected-agents 2 "$@"
        SCRIPT
      - chmod +x /usr/local/bin/test-cluster
      - |
        cat > /usr/local/bin/generate-traffic << 'SCRIPT'
        #!/bin/bash
        # Generate traffic using hey with persistent connections for latency learning
        # Usage: generate-traffic [rate_per_second] [duration_seconds]
        RATE=$${1:-5}
        DURATION=$${2:-60}
        OVERWATCH_IP="${local.overwatch_ip}"

        # Resolve backend IPs via DNS
        echo "Resolving backends via DNS..."
        BACKEND1=$(dig @$OVERWATCH_IP web.test.opengslb.local +short | head -1)
        BACKEND2=$(dig @$OVERWATCH_IP web.test.opengslb.local +short | tail -1)

        if [ -z "$BACKEND1" ]; then
          echo "ERROR: Could not resolve any backends from DNS"
          exit 1
        fi

        echo "Generating traffic at $RATE req/s for $DURATION seconds..."
        echo "Using backends: $BACKEND1 $BACKEND2"
        echo ""

        # Use hey with persistent connections (keep-alive enabled by default)
        # -z duration, -q rate limit, -c concurrent connections
        for IP in $BACKEND1 $BACKEND2; do
          if [ -n "$IP" ]; then
            echo "Starting load to $IP..."
            hey -z $${DURATION}s -q $RATE -c 5 -disable-compression "http://$IP/" &
          fi
        done

        wait
        echo ""
        echo "Done. Check latency data with: curl http://$OVERWATCH_IP:8080/api/v1/overwatch/latency | jq ."
        SCRIPT
      - chmod +x /usr/local/bin/generate-traffic
      - |
        cat > /usr/local/bin/show-latency << 'SCRIPT'
        #!/bin/bash
        curl -s "http://${local.overwatch_ip}:8080/api/v1/overwatch/latency" | jq .
        SCRIPT
      - chmod +x /usr/local/bin/show-latency
      - |
        cat > /usr/local/bin/show-backends << 'SCRIPT'
        #!/bin/bash
        curl -s "http://${local.overwatch_ip}:8080/api/v1/overwatch/backends" | jq .
        SCRIPT
      - chmod +x /usr/local/bin/show-backends
      - echo "Traffic generator ready. Commands: generate-traffic [rate] [duration], show-latency, show-backends, test-cluster" > /etc/motd
  EOF
  )
}
