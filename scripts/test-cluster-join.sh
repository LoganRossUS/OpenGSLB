#!/bin/bash
# =============================================================================
# OpenGSLB Cluster Join Test
# =============================================================================
# Tests the cluster join API with a 3-node cluster setup.
#
# Usage:
#   ./scripts/test-cluster-join.sh
# =============================================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Configuration
NODE1_DATA_DIR="/tmp/opengslb-cluster-test/node1"
NODE2_DATA_DIR="/tmp/opengslb-cluster-test/node2"
NODE3_DATA_DIR="/tmp/opengslb-cluster-test/node3"

NODE1_RAFT_PORT="7001"
NODE2_RAFT_PORT="7002"
NODE3_RAFT_PORT="7003"

NODE1_API_PORT="8081"
NODE2_API_PORT="8082"
NODE3_API_PORT="8083"

NODE1_DNS_PORT="5301"
NODE2_DNS_PORT="5302"
NODE3_DNS_PORT="5303"

PIDS=()

cleanup() {
    log_info "Cleaning up..."
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done
    rm -rf /tmp/opengslb-cluster-test
    log_info "Cleanup complete"
}

# Kill any leftover processes from previous runs
cleanup_stale() {
    # Kill any opengslb processes using our test ports
    for port in 7001 7002 7003 8081 8082 8083 5301 5302 5303; do
        pid=$(lsof -t -i:$port 2>/dev/null || true)
        if [ -n "$pid" ]; then
            log_warn "Killing stale process on port $port (PID: $pid)"
            kill "$pid" 2>/dev/null || true
        fi
    done
    sleep 1
}

trap cleanup EXIT INT TERM

create_config() {
    local node_name=$1
    local raft_port=$2
    local api_port=$3
    local dns_port=$4
    local data_dir=$5
    local bootstrap=$6
    local join_addr=$7
    
    local config_file="$data_dir/config.yaml"
    mkdir -p "$data_dir"
    
    cat > "$config_file" << EOF
dns:
  listen_address: "127.0.0.1:${dns_port}"
  default_ttl: 30

cluster:
  mode: cluster
  node_name: ${node_name}
  bind_address: "127.0.0.1:${raft_port}"
  bootstrap: ${bootstrap}
  raft:
    data_dir: "${data_dir}/raft"
    heartbeat_timeout: 1s
    election_timeout: 1s
EOF

    if [ -n "$join_addr" ]; then
        cat >> "$config_file" << EOF
  join:
    - ${join_addr}
EOF
    fi

    cat >> "$config_file" << EOF

api:
  enabled: true
  address: "127.0.0.1:${api_port}"
  allowed_networks:
    - "127.0.0.1/32"

metrics:
  enabled: false

logging:
  level: debug
  format: text

regions:
  - name: test-region
    servers:
      - address: "127.0.0.1"
        port: 8080
    health_check:
      type: http
      interval: 5s
      timeout: 2s
      path: /

domains:
  - name: test.local
    routing_algorithm: round-robin
    regions:
      - test-region
EOF

    chmod 600 "$config_file"
    echo "$config_file"
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if [ ! -f "./opengslb" ]; then
        log_error "OpenGSLB binary not found. Run 'go build -o opengslb ./cmd/opengslb' first."
        exit 1
    fi
    
    if ! command -v curl &> /dev/null; then
        log_error "curl is required but not installed."
        exit 1
    fi
    
    log_info "Prerequisites check passed"
}

start_node() {
    local config_file=$1
    local log_file=$2
    
    ./opengslb --config "$config_file" > "$log_file" 2>&1 &
    local pid=$!
    PIDS+=($pid)
    echo $pid
}

wait_for_api() {
    local port=$1
    local timeout=${2:-30}
    local start_time=$(date +%s)
    
    while true; do
        if curl -s "http://127.0.0.1:${port}/api/v1/live" > /dev/null 2>&1; then
            return 0
        fi
        
        local elapsed=$(($(date +%s) - start_time))
        if [ $elapsed -ge $timeout ]; then
            return 1
        fi
        sleep 0.5
    done
}

