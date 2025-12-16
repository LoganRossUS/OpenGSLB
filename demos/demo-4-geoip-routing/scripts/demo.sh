#!/bin/bash
# demo.sh
# Interactive GeoIP Routing Demo for OpenGSLB Demo 4

set -e

OVERWATCH="172.28.0.10"
DNS_PORT="53"
API_PORT="8080"
DOMAIN="app.global.example.com"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'
BOLD='\033[1m'

# Box drawing
print_header() {
    echo ""
    echo -e "${CYAN}=================================================================${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}=================================================================${NC}"
    echo ""
}

print_subheader() {
    echo -e "${YELLOW}--- $1 ---${NC}"
}

run_cmd() {
    echo -e "${YELLOW}\$ $1${NC}"
    eval "$1"
    echo ""
}

pause() {
    echo -e "${GREEN}Press Enter to continue...${NC}"
    read -r
}

# Query DNS with simulated source IP using EDNS Client Subnet
geo_query() {
    local SOURCE_IP=$1
    local QUERY_DOMAIN=$2

    echo -e "${BLUE}Querying from: ${BOLD}$SOURCE_IP${NC}"

    # Use dig with +subnet option to simulate EDNS Client Subnet
    RESULT=$(dig @${OVERWATCH} -p ${DNS_PORT} ${QUERY_DOMAIN} A +short +subnet=${SOURCE_IP}/32 2>/dev/null | head -1)

    if [ -z "$RESULT" ]; then
        echo -e "${RED}   No response${NC}"
        return 1
    fi

    echo -e "${GREEN}   Routed to: ${BOLD}$RESULT${NC}"
    echo ""
}

# Test geo routing via API (without DNS)
api_geo_test() {
    local SOURCE_IP=$1

    echo -e "${BLUE}API Geo Test for: ${BOLD}$SOURCE_IP${NC}"

    RESULT=$(curl -s "http://${OVERWATCH}:${API_PORT}/api/v1/geo/test?ip=${SOURCE_IP}" 2>/dev/null)

    if [ -z "$RESULT" ] || [ "$RESULT" = "null" ]; then
        echo -e "${RED}   API not responding or geo endpoint not available${NC}"
        return 1
    fi

    COUNTRY=$(echo "$RESULT" | jq -r '.country // "N/A"')
    CONTINENT=$(echo "$RESULT" | jq -r '.continent // "N/A"')
    REGION=$(echo "$RESULT" | jq -r '.region // .matched_region // "N/A"')
    MATCH_TYPE=$(echo "$RESULT" | jq -r '.match_type // "N/A"')

    echo -e "   Country:    ${BOLD}$COUNTRY${NC}"
    echo -e "   Continent:  ${BOLD}$CONTINENT${NC}"
    echo -e "   Region:     ${GREEN}${BOLD}$REGION${NC}"
    echo -e "   Match Type: ${CYAN}$MATCH_TYPE${NC}"

    if [ "$MATCH_TYPE" = "custom_mapping" ] || [ "$MATCH_TYPE" = "custom_cidr" ]; then
        CIDR=$(echo "$RESULT" | jq -r '.cidr // .matched_cidr // "N/A"')
        COMMENT=$(echo "$RESULT" | jq -r '.comment // "N/A"')
        echo -e "   CIDR Match: ${MAGENTA}$CIDR${NC} ($COMMENT)"
    fi
    echo ""
}

# ==========================================================================
# DEMO SCENARIOS
# ==========================================================================

