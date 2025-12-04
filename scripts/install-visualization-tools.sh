#!/bin/bash
# =============================================================================
# OpenGSLB Observability Stack Setup
# =============================================================================
# This script:
#   - Installs Docker and Docker Compose if not already installed
#   - Creates Prometheus configuration to scrape OpenGSLB metrics
#   - Deploys Prometheus and Grafana via Docker Compose
#   - Configures Grafana with Prometheus data source
#
# Prerequisites:
#   - Ubuntu/Debian-based system (Pop!_OS)
#   - sudo access
#   - OpenGSLB running with metrics on port 9090
#
# Usage:
#   ./scripts/setup-observability.sh
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="${HOME}/opengslb-observability"
PROMETHEUS_PORT=9091
GRAFANA_PORT=3000
OPENGSLB_METRICS_URL="host.docker.internal:9090"

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "\n${BLUE}=== $1 ===${NC}"
}

check_docker() {
    if command -v docker &> /dev/null; then
        log_info "Docker is already installed ($(docker --version))"
        return 0
    else
        log_warn "Docker is not installed"
        return 1
    fi
}

install_docker() {
    log_step "Installing Docker"
    
    # Update package index
    log_info "Updating package index..."
    sudo apt-get update
    
    # Install prerequisites
    log_info "Installing prerequisites..."
    sudo apt-get install -y \
        ca-certificates \
        curl \
        gnupg \
        lsb-release
    
    # Add Docker's official GPG key
    log_info "Adding Docker GPG key..."
    sudo install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    sudo chmod a+r /etc/apt/keyrings/docker.gpg
    
    # Set up the repository
    log_info "Adding Docker repository..."
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(. /etc/os-release && echo "$UBUNTU_CODENAME") stable" | \
      sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    
    # Install Docker Engine
    log_info "Installing Docker Engine..."
    sudo apt-get update
    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    
    # Add current user to docker group
    log_info "Adding ${USER} to docker group..."
    sudo usermod -aG docker "${USER}"
    
    log_info "Docker installation complete"
}

create_prometheus_config() {
    log_step "Creating Prometheus Configuration"
    
    mkdir -p "${INSTALL_DIR}/prometheus"
    
    cat > "${INSTALL_DIR}/prometheus/prometheus.yml" << EOF
# Prometheus configuration for OpenGSLB
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    monitor: 'opengslb-monitor'

# Scrape configuration
scrape_configs:
  # OpenGSLB metrics
  - job_name: 'opengslb'
    static_configs:
      - targets: ['${OPENGSLB_METRICS_URL}']
        labels:
          service: 'opengslb'
          environment: 'development'
    
    # Increase timeout for slower health checks
    scrape_timeout: 10s
    
    # Metrics path (default is /metrics)
    metrics_path: '/metrics'

  # Prometheus self-monitoring
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
        labels:
          service: 'prometheus'
EOF
    
    log_info "Prometheus configuration created at ${INSTALL_DIR}/prometheus/prometheus.yml"
}

create_grafana_provisioning() {
    log_step "Creating Grafana Provisioning"
    
    # Create datasources directory
    mkdir -p "${INSTALL_DIR}/grafana/provisioning/datasources"
    
    # Prometheus datasource
    cat > "${INSTALL_DIR}/grafana/provisioning/datasources/prometheus.yml" << EOF
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: true
    jsonData:
      timeInterval: 15s
EOF
    
    # Create dashboards directory
    mkdir -p "${INSTALL_DIR}/grafana/provisioning/dashboards"
    
    # Dashboard provider
    cat > "${INSTALL_DIR}/grafana/provisioning/dashboards/default.yml" << EOF
apiVersion: 1

providers:
  - name: 'OpenGSLB'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /etc/grafana/provisioning/dashboards
EOF
    
    log_info "Grafana provisioning created"
}

create_docker_compose() {
    log_step "Creating Docker Compose Configuration"
    
    cat > "${INSTALL_DIR}/docker-compose.yml" << EOF
version: '3.8'

services:
  prometheus:
    image: prom/prometheus:latest
    container_name: opengslb-prometheus
    restart: unless-stopped
    ports:
      - "${PROMETHEUS_PORT}:9090"
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/usr/share/prometheus/console_libraries'
      - '--web.console.templates=/usr/share/prometheus/consoles'
      - '--web.enable-lifecycle'
    extra_hosts:
      - "host.docker.internal:host-gateway"
    networks:
      - monitoring

  grafana:
    image: grafana/grafana:latest
    container_name: opengslb-grafana
    restart: unless-stopped
    ports:
      - "${GRAFANA_PORT}:3000"
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_USERS_ALLOW_SIGN_UP=false
      - GF_SERVER_ROOT_URL=http://localhost:${GRAFANA_PORT}
      - GF_INSTALL_PLUGINS=
    volumes:
      - ./grafana/provisioning:/etc/grafana/provisioning:ro
      - grafana-data:/var/lib/grafana
    depends_on:
      - prometheus
    networks:
      - monitoring

networks:
  monitoring:
    driver: bridge

volumes:
  prometheus-data:
  grafana-data:
EOF
    
    log_info "Docker Compose configuration created at ${INSTALL_DIR}/docker-compose.yml"
}

