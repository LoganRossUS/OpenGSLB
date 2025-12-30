#!/bin/bash
# OpenGSLB Bootstrap Script for Linux
# This script downloads and configures OpenGSLB for automated deployments.
#
# Usage:
#   curl -sL https://github.com/OWNER/OpenGSLB/releases/download/VERSION/bootstrap-linux.sh | \
#     sudo bash -s -- --role ROLE --overwatch-ip IP [OPTIONS]
#
# Required Arguments:
#   --role              Role: 'overwatch' or 'agent'
#   --overwatch-ip      IP address of the Overwatch node (required for agents)
#
# Optional Arguments:
#   --version           OpenGSLB version to install (default: latest)
#   --region            Region identifier (e.g., 'us-east', 'eu-west')
#   --service-token     Service token for agent authentication
#   --gossip-key        Base64-encoded 32-byte gossip encryption key
#   --service-name      Service name for backend registration (agents only)
#   --backend-port      Backend port (agents only, default: 80)
#   --node-id           Node identifier (default: hostname)
#   --github-repo       GitHub repository (default: LoganRossUS/OpenGSLB)
#   --skip-start        Don't start the service after installation
#   --verbose           Enable verbose output
#
# Exit codes:
#   0 - Success
#   1 - Invalid arguments
#   2 - Download failed
#   3 - Configuration failed
#   4 - Service start failed
#   5 - Health check failed

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Defaults
VERSION="latest"
ROLE=""
OVERWATCH_IP=""
REGION=""
SERVICE_TOKEN=""
GOSSIP_KEY=""
SERVICE_NAME="web"
BACKEND_PORT=80
NODE_ID=""
GITHUB_REPO="LoganRossUS/OpenGSLB"
SKIP_START=false
VERBOSE=false

# Installation paths
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/opengslb"
LOG_DIR="/var/log/opengslb"
BINARY_NAME="opengslb"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_debug() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "[DEBUG] $1"
    fi
}

log_section() {
    echo ""
    echo "================================================================================"
    echo " $1"
    echo "================================================================================"
}

# Print verbose debug information for troubleshooting
print_debug_info() {
    log_section "DEBUG INFORMATION"
    echo "Timestamp:        $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "Hostname:         $(hostname)"
    echo "OS:               $(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d= -f2 | tr -d '"' || echo 'Unknown')"
    echo "Kernel:           $(uname -r)"
    echo "Architecture:     $(uname -m)"
    echo ""
    echo "Network Interfaces:"
    ip -4 addr show 2>/dev/null | grep -E "inet|^[0-9]:" | head -20 || echo "  Unable to get network info"
    echo ""
    echo "Configuration:"
    echo "  Role:           $ROLE"
    echo "  Version:        $VERSION"
    echo "  Overwatch IP:   $OVERWATCH_IP"
    echo "  Region:         $REGION"
    echo "  Node ID:        $NODE_ID"
    echo "  Service Name:   $SERVICE_NAME"
    echo "  Backend Port:   $BACKEND_PORT"
    echo "  GitHub Repo:    $GITHUB_REPO"
    echo ""
}

# Parse command-line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --role)
                ROLE="$2"
                shift 2
                ;;
            --overwatch-ip)
                OVERWATCH_IP="$2"
                shift 2
                ;;
            --version)
                VERSION="$2"
                shift 2
                ;;
            --region)
                REGION="$2"
                shift 2
                ;;
            --service-token)
                SERVICE_TOKEN="$2"
                shift 2
                ;;
            --gossip-key)
                GOSSIP_KEY="$2"
                shift 2
                ;;
            --service-name)
                SERVICE_NAME="$2"
                shift 2
                ;;
            --backend-port)
                BACKEND_PORT="$2"
                shift 2
                ;;
            --node-id)
                NODE_ID="$2"
                shift 2
                ;;
            --github-repo)
                GITHUB_REPO="$2"
                shift 2
                ;;
            --skip-start)
                SKIP_START=true
                shift
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            --help|-h)
                echo "Usage: $0 --role ROLE --overwatch-ip IP [OPTIONS]"
                echo ""
                echo "Run with --verbose for detailed output."
                exit 0
                ;;
            *)
                log_error "Unknown argument: $1"
                exit 1
                ;;
        esac
    done
}