demo_geoip_countries() {
    print_header "SCENARIO 1: GeoIP Routing by Country"

    echo "Demonstrating how public IPs from different countries route to"
    echo "their nearest regional datacenter using the GeoIP database."
    echo ""

    print_subheader "United States (8.8.8.8 - Google DNS) -> us-east"
    geo_query "8.8.8.8" "$DOMAIN"
    api_geo_test "8.8.8.8"
    sleep 1

    print_subheader "Germany (185.228.168.9 - CleanBrowsing) -> eu-west"
    geo_query "185.228.168.9" "$DOMAIN"
    api_geo_test "185.228.168.9"
    sleep 1

    print_subheader "Japan (202.12.29.205 - JPNIC) -> ap-southeast"
    geo_query "202.12.29.205" "$DOMAIN"
    api_geo_test "202.12.29.205"
    sleep 1

    print_subheader "Australia (1.1.1.1 - Cloudflare) -> ap-southeast"
    geo_query "1.1.1.1" "$DOMAIN"
    api_geo_test "1.1.1.1"
    sleep 1

    print_subheader "Brazil (200.160.0.8) -> us-east (South America fallback)"
    geo_query "200.160.0.8" "$DOMAIN"
    api_geo_test "200.160.0.8"
}

demo_custom_cidrs() {
    print_header "SCENARIO 2: Custom CIDR Mappings"

    echo "Private/internal IPs use custom CIDR mappings instead of GeoIP."
    echo "These are checked FIRST, before any GeoIP database lookup."
    echo ""

    print_subheader "Corporate HQ (10.50.x.x) -> us-chicago"
    geo_query "10.50.100.50" "$DOMAIN"
    api_geo_test "10.50.100.50"
    sleep 1

    print_subheader "VPN Users (172.16.x.x) -> eu-london"
    geo_query "172.16.50.100" "$DOMAIN"
    api_geo_test "172.16.50.100"
    sleep 1

    print_subheader "Home Office (192.168.x.x) -> us-east"
    geo_query "192.168.1.100" "$DOMAIN"
    api_geo_test "192.168.1.100"
}

demo_fallback() {
    print_header "SCENARIO 3: Fallback for Unknown IPs"

    echo "IPs not in GeoIP database and not matching any custom CIDR"
    echo "fall back to the configured default region (us-east)."
    echo ""

    print_subheader "TEST-NET-1 (192.0.2.1 - Reserved, not in GeoIP)"
    geo_query "192.0.2.1" "$DOMAIN"
    api_geo_test "192.0.2.1"
    sleep 1

    print_subheader "TEST-NET-2 (198.51.100.1 - Reserved, not in GeoIP)"
    geo_query "198.51.100.1" "$DOMAIN"
    api_geo_test "198.51.100.1"
}

demo_region_switch() {
    print_header "SCENARIO 4: Real-Time Region Switching (User Journey)"

    echo "Simulating a user 'traveling' between locations."
    echo "Each query from a different IP routes to the appropriate region."
    echo ""

    echo -e "${BOLD}User Journey:${NC}"
    echo ""

    echo -e "${YELLOW}Starting in New York...${NC}"
    geo_query "8.8.8.8" "$DOMAIN"
    sleep 2

    echo -e "${YELLOW}Flying to London...${NC}"
    geo_query "185.228.168.9" "$DOMAIN"
    sleep 2

    echo -e "${YELLOW}Connecting to Corporate VPN...${NC}"
    geo_query "172.16.50.50" "$DOMAIN"
    sleep 2

    echo -e "${YELLOW}Flying to Tokyo...${NC}"
    geo_query "202.12.29.205" "$DOMAIN"
    sleep 2

    echo -e "${YELLOW}Arriving at Kentucky Office...${NC}"
    geo_query "10.50.100.50" "$DOMAIN"
}

demo_api_explorer() {
    print_header "SCENARIO 5: GeoIP API Explorer"

    echo "Explore geo routing configuration via the REST API."
    echo ""

    print_subheader "List Custom CIDR Mappings"
    run_cmd "curl -s 'http://${OVERWATCH}:${API_PORT}/api/v1/geo/mappings' | jq '.mappings // .'"

    print_subheader "Test IP Routing Decision"
    run_cmd "curl -s 'http://${OVERWATCH}:${API_PORT}/api/v1/geo/test?ip=8.8.8.8' | jq ."

    print_subheader "Check Backend Health by Region"
    run_cmd "curl -s 'http://${OVERWATCH}:${API_PORT}/api/v1/health/servers' | jq '.servers | group_by(.region) | map({region: .[0].region, servers: map({address, healthy})})'"
}