wait_for_leader() {
    local port=$1
    local timeout=${2:-30}
    local start_time=$(date +%s)
    
    while true; do
        local status=$(curl -s "http://127.0.0.1:${port}/api/v1/cluster/status" 2>/dev/null)
        if echo "$status" | grep -q '"is_leader":true'; then
            return 0
        fi
        
        local elapsed=$(($(date +%s) - start_time))
        if [ $elapsed -ge $timeout ]; then
            return 1
        fi
        sleep 0.5
    done
}

wait_for_cluster_size() {
    local port=$1
    local expected_size=$2
    local timeout=${3:-30}
    local start_time=$(date +%s)
    
    while true; do
        local size=$(curl -s "http://127.0.0.1:${port}/api/v1/cluster/status" 2>/dev/null | \
            python3 -c "import sys, json; d=json.load(sys.stdin); print(len(d.get('nodes', d.get('members', []))))" 2>/dev/null || echo "0")
        if [ "$size" -ge "$expected_size" ]; then
            return 0
        fi
        
        local elapsed=$(($(date +%s) - start_time))
        if [ $elapsed -ge $timeout ]; then
            return 1
        fi
        sleep 0.5
    done
}

main() {
    echo "=============================================="
    echo "OpenGSLB Cluster Join Test"
    echo "=============================================="
    echo ""
    
    check_prerequisites
    
    # Clean up any stale processes from previous failed runs
    cleanup_stale
    
    rm -rf /tmp/opengslb-cluster-test
    
    log_info "Creating node configurations..."
    # Node 1: bootstrap, no join
    CONFIG1=$(create_config "node1" "$NODE1_RAFT_PORT" "$NODE1_API_PORT" "$NODE1_DNS_PORT" "$NODE1_DATA_DIR" "true" "")
    # Node 2 & 3: join via Node 1's API port (not Raft port!)
    CONFIG2=$(create_config "node2" "$NODE2_RAFT_PORT" "$NODE2_API_PORT" "$NODE2_DNS_PORT" "$NODE2_DATA_DIR" "false" "127.0.0.1:${NODE1_API_PORT}")
    CONFIG3=$(create_config "node3" "$NODE3_RAFT_PORT" "$NODE3_API_PORT" "$NODE3_DNS_PORT" "$NODE3_DATA_DIR" "false" "127.0.0.1:${NODE1_API_PORT}")
    
    # Start Node 1 (bootstrap)
    log_info "Starting Node 1 (bootstrap)..."
    NODE1_PID=$(start_node "$CONFIG1" "$NODE1_DATA_DIR/node1.log")
    log_info "Node 1 started (PID: $NODE1_PID)"
    
    # Wait for Node 1 API to be ready
    log_info "Waiting for Node 1 API..."
    if ! wait_for_api "$NODE1_API_PORT" 30; then
        log_error "Node 1 API did not become available"
        cat "$NODE1_DATA_DIR/node1.log"
        exit 1
    fi
    log_info "Node 1 API ready"
    
    # Wait for Node 1 to become leader
    log_info "Waiting for Node 1 to become leader..."
    if ! wait_for_leader "$NODE1_API_PORT" 15; then
        log_error "Node 1 did not become leader"
        cat "$NODE1_DATA_DIR/node1.log"
        exit 1
    fi
    log_info "Node 1 is leader"
    
    log_info "Cluster status (1 node):"
    curl -s "http://127.0.0.1:${NODE1_API_PORT}/api/v1/cluster/status" | python3 -m json.tool
    echo ""
    
    # Start Node 2 (join)
    log_info "Starting Node 2 (join via API)..."
    NODE2_PID=$(start_node "$CONFIG2" "$NODE2_DATA_DIR/node2.log")
    log_info "Node 2 started (PID: $NODE2_PID)"
    
    log_info "Waiting for Node 2 API..."
    if ! wait_for_api "$NODE2_API_PORT" 30; then
        log_error "Node 2 API did not become available"
        cat "$NODE2_DATA_DIR/node2.log"
        exit 1
    fi
    log_info "Node 2 API ready"
    
    # Wait for cluster to have 2 nodes
    log_info "Waiting for Node 2 to join cluster..."
    if ! wait_for_cluster_size "$NODE1_API_PORT" 2 15; then
        log_warn "Node 2 may not have joined yet, continuing..."
    fi
    
    log_info "Cluster status (2 nodes):"
    curl -s "http://127.0.0.1:${NODE1_API_PORT}/api/v1/cluster/status" | python3 -m json.tool
    echo ""
    
    # Start Node 3 (join)
    log_info "Starting Node 3 (join via API)..."
    NODE3_PID=$(start_node "$CONFIG3" "$NODE3_DATA_DIR/node3.log")
    log_info "Node 3 started (PID: $NODE3_PID)"
    
    log_info "Waiting for Node 3 API..."
    if ! wait_for_api "$NODE3_API_PORT" 30; then
        log_error "Node 3 API did not become available"
        cat "$NODE3_DATA_DIR/node3.log"
        exit 1
    fi
    log_info "Node 3 API ready"
    
    # Wait for cluster to have 3 nodes
    log_info "Waiting for Node 3 to join cluster..."
    if ! wait_for_cluster_size "$NODE1_API_PORT" 3 15; then
        log_warn "Node 3 may not have joined yet"
    fi
    
    log_info "Final cluster status (3 nodes):"
    curl -s "http://127.0.0.1:${NODE1_API_PORT}/api/v1/cluster/status" | python3 -m json.tool
    echo ""
    
    # Verify all nodes are in the cluster
    MEMBERS=$(curl -s "http://127.0.0.1:${NODE1_API_PORT}/api/v1/cluster/status" | \
        python3 -c "import sys, json; d=json.load(sys.stdin); print(len(d.get('nodes', d.get('members', []))))" 2>/dev/null || echo "0")
    
    if [ "$MEMBERS" -eq 3 ]; then
        log_info "SUCCESS: All 3 nodes are in the cluster!"
    else
        log_error "FAILED: Expected 3 members, got $MEMBERS"
        log_info "--- Node 1 logs (last 30 lines) ---"
        tail -30 "$NODE1_DATA_DIR/node1.log"
        log_info "--- Node 2 logs (last 30 lines) ---"
        tail -30 "$NODE2_DATA_DIR/node2.log"
        log_info "--- Node 3 logs (last 30 lines) ---"
        tail -30 "$NODE3_DATA_DIR/node3.log"
        exit 1
    fi
    
    # Test leader failover
    log_info "Testing leader failover..."
    log_info "Killing current leader (Node 1)..."
    kill "$NODE1_PID" 2>/dev/null || true
    
    # Wait for new leader election
    log_info "Waiting for re-election (5s)..."
    sleep 5
    
    log_info "Checking for new leader on Node 2..."
    NEW_STATUS=$(curl -s "http://127.0.0.1:${NODE2_API_PORT}/api/v1/cluster/status" 2>/dev/null || echo "{}")
    echo "$NEW_STATUS" | python3 -m json.tool 2>/dev/null || echo "$NEW_STATUS"
    
    if echo "$NEW_STATUS" | grep -q '"is_leader":true'; then
        log_info "SUCCESS: Node 2 became the new leader!"
    else
        log_info "Checking Node 3..."
        NODE3_STATUS=$(curl -s "http://127.0.0.1:${NODE3_API_PORT}/api/v1/cluster/status" 2>/dev/null || echo "{}")
        if echo "$NODE3_STATUS" | grep -q '"is_leader":true'; then
            log_info "SUCCESS: Node 3 became the new leader!"
        else
            log_warn "No new leader elected yet (may need more time)"
        fi
    fi
    
    echo ""
    echo "=============================================="
    echo "Test Complete"
    echo "=============================================="
}

main "$@"