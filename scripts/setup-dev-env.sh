#!/bin/bash
set -e

echo "=== OpenGSLB Development Environment Setup ==="
echo "Target: Pop!_OS / Ubuntu"
echo ""

# Update package list
sudo apt update

# Install Docker
if ! command -v docker &> /dev/null; then
    echo "Installing Docker..."
    sudo apt install -y apt-transport-https ca-certificates curl gnupg lsb-release
    
    # Add Docker's official GPG key
    sudo install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    sudo chmod a+r /etc/apt/keyrings/docker.gpg
    
    # Set up repository (use Ubuntu jammy as base for Pop!_OS)
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      jammy stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    
    sudo apt update
    sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    
    # Add current user to docker group
    sudo usermod -aG docker $USER
    echo "NOTE: Log out and back in for docker group to take effect"
else
    echo "Docker already installed: $(docker --version)"
fi

# Install Go (if not present)
if ! command -v go &> /dev/null; then
    echo "Installing Go 1.22..."
    wget -q https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
    rm go1.22.4.linux-amd64.tar.gz
    
    # Add to path if not already there
    if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
        echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
    fi
    export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
else
    echo "Go already installed: $(go version)"
fi

# Install golangci-lint
if ! command -v golangci-lint &> /dev/null; then
    echo "Installing golangci-lint..."
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin latest
else
    echo "golangci-lint already installed: $(golangci-lint --version)"
fi

# Install make (usually present but just in case)
if ! command -v make &> /dev/null; then
    echo "Installing make..."
    sudo apt install -y build-essential
else
    echo "make already installed"
fi

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Installed versions:"
docker --version 2>/dev/null || echo "Docker: requires logout/login"
docker compose version 2>/dev/null || echo "Docker Compose: requires logout/login"
go version 2>/dev/null || echo "Go: restart shell"
golangci-lint --version 2>/dev/null || echo "golangci-lint: restart shell"
echo ""
echo "ACTION REQUIRED: Log out and back in for docker group permissions"