show_status() {
    print_header "SYSTEM STATUS"

    print_subheader "Backend Health"
    curl -s "http://${OVERWATCH}:${API_PORT}/api/v1/health/servers" 2>/dev/null | \
      jq -r '.servers[] | "\(.region)\t\(.address)\t\(.healthy)"' 2>/dev/null | \
      column -t -s $'\t' || echo "Could not fetch health status"
    echo ""
}

interactive_mode() {
    print_header "INTERACTIVE QUERY MODE"
    echo "Enter an IP address to test geo routing, or 'exit' to return to menu."
    echo ""
    echo "Example IPs to try:"
    echo "  8.8.8.8       (US - Google DNS)"
    echo "  185.228.168.9 (Germany)"
    echo "  1.1.1.1       (Australia)"
    echo "  202.12.29.205 (Japan)"
    echo "  10.50.100.50  (Custom CIDR - Kentucky)"
    echo "  172.16.50.50  (Custom CIDR - VPN)"
    echo ""

    while true; do
        read -p "Enter IP address (or 'exit'): " IP

        if [ "$IP" = "exit" ] || [ "$IP" = "quit" ] || [ "$IP" = "q" ]; then
            break
        fi

        if [[ ! $IP =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo -e "${RED}Invalid IP address format${NC}"
            continue
        fi

        echo ""
        geo_query "$IP" "$DOMAIN"
        api_geo_test "$IP"
    done
}

show_menu() {
    echo ""
    echo -e "${BOLD}+---------------------------------------------------------------+${NC}"
    echo -e "${BOLD}|           OpenGSLB Demo 4: GeoIP Routing                      |${NC}"
    echo -e "${BOLD}+---------------------------------------------------------------+${NC}"
    echo -e "${BOLD}|${NC}  1) GeoIP Routing by Country                                 ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  2) Custom CIDR Mappings                                     ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  3) Fallback for Unknown IPs                                 ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  4) Real-Time Region Switching (User Journey)               ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  5) API Explorer                                            ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  6) Show System Status                                      ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  7) Interactive Query Mode                                  ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  0) Run All Demos                                           ${BOLD}|${NC}"
    echo -e "${BOLD}|${NC}  q) Quit                                                    ${BOLD}|${NC}"
    echo -e "${BOLD}+---------------------------------------------------------------+${NC}"
    echo ""
}

# Main loop
main() {
    clear
    echo -e "${CYAN}"
    echo "  +---------------------------------------------------------------+"
    echo "  |                                                               |"
    echo "  |    ___                   ____ ____  _     ____                |"
    echo "  |   / _ \ _ __   ___ _ __ / ___/ ___|| |   | __ )               |"
    echo "  |  | | | | '_ \ / _ \ '_ \| |  \___ \| |   |  _ \               |"
    echo "  |  | |_| | |_) |  __/ | | | |__ ___) | |___| |_) |              |"
    echo "  |   \___/| .__/ \___|_| |_|\____|____/|_____|____/              |"
    echo "  |        |_|                                                    |"
    echo "  |                                                               |"
    echo "  |              Demo 4: GeoIP-Based Routing                      |"
    echo "  |                                                               |"
    echo "  +---------------------------------------------------------------+"
    echo -e "${NC}"

    while true; do
        show_menu
        read -p "Select option: " choice

        case $choice in
            1) demo_geoip_countries ;;
            2) demo_custom_cidrs ;;
            3) demo_fallback ;;
            4) demo_region_switch ;;
            5) demo_api_explorer ;;
            6) show_status ;;
            7) interactive_mode ;;
            0)
                demo_geoip_countries
                pause
                demo_custom_cidrs
                pause
                demo_fallback
                pause
                demo_region_switch
                ;;
            q|Q)
                echo "Goodbye!"
                exit 0
                ;;
            *)
                echo -e "${RED}Invalid option${NC}"
                ;;
        esac

        echo ""
        pause
    done
}

main "$@"