create_management_script() {
    log_step "Creating Management Script"
    
    cat > "${INSTALL_DIR}/manage.sh" << 'EOF'
#!/bin/bash
# OpenGSLB Observability Stack Management

set -e

COMPOSE_FILE="docker-compose.yml"

case "$1" in
    start)
        echo "Starting observability stack..."
        docker compose -f "$COMPOSE_FILE" up -d
        echo "✓ Prometheus: http://localhost:9091"
        echo "✓ Grafana: http://localhost:3000 (admin/admin)"
        ;;
    stop)
        echo "Stopping observability stack..."
        docker compose -f "$COMPOSE_FILE" down
        ;;
    restart)
        echo "Restarting observability stack..."
        docker compose -f "$COMPOSE_FILE" restart
        ;;
    logs)
        docker compose -f "$COMPOSE_FILE" logs -f "${2:-}"
        ;;
    status)
        docker compose -f "$COMPOSE_FILE" ps
        ;;
    reload-prometheus)
        echo "Reloading Prometheus configuration..."
        docker exec opengslb-prometheus kill -HUP 1
        echo "✓ Prometheus configuration reloaded"
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|logs|status|reload-prometheus}"
        echo ""
        echo "Commands:"
        echo "  start              - Start Prometheus and Grafana"
        echo "  stop               - Stop all containers"
        echo "  restart            - Restart all containers"
        echo "  logs [service]     - View logs (optionally for specific service)"
        echo "  status             - Show container status"
        echo "  reload-prometheus  - Reload Prometheus config without restart"
        exit 1
        ;;
esac
EOF
    
    chmod +x "${INSTALL_DIR}/manage.sh"
    log_info "Management script created at ${INSTALL_DIR}/manage.sh"
}

