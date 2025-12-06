#!/bin/bash
# =============================================================================
# OpenGSLB Observability Stack Setup
# =============================================================================
# This script:
#   - Installs Docker and Docker Compose if not already installed
#   - Creates Prometheus configuration to scrape OpenGSLB metrics
#   - Deploys Prometheus and Grafana via Docker Compose
#   - Configures Grafana with Prometheus data source
#   - Installs pre-built OpenGSLB dashboard
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
    
    log_info "Updating package index..."
    sudo apt-get update
    
    log_info "Installing prerequisites..."
    sudo apt-get install -y \
        ca-certificates \
        curl \
        gnupg \
        lsb-release
    
    log_info "Adding Docker GPG key..."
    sudo install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    sudo chmod a+r /etc/apt/keyrings/docker.gpg
    
    log_info "Adding Docker repository..."
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
      sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    
    log_info "Installing Docker Engine..."
    sudo apt-get update
    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    
    log_info "Adding user to docker group..."
    sudo usermod -aG docker "$USER"
    
    log_info "Docker installed successfully"
}

check_docker_compose() {
    if docker compose version &> /dev/null; then
        log_info "Docker Compose is available ($(docker compose version --short))"
        return 0
    else
        log_warn "Docker Compose plugin not available"
        return 1
    fi
}

create_directory_structure() {
    log_step "Creating Directory Structure"
    
    mkdir -p "${INSTALL_DIR}/prometheus"
    mkdir -p "${INSTALL_DIR}/grafana/provisioning/datasources"
    mkdir -p "${INSTALL_DIR}/grafana/provisioning/dashboards"
    
    log_info "Created directory structure at ${INSTALL_DIR}"
}

create_prometheus_config() {
    log_step "Creating Prometheus Configuration"
    
    cat > "${INSTALL_DIR}/prometheus/prometheus.yml" << EOF
# Prometheus configuration for OpenGSLB monitoring
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  # Scrape OpenGSLB metrics
  - job_name: 'opengslb'
    static_configs:
      - targets: ['${OPENGSLB_METRICS_URL}']
    metrics_path: /metrics
    scheme: http

  # Scrape Prometheus itself
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
EOF
    
    log_info "Prometheus configuration created at ${INSTALL_DIR}/prometheus/prometheus.yml"
}