# Validate required arguments
validate_args() {
    log_section "Validating Arguments"

    if [[ -z "$ROLE" ]]; then
        log_error "Missing required argument: --role"
        log_error "Role must be 'overwatch' or 'agent'"
        exit 1
    fi

    if [[ "$ROLE" != "overwatch" && "$ROLE" != "agent" ]]; then
        log_error "Invalid role: $ROLE"
        log_error "Role must be 'overwatch' or 'agent'"
        exit 1
    fi

    if [[ "$ROLE" == "agent" && -z "$OVERWATCH_IP" ]]; then
        log_error "Missing required argument for agent: --overwatch-ip"
        exit 1
    fi

    # Set defaults
    if [[ -z "$NODE_ID" ]]; then
        NODE_ID=$(hostname)
    fi

    # Generate random gossip key if not provided
    if [[ -z "$GOSSIP_KEY" ]]; then
        log_warn "No gossip key provided, generating random key"
        GOSSIP_KEY=$(openssl rand -base64 32)
    fi

    # Generate random service token if not provided (for agents)
    if [[ "$ROLE" == "agent" && -z "$SERVICE_TOKEN" ]]; then
        log_warn "No service token provided, generating random token"
        SERVICE_TOKEN=$(openssl rand -base64 32 | tr -d '=' | head -c 32)
    fi

    log_success "Arguments validated"
    log_debug "Role: $ROLE, Node ID: $NODE_ID, Overwatch IP: $OVERWATCH_IP"
}

# Discover local IP address
# Note: All log output goes to stderr so only the IP is captured when called as $(discover_local_ip)
discover_local_ip() {
    log_section "Discovering Local IP Address" >&2

    local ip=""

    # Try Azure Instance Metadata Service first
    log_debug "Trying Azure IMDS..." >&2
    ip=$(curl -s -H "Metadata: true" --connect-timeout 2 \
        "http://169.254.169.254/metadata/instance/network/interface/0/ipv4/ipAddress/0/privateIpAddress?api-version=2021-02-01&format=text" 2>/dev/null || true)

    if [[ -n "$ip" && "$ip" != "null" ]]; then
        log_success "Discovered IP from Azure IMDS: $ip" >&2
        echo "$ip"
        return
    fi

    # Try AWS Instance Metadata Service
    log_debug "Trying AWS IMDS..." >&2
    ip=$(curl -s --connect-timeout 2 "http://169.254.169.254/latest/meta-data/local-ipv4" 2>/dev/null || true)

    if [[ -n "$ip" && "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        log_success "Discovered IP from AWS IMDS: $ip" >&2
        echo "$ip"
        return
    fi

    # Try GCP Instance Metadata Service
    log_debug "Trying GCP IMDS..." >&2
    ip=$(curl -s -H "Metadata-Flavor: Google" --connect-timeout 2 \
        "http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/ip" 2>/dev/null || true)

    if [[ -n "$ip" && "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        log_success "Discovered IP from GCP IMDS: $ip" >&2
        echo "$ip"
        return
    fi

    # Fallback: get IP from primary interface
    log_debug "Falling back to interface discovery..." >&2
    ip=$(ip -4 addr show scope global | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1)

    if [[ -n "$ip" ]]; then
        log_success "Discovered IP from interface: $ip" >&2
        echo "$ip"
        return
    fi

    log_error "Failed to discover local IP address" >&2
    log_error "" >&2
    log_error "Network interfaces:" >&2
    ip addr show 2>/dev/null || ifconfig 2>/dev/null || echo "  Unable to list interfaces" >&2
    exit 3
}

# Download the binary
download_binary() {
    log_section "Downloading OpenGSLB Binary"

    local download_url

    if [[ "$VERSION" == "latest" ]]; then
        log_info "Fetching latest release version..."
        VERSION=$(curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ -z "$VERSION" ]]; then
            log_error "Failed to fetch latest version from GitHub"
            log_error ""
            log_error "Possible causes:"
            log_error "  1. GitHub API rate limit exceeded"
            log_error "  2. Repository does not exist or is private"
            log_error "  3. No releases have been published"
            log_error ""
            log_error "Try specifying a version explicitly: --version v0.1.0"
            exit 2
        fi
        log_info "Latest version: $VERSION"
    fi

    download_url="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/opengslb-linux-amd64"

    log_info "Downloading from: $download_url"

    mkdir -p "$INSTALL_DIR"

    if ! curl -fsSL "$download_url" -o "${INSTALL_DIR}/${BINARY_NAME}"; then
        log_error "Failed to download binary"
        log_error ""
        log_error "Possible causes:"
        log_error "  1. Version $VERSION does not exist"
        log_error "  2. Release does not include linux-amd64 binary"
        log_error "  3. Network connectivity issues"
        log_error ""
        log_error "Available releases:"
        curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases" | grep '"tag_name"' | head -5 | sed 's/.*"\(v[^"]*\)".*/  - \1/' || echo "  Unable to fetch releases"
        exit 2
    fi

    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

    # Verify binary works
    if ! "${INSTALL_DIR}/${BINARY_NAME}" --version >/dev/null 2>&1; then
        log_error "Downloaded binary is not executable or corrupt"
        log_error ""
        log_error "Binary info:"
        file "${INSTALL_DIR}/${BINARY_NAME}" || echo "  file command not available"
        exit 2
    fi

    local installed_version
    installed_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>&1 | head -1)
    log_success "Installed: $installed_version"
}

# Create configuration directory and file
create_config() {
    log_section "Creating Configuration"

    mkdir -p "$CONFIG_DIR"
    mkdir -p "$LOG_DIR"

    local local_ip
    local_ip=$(discover_local_ip)

    local config_file="${CONFIG_DIR}/config.yaml"

    if [[ "$ROLE" == "overwatch" ]]; then
        log_info "Generating Overwatch configuration..."
        cat > "$config_file" << EOF
# OpenGSLB Overwatch Configuration
# Generated by bootstrap script at $(date -u +"%Y-%m-%dT%H:%M:%SZ")

mode: overwatch

logging:
  level: info
  format: json

dns:
  listen_address: "0.0.0.0:53"
  zone: "test.opengslb.local"

api:
  enabled: true
  address: "0.0.0.0:8080"
  allowed_networks:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
    - "127.0.0.0/8"

metrics:
  enabled: true
  address: "0.0.0.0:9090"

overwatch:
  identity:
    node_id: "${NODE_ID}"
    region: "${REGION:-default}"
  gossip:
    bind_address: "0.0.0.0:7946"
    encryption_key: "${GOSSIP_KEY}"
  routing:
    algorithm: latency
    fallback: geo

domains:
  - name: "${SERVICE_NAME}.test.opengslb.local"
    ttl: 60
    routing_algorithm: learned_latency  # ADR-017: Use passive TCP RTT learning

regions:
  - id: "us-east"
    name: "US East"
  - id: "eu-west"
    name: "EU West"
  - id: "ap-southeast"
    name: "Asia Pacific Southeast"
EOF
    else
        # Agent configuration
        log_info "Generating Agent configuration..."
        cat > "$config_file" << EOF
# OpenGSLB Agent Configuration
# Generated by bootstrap script at $(date -u +"%Y-%m-%dT%H:%M:%SZ")

mode: agent

logging:
  level: info
  format: json

agent:
  identity:
    agent_id: "${NODE_ID}"
    region: "${REGION:-default}"
    service_token: "${SERVICE_TOKEN}"
  backends:
    - service: "${SERVICE_NAME}.test.opengslb.local"
      address: "${local_ip}"
      port: ${BACKEND_PORT}
      weight: 100
      health_check:
        type: http
        path: /
        interval: 10s
        timeout: 5s
  gossip:
    overwatch_nodes:
      - "${OVERWATCH_IP}:7946"
    encryption_key: "${GOSSIP_KEY}"
  latency_learning:
    enabled: true
    poll_interval: 10s
    ipv4_prefix: 24
    min_connection_age: 5s
    max_subnets: 10000
    subnet_ttl: 168h
    min_samples: 3
    report_interval: 30s
    ewma_alpha: 0.3

metrics:
  enabled: true
  address: "0.0.0.0:9090"
EOF
    fi

    # Secure the config file (contains secrets)
    chmod 600 "$config_file"

    log_success "Configuration created: $config_file"
}

# Create systemd service
create_service() {
    log_section "Creating Systemd Service"

    local service_name="opengslb-${ROLE}"
    local service_file="/etc/systemd/system/${service_name}.service"

    cat > "$service_file" << EOF
[Unit]
Description=OpenGSLB ${ROLE^}
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} --config ${CONFIG_DIR}/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
ReadOnlyPaths=${CONFIG_DIR}
ReadWritePaths=${LOG_DIR}

[Install]
WantedBy=multi-user.target
EOF

    # Set capabilities based on role
    if [[ "$ROLE" == "overwatch" ]]; then
        # Overwatch needs to bind to port 53
        setcap 'cap_net_bind_service=+ep' "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || {
            log_warn "Failed to set cap_net_bind_service capability"
            log_warn "DNS server may not be able to bind to port 53 without root"
        }

        # Stop systemd-resolved if it's using port 53
        if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
            log_info "Stopping systemd-resolved to free port 53..."
            systemctl stop systemd-resolved 2>/dev/null || true
            systemctl disable systemd-resolved 2>/dev/null || true

            # Update resolv.conf to use external DNS
            cat > /etc/resolv.conf << EOF
# Generated by OpenGSLB bootstrap
nameserver 8.8.8.8
nameserver 8.8.4.4
EOF
        fi
    else
        # Agent needs cap_net_admin for reading TCP stats
        setcap 'cap_net_admin=+ep' "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || {
            log_warn "Failed to set cap_net_admin capability"
            log_warn "Latency learning may not work without root"
        }
    fi

    systemctl daemon-reload
    systemctl enable "${service_name}" 2>/dev/null || true

    log_success "Service created: ${service_name}"
}

# Start the service
start_service() {
    log_section "Starting Service"

    if [[ "$SKIP_START" == "true" ]]; then
        log_info "Skipping service start (--skip-start flag)"
        return
    fi

    local service_name="opengslb-${ROLE}"

    log_info "Starting ${service_name}..."

    if ! systemctl start "${service_name}"; then
        log_error "Failed to start service"
        log_error ""
        log_error "Service status:"
        systemctl status "${service_name}" --no-pager || true
        log_error ""
        log_error "Recent logs:"
        journalctl -u "${service_name}" --no-pager -n 50 || true
        exit 4
    fi

    # Wait a moment for service to initialize
    sleep 2

    if ! systemctl is-active --quiet "${service_name}"; then
        log_error "Service started but is not running"
        log_error ""
        log_error "Service status:"
        systemctl status "${service_name}" --no-pager || true
        log_error ""
        log_error "Recent logs:"
        journalctl -u "${service_name}" --no-pager -n 50 || true
        exit 4
    fi

    log_success "Service started successfully"
}

# Run health check
run_health_check() {
    log_section "Running Health Check"

    if [[ "$SKIP_START" == "true" ]]; then
        log_info "Skipping health check (service not started)"
        return
    fi

    local metrics_port=9090
    local max_attempts=30
    local attempt=1

    log_info "Waiting for metrics endpoint to be ready..."

    while [[ $attempt -le $max_attempts ]]; do
        if curl -sf "http://localhost:${metrics_port}/metrics" >/dev/null 2>&1; then
            log_success "Metrics endpoint responding on port ${metrics_port}"
            break
        fi

        if [[ $attempt -eq $max_attempts ]]; then
            log_error "Health check failed after ${max_attempts} attempts"
            log_error ""
            log_error "Debugging information:"
            log_error ""
            log_error "Service status:"
            systemctl status "opengslb-${ROLE}" --no-pager 2>&1 || true
            log_error ""
            log_error "Listening ports:"
            ss -tlnp 2>/dev/null | grep -E "(9090|8080|53|7946)" || netstat -tlnp 2>/dev/null | grep -E "(9090|8080|53|7946)" || true
            log_error ""
            log_error "Recent logs:"
            journalctl -u "opengslb-${ROLE}" --no-pager -n 100 || true
            log_error ""
            log_error "Configuration file:"
            cat "${CONFIG_DIR}/config.yaml" | grep -v -E "(encryption_key|service_token)" || true
            exit 5
        fi

        log_debug "Attempt $attempt/$max_attempts - waiting..."
        sleep 1
        ((attempt++))
    done

    # Role-specific checks
    if [[ "$ROLE" == "overwatch" ]]; then
        # Check DNS port
        if ! ss -tlnp 2>/dev/null | grep -q ":53 " && ! netstat -tlnp 2>/dev/null | grep -q ":53 "; then
            log_warn "DNS server may not be listening on port 53"
        else
            log_success "DNS server listening on port 53"
        fi

        # Check API port
        if curl -sf "http://localhost:8080/api/v1/cluster/status" >/dev/null 2>&1; then
            log_success "API server responding on port 8080"
        else
            log_warn "API server not responding on port 8080"
        fi
    else
        # Agent: check gossip connectivity
        log_info "Verifying gossip connectivity to Overwatch..."

        local gossip_check_attempts=10
        local gossip_attempt=1

        while [[ $gossip_attempt -le $gossip_check_attempts ]]; do
            # Check if we can reach the overwatch gossip port
            if nc -z -w 2 "${OVERWATCH_IP}" 7946 2>/dev/null; then
                log_success "Gossip port reachable at ${OVERWATCH_IP}:7946"
                break
            fi

            if [[ $gossip_attempt -eq $gossip_check_attempts ]]; then
                log_warn "Cannot reach Overwatch gossip port at ${OVERWATCH_IP}:7946"
                log_warn "This may be normal if Overwatch is still starting"
            fi

            sleep 1
            ((gossip_attempt++))
        done
    fi
}

# Write completion marker
write_completion_marker() {
    local marker_file="/var/run/opengslb-ready"
    echo "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" > "$marker_file"
    log_success "Completion marker written: $marker_file"
}

# Print summary
print_summary() {
    log_section "Installation Complete"

    echo ""
    echo "OpenGSLB ${ROLE^} has been installed and configured."
    echo ""
    echo "Details:"
    echo "  Binary:          ${INSTALL_DIR}/${BINARY_NAME}"
    echo "  Configuration:   ${CONFIG_DIR}/config.yaml"
    echo "  Service:         opengslb-${ROLE}"
    echo "  Logs:            journalctl -u opengslb-${ROLE} -f"
    echo ""

    if [[ "$ROLE" == "overwatch" ]]; then
        echo "Endpoints:"
        echo "  DNS:             0.0.0.0:53"
        echo "  API:             http://localhost:8080/api/v1/"
        echo "  Metrics:         http://localhost:9090/metrics"
        echo "  Cluster Status:  http://localhost:8080/api/v1/cluster/status"
        echo ""
        echo "Test DNS:"
        echo "  dig @localhost ${SERVICE_NAME}.test.opengslb.local A"
        echo ""
    else
        echo "Endpoints:"
        echo "  Metrics:         http://localhost:9090/metrics"
        echo ""
        echo "Agent is registered with Overwatch at: ${OVERWATCH_IP}:7946"
        echo ""
    fi

    echo "Useful commands:"
    echo "  Status:     systemctl status opengslb-${ROLE}"
    echo "  Logs:       journalctl -u opengslb-${ROLE} -f"
    echo "  Restart:    systemctl restart opengslb-${ROLE}"
    echo ""
}

# Main
main() {
    log_section "OpenGSLB Bootstrap Script"
    log_info "Starting installation at $(date -u +"%Y-%m-%dT%H:%M:%SZ")"

    parse_args "$@"

    if [[ "$VERBOSE" == "true" ]]; then
        print_debug_info
    fi

    validate_args
    download_binary
    create_config
    create_service
    start_service
    run_health_check
    write_completion_marker
    print_summary

    log_success "Bootstrap completed successfully!"
    exit 0
}

main "$@"
