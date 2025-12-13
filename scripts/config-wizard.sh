#!/bin/bash
#
# OpenGSLB Configuration Wizard
# =============================
# An interactive wizard to generate OpenGSLB configuration files.
# Supports both Overwatch (DNS authority) and Agent (health reporter) modes.
#
# Usage: ./config-wizard.sh [output-file]
#        Default output: ./opengslb-config.yaml
#

set -e

# =============================================================================
# CONFIGURATION AND GLOBALS
# =============================================================================

VERSION="1.0.0"
OUTPUT_FILE="${1:-./opengslb-config.yaml}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Configuration state variables
declare MODE=""
declare -a REGIONS=()
declare -a DOMAINS=()
declare -a AGENT_BACKENDS=()
declare DNS_LISTEN_ADDRESS=":53"
declare DNS_DEFAULT_TTL="60"
declare DNS_RETURN_LAST_HEALTHY="false"
declare LOG_LEVEL="info"
declare LOG_FORMAT="json"
declare METRICS_ENABLED="false"
declare METRICS_ADDRESS=":9090"
declare API_ENABLED="false"
declare API_ADDRESS="127.0.0.1:8080"
declare -a API_ALLOWED_NETWORKS=()
declare API_TRUST_PROXY="false"
declare GOSSIP_BIND_ADDRESS="0.0.0.0:7946"
declare GOSSIP_ENCRYPTION_KEY=""
declare GOSSIP_PROBE_INTERVAL="1s"
declare GOSSIP_PROBE_TIMEOUT="500ms"
declare GOSSIP_INTERVAL="200ms"
declare VALIDATION_ENABLED="true"
declare VALIDATION_CHECK_INTERVAL="30s"
declare VALIDATION_CHECK_TIMEOUT="5s"
declare STALE_THRESHOLD="30s"
declare STALE_REMOVE_AFTER="5m"
declare DNSSEC_ENABLED="true"
declare DNSSEC_ALGORITHM="ECDSAP256SHA256"
declare -a DNSSEC_KEY_SYNC_PEERS=()
declare DNSSEC_KEY_SYNC_POLL_INTERVAL="1h"
declare DNSSEC_KEY_SYNC_TIMEOUT="30s"
declare GEOLOCATION_DATABASE_PATH=""
declare GEOLOCATION_DEFAULT_REGION=""
declare GEOLOCATION_ECS_ENABLED="true"
declare -a GEOLOCATION_CUSTOM_MAPPINGS=()
declare OVERWATCH_NODE_ID=""
declare OVERWATCH_REGION=""
declare -a OVERWATCH_AGENT_TOKENS=()
declare OVERWATCH_DATA_DIR="/var/lib/opengslb"
declare AGENT_SERVICE_TOKEN=""
declare AGENT_REGION=""
declare AGENT_CERT_PATH="/var/lib/opengslb/agent.crt"
declare AGENT_KEY_PATH="/var/lib/opengslb/agent.key"
declare -a AGENT_GOSSIP_OVERWATCH_NODES=()
declare AGENT_HEARTBEAT_INTERVAL="10s"
declare AGENT_HEARTBEAT_MISSED_THRESHOLD="3"
declare AGENT_PREDICTIVE_ENABLED="false"
declare AGENT_PREDICTIVE_CHECK_INTERVAL="10s"
declare AGENT_PREDICTIVE_CPU_THRESHOLD="90.0"
declare AGENT_PREDICTIVE_CPU_BLEED="30s"
declare AGENT_PREDICTIVE_MEMORY_THRESHOLD="85.0"
declare AGENT_PREDICTIVE_MEMORY_BLEED="30s"
declare AGENT_PREDICTIVE_ERROR_THRESHOLD="10.0"
declare AGENT_PREDICTIVE_ERROR_WINDOW="60s"
declare AGENT_PREDICTIVE_ERROR_BLEED="30s"

# =============================================================================
# UTILITY FUNCTIONS
# =============================================================================

print_banner() {
    clear
    echo -e "${CYAN}"
    cat << 'EOF'
   ___                    ____ ____  _     ____
  / _ \ _ __   ___ _ __  / ___/ ___|| |   | __ )
 | | | | '_ \ / _ \ '_ \| |  _\___ \| |   |  _ \
 | |_| | |_) |  __/ | | | |_| |___) | |___| |_) |
  \___/| .__/ \___|_| |_|\____|____/|_____|____/
       |_|
    Configuration Wizard v${VERSION}
EOF
    echo -e "${NC}"
    echo ""
}

print_section() {
    local title="$1"
    echo ""
    echo -e "${BOLD}${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${BLUE}  $title${NC}"
    echo -e "${BOLD}${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_subsection() {
    local title="$1"
    echo ""
    echo -e "${BOLD}${CYAN}───────────────────────────────────────────────────────────────────${NC}"
    echo -e "${BOLD}${CYAN}  $title${NC}"
    echo -e "${BOLD}${CYAN}───────────────────────────────────────────────────────────────────${NC}"
    echo ""
}

# All print functions output to stderr so they're visible in $() subshell contexts
print_info() {
    echo -e "${BLUE}ℹ${NC}  $1" >&2
}

print_success() {
    echo -e "${GREEN}✓${NC}  $1" >&2
}

print_warning() {
    echo -e "${YELLOW}⚠${NC}  $1" >&2
}

print_error() {
    echo -e "${RED}✗${NC}  $1" >&2
}

print_help() {
    echo -e "${CYAN}?${NC}  $1" >&2
}

# Display contextual help box (outputs to stderr so visible in $() contexts)
show_help_box() {
    local title="$1"
    shift
    echo "" >&2
    echo -e "  ${BOLD}${YELLOW}┌─ $title ─────────────────────────────────────────────────${NC}" >&2
    for line in "$@"; do
        echo -e "  ${YELLOW}│${NC} $line" >&2
    done
    echo -e "  ${BOLD}${YELLOW}└───────────────────────────────────────────────────────────────${NC}" >&2
    echo "" >&2
}

# Prompt for yes/no with default
ask_yes_no() {
    local prompt="$1"
    local default="${2:-y}"
    local result

    if [[ "$default" == "y" ]]; then
        prompt="$prompt [Y/n]: "
    else
        prompt="$prompt [y/N]: "
    fi

    while true; do
        read -r -p "$prompt" result
        result="${result:-$default}"
        case "${result,,}" in
            y|yes) return 0 ;;
            n|no) return 1 ;;
            *) print_error "Please enter 'y' or 'n'" ;;
        esac
    done
}

# Prompt for input with default value
# Shows clear prompt with default, outputs prompt to stderr
ask_input() {
    local prompt="$1"
    local default="$2"
    local result

    if [[ -n "$default" ]]; then
        echo -ne "${BOLD}$prompt${NC} [default: $default]: " >&2
        read -r result
        echo "${result:-$default}"
    else
        echo -ne "${BOLD}$prompt${NC}: " >&2
        read -r result
        echo "$result"
    fi
}

# Prompt for required input (cannot be empty)
ask_required() {
    local prompt="$1"
    local result=""

    while [[ -z "$result" ]]; do
        echo -ne "${BOLD}$prompt${NC} (required): " >&2
        read -r result
        if [[ -z "$result" ]]; then
            print_error "This field is required. Please enter a value."
        fi
    done
    echo "$result"
}

# Prompt for numeric input with validation
# Shows the valid range in the prompt
ask_number() {
    local prompt="$1"
    local default="$2"
    local min="${3:-}"
    local max="${4:-}"
    local result
    local range_hint=""

    # Build range hint for the prompt
    if [[ -n "$min" && -n "$max" ]]; then
        range_hint=" (range: $min-$max)"
    elif [[ -n "$min" ]]; then
        range_hint=" (min: $min)"
    elif [[ -n "$max" ]]; then
        range_hint=" (max: $max)"
    fi

    while true; do
        if [[ -n "$default" ]]; then
            echo -ne "${BOLD}$prompt${NC}$range_hint [default: $default]: " >&2
            read -r result
            result="${result:-$default}"
        else
            echo -ne "${BOLD}$prompt${NC}$range_hint: " >&2
            read -r result
        fi

        if ! [[ "$result" =~ ^[0-9]+$ ]]; then
            print_error "Please enter a valid number."
            continue
        fi

        if [[ -n "$min" ]] && (( result < min )); then
            print_error "Value must be at least $min."
            continue
        fi

        if [[ -n "$max" ]] && (( result > max )); then
            print_error "Value must be at most $max."
            continue
        fi

        echo "$result"
        return
    done
}

# Prompt for float input with validation
ask_float() {
    local prompt="$1"
    local default="$2"
    local min="${3:-}"
    local max="${4:-}"
    local result
    local range_hint=""

    # Build range hint for the prompt
    if [[ -n "$min" && -n "$max" ]]; then
        range_hint=" (range: $min-$max)"
    elif [[ -n "$min" ]]; then
        range_hint=" (min: $min)"
    elif [[ -n "$max" ]]; then
        range_hint=" (max: $max)"
    fi

    while true; do
        if [[ -n "$default" ]]; then
            echo -ne "${BOLD}$prompt${NC}$range_hint [default: $default]: " >&2
            read -r result
            result="${result:-$default}"
        else
            echo -ne "${BOLD}$prompt${NC}$range_hint: " >&2
            read -r result
        fi

        if ! [[ "$result" =~ ^[0-9]+\.?[0-9]*$ ]]; then
            print_error "Please enter a valid number."
            continue
        fi

        if [[ -n "$min" ]] && (( $(echo "$result < $min" | bc -l) )); then
            print_error "Value must be at least $min."
            continue
        fi

        if [[ -n "$max" ]] && (( $(echo "$result > $max" | bc -l) )); then
            print_error "Value must be at most $max."
            continue
        fi

        echo "$result"
        return
    done
}

# Prompt for duration input (e.g., 30s, 5m, 1h)
# Shows format hint in the prompt
ask_duration() {
    local prompt="$1"
    local default="$2"
    local result

    while true; do
        if [[ -n "$default" ]]; then
            echo -ne "${BOLD}$prompt${NC} (format: 30s, 5m, 1h, 500ms) [default: $default]: " >&2
            read -r result
            result="${result:-$default}"
        else
            echo -ne "${BOLD}$prompt${NC} (format: 30s, 5m, 1h, 500ms): " >&2
            read -r result
        fi

        if ! [[ "$result" =~ ^[0-9]+(ms|s|m|h)$ ]]; then
            print_error "Please enter a valid duration (e.g., 30s, 5m, 1h, 500ms)"
            continue
        fi

        echo "$result"
        return
    done
}

# Prompt for port number
ask_port() {
    local prompt="$1"
    local default="$2"
    ask_number "$prompt (port number)" "$default" 1 65535
}

# Validate IPv4 address
validate_ipv4() {
    local ip="$1"
    local IFS='.'
    read -ra octets <<< "$ip"

    [[ ${#octets[@]} -ne 4 ]] && return 1

    for octet in "${octets[@]}"; do
        [[ ! "$octet" =~ ^[0-9]+$ ]] && return 1
        (( octet < 0 || octet > 255 )) && return 1
    done

    return 0
}

# Validate IPv6 address (basic check)
validate_ipv6() {
    local ip="$1"
    # Basic IPv6 validation - allows full and compressed formats
    if [[ "$ip" =~ ^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$ ]] || \
       [[ "$ip" =~ ^::$ ]] || \
       [[ "$ip" =~ ^::1$ ]] || \
       [[ "$ip" =~ ^[0-9a-fA-F:]+$ ]]; then
        return 0
    fi
    return 1
}

# Validate IP address (IPv4 or IPv6)
validate_ip() {
    local ip="$1"
    validate_ipv4 "$ip" || validate_ipv6 "$ip"
}

# Prompt for IP address with validation
ask_ip_address() {
    local prompt="$1"
    local default="$2"
    local allow_hostname="${3:-false}"
    local result
    local format_hint="IPv4 (e.g., 192.168.1.100) or IPv6 (e.g., 2001:db8::1)"

    if [[ "$allow_hostname" == "true" ]]; then
        format_hint="$format_hint or hostname"
    fi

    while true; do
        if [[ -n "$default" ]]; then
            echo -ne "${BOLD}$prompt${NC}\n  Format: $format_hint\n  [default: $default]: " >&2
            read -r result
            result="${result:-$default}"
        else
            echo -ne "${BOLD}$prompt${NC}\n  Format: $format_hint\n  Enter value: " >&2
            read -r result
        fi

        if [[ -z "$result" ]]; then
            print_error "Please enter an IP address."
            continue
        fi

        if validate_ip "$result"; then
            echo "$result"
            return
        fi

        if [[ "$allow_hostname" == "true" ]] && [[ "$result" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]+$ ]]; then
            echo "$result"
            return
        fi

        print_error "Invalid format. Please enter a valid IPv4, IPv6 address$([ "$allow_hostname" == "true" ] && echo ", or hostname")."
    done
}

# Prompt for CIDR notation
ask_cidr() {
    local prompt="$1"
    local default="$2"
    local result

    while true; do
        if [[ -n "$default" ]]; then
            echo -ne "${BOLD}$prompt${NC}\n  Format: IPv4 CIDR (e.g., 10.0.0.0/8) or IPv6 CIDR (e.g., 2001:db8::/32)\n  [default: $default]: " >&2
            read -r result
            result="${result:-$default}"
        else
            echo -ne "${BOLD}$prompt${NC}\n  Format: IPv4 CIDR (e.g., 10.0.0.0/8) or IPv6 CIDR (e.g., 2001:db8::/32)\n  Enter value: " >&2
            read -r result
        fi

        if [[ -z "$result" ]]; then
            echo ""
            return
        fi

        # Basic CIDR validation
        if [[ "$result" =~ ^[0-9a-fA-F.:]+/[0-9]+$ ]]; then
            echo "$result"
            return
        fi

        print_error "Invalid CIDR format. Use format like 10.0.0.0/8 or 2001:db8::/32"
    done
}

# Prompt for host:port
ask_host_port() {
    local prompt="$1"
    local default="$2"
    local result

    while true; do
        if [[ -n "$default" ]]; then
            echo -ne "${BOLD}$prompt${NC}\n  Format: :port, ip:port, [ipv6]:port, or hostname:port\n  Examples: :53, 0.0.0.0:8080, [::1]:9090, server.local:7946\n  [default: $default]: " >&2
            read -r result
            result="${result:-$default}"
        else
            echo -ne "${BOLD}$prompt${NC}\n  Format: :port, ip:port, [ipv6]:port, or hostname:port\n  Examples: :53, 0.0.0.0:8080, [::1]:9090, server.local:7946\n  Enter value: " >&2
            read -r result
        fi

        # Allow formats: :port, ip:port, [ipv6]:port, hostname:port
        if [[ "$result" =~ ^:[0-9]+$ ]] || \
           [[ "$result" =~ ^[0-9.]+:[0-9]+$ ]] || \
           [[ "$result" =~ ^\[[0-9a-fA-F:]+\]:[0-9]+$ ]] || \
           [[ "$result" =~ ^[a-zA-Z0-9.-]+:[0-9]+$ ]]; then
            echo "$result"
            return
        fi

        print_error "Invalid format. Use: :port, ip:port, [ipv6]:port, or hostname:port"
    done
}

# Select from a list of options
# NOTE: All display output goes to stderr so it's visible when called in $()
ask_choice() {
    local prompt="$1"
    shift
    local options=("$@")
    local choice

    # Display menu to stderr so it's visible (stdout gets captured by $())
    echo "" >&2
    echo -e "${BOLD}$prompt${NC}" >&2
    echo "" >&2
    for i in "${!options[@]}"; do
        echo -e "  ${CYAN}$((i+1)))${NC} ${options[$i]}" >&2
    done
    echo "" >&2

    while true; do
        # Build a helpful prompt showing the options inline
        local options_hint=""
        for i in "${!options[@]}"; do
            if [[ -n "$options_hint" ]]; then
                options_hint+=", "
            fi
            options_hint+="$((i+1))=${options[$i]}"
        done

        read -r -p "Enter choice ($options_hint): " choice

        if [[ "$choice" =~ ^[0-9]+$ ]] && (( choice >= 1 && choice <= ${#options[@]} )); then
            echo "${options[$((choice-1))]}"
            return
        fi

        print_error "Invalid choice. Please enter a number between 1 and ${#options[@]}" >&2
    done
}

# Generate a random encryption key
generate_encryption_key() {
    if command -v openssl &> /dev/null; then
        openssl rand -base64 32
    else
        # Fallback using /dev/urandom
        head -c 32 /dev/urandom | base64
    fi
}

# Wait for user to press enter
press_enter() {
    echo ""
    read -r -p "Press Enter to continue..."
}

# =============================================================================
# MODE SELECTION
# =============================================================================

select_mode() {
    print_section "Step 1: Select Operation Mode"

    show_help_box "Operation Modes" \
        "OpenGSLB has two modes of operation:" \
        "" \
        "OVERWATCH MODE (DNS Authority):" \
        "  • Runs as the DNS server that answers queries" \
        "  • Performs health checks on backend servers" \
        "  • Makes routing decisions based on algorithms" \
        "  • Receives health reports from agents" \
        "  • This is the 'brain' of your GSLB setup" \
        "" \
        "AGENT MODE (Health Reporter):" \
        "  • Runs on application servers" \
        "  • Reports local health status to Overwatch" \
        "  • Can perform local health checks" \
        "  • Supports predictive health (CPU, memory)" \
        "  • Lightweight and minimal configuration"

    MODE=$(ask_choice "Which mode do you want to configure?" "overwatch" "agent")

    print_success "Selected mode: $MODE"
}

# =============================================================================
# LOGGING CONFIGURATION
# =============================================================================

configure_logging() {
    print_section "Logging Configuration"

    show_help_box "Logging Options" \
        "Configure how OpenGSLB logs its activity." \
        "" \
        "LOG LEVELS:" \
        "  • debug: Very verbose, includes internal state" \
        "  • info:  Normal operation messages (recommended)" \
        "  • warn:  Warnings and potential issues" \
        "  • error: Only errors and critical issues" \
        "" \
        "LOG FORMATS:" \
        "  • json: Structured logging for log aggregation" \
        "         (ELK, Splunk, Loki, etc.)" \
        "  • text: Human-readable for development/debugging"

    LOG_LEVEL=$(ask_choice "Select log level:" "debug" "info" "warn" "error")
    print_success "Log level: $LOG_LEVEL"

    LOG_FORMAT=$(ask_choice "Select log format:" "json" "text")
    print_success "Log format: $LOG_FORMAT"
}

# =============================================================================
# METRICS CONFIGURATION
# =============================================================================

configure_metrics() {
    print_section "Metrics Configuration"

    show_help_box "Prometheus Metrics" \
        "OpenGSLB can expose Prometheus-compatible metrics." \
        "" \
        "When enabled, metrics are available at:" \
        "  • http://<address>/metrics - Prometheus scrape endpoint" \
        "  • http://<address>/health  - Health check endpoint" \
        "" \
        "Useful for monitoring:" \
        "  • DNS query rates and latencies" \
        "  • Health check results" \
        "  • Backend status" \
        "  • Gossip cluster health"

    if ask_yes_no "Enable Prometheus metrics endpoint?" "n"; then
        METRICS_ENABLED="true"
        METRICS_ADDRESS=$(ask_host_port "Metrics listen address" ":9090")
        print_success "Metrics enabled on $METRICS_ADDRESS"
    else
        METRICS_ENABLED="false"
        print_info "Metrics disabled"
    fi
}

# =============================================================================
# GOSSIP CONFIGURATION (Common to both modes)
# =============================================================================

configure_gossip_common() {
    print_subsection "Gossip Protocol Configuration"

    show_help_box "Gossip Protocol" \
        "OpenGSLB uses a gossip protocol (based on SWIM) for:" \
        "  • Agent-to-Overwatch communication" \
        "  • Health status propagation" \
        "  • Cluster membership" \
        "" \
        "ENCRYPTION (REQUIRED):" \
        "  A 32-byte AES-256 encryption key is MANDATORY." \
        "  This ensures all gossip traffic is encrypted." \
        "  The same key must be used by all nodes in the cluster."

    echo ""
    if ask_yes_no "Generate a new encryption key automatically?" "y"; then
        GOSSIP_ENCRYPTION_KEY=$(generate_encryption_key)
        print_success "Generated encryption key (save this!):"
        echo ""
        echo -e "  ${BOLD}${GREEN}$GOSSIP_ENCRYPTION_KEY${NC}"
        echo ""
        print_warning "IMPORTANT: Copy this key! You'll need it for other nodes."
    else
        print_info "Enter your existing encryption key."
        print_info "Generate one with: openssl rand -base64 32"
        while true; do
            GOSSIP_ENCRYPTION_KEY=$(ask_required "Encryption key (base64, 32 bytes)")
            # Basic validation - check it's base64 and roughly the right length
            if [[ ${#GOSSIP_ENCRYPTION_KEY} -ge 40 ]]; then
                break
            fi
            print_error "Key appears too short. Should be ~44 characters (32 bytes base64 encoded)."
        done
    fi
}

# =============================================================================
# OVERWATCH MODE CONFIGURATION
# =============================================================================

configure_overwatch() {
    print_section "Step 2: Overwatch Identity"

    show_help_box "Overwatch Identity" \
        "Configure how this Overwatch node identifies itself." \
        "" \
        "NODE ID:" \
        "  Unique identifier for this node in the cluster." \
        "  Defaults to the hostname if not specified." \
        "" \
        "REGION:" \
        "  Geographic region this Overwatch serves." \
        "  Used for multi-region deployments."

    local hostname_default
    hostname_default=$(hostname 2>/dev/null || echo "overwatch-1")

    OVERWATCH_NODE_ID=$(ask_input "Node ID" "$hostname_default")
    OVERWATCH_REGION=$(ask_input "Region (optional, e.g., us-east-1)" "")

    print_success "Node ID: $OVERWATCH_NODE_ID"
    [[ -n "$OVERWATCH_REGION" ]] && print_success "Region: $OVERWATCH_REGION"

    # DNS Configuration
    configure_dns

    # Gossip Configuration
    print_section "Step 3: Gossip Protocol"
    configure_gossip_common
    configure_overwatch_gossip

    # Agent Tokens
    configure_agent_tokens

    # Validation
    configure_validation

    # Stale Backend Handling
    configure_stale

    # DNSSEC
    configure_dnssec

    # Geolocation
    configure_geolocation

    # API
    configure_api

    # Data Directory
    configure_data_dir

    # Regions
    configure_regions

    # Domains
    configure_domains
}

configure_dns() {
    print_section "DNS Server Configuration"

    show_help_box "DNS Server Settings" \
        "Configure how OpenGSLB serves DNS queries." \
        "" \
        "LISTEN ADDRESS:" \
        "  • ':53' - Listen on all interfaces, port 53 (default)" \
        "  • '0.0.0.0:53' - IPv4 only, all interfaces" \
        "  • '[::]:53' - IPv6 only, all interfaces" \
        "  • '10.0.0.1:53' - Specific interface" \
        "" \
        "  NOTE: Port 53 requires root privileges." \
        "        Use a higher port (e.g., 5353) for testing." \
        "" \
        "DEFAULT TTL:" \
        "  Time-to-live for DNS responses in seconds." \
        "  • Lower TTL = Faster failover, more DNS queries" \
        "  • Higher TTL = Fewer queries, slower failover" \
        "  • 60 seconds is a good default" \
        "" \
        "RETURN LAST HEALTHY:" \
        "  When ALL backends are unhealthy:" \
        "  • false: Return SERVFAIL (recommended)" \
        "  • true: Return last known healthy IP ('limp mode')"

    DNS_LISTEN_ADDRESS=$(ask_host_port "DNS listen address" ":53")
    DNS_DEFAULT_TTL=$(ask_number "Default TTL (seconds)" "60" 1 86400)

    if ask_yes_no "Enable 'limp mode' (return last healthy when all down)?" "n"; then
        DNS_RETURN_LAST_HEALTHY="true"
        print_warning "Limp mode enabled - may serve stale IPs when all backends down"
    else
        DNS_RETURN_LAST_HEALTHY="false"
    fi

    print_success "DNS configured: $DNS_LISTEN_ADDRESS, TTL=$DNS_DEFAULT_TTL"
}

configure_overwatch_gossip() {
    print_subsection "Overwatch Gossip Settings"

    show_help_box "Gossip Bind Settings" \
        "Configure how Overwatch listens for gossip traffic." \
        "" \
        "BIND ADDRESS:" \
        "  The address:port to listen for agent gossip." \
        "  • '0.0.0.0:7946' - All interfaces (default)" \
        "  • Specific IP to restrict to one interface" \
        "" \
        "PROBE SETTINGS:" \
        "  • Probe Interval: Time between failure detection probes" \
        "  • Probe Timeout: How long to wait for probe response" \
        "  • Gossip Interval: Time between gossip message broadcasts"

    GOSSIP_BIND_ADDRESS=$(ask_host_port "Gossip bind address" "0.0.0.0:7946")

    if ask_yes_no "Configure advanced gossip timing? (usually not needed)" "n"; then
        GOSSIP_PROBE_INTERVAL=$(ask_duration "Probe interval" "1s")
        GOSSIP_PROBE_TIMEOUT=$(ask_duration "Probe timeout" "500ms")
        GOSSIP_INTERVAL=$(ask_duration "Gossip interval" "200ms")
    fi

    print_success "Gossip configured: $GOSSIP_BIND_ADDRESS"
}

configure_agent_tokens() {
    print_section "Agent Authentication Tokens"

    show_help_box "Agent Tokens" \
        "Pre-shared tokens that agents must provide to register." \
        "" \
        "Each service that agents report for needs a token." \
        "Agents must know this token to connect." \
        "" \
        "Example:" \
        "  Service: web-service" \
        "  Token: my-secure-token-for-web-service" \
        "" \
        "Tokens should be at least 16 characters."

    OVERWATCH_AGENT_TOKENS=()

    if ask_yes_no "Configure agent authentication tokens?" "y"; then
        while true; do
            echo ""
            local service_name token

            service_name=$(ask_input "Service name (e.g., webapp, api)" "")
            if [[ -z "$service_name" ]]; then
                print_info "Skipping token configuration."
                break
            fi

            if ask_yes_no "Generate a random token for '$service_name'?" "y"; then
                token=$(openssl rand -base64 24 2>/dev/null || head -c 24 /dev/urandom | base64)
                print_success "Generated token for $service_name:"
                echo -e "  ${BOLD}${GREEN}$token${NC}"
            else
                while true; do
                    token=$(ask_required "Token for '$service_name'")
                    if [[ ${#token} -ge 16 ]]; then
                        break
                    fi
                    print_error "Token must be at least 16 characters."
                done
            fi

            OVERWATCH_AGENT_TOKENS+=("$service_name:$token")
            print_success "Added token for service: $service_name"

            if ! ask_yes_no "Add another agent token?" "n"; then
                break
            fi
        done
    fi

    print_success "Configured ${#OVERWATCH_AGENT_TOKENS[@]} agent token(s)"
}

configure_validation() {
    print_section "Health Validation Configuration"

    show_help_box "Overwatch Validation" \
        "Overwatch can independently validate health claims from agents." \
        "" \
        "When enabled (RECOMMENDED):" \
        "  • Overwatch performs its own health checks" \
        "  • Agent claims are verified, not blindly trusted" \
        "  • Prevents compromised agents from lying about health" \
        "  • Overwatch decision ALWAYS wins over agent claims" \
        "" \
        "This is a security feature - disable only if you have" \
        "network restrictions preventing Overwatch from checking backends."

    if ask_yes_no "Enable health validation? (recommended)" "y"; then
        VALIDATION_ENABLED="true"
        VALIDATION_CHECK_INTERVAL=$(ask_duration "Validation check interval" "30s")
        VALIDATION_CHECK_TIMEOUT=$(ask_duration "Validation check timeout" "5s")
        print_success "Validation enabled"
    else
        VALIDATION_ENABLED="false"
        print_warning "Validation disabled - agents will be trusted"
    fi
}

configure_stale() {
    print_section "Stale Backend Handling"

    show_help_box "Stale Backend Detection" \
        "Configure how OpenGSLB handles backends that stop reporting." \
        "" \
        "STALE THRESHOLD:" \
        "  Time without heartbeat before marking a backend as 'stale'." \
        "  Stale backends are deprioritized in routing." \
        "" \
        "REMOVE AFTER:" \
        "  Time after which stale backends are completely removed." \
        "  This cleans up backends that are truly gone."

    STALE_THRESHOLD=$(ask_duration "Stale threshold (time without heartbeat)" "30s")
    STALE_REMOVE_AFTER=$(ask_duration "Remove stale backends after" "5m")

    print_success "Stale handling: threshold=$STALE_THRESHOLD, remove=$STALE_REMOVE_AFTER"
}

configure_dnssec() {
    print_section "DNSSEC Configuration"

    show_help_box "DNSSEC (DNS Security Extensions)" \
        "DNSSEC cryptographically signs DNS responses to prevent:" \
        "  • DNS spoofing attacks" \
        "  • Cache poisoning" \
        "  • Man-in-the-middle attacks" \
        "" \
        "ENABLED BY DEFAULT for security!" \
        "" \
        "ALGORITHM OPTIONS:" \
        "  • ECDSAP256SHA256 - Modern, fast, recommended" \
        "  • ECDSAP384SHA384 - Stronger, slightly slower" \
        "  • RSASHA256 - Legacy compatibility" \
        "  • ED25519 - Newest, very fast (if supported)" \
        "" \
        "KEY SYNC:" \
        "  For multi-node deployments, DNSSEC keys must be" \
        "  synchronized between Overwatch nodes."

    if ask_yes_no "Enable DNSSEC? (highly recommended)" "y"; then
        DNSSEC_ENABLED="true"
        DNSSEC_ALGORITHM=$(ask_choice "DNSSEC algorithm:" \
            "ECDSAP256SHA256" "ECDSAP384SHA384" "RSASHA256" "ED25519")

        if ask_yes_no "Configure DNSSEC key sync with other Overwatch nodes?" "n"; then
            DNSSEC_KEY_SYNC_PEERS=()
            while true; do
                local peer
                peer=$(ask_input "Peer Overwatch API address (e.g., overwatch-2:8080)" "")
                if [[ -z "$peer" ]]; then
                    break
                fi
                DNSSEC_KEY_SYNC_PEERS+=("$peer")
                print_success "Added key sync peer: $peer"

                if ! ask_yes_no "Add another key sync peer?" "n"; then
                    break
                fi
            done

            if [[ ${#DNSSEC_KEY_SYNC_PEERS[@]} -gt 0 ]]; then
                DNSSEC_KEY_SYNC_POLL_INTERVAL=$(ask_duration "Key sync poll interval" "1h")
                DNSSEC_KEY_SYNC_TIMEOUT=$(ask_duration "Key sync timeout" "30s")
            fi
        fi

        print_success "DNSSEC enabled with $DNSSEC_ALGORITHM"
    else
        DNSSEC_ENABLED="false"
        print_warning "DNSSEC disabled - DNS responses will not be signed"
        print_warning "This is a security risk. Only disable for testing."
    fi
}

configure_geolocation() {
    print_section "Geolocation Configuration"

    show_help_box "Geographic Routing" \
        "Route users to the nearest datacenter based on their location." \
        "" \
        "REQUIREMENTS:" \
        "  • MaxMind GeoLite2-Country database (free with registration)" \
        "  • Download from: https://dev.maxmind.com/geoip/geolite2-free-geolocation-data" \
        "" \
        "EDNS CLIENT SUBNET (ECS):" \
        "  Allows DNS resolvers to send client subnet info" \
        "  for more accurate geolocation routing." \
        "" \
        "CUSTOM MAPPINGS:" \
        "  Override database lookups for specific IP ranges." \
        "  Useful for internal networks or known CDN ranges."

    if ask_yes_no "Enable geolocation-based routing?" "n"; then
        GEOLOCATION_DATABASE_PATH=$(ask_required "Path to GeoLite2-Country.mmdb")
        GEOLOCATION_DEFAULT_REGION=$(ask_required "Default region for unknown IPs")

        if ask_yes_no "Enable EDNS Client Subnet (ECS)?" "y"; then
            GEOLOCATION_ECS_ENABLED="true"
        else
            GEOLOCATION_ECS_ENABLED="false"
        fi

        # Custom mappings
        if ask_yes_no "Configure custom IP-to-region mappings?" "n"; then
            GEOLOCATION_CUSTOM_MAPPINGS=()
            while true; do
                local cidr region comment

                cidr=$(ask_cidr "CIDR range (e.g., 10.0.0.0/8)" "")
                if [[ -z "$cidr" ]]; then
                    break
                fi

                region=$(ask_required "Region for $cidr")
                comment=$(ask_input "Comment (optional)" "")

                GEOLOCATION_CUSTOM_MAPPINGS+=("$cidr|$region|$comment")
                print_success "Added mapping: $cidr → $region"

                if ! ask_yes_no "Add another custom mapping?" "n"; then
                    break
                fi
            done
        fi

        print_success "Geolocation enabled"
    else
        print_info "Geolocation disabled - will use other routing algorithms"
    fi
}

configure_api() {
    print_section "Management API Configuration"

    show_help_box "Management API" \
        "RESTful API for runtime management and monitoring." \
        "" \
        "ENDPOINTS:" \
        "  • GET /health - API health check" \
        "  • GET /api/v1/backends - List all backends" \
        "  • GET /api/v1/regions - List regions" \
        "  • GET /api/v1/domains - List domains" \
        "" \
        "SECURITY:" \
        "  • Bind to localhost by default (127.0.0.1:8080)" \
        "  • allowed_networks restricts access by IP/CIDR" \
        "  • Use a reverse proxy for external access with auth" \
        "" \
        "TRUST PROXY HEADERS:" \
        "  Enable only if behind a trusted reverse proxy" \
        "  that sets X-Forwarded-For headers."

    if ask_yes_no "Enable the management API?" "y"; then
        API_ENABLED="true"
        API_ADDRESS=$(ask_host_port "API listen address" "127.0.0.1:8080")

        # Allowed networks
        echo ""
        print_info "Configure which networks can access the API."
        print_info "Default: localhost only (127.0.0.1/32, ::1/128)"

        API_ALLOWED_NETWORKS=()
        if ask_yes_no "Use default (localhost only)?" "y"; then
            API_ALLOWED_NETWORKS=("127.0.0.1/32" "::1/128")
        else
            while true; do
                local network
                network=$(ask_cidr "Allowed network CIDR" "")
                if [[ -z "$network" ]]; then
                    break
                fi
                API_ALLOWED_NETWORKS+=("$network")
                print_success "Added: $network"

                if ! ask_yes_no "Add another network?" "y"; then
                    break
                fi
            done

            if [[ ${#API_ALLOWED_NETWORKS[@]} -eq 0 ]]; then
                API_ALLOWED_NETWORKS=("127.0.0.1/32" "::1/128")
                print_info "No networks added, using localhost default"
            fi
        fi

        if ask_yes_no "Trust proxy headers (X-Forwarded-For)?" "n"; then
            API_TRUST_PROXY="true"
            print_warning "Proxy headers trusted - ensure you're behind a trusted proxy"
        else
            API_TRUST_PROXY="false"
        fi

        print_success "API enabled on $API_ADDRESS"
    else
        API_ENABLED="false"
        print_info "API disabled"
    fi
}

configure_data_dir() {
    print_subsection "Data Directory"

    show_help_box "Data Storage" \
        "Directory where OpenGSLB stores persistent data." \
        "" \
        "STORED DATA:" \
        "  • DNSSEC keys" \
        "  • Backend state database (bbolt)" \
        "  • Runtime state" \
        "" \
        "REQUIREMENTS:" \
        "  • Must be writable by the OpenGSLB process" \
        "  • Should persist across restarts" \
        "  • Use a dedicated directory, not /tmp"

    OVERWATCH_DATA_DIR=$(ask_input "Data directory" "/var/lib/opengslb")
    print_success "Data directory: $OVERWATCH_DATA_DIR"
}

# =============================================================================
# REGIONS CONFIGURATION
# =============================================================================

configure_regions() {
    print_section "Regions Configuration"

    show_help_box "Regions" \
        "Regions represent groups of backend servers, typically" \
        "organized by geographic location or datacenter." \
        "" \
        "EACH REGION CONTAINS:" \
        "  • One or more backend servers" \
        "  • Health check configuration" \
        "  • Optional country/continent mappings for geolocation" \
        "" \
        "EXAMPLES:" \
        "  • us-east, us-west, eu-central, asia-pacific" \
        "  • datacenter-1, datacenter-2" \
        "  • primary, secondary, tertiary"

    REGIONS=()
    local region_count=0

    while true; do
        region_count=$((region_count + 1))
        print_subsection "Region #$region_count"

        local region_data
        region_data=$(configure_single_region)

        if [[ -n "$region_data" ]]; then
            REGIONS+=("$region_data")
            print_success "Region added successfully"
        fi

        echo ""
        if ! ask_yes_no "Add another region?" "y"; then
            break
        fi
    done

    if [[ ${#REGIONS[@]} -eq 0 ]]; then
        print_warning "No regions configured. You'll need at least one region."
        if ask_yes_no "Add a region now?" "y"; then
            configure_regions
        fi
    else
        print_success "Configured ${#REGIONS[@]} region(s)"
    fi
}

configure_single_region() {
    local region_name servers_json health_check_json countries continents

    region_name=$(ask_required "Region name (e.g., us-east-1)")

    # Servers
    echo ""
    print_info "Now configure backend servers for region '$region_name'"

    local servers=()
    local server_count=0

    while true; do
        server_count=$((server_count + 1))
        echo ""
        print_info "Server #$server_count for $region_name"

        local server_data
        server_data=$(configure_single_server)

        if [[ -n "$server_data" ]]; then
            servers+=("$server_data")
        fi

        if ! ask_yes_no "Add another server to '$region_name'?" "n"; then
            break
        fi
    done

    if [[ ${#servers[@]} -eq 0 ]]; then
        print_error "Region must have at least one server!"
        return 1
    fi

    # Health Check
    echo ""
    print_info "Configure health check for region '$region_name'"
    local health_check
    health_check=$(configure_health_check)

    # Countries/Continents for geolocation
    countries=""
    continents=""

    if [[ -n "$GEOLOCATION_DATABASE_PATH" ]]; then
        echo ""
        print_info "Configure geolocation mapping for '$region_name'"

        show_help_box "Country & Continent Codes" \
            "COUNTRY CODES (ISO 3166-1 alpha-2):" \
            "  US, CA, GB, DE, FR, JP, AU, BR, IN, CN, etc." \
            "" \
            "CONTINENT CODES:" \
            "  AF - Africa" \
            "  AN - Antarctica" \
            "  AS - Asia" \
            "  EU - Europe" \
            "  NA - North America" \
            "  OC - Oceania" \
            "  SA - South America"

        local country_list
        country_list=$(ask_input "Country codes (comma-separated, e.g., US,CA)" "")
        if [[ -n "$country_list" ]]; then
            countries="$country_list"
        fi

        local continent_list
        continent_list=$(ask_input "Continent codes (comma-separated, e.g., NA,SA)" "")
        if [[ -n "$continent_list" ]]; then
            continents="$continent_list"
        fi
    fi

    # Build region data as a delimited string
    # Format: name|servers|health_check|countries|continents
    local servers_combined
    servers_combined=$(IFS='§'; echo "${servers[*]}")

    echo "$region_name|$servers_combined|$health_check|$countries|$continents"
}

configure_single_server() {
    local address port weight host

    address=$(ask_ip_address "Server IP address" "" "true")
    port=$(ask_port "Port" "80")
    weight=$(ask_number "Weight (1-1000, 0=disabled)" "100" 0 1000)
    host=$(ask_input "Hostname for TLS SNI (optional)" "")

    # Format: address:port:weight:host
    echo "$address:$port:$weight:$host"
}

configure_health_check() {
    local check_type interval timeout path host failure_threshold success_threshold

    show_help_box "Health Check Types" \
        "HTTP:  Performs HTTP GET request, expects 2xx response" \
        "HTTPS: Same as HTTP but with TLS" \
        "TCP:   Just verifies TCP connection succeeds"

    check_type=$(ask_choice "Health check type:" "http" "https" "tcp")
    interval=$(ask_duration "Check interval" "30s")
    timeout=$(ask_duration "Check timeout" "5s")

    path=""
    host=""
    if [[ "$check_type" == "http" || "$check_type" == "https" ]]; then
        path=$(ask_input "Health check path" "/health")
        host=$(ask_input "Host header (for TLS SNI, optional)" "")
    fi

    failure_threshold=$(ask_number "Failure threshold (consecutive failures to mark unhealthy)" "3" 1 10)
    success_threshold=$(ask_number "Success threshold (consecutive successes to mark healthy)" "2" 1 10)

    # Format: type:interval:timeout:path:host:failure:success
    echo "$check_type:$interval:$timeout:$path:$host:$failure_threshold:$success_threshold"
}

# =============================================================================
# DOMAINS CONFIGURATION
# =============================================================================

configure_domains() {
    print_section "Domains Configuration"

    show_help_box "Domains" \
        "Domains are the DNS names that OpenGSLB will serve." \
        "" \
        "EACH DOMAIN REQUIRES:" \
        "  • Fully qualified domain name (FQDN)" \
        "  • Routing algorithm" \
        "  • List of regions to route to" \
        "" \
        "ROUTING ALGORITHMS:" \
        "  • round-robin: Equal distribution, deterministic order" \
        "  • weighted: Proportional to server weights" \
        "  • failover: Highest priority healthy server" \
        "  • geolocation: Based on client geographic location" \
        "  • latency: Lowest measured latency server"

    DOMAINS=()
    local domain_count=0

    while true; do
        domain_count=$((domain_count + 1))
        print_subsection "Domain #$domain_count"

        local domain_data
        domain_data=$(configure_single_domain)

        if [[ -n "$domain_data" ]]; then
            DOMAINS+=("$domain_data")
            print_success "Domain added successfully"
        fi

        echo ""
        if ! ask_yes_no "Add another domain?" "y"; then
            break
        fi
    done

    if [[ ${#DOMAINS[@]} -eq 0 ]]; then
        print_warning "No domains configured. You'll need at least one domain."
        if ask_yes_no "Add a domain now?" "y"; then
            configure_domains
        fi
    else
        print_success "Configured ${#DOMAINS[@]} domain(s)"
    fi
}

configure_single_domain() {
    local domain_name routing_algorithm domain_regions ttl latency_config

    domain_name=$(ask_required "Domain name (FQDN, e.g., api.example.com)")

    routing_algorithm=$(ask_choice "Routing algorithm:" \
        "round-robin" "weighted" "failover" "geolocation" "latency")

    # Select regions
    echo ""
    print_info "Select which regions this domain should route to."
    print_info "You can add multiple regions. Press Enter without input when done."
    echo ""

    # Build region options display
    local region_options=""
    for i in "${!REGIONS[@]}"; do
        local region_name
        region_name=$(echo "${REGIONS[$i]}" | cut -d'|' -f1)
        echo -e "  ${CYAN}$((i+1)))${NC} $region_name"
        if [[ -n "$region_options" ]]; then
            region_options+=", "
        fi
        region_options+="$((i+1))=$region_name"
    done
    echo ""

    local selected_regions=()
    while true; do
        local region_choice
        local selected_display=""
        if [[ ${#selected_regions[@]} -gt 0 ]]; then
            selected_display=" [Selected: ${selected_regions[*]}]"
        fi

        read -r -p "Add region ($region_options) or Enter to finish$selected_display: " region_choice

        if [[ -z "$region_choice" ]]; then
            break
        fi

        if [[ "$region_choice" =~ ^[0-9]+$ ]] && (( region_choice >= 1 && region_choice <= ${#REGIONS[@]} )); then
            local region_name
            region_name=$(echo "${REGIONS[$((region_choice-1))]}" | cut -d'|' -f1)
            # Check if already selected
            local already_selected=false
            for existing in "${selected_regions[@]}"; do
                if [[ "$existing" == "$region_name" ]]; then
                    already_selected=true
                    break
                fi
            done
            if [[ "$already_selected" == "true" ]]; then
                print_warning "Region '$region_name' is already selected"
            else
                selected_regions+=("$region_name")
                print_success "Added region: $region_name"
            fi
        else
            print_error "Invalid selection. Enter a number from 1 to ${#REGIONS[@]}"
        fi
    done

    if [[ ${#selected_regions[@]} -eq 0 ]]; then
        print_error "Domain must have at least one region!"
        return 1
    fi

    domain_regions=$(IFS=','; echo "${selected_regions[*]}")

    ttl=$(ask_number "TTL for this domain (seconds, 0 for default)" "0" 0 86400)

    # Latency config
    latency_config=""
    if [[ "$routing_algorithm" == "latency" ]]; then
        echo ""
        print_info "Configure latency routing parameters"

        local smoothing max_latency min_samples
        smoothing=$(ask_float "Smoothing factor (0.0-1.0, higher=more responsive)" "0.3" 0 1)
        max_latency=$(ask_number "Max acceptable latency (ms, 0=disabled)" "500" 0 10000)
        min_samples=$(ask_number "Minimum samples before using latency data" "3" 1 100)

        latency_config="$smoothing:$max_latency:$min_samples"
    fi

    # Format: name|algorithm|regions|ttl|latency_config
    echo "$domain_name|$routing_algorithm|$domain_regions|$ttl|$latency_config"
}

# =============================================================================
# AGENT MODE CONFIGURATION
# =============================================================================

configure_agent() {
    print_section "Step 2: Agent Identity"

    show_help_box "Agent Identity" \
        "Configure how this agent identifies itself to Overwatch." \
        "" \
        "SERVICE TOKEN:" \
        "  Pre-shared secret that Overwatch uses to authenticate" \
        "  this agent. Must match the token configured in Overwatch." \
        "  Minimum 16 characters." \
        "" \
        "REGION:" \
        "  The geographic region this agent belongs to." \
        "  Must match a region configured in Overwatch."

    while true; do
        AGENT_SERVICE_TOKEN=$(ask_required "Service token")
        if [[ ${#AGENT_SERVICE_TOKEN} -ge 16 ]]; then
            break
        fi
        print_error "Token must be at least 16 characters."
    done

    AGENT_REGION=$(ask_required "Region (e.g., us-east-1)")

    echo ""
    print_info "Agent certificate paths (for mTLS)"
    AGENT_CERT_PATH=$(ask_input "Certificate path" "/var/lib/opengslb/agent.crt")
    AGENT_KEY_PATH=$(ask_input "Private key path" "/var/lib/opengslb/agent.key")

    print_success "Agent identity configured"

    # Backends
    configure_agent_backends

    # Gossip
    print_section "Step 3: Gossip Protocol"
    configure_gossip_common
    configure_agent_gossip

    # Heartbeat
    configure_heartbeat

    # Predictive Health
    configure_predictive_health
}

configure_agent_backends() {
    print_section "Agent Backends Configuration"

    show_help_box "Agent Backends" \
        "Backends are the local services this agent monitors." \
        "" \
        "The agent performs health checks on these backends" \
        "and reports their status to Overwatch." \
        "" \
        "EACH BACKEND REQUIRES:" \
        "  • Service name (maps to domain in Overwatch)" \
        "  • Address and port of the backend" \
        "  • Health check configuration"

    AGENT_BACKENDS=()
    local backend_count=0

    while true; do
        backend_count=$((backend_count + 1))
        print_subsection "Backend #$backend_count"

        local backend_data
        backend_data=$(configure_single_agent_backend)

        if [[ -n "$backend_data" ]]; then
            AGENT_BACKENDS+=("$backend_data")
            print_success "Backend added successfully"
        fi

        echo ""
        if ! ask_yes_no "Add another backend?" "n"; then
            break
        fi
    done

    if [[ ${#AGENT_BACKENDS[@]} -eq 0 ]]; then
        print_warning "No backends configured. Agent needs at least one backend."
        if ask_yes_no "Add a backend now?" "y"; then
            configure_agent_backends
        fi
    else
        print_success "Configured ${#AGENT_BACKENDS[@]} backend(s)"
    fi
}

configure_single_agent_backend() {
    local service address port weight health_check

    service=$(ask_required "Service name (e.g., webapp)")
    address=$(ask_ip_address "Backend address" "127.0.0.1" "true")
    port=$(ask_port "Backend port" "8080")
    weight=$(ask_number "Weight (1-1000, 0=disabled)" "100" 0 1000)

    echo ""
    print_info "Configure health check for this backend"
    health_check=$(configure_health_check)

    # Format: service:address:port:weight:health_check
    echo "$service:$address:$port:$weight:$health_check"
}

configure_agent_gossip() {
    print_subsection "Agent Gossip Settings"

    show_help_box "Overwatch Nodes" \
        "List of Overwatch gossip addresses to connect to." \
        "" \
        "Format: hostname:port or ip:port" \
        "Default port is 7946" \
        "" \
        "Examples:" \
        "  overwatch-1.internal:7946" \
        "  10.0.1.10:7946" \
        "  [2001:db8::1]:7946"

    AGENT_GOSSIP_OVERWATCH_NODES=()

    while true; do
        local node
        node=$(ask_host_port "Overwatch node address" "")

        if [[ -z "$node" ]]; then
            if [[ ${#AGENT_GOSSIP_OVERWATCH_NODES[@]} -eq 0 ]]; then
                print_error "At least one Overwatch node is required."
                continue
            fi
            break
        fi

        AGENT_GOSSIP_OVERWATCH_NODES+=("$node")
        print_success "Added Overwatch node: $node"

        if ! ask_yes_no "Add another Overwatch node?" "n"; then
            break
        fi
    done

    print_success "Configured ${#AGENT_GOSSIP_OVERWATCH_NODES[@]} Overwatch node(s)"
}

configure_heartbeat() {
    print_section "Heartbeat Configuration"

    show_help_box "Heartbeat Settings" \
        "Heartbeats let Overwatch know this agent is alive." \
        "" \
        "INTERVAL:" \
        "  How often to send heartbeat messages." \
        "  Default: 10 seconds" \
        "" \
        "MISSED THRESHOLD:" \
        "  Number of missed heartbeats before Overwatch" \
        "  deregisters this agent." \
        "  Default: 3 (30 seconds with 10s interval)"

    AGENT_HEARTBEAT_INTERVAL=$(ask_duration "Heartbeat interval" "10s")
    AGENT_HEARTBEAT_MISSED_THRESHOLD=$(ask_number "Missed threshold" "3" 1 10)

    print_success "Heartbeat configured: interval=$AGENT_HEARTBEAT_INTERVAL, threshold=$AGENT_HEARTBEAT_MISSED_THRESHOLD"
}

configure_predictive_health() {
    print_section "Predictive Health Configuration"

    show_help_box "Predictive Health" \
        "Proactively drain traffic before servers become unhealthy." \
        "" \
        "MONITORS:" \
        "  • CPU Usage: Drain when CPU exceeds threshold" \
        "  • Memory Usage: Drain when memory exceeds threshold" \
        "  • Error Rate: Drain when errors exceed threshold" \
        "" \
        "BLEED DURATION:" \
        "  Time to gradually reduce traffic weight to zero." \
        "  Allows graceful connection draining."

    if ask_yes_no "Enable predictive health monitoring?" "n"; then
        AGENT_PREDICTIVE_ENABLED="true"
        AGENT_PREDICTIVE_CHECK_INTERVAL=$(ask_duration "Check interval" "10s")

        # CPU
        echo ""
        print_info "CPU monitoring configuration"
        AGENT_PREDICTIVE_CPU_THRESHOLD=$(ask_float "CPU threshold (%)" "90.0" 0 100)
        AGENT_PREDICTIVE_CPU_BLEED=$(ask_duration "CPU bleed duration" "30s")

        # Memory
        echo ""
        print_info "Memory monitoring configuration"
        AGENT_PREDICTIVE_MEMORY_THRESHOLD=$(ask_float "Memory threshold (%)" "85.0" 0 100)
        AGENT_PREDICTIVE_MEMORY_BLEED=$(ask_duration "Memory bleed duration" "30s")

        # Error Rate
        echo ""
        print_info "Error rate monitoring configuration"
        AGENT_PREDICTIVE_ERROR_THRESHOLD=$(ask_float "Error rate threshold (errors/min)" "10.0" 0 10000)
        AGENT_PREDICTIVE_ERROR_WINDOW=$(ask_duration "Error rate window" "60s")
        AGENT_PREDICTIVE_ERROR_BLEED=$(ask_duration "Error bleed duration" "30s")

        print_success "Predictive health enabled"
    else
        AGENT_PREDICTIVE_ENABLED="false"
        print_info "Predictive health disabled"
    fi
}

# =============================================================================
# YAML GENERATION
# =============================================================================

generate_yaml() {
    print_section "Generating Configuration File"

    local yaml=""

    # Header comment
    yaml+="# OpenGSLB Configuration\n"
    yaml+="# Generated by config-wizard.sh v${VERSION}\n"
    yaml+="# Generated on: $(date)\n"
    yaml+="#\n"
    yaml+="# Documentation: https://opengslb.readthedocs.io\n"
    yaml+="\n"

    # Mode
    yaml+="mode: $MODE\n"
    yaml+="\n"

    # Logging
    yaml+="# Logging configuration\n"
    yaml+="logging:\n"
    yaml+="  level: $LOG_LEVEL\n"
    yaml+="  format: $LOG_FORMAT\n"
    yaml+="\n"

    # Metrics
    yaml+="# Prometheus metrics\n"
    yaml+="metrics:\n"
    yaml+="  enabled: $METRICS_ENABLED\n"
    if [[ "$METRICS_ENABLED" == "true" ]]; then
        yaml+="  address: \"$METRICS_ADDRESS\"\n"
    fi
    yaml+="\n"

    if [[ "$MODE" == "overwatch" ]]; then
        yaml+=$(generate_overwatch_yaml)
    else
        yaml+=$(generate_agent_yaml)
    fi

    # Write to file
    echo -e "$yaml" > "$OUTPUT_FILE"

    print_success "Configuration written to: $OUTPUT_FILE"
    echo ""
    print_info "Review the generated configuration before using it."
    print_warning "Remember to secure the file: chmod 640 $OUTPUT_FILE"
}

generate_overwatch_yaml() {
    local yaml=""

    # DNS
    yaml+="# DNS server configuration\n"
    yaml+="dns:\n"
    yaml+="  listen_address: \"$DNS_LISTEN_ADDRESS\"\n"
    yaml+="  default_ttl: $DNS_DEFAULT_TTL\n"
    yaml+="  return_last_healthy: $DNS_RETURN_LAST_HEALTHY\n"
    yaml+="\n"

    # Overwatch section
    yaml+="# Overwatch configuration\n"
    yaml+="overwatch:\n"
    yaml+="  identity:\n"
    yaml+="    node_id: \"$OVERWATCH_NODE_ID\"\n"
    if [[ -n "$OVERWATCH_REGION" ]]; then
        yaml+="    region: \"$OVERWATCH_REGION\"\n"
    fi
    yaml+="\n"
    yaml+="  data_dir: \"$OVERWATCH_DATA_DIR\"\n"
    yaml+="\n"

    # Agent tokens
    if [[ ${#OVERWATCH_AGENT_TOKENS[@]} -gt 0 ]]; then
        yaml+="  agent_tokens:\n"
        for token_pair in "${OVERWATCH_AGENT_TOKENS[@]}"; do
            local service token
            service=$(echo "$token_pair" | cut -d':' -f1)
            token=$(echo "$token_pair" | cut -d':' -f2-)
            yaml+="    $service: \"$token\"\n"
        done
        yaml+="\n"
    fi

    # Gossip
    yaml+="  gossip:\n"
    yaml+="    bind_address: \"$GOSSIP_BIND_ADDRESS\"\n"
    yaml+="    encryption_key: \"$GOSSIP_ENCRYPTION_KEY\"\n"
    yaml+="    probe_interval: $GOSSIP_PROBE_INTERVAL\n"
    yaml+="    probe_timeout: $GOSSIP_PROBE_TIMEOUT\n"
    yaml+="    gossip_interval: $GOSSIP_INTERVAL\n"
    yaml+="\n"

    # Validation
    yaml+="  validation:\n"
    yaml+="    enabled: $VALIDATION_ENABLED\n"
    if [[ "$VALIDATION_ENABLED" == "true" ]]; then
        yaml+="    check_interval: $VALIDATION_CHECK_INTERVAL\n"
        yaml+="    check_timeout: $VALIDATION_CHECK_TIMEOUT\n"
    fi
    yaml+="\n"

    # Stale
    yaml+="  stale:\n"
    yaml+="    threshold: $STALE_THRESHOLD\n"
    yaml+="    remove_after: $STALE_REMOVE_AFTER\n"
    yaml+="\n"

    # DNSSEC
    yaml+="  dnssec:\n"
    yaml+="    enabled: $DNSSEC_ENABLED\n"
    if [[ "$DNSSEC_ENABLED" == "true" ]]; then
        yaml+="    algorithm: $DNSSEC_ALGORITHM\n"
        if [[ ${#DNSSEC_KEY_SYNC_PEERS[@]} -gt 0 ]]; then
            yaml+="    key_sync:\n"
            yaml+="      peers:\n"
            for peer in "${DNSSEC_KEY_SYNC_PEERS[@]}"; do
                yaml+="        - \"$peer\"\n"
            done
            yaml+="      poll_interval: $DNSSEC_KEY_SYNC_POLL_INTERVAL\n"
            yaml+="      timeout: $DNSSEC_KEY_SYNC_TIMEOUT\n"
        fi
    else
        yaml+="    security_acknowledgment: \"I understand that disabling DNSSEC reduces security and I accept the risks\"\n"
    fi
    yaml+="\n"

    # Geolocation
    if [[ -n "$GEOLOCATION_DATABASE_PATH" ]]; then
        yaml+="  geolocation:\n"
        yaml+="    database_path: \"$GEOLOCATION_DATABASE_PATH\"\n"
        yaml+="    default_region: \"$GEOLOCATION_DEFAULT_REGION\"\n"
        yaml+="    ecs_enabled: $GEOLOCATION_ECS_ENABLED\n"
        if [[ ${#GEOLOCATION_CUSTOM_MAPPINGS[@]} -gt 0 ]]; then
            yaml+="    custom_mappings:\n"
            for mapping in "${GEOLOCATION_CUSTOM_MAPPINGS[@]}"; do
                local cidr region comment
                cidr=$(echo "$mapping" | cut -d'|' -f1)
                region=$(echo "$mapping" | cut -d'|' -f2)
                comment=$(echo "$mapping" | cut -d'|' -f3)
                yaml+="      - cidr: \"$cidr\"\n"
                yaml+="        region: \"$region\"\n"
                if [[ -n "$comment" ]]; then
                    yaml+="        comment: \"$comment\"\n"
                fi
            done
        fi
        yaml+="\n"
    fi

    # API
    yaml+="# Management API\n"
    yaml+="api:\n"
    yaml+="  enabled: $API_ENABLED\n"
    if [[ "$API_ENABLED" == "true" ]]; then
        yaml+="  address: \"$API_ADDRESS\"\n"
        yaml+="  allowed_networks:\n"
        for network in "${API_ALLOWED_NETWORKS[@]}"; do
            yaml+="    - \"$network\"\n"
        done
        yaml+="  trust_proxy_headers: $API_TRUST_PROXY\n"
    fi
    yaml+="\n"

    # Regions
    yaml+="# Regions configuration\n"
    yaml+="regions:\n"
    for region_data in "${REGIONS[@]}"; do
        yaml+=$(generate_region_yaml "$region_data")
    done
    yaml+="\n"

    # Domains
    yaml+="# Domains configuration\n"
    yaml+="domains:\n"
    for domain_data in "${DOMAINS[@]}"; do
        yaml+=$(generate_domain_yaml "$domain_data")
    done

    echo "$yaml"
}

generate_region_yaml() {
    local region_data="$1"
    local yaml=""

    # Parse region data: name|servers|health_check|countries|continents
    local region_name servers_combined health_check countries continents
    region_name=$(echo "$region_data" | cut -d'|' -f1)
    servers_combined=$(echo "$region_data" | cut -d'|' -f2)
    health_check=$(echo "$region_data" | cut -d'|' -f3)
    countries=$(echo "$region_data" | cut -d'|' -f4)
    continents=$(echo "$region_data" | cut -d'|' -f5)

    yaml+="  - name: \"$region_name\"\n"

    # Countries
    if [[ -n "$countries" ]]; then
        yaml+="    countries:\n"
        IFS=',' read -ra country_arr <<< "$countries"
        for country in "${country_arr[@]}"; do
            yaml+="      - \"$(echo "$country" | tr -d ' ')\"\n"
        done
    fi

    # Continents
    if [[ -n "$continents" ]]; then
        yaml+="    continents:\n"
        IFS=',' read -ra continent_arr <<< "$continents"
        for continent in "${continent_arr[@]}"; do
            yaml+="      - \"$(echo "$continent" | tr -d ' ')\"\n"
        done
    fi

    # Servers
    yaml+="    servers:\n"
    IFS='§' read -ra server_arr <<< "$servers_combined"
    for server_data in "${server_arr[@]}"; do
        # Parse: address:port:weight:host
        local address port weight host
        address=$(echo "$server_data" | cut -d':' -f1)
        port=$(echo "$server_data" | cut -d':' -f2)
        weight=$(echo "$server_data" | cut -d':' -f3)
        host=$(echo "$server_data" | cut -d':' -f4)

        yaml+="      - address: \"$address\"\n"
        yaml+="        port: $port\n"
        yaml+="        weight: $weight\n"
        if [[ -n "$host" ]]; then
            yaml+="        host: \"$host\"\n"
        fi
    done

    # Health check
    # Parse: type:interval:timeout:path:host:failure:success
    local check_type interval timeout path host failure_threshold success_threshold
    check_type=$(echo "$health_check" | cut -d':' -f1)
    interval=$(echo "$health_check" | cut -d':' -f2)
    timeout=$(echo "$health_check" | cut -d':' -f3)
    path=$(echo "$health_check" | cut -d':' -f4)
    host=$(echo "$health_check" | cut -d':' -f5)
    failure_threshold=$(echo "$health_check" | cut -d':' -f6)
    success_threshold=$(echo "$health_check" | cut -d':' -f7)

    yaml+="    health_check:\n"
    yaml+="      type: $check_type\n"
    yaml+="      interval: $interval\n"
    yaml+="      timeout: $timeout\n"
    if [[ "$check_type" == "http" || "$check_type" == "https" ]]; then
        yaml+="      path: \"$path\"\n"
        if [[ -n "$host" ]]; then
            yaml+="      host: \"$host\"\n"
        fi
    fi
    yaml+="      failure_threshold: $failure_threshold\n"
    yaml+="      success_threshold: $success_threshold\n"

    echo "$yaml"
}

generate_domain_yaml() {
    local domain_data="$1"
    local yaml=""

    # Parse: name|algorithm|regions|ttl|latency_config
    local domain_name algorithm domain_regions ttl latency_config
    domain_name=$(echo "$domain_data" | cut -d'|' -f1)
    algorithm=$(echo "$domain_data" | cut -d'|' -f2)
    domain_regions=$(echo "$domain_data" | cut -d'|' -f3)
    ttl=$(echo "$domain_data" | cut -d'|' -f4)
    latency_config=$(echo "$domain_data" | cut -d'|' -f5)

    yaml+="  - name: \"$domain_name\"\n"
    yaml+="    routing_algorithm: $algorithm\n"

    if [[ "$ttl" != "0" && -n "$ttl" ]]; then
        yaml+="    ttl: $ttl\n"
    fi

    yaml+="    regions:\n"
    IFS=',' read -ra region_arr <<< "$domain_regions"
    for region in "${region_arr[@]}"; do
        yaml+="      - \"$region\"\n"
    done

    # Latency config
    if [[ -n "$latency_config" ]]; then
        local smoothing max_latency min_samples
        smoothing=$(echo "$latency_config" | cut -d':' -f1)
        max_latency=$(echo "$latency_config" | cut -d':' -f2)
        min_samples=$(echo "$latency_config" | cut -d':' -f3)

        yaml+="    latency_config:\n"
        yaml+="      smoothing_factor: $smoothing\n"
        yaml+="      max_latency_ms: $max_latency\n"
        yaml+="      min_samples: $min_samples\n"
    fi

    echo "$yaml"
}

generate_agent_yaml() {
    local yaml=""

    # Agent section
    yaml+="# Agent configuration\n"
    yaml+="agent:\n"
    yaml+="  identity:\n"
    yaml+="    service_token: \"$AGENT_SERVICE_TOKEN\"\n"
    yaml+="    region: \"$AGENT_REGION\"\n"
    yaml+="    cert_path: \"$AGENT_CERT_PATH\"\n"
    yaml+="    key_path: \"$AGENT_KEY_PATH\"\n"
    yaml+="\n"

    # Backends
    yaml+="  backends:\n"
    for backend_data in "${AGENT_BACKENDS[@]}"; do
        yaml+=$(generate_agent_backend_yaml "$backend_data")
    done
    yaml+="\n"

    # Gossip
    yaml+="  gossip:\n"
    yaml+="    encryption_key: \"$GOSSIP_ENCRYPTION_KEY\"\n"
    yaml+="    overwatch_nodes:\n"
    for node in "${AGENT_GOSSIP_OVERWATCH_NODES[@]}"; do
        yaml+="      - \"$node\"\n"
    done
    yaml+="\n"

    # Heartbeat
    yaml+="  heartbeat:\n"
    yaml+="    interval: $AGENT_HEARTBEAT_INTERVAL\n"
    yaml+="    missed_threshold: $AGENT_HEARTBEAT_MISSED_THRESHOLD\n"
    yaml+="\n"

    # Predictive
    yaml+="  predictive:\n"
    yaml+="    enabled: $AGENT_PREDICTIVE_ENABLED\n"
    if [[ "$AGENT_PREDICTIVE_ENABLED" == "true" ]]; then
        yaml+="    check_interval: $AGENT_PREDICTIVE_CHECK_INTERVAL\n"
        yaml+="    cpu:\n"
        yaml+="      threshold: $AGENT_PREDICTIVE_CPU_THRESHOLD\n"
        yaml+="      bleed_duration: $AGENT_PREDICTIVE_CPU_BLEED\n"
        yaml+="    memory:\n"
        yaml+="      threshold: $AGENT_PREDICTIVE_MEMORY_THRESHOLD\n"
        yaml+="      bleed_duration: $AGENT_PREDICTIVE_MEMORY_BLEED\n"
        yaml+="    error_rate:\n"
        yaml+="      threshold: $AGENT_PREDICTIVE_ERROR_THRESHOLD\n"
        yaml+="      window: $AGENT_PREDICTIVE_ERROR_WINDOW\n"
        yaml+="      bleed_duration: $AGENT_PREDICTIVE_ERROR_BLEED\n"
    fi

    echo "$yaml"
}

generate_agent_backend_yaml() {
    local backend_data="$1"
    local yaml=""

    # Parse: service:address:port:weight:health_check
    local service address port weight health_check
    service=$(echo "$backend_data" | cut -d':' -f1)
    address=$(echo "$backend_data" | cut -d':' -f2)
    port=$(echo "$backend_data" | cut -d':' -f3)
    weight=$(echo "$backend_data" | cut -d':' -f4)
    health_check=$(echo "$backend_data" | cut -d':' -f5-)

    yaml+="    - service: \"$service\"\n"
    yaml+="      address: \"$address\"\n"
    yaml+="      port: $port\n"
    yaml+="      weight: $weight\n"

    # Health check: type:interval:timeout:path:host:failure:success
    local check_type interval timeout path host failure_threshold success_threshold
    check_type=$(echo "$health_check" | cut -d':' -f1)
    interval=$(echo "$health_check" | cut -d':' -f2)
    timeout=$(echo "$health_check" | cut -d':' -f3)
    path=$(echo "$health_check" | cut -d':' -f4)
    host=$(echo "$health_check" | cut -d':' -f5)
    failure_threshold=$(echo "$health_check" | cut -d':' -f6)
    success_threshold=$(echo "$health_check" | cut -d':' -f7)

    yaml+="      health_check:\n"
    yaml+="        type: $check_type\n"
    yaml+="        interval: $interval\n"
    yaml+="        timeout: $timeout\n"
    if [[ "$check_type" == "http" || "$check_type" == "https" ]]; then
        yaml+="        path: \"$path\"\n"
        if [[ -n "$host" ]]; then
            yaml+="        host: \"$host\"\n"
        fi
    fi
    yaml+="      failure_threshold: $failure_threshold\n"
    yaml+="      success_threshold: $success_threshold\n"

    echo "$yaml"
}

# =============================================================================
# SUMMARY AND REVIEW
# =============================================================================

show_summary() {
    print_section "Configuration Summary"

    echo -e "${BOLD}Mode:${NC} $MODE"
    echo -e "${BOLD}Logging:${NC} $LOG_LEVEL ($LOG_FORMAT)"
    echo -e "${BOLD}Metrics:${NC} $METRICS_ENABLED"
    if [[ "$METRICS_ENABLED" == "true" ]]; then
        echo "  Address: $METRICS_ADDRESS"
    fi

    if [[ "$MODE" == "overwatch" ]]; then
        echo ""
        echo -e "${BOLD}Overwatch Settings:${NC}"
        echo "  Node ID: $OVERWATCH_NODE_ID"
        echo "  DNS Listen: $DNS_LISTEN_ADDRESS"
        echo "  Default TTL: $DNS_DEFAULT_TTL seconds"
        echo "  DNSSEC: $DNSSEC_ENABLED"
        echo "  Validation: $VALIDATION_ENABLED"
        echo "  API: $API_ENABLED"
        echo ""
        echo -e "${BOLD}Regions:${NC} ${#REGIONS[@]}"
        for region_data in "${REGIONS[@]}"; do
            local region_name
            region_name=$(echo "$region_data" | cut -d'|' -f1)
            echo "  - $region_name"
        done
        echo ""
        echo -e "${BOLD}Domains:${NC} ${#DOMAINS[@]}"
        for domain_data in "${DOMAINS[@]}"; do
            local domain_name algorithm
            domain_name=$(echo "$domain_data" | cut -d'|' -f1)
            algorithm=$(echo "$domain_data" | cut -d'|' -f2)
            echo "  - $domain_name ($algorithm)"
        done
    else
        echo ""
        echo -e "${BOLD}Agent Settings:${NC}"
        echo "  Region: $AGENT_REGION"
        echo "  Backends: ${#AGENT_BACKENDS[@]}"
        for backend_data in "${AGENT_BACKENDS[@]}"; do
            local service
            service=$(echo "$backend_data" | cut -d':' -f1)
            echo "    - $service"
        done
        echo "  Overwatch Nodes: ${#AGENT_GOSSIP_OVERWATCH_NODES[@]}"
        echo "  Predictive Health: $AGENT_PREDICTIVE_ENABLED"
    fi

    echo ""
    echo -e "${BOLD}Output File:${NC} $OUTPUT_FILE"
}

# =============================================================================
# MAIN ENTRY POINT
# =============================================================================

main() {
    # Check for help
    if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
        echo "OpenGSLB Configuration Wizard v${VERSION}"
        echo ""
        echo "Usage: $0 [output-file]"
        echo ""
        echo "An interactive wizard to generate OpenGSLB configuration files."
        echo ""
        echo "Arguments:"
        echo "  output-file   Path to write the configuration (default: ./opengslb-config.yaml)"
        echo ""
        echo "Examples:"
        echo "  $0                           # Interactive wizard, outputs to ./opengslb-config.yaml"
        echo "  $0 /etc/opengslb/config.yaml # Output to specific location"
        echo ""
        exit 0
    fi

    print_banner

    echo "Welcome to the OpenGSLB Configuration Wizard!"
    echo ""
    echo "This wizard will guide you through creating a configuration file"
    echo "for OpenGSLB. It will explain each option and provide sensible defaults."
    echo ""
    echo "Output file: $OUTPUT_FILE"
    echo ""

    if ! ask_yes_no "Ready to begin?" "y"; then
        echo "Wizard cancelled."
        exit 0
    fi

    # Mode selection
    select_mode

    # Common configuration
    configure_logging
    configure_metrics

    # Mode-specific configuration
    if [[ "$MODE" == "overwatch" ]]; then
        configure_overwatch
    else
        configure_agent
    fi

    # Summary
    show_summary

    echo ""
    if ask_yes_no "Generate configuration file?" "y"; then
        generate_yaml

        echo ""
        echo -e "${GREEN}${BOLD}Configuration wizard complete!${NC}"
        echo ""
        echo "Next steps:"
        echo "  1. Review the generated configuration: less $OUTPUT_FILE"
        echo "  2. Set proper permissions: chmod 640 $OUTPUT_FILE"
        echo "  3. Move to final location: sudo mv $OUTPUT_FILE /etc/opengslb/config.yaml"
        echo "  4. Start OpenGSLB: opengslb -config /etc/opengslb/config.yaml"
        echo ""

        if [[ "$MODE" == "overwatch" ]]; then
            echo -e "${YELLOW}Important:${NC}"
            echo "  - Save the gossip encryption key for agent configuration"
            echo "  - Save any generated agent tokens"
            echo "  - Ensure DNS port ($DNS_LISTEN_ADDRESS) is accessible"
        else
            echo -e "${YELLOW}Important:${NC}"
            echo "  - Ensure the gossip encryption key matches Overwatch"
            echo "  - Ensure the service token matches Overwatch agent_tokens"
            echo "  - Verify Overwatch nodes are reachable"
        fi
    else
        echo "Configuration generation cancelled."
    fi
}

# Run main function
main "$@"