create_grafana_provisioning() {
    log_step "Creating Grafana Provisioning"
    
    # Datasource configuration
    cat > "${INSTALL_DIR}/grafana/provisioning/datasources/prometheus.yml" << EOF
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: true
EOF
    
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

create_grafana_dashboard() {
    log_step "Creating OpenGSLB Grafana Dashboard"
    
    cat > "${INSTALL_DIR}/grafana/provisioning/dashboards/opengslb-overview.json" << 'DASHBOARD_EOF'
{
  "annotations": {
    "list": []
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "panels": [
    {
      "datasource": {
        "type": "prometheus"
      },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "green", "value": null }]
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 0 },
      "id": 1,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "sum(rate(opengslb_dns_queries_total[5m])) * 60",
          "refId": "A"
        }
      ],
      "title": "DNS Queries per Minute",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "red", "value": null },
              { "color": "orange", "value": 1 },
              { "color": "green", "value": 2 }
            ]
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 0 },
      "id": 2,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "count(opengslb_health_check_results_total{result=\"success\"} > 0) or vector(0)",
          "refId": "A"
        }
      ],
      "title": "Healthy Servers",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "yellow", "value": 5 },
              { "color": "red", "value": 10 }
            ]
          },
          "unit": "ms"
        }
      },
      "gridPos": { "h": 4, "w": 6, "x": 12, "y": 0 },
      "id": 3,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["mean"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "histogram_quantile(0.95, sum(rate(opengslb_dns_query_duration_seconds_bucket[5m])) by (le)) * 1000",
          "refId": "A"
        }
      ],
      "title": "DNS Query Latency (p95)",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "blue", "value": null }]
          }
        }
      },
      "gridPos": { "h": 4, "w": 6, "x": 18, "y": 0 },
      "id": 4,
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "opengslb_build_info",
          "legendFormat": "{{version}}",
          "refId": "A"
        }
      ],
      "title": "OpenGSLB Version",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "tooltip": false, "viz": false, "legend": false },
            "lineInterpolation": "smooth",
            "lineWidth": 2,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "never",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "green", "value": null }]
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 4 },
      "id": 5,
      "options": {
        "legend": { "calcs": ["mean", "lastNotNull", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "sum(rate(opengslb_dns_queries_total[5m])) by (status) * 60",
          "legendFormat": "{{status}}",
          "refId": "A"
        }
      ],
      "title": "DNS Query Rate by Status",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "tooltip": false, "viz": false, "legend": false },
            "lineInterpolation": "smooth",
            "lineWidth": 2,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "never",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "green", "value": null }]
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 4 },
      "id": 6,
      "options": {
        "legend": { "calcs": ["mean", "lastNotNull", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "sum(rate(opengslb_dns_queries_total[5m])) by (domain) * 60",
          "legendFormat": "{{domain}}",
          "refId": "A"
        }
      ],
      "title": "DNS Query Rate by Domain",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "Latency (ms)",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "tooltip": false, "viz": false, "legend": false },
            "lineInterpolation": "smooth",
            "lineWidth": 2,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "never",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "yellow", "value": 5 },
              { "color": "red", "value": 10 }
            ]
          },
          "unit": "ms"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 12 },
      "id": 7,
      "options": {
        "legend": { "calcs": ["mean", "lastNotNull", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "histogram_quantile(0.50, sum(rate(opengslb_dns_query_duration_seconds_bucket[5m])) by (le)) * 1000",
          "legendFormat": "p50",
          "refId": "A"
        },
        {
          "datasource": { "type": "prometheus" },
          "expr": "histogram_quantile(0.95, sum(rate(opengslb_dns_query_duration_seconds_bucket[5m])) by (le)) * 1000",
          "legendFormat": "p95",
          "refId": "B"
        },
        {
          "datasource": { "type": "prometheus" },
          "expr": "histogram_quantile(0.99, sum(rate(opengslb_dns_query_duration_seconds_bucket[5m])) by (le)) * 1000",
          "legendFormat": "p99",
          "refId": "C"
        }
      ],
      "title": "DNS Query Latency Percentiles",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "tooltip": false, "viz": false, "legend": false },
            "lineInterpolation": "smooth",
            "lineWidth": 2,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "never",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "green", "value": null }]
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 12 },
      "id": 8,
      "options": {
        "legend": { "calcs": ["mean", "lastNotNull", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "sum(rate(opengslb_routing_decisions_total[5m])) by (server) * 60",
          "legendFormat": "{{server}}",
          "refId": "A"
        }
      ],
      "title": "Routing Decisions by Server",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "custom": {
            "align": "auto",
            "cellOptions": { "type": "color-text" },
            "inspect": false
          },
          "mappings": [
            { "options": { "success": { "color": "green", "index": 0, "text": "Healthy" } }, "type": "value" },
            { "options": { "failure": { "color": "red", "index": 1, "text": "Unhealthy" } }, "type": "value" }
          ],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "text", "value": null }]
          }
        },
        "overrides": [
          {
            "matcher": { "id": "byName", "options": "Server" },
            "properties": [{ "id": "custom.width", "value": 200 }]
          },
          {
            "matcher": { "id": "byName", "options": "Region" },
            "properties": [{ "id": "custom.width", "value": 150 }]
          }
        ]
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 20 },
      "id": 9,
      "options": {
        "cellHeight": "sm",
        "footer": { "countRows": false, "fields": "", "reducer": ["sum"], "show": false },
        "showHeader": true
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "topk(100, opengslb_health_check_results_total) by (region, server, result)",
          "format": "table",
          "instant": true,
          "refId": "A"
        }
      ],
      "title": "Server Health Status",
      "transformations": [
        { "id": "groupBy", "options": { "fields": { "region": { "aggregations": [], "operation": "groupby" }, "result": { "aggregations": ["lastNotNull"], "operation": "aggregate" }, "server": { "aggregations": [], "operation": "groupby" } } } }
      ],
      "type": "table"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "tooltip": false, "viz": false, "legend": false },
            "lineInterpolation": "smooth",
            "lineWidth": 2,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "never",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [{ "color": "green", "value": null }]
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 20 },
      "id": 10,
      "options": {
        "legend": { "calcs": ["mean", "lastNotNull", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "sum(rate(opengslb_health_check_results_total[5m])) by (result) * 60",
          "legendFormat": "{{result}}",
          "refId": "A"
        }
      ],
      "title": "Health Check Results",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "Latency (ms)",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "tooltip": false, "viz": false, "legend": false },
            "lineInterpolation": "smooth",
            "lineWidth": 2,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "never",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "yellow", "value": 100 },
              { "color": "red", "value": 500 }
            ]
          },
          "unit": "ms"
        }
      },
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 28 },
      "id": 11,
      "options": {
        "legend": { "calcs": ["mean", "lastNotNull", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "datasource": { "type": "prometheus" },
          "expr": "histogram_quantile(0.95, sum(rate(opengslb_health_check_duration_seconds_bucket[5m])) by (region, server, le)) * 1000",
          "legendFormat": "{{region}}/{{server}}",
          "refId": "A"
        }
      ],
      "title": "Health Check Latency by Server (p95)",
      "type": "timeseries"
    }
  ],
  "refresh": "5s",
  "schemaVersion": 38,
  "style": "dark",
  "tags": ["opengslb", "dns", "gslb"],
  "templating": { "list": [] },
  "time": { "from": "now-15m", "to": "now" },
  "timepicker": {},
  "timezone": "",
  "title": "OpenGSLB Overview",
  "uid": "opengslb-overview",
  "version": 1,
  "weekStart": ""
}
DASHBOARD_EOF
    
    log_info "Grafana dashboard created at ${INSTALL_DIR}/grafana/provisioning/dashboards/opengslb-overview.json"
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

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