create_readme() {
    log_step "Creating README"
    
    cat > "${INSTALL_DIR}/README.md" << EOF
# OpenGSLB Observability Stack

This directory contains Prometheus and Grafana configured to monitor OpenGSLB.

## Services

- **Prometheus**: Metrics collection and storage
  - Port: ${PROMETHEUS_PORT}
  - URL: http://localhost:${PROMETHEUS_PORT}
  - Scrapes OpenGSLB metrics from localhost:9090

- **Grafana**: Metrics visualization
  - Port: ${GRAFANA_PORT}
  - URL: http://localhost:${GRAFANA_PORT}
  - Default credentials: admin/admin

## Quick Start

\`\`\`bash
# Start the stack
./manage.sh start

# View logs
./manage.sh logs

# Stop the stack
./manage.sh stop
\`\`\`

## Management Commands

\`\`\`bash
./manage.sh start              # Start all services
./manage.sh stop               # Stop all services
./manage.sh restart            # Restart all services
./manage.sh logs [service]     # View logs
./manage.sh status             # Show container status
./manage.sh reload-prometheus  # Reload Prometheus config
\`\`\`

## Accessing Services

### Prometheus
1. Open http://localhost:${PROMETHEUS_PORT}
2. Go to Status → Targets to verify OpenGSLB is being scraped
3. Use the Graph tab to query metrics:
   - \`opengslb_dns_queries_total\`
   - \`opengslb_health_check_results_total\`
   - \`opengslb_routing_decisions_total\`

### Grafana
1. Open http://localhost:${GRAFANA_PORT}
2. Login with admin/admin (you'll be prompted to change password)
3. Prometheus datasource is pre-configured
4. Create dashboards or import community dashboards

## Creating Grafana Dashboards

1. Click "+" → Dashboard
2. Add Panel
3. Select Prometheus as data source
4. Enter PromQL queries for OpenGSLB metrics
5. Save dashboard

## Troubleshooting

### Prometheus can't reach OpenGSLB
- Verify OpenGSLB is running: \`curl http://localhost:9090/metrics\`
- Check Prometheus targets: http://localhost:${PROMETHEUS_PORT}/targets
- View Prometheus logs: \`./manage.sh logs prometheus\`

### Grafana can't connect to Prometheus
- Check Prometheus is running: \`./manage.sh status\`
- Verify datasource: Grafana → Configuration → Data Sources

## Configuration Files

- \`prometheus/prometheus.yml\` - Prometheus scrape config
- \`grafana/provisioning/datasources/\` - Auto-configured datasources
- \`docker-compose.yml\` - Container orchestration

## Data Persistence

Metrics data is persisted in Docker volumes:
- \`prometheus-data\` - Prometheus time-series database
- \`grafana-data\` - Grafana dashboards and settings

To reset all data:
\`\`\`bash
./manage.sh stop
docker volume rm opengslb-observability_prometheus-data
docker volume rm opengslb-observability_grafana-data
./manage.sh start
\`\`\`
EOF
    
    log_info "README created at ${INSTALL_DIR}/README.md"
}

start_stack() {
    log_step "Starting Observability Stack"
    
    cd "${INSTALL_DIR}"
    
    # Try to start with docker compose plugin first
    if docker compose version &> /dev/null; then
        docker compose up -d
    else
        log_error "Docker Compose plugin not found. Please log out and back in, then run:"
        log_error "  cd ${INSTALL_DIR} && docker compose up -d"
        return 1
    fi
    
    log_info "Observability stack started"
}

print_summary() {
    log_step "Setup Complete!"
    
    echo ""
    echo -e "${GREEN}✓ Installation directory: ${INSTALL_DIR}${NC}"
    echo ""
    echo -e "${BLUE}Services:${NC}"
    echo -e "  • Prometheus: http://localhost:${PROMETHEUS_PORT}"
    echo -e "  • Grafana:    http://localhost:${GRAFANA_PORT} (admin/admin)"
    echo ""
    echo -e "${BLUE}Management:${NC}"
    echo -e "  cd ${INSTALL_DIR}"
    echo -e "  ./manage.sh start    # Start services"
    echo -e "  ./manage.sh stop     # Stop services"
    echo -e "  ./manage.sh logs     # View logs"
    echo ""
    echo -e "${BLUE}Next Steps:${NC}"
    echo -e "  1. Ensure OpenGSLB is running with metrics on port 9090"
    echo -e "  2. Open Prometheus to verify scraping: http://localhost:${PROMETHEUS_PORT}/targets"
    echo -e "  3. Open Grafana and start building dashboards: http://localhost:${GRAFANA_PORT}"
    echo ""
}

main() {
    log_step "OpenGSLB Observability Stack Setup"
    
    # Check if Docker is installed
    if ! check_docker; then
        install_docker
        
        echo ""
        echo -e "${YELLOW}╔════════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${YELLOW}║                    IMPORTANT NOTICE                            ║${NC}"
        echo -e "${YELLOW}╠════════════════════════════════════════════════════════════════╣${NC}"
        echo -e "${YELLOW}║ Docker has been installed and you have been added to the      ║${NC}"
        echo -e "${YELLOW}║ 'docker' group.                                               ║${NC}"
        echo -e "${YELLOW}║                                                                ║${NC}"
        echo -e "${YELLOW}║ You MUST log out and log back in for group changes to take    ║${NC}"
        echo -e "${YELLOW}║ effect before you can use Docker without sudo.                ║${NC}"
        echo -e "${YELLOW}║                                                                ║${NC}"
        echo -e "${YELLOW}║ After logging back in, run:                                   ║${NC}"
        echo -e "${YELLOW}║   cd ${INSTALL_DIR}${NC}"
        echo -e "${YELLOW}║   ./manage.sh start                                           ║${NC}"
        echo -e "${YELLOW}╚════════════════════════════════════════════════════════════════╝${NC}"
        echo ""
        
        # Create config files so they're ready when user logs back in
        create_prometheus_config
        create_grafana_provisioning
        create_docker_compose
        create_management_script
        create_readme
        
        exit 0
    fi
    
    # Docker is already installed, proceed with setup
    create_prometheus_config
    create_grafana_provisioning
    create_docker_compose
    create_management_script
    create_readme
    
    # Try to start (will fail if user isn't in docker group yet)
    if ! start_stack; then
        echo ""
        echo -e "${YELLOW}╔════════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${YELLOW}║ Unable to start Docker containers. This usually means you     ║${NC}"
        echo -e "${YELLOW}║ need to log out and back in for Docker group permissions.     ║${NC}"
        echo -e "${YELLOW}║                                                                ║${NC}"
        echo -e "${YELLOW}║ After logging back in, run:                                   ║${NC}"
        echo -e "${YELLOW}║   cd ${INSTALL_DIR}${NC}"
        echo -e "${YELLOW}║   ./manage.sh start                                           ║${NC}"
        echo -e "${YELLOW}╚════════════════════════════════════════════════════════════════╝${NC}"
        echo ""
    else
        print_summary
        
        echo -e "${YELLOW}Note: If you just installed Docker, you may need to log out${NC}"
        echo -e "${YELLOW}and back in for all permissions to take effect.${NC}"
        echo ""
    fi
}

main "$@"