create_diagnostic_script() {
    log_step "Creating Diagnostic Script"
    
    cat > "${INSTALL_DIR}/diagnose.sh" << 'EOF'
#!/bin/bash
# =============================================================================
# OpenGSLB Observability Diagnostics
# =============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step() { echo -e "\n${BLUE}=== $1 ===${NC}"; }

log_step "Checking OpenGSLB Metrics Endpoint"

if curl -s http://localhost:9090/metrics > /dev/null; then
    log_info "OpenGSLB metrics endpoint is accessible"
    echo ""
    echo "Sample metrics:"
    curl -s http://localhost:9090/metrics | grep "opengslb_" | head -10
else
    log_error "Cannot reach OpenGSLB metrics at http://localhost:9090/metrics"
    log_error "Is OpenGSLB running?"
    exit 1
fi

log_step "Checking Prometheus"

if curl -s http://localhost:9091/-/healthy > /dev/null; then
    log_info "Prometheus is running"
else
    log_error "Prometheus is not accessible at http://localhost:9091"
    exit 1
fi

log_step "Checking Prometheus Targets"

targets=$(curl -s http://localhost:9091/api/v1/targets | jq -r '.data.activeTargets[] | select(.labels.job=="opengslb") | .health' 2>/dev/null || echo "unknown")

if [ "$targets" == "up" ]; then
    log_info "OpenGSLB target is UP in Prometheus"
else
    log_error "OpenGSLB target is DOWN or unknown in Prometheus"
    echo ""
    echo "Target details:"
    curl -s http://localhost:9091/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job=="opengslb")' 2>/dev/null || echo "Could not fetch target details (is jq installed?)"
fi

log_step "Checking Prometheus Metrics"

metric_count=$(curl -s 'http://localhost:9091/api/v1/query?query=opengslb_dns_queries_total' | jq -r '.data.result | length' 2>/dev/null || echo "0")

if [ "$metric_count" -gt 0 ]; then
    log_info "Prometheus has scraped OpenGSLB metrics ($metric_count series found)"
    echo ""
    echo "Sample query result:"
    curl -s 'http://localhost:9091/api/v1/query?query=opengslb_dns_queries_total' | jq '.data.result[0]' 2>/dev/null || echo "Could not display result"
else
    log_error "Prometheus has not scraped any OpenGSLB metrics yet"
    log_error "Wait a few seconds and try again, or check Prometheus logs"
fi

log_step "Checking Grafana"

if curl -s http://localhost:3000/api/health > /dev/null; then
    log_info "Grafana is running"
else
    log_error "Grafana is not accessible at http://localhost:3000"
    exit 1
fi

log_step "Checking Grafana Datasource"

datasource_uid=$(curl -s -u admin:admin http://localhost:3000/api/datasources 2>/dev/null | jq -r '.[0].uid' 2>/dev/null || echo "")

if [ -n "$datasource_uid" ] && [ "$datasource_uid" != "null" ]; then
    log_info "Grafana datasource UID: $datasource_uid"
else
    log_error "No Prometheus datasource found in Grafana or could not authenticate"
fi

log_step "All Checks Complete"
echo ""
echo "If all checks passed but dashboard shows 'No Data':"
echo "  1. Make sure OpenGSLB is receiving DNS queries"
echo "  2. Wait 30 seconds for metrics to accumulate"
echo "  3. Try refreshing the Grafana dashboard"
EOF
    
    chmod +x "${INSTALL_DIR}/diagnose.sh"
    log_info "Diagnostic script created at ${INSTALL_DIR}/diagnose.sh"
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
  - Pre-loaded OpenGSLB dashboard

## Quick Start

\`\`\`bash
# Start the stack
./manage.sh start

# View logs
./manage.sh logs

# Stop the stack
./manage.sh stop

# Diagnose issues
./diagnose.sh
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

## Pre-installed Dashboard

The OpenGSLB Overview dashboard is automatically loaded and includes:

- DNS Queries per Minute
- Healthy Servers count
- DNS Query Latency (p95)
- OpenGSLB Version
- DNS Query Rate by Status
- DNS Query Rate by Domain
- DNS Query Latency Percentiles (p50, p95, p99)
- Routing Decisions by Server
- Server Health Status table
- Health Check Results
- Health Check Latency by Server

## Troubleshooting

Run the diagnostic script:
\`\`\`bash
./diagnose.sh
\`\`\`

### Common Issues

**Prometheus can't reach OpenGSLB**
- Verify OpenGSLB is running: \`curl http://localhost:9090/metrics\`
- Check Prometheus targets: http://localhost:${PROMETHEUS_PORT}/targets
- View Prometheus logs: \`./manage.sh logs prometheus\`

**Dashboard shows "No Data"**
- Make sure OpenGSLB is receiving DNS queries
- Wait 30 seconds for metrics to accumulate
- Run \`./diagnose.sh\` to check connectivity

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
    echo -e "  ./diagnose.sh        # Troubleshoot issues"
    echo ""
    echo -e "${BLUE}Dashboard:${NC}"
    echo -e "  OpenGSLB Overview dashboard is pre-loaded in Grafana"
    echo ""
    echo -e "${BLUE}Next Steps:${NC}"
    echo -e "  1. Ensure OpenGSLB is running with metrics on port 9090"
    echo -e "  2. Open Prometheus to verify scraping: http://localhost:${PROMETHEUS_PORT}/targets"
    echo -e "  3. Open Grafana dashboard: http://localhost:${GRAFANA_PORT}"
    echo ""
}

main() {
    echo "=============================================="
    echo "OpenGSLB Observability Stack Setup"
    echo "=============================================="
    
    local docker_installed=false
    
    if ! check_docker; then
        install_docker
        docker_installed=true
    fi
    
    if ! check_docker_compose && ! $docker_installed; then
        log_error "Docker Compose plugin not found. Try reinstalling Docker."
        exit 1
    fi
    
    create_directory_structure
    create_prometheus_config
    create_grafana_provisioning
    create_grafana_dashboard
    create_docker_compose
    create_management_script
    create_diagnostic_script
    create_readme
    
    if ! start_stack; then
        echo ""
        echo -e "${YELLOW}╔════════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${YELLOW}║ Unable to start Docker containers. This usually means you     ║${NC}"
        echo -e "${YELLOW}║ need to log out and back in for Docker group permissions.     ║${NC}"
        echo -e "${YELLOW}║                                                                ║${NC}"
        echo -e "${YELLOW}║ After logging back in, run:                                   ║${NC}"
        echo -e "${YELLOW}║   cd ${INSTALL_DIR}                            ║${NC}"
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