#!/bin/bash
# =============================================================================
# OpenGSLB 3-Node Cluster Integration Test
# =============================================================================
# Tests Raft leader election with a 3-node cluster on localhost
#
# Usage:
#   ./scripts/test-cluster.sh
# =============================================================================

set -e

# Configuration
RAFT_BASE_PORT=7946
DNS_BASE_PORT=15353
METRICS_BASE_PORT=9090
API_BASE_PORT=9190
DATA_DIR="/tmp/opengslb-cluster-test"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Track PIDs
declare -a NODE_PIDS

cleanup() {
    log_info "Cleaning up..."
    for pid in "${NODE_PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done
    rm -rf "$DATA_DIR"
    log_info "Cleanup complete"
}

trap cleanup EXIT INT TERM

# Check prerequisites
if [ ! -f "./opengslb" ]; then
    log_error "opengslb binary not found. Run 'go build -o opengslb ./cmd/opengslb' first."
    exit 1
fi

# Create data directories
mkdir -p "$DATA_DIR/node1" "$DATA_DIR/node2" "$DATA_DIR/node3"

# Create config files
create_config() {
    local node_num=$1
    local bootstrap=$2
    local join_api_addr=$3  # This should be the API address, not Raft address
    
    local raft_port=$((RAFT_BASE_PORT + node_num - 1))
    local dns_port=$((DNS_BASE_PORT + node_num - 1))
    local metrics_port=$((METRICS_BASE_PORT + node_num - 1))
    local api_port=$((API_BASE_PORT + node_num - 1))
    
    cat > "$DATA_DIR/node${node_num}/config.yaml" << EOF
dns:
  listen_address: "127.0.0.1:${dns_port}"
  default_ttl: 30

cluster:
  mode: cluster
  node_name: "node${node_num}"
  bind_address: "127.0.0.1:${raft_port}"
  bootstrap: ${bootstrap}
  join: ${join_api_addr}
  raft:
    data_dir: "${DATA_DIR}/node${node_num}/raft"
    heartbeat_timeout: 1s
    election_timeout: 1s

regions:
  - name: test-region
    servers:
      - address: "127.0.0.1"
        port: 8080
        weight: 100
    health_check:
      type: http
      interval: 5s
      timeout: 2s
      path: /health

domains:
  - name: test.example.com
    routing_algorithm: round-robin
    regions:
      - test-region

logging:
  level: info
  format: text

metrics:
  enabled: true
  address: "127.0.0.1:${metrics_port}"

api:
  enabled: true
  address: ":${api_port}"
  allowed_networks:
    - "127.0.0.1/32"
EOF
    chmod 600 "$DATA_DIR/node${node_num}/config.yaml"
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

# Create configs
# IMPORTANT: join addresses must be API ports, not Raft ports!
# Node 1 API will be on port 9190
log_info "Creating node configurations..."
create_config 1 "true" "[]"                         # Bootstrap node
create_config 2 "false" '["127.0.0.1:9190"]'        # Join via node 1's API port
create_config 3 "false" '["127.0.0.1:9190"]'        # Join via node 1's API port

# Start node 1 (bootstrap)
log_info "Starting node 1 (bootstrap)..."
./opengslb --config "$DATA_DIR/node1/config.yaml" > "$DATA_DIR/node1/output.log" 2>&1 &
NODE_PIDS+=($!)

# Wait for node 1 API to be ready
log_info "Waiting for node 1 API..."
if ! wait_for_api "$API_BASE_PORT" 30; then
    log_error "Node 1 API did not become available"
    cat "$DATA_DIR/node1/output.log"
    exit 1
fi
log_info "Node 1 started (PID: ${NODE_PIDS[0]})"

# Wait for node 1 to become leader
log_info "Waiting for node 1 to become leader..."
if ! wait_for_leader "$API_BASE_PORT" 15; then
    log_error "Node 1 did not become leader"
    cat "$DATA_DIR/node1/output.log"
    exit 1
fi
log_info "Node 1 is leader"

# Start node 2
log_info "Starting node 2..."
./opengslb --config "$DATA_DIR/node2/config.yaml" > "$DATA_DIR/node2/output.log" 2>&1 &
NODE_PIDS+=($!)

# Wait for node 2 API
log_info "Waiting for node 2 API..."
if ! wait_for_api "$((API_BASE_PORT + 1))" 30; then
    log_error "Node 2 API did not become available"
    cat "$DATA_DIR/node2/output.log"
    exit 1
fi
log_info "Node 2 started (PID: ${NODE_PIDS[1]})"

# Wait for node 2 to join
log_info "Waiting for node 2 to join cluster..."
wait_for_cluster_size "$API_BASE_PORT" 2 15 || log_warn "Node 2 may not have joined yet"

# Start node 3
log_info "Starting node 3..."
./opengslb --config "$DATA_DIR/node3/config.yaml" > "$DATA_DIR/node3/output.log" 2>&1 &
NODE_PIDS+=($!)

# Wait for node 3 API
log_info "Waiting for node 3 API..."
if ! wait_for_api "$((API_BASE_PORT + 2))" 30; then
    log_error "Node 3 API did not become available"
    cat "$DATA_DIR/node3/output.log"
    exit 1
fi
log_info "Node 3 started (PID: ${NODE_PIDS[2]})"

# Wait for node 3 to join
log_info "Waiting for node 3 to join cluster..."
wait_for_cluster_size "$API_BASE_PORT" 3 15 || log_warn "Node 3 may not have joined yet"

# Verify all nodes running
log_info "Verifying all nodes are running..."
for i in 1 2 3; do
    idx=$((i - 1))
    if ! kill -0 "${NODE_PIDS[$idx]}" 2>/dev/null; then
        log_error "Node $i is not running"
        cat "$DATA_DIR/node${i}/output.log"
        exit 1
    fi
done
log_info "All 3 nodes are running"

# Check cluster status
log_info "Cluster status:"
curl -s "http://127.0.0.1:${API_BASE_PORT}/api/v1/cluster/status" | python3 -m json.tool 2>/dev/null || true
echo ""


wait_for_metrics() {
    local port=$1
    local timeout=${2:-30}
    local start_time=$(date +%s)
    
    while true; do
        if curl -s "http://127.0.0.1:${port}/metrics" > /dev/null 2>&1; then
            return 0
        fi
        
        local elapsed=$(($(date +%s) - start_time))
        if [ $elapsed -ge $timeout ]; then
            return 1
        fi
        sleep 0.5
    done
}

# Check metrics for leader
log_info "Checking cluster leadership via metrics..."

# Wait for metrics to be available on all nodes
log_info "Waiting for metrics servers to be ready..."
for i in 1 2 3; do
    port=$((METRICS_BASE_PORT + i - 1))
    if ! wait_for_metrics "$port" 30; then
        log_warn "Metrics server on node $i (port $port) not ready, proceeding anyway..."
    fi
done

leader_count=0
leader_node=0
for i in 1 2 3; do
    port=$((METRICS_BASE_PORT + i - 1))
    is_leader=$(curl -s "http://127.0.0.1:${port}/metrics" 2>/dev/null | grep "opengslb_cluster_is_leader" | grep -v "#" | awk '{print $2}' || echo "0")
    if [ "$is_leader" = "1" ]; then
        leader_count=$((leader_count + 1))
        leader_node=$i
        log_info "Node $i (port $port) is the LEADER"
    else
        log_info "Node $i (port $port) is a follower"
    fi
done

# Verify exactly one leader
if [ "$leader_count" -eq 1 ]; then
    log_info "SUCCESS: Exactly one leader elected"
else
    log_error "FAILED: Expected 1 leader, found $leader_count"
    for i in 1 2 3; do
        log_info "--- Node $i logs ---"
        tail -30 "$DATA_DIR/node${i}/output.log"
    done
    exit 1
fi

# Test DNS on leader
log_info "Testing DNS query on leader node..."
leader_dns_port=$((DNS_BASE_PORT + leader_node - 1))
dns_result=$(dig @127.0.0.1 -p "$leader_dns_port" test.example.com A +short +tries=1 +time=2 2>/dev/null || echo "")
if [ -n "$dns_result" ]; then
    log_info "DNS response: $dns_result"
else
    log_warn "DNS query returned empty (expected - backend is not running)"
fi

# Test leader failover
log_info ""
log_info "=== Testing Leader Failover ==="

# Kill the leader
leader_idx=$((leader_node - 1))
log_info "Killing leader (node $leader_node, PID ${NODE_PIDS[$leader_idx]})..."
kill "${NODE_PIDS[$leader_idx]}" 2>/dev/null || true

# Wait for re-election
log_info "Waiting for re-election (5s)..."
sleep 5

# Check for new leader
new_leader_count=0
new_leader_node=0
for i in 1 2 3; do
    if [ "$i" -eq "$leader_node" ]; then
        continue  # Skip the killed node
    fi
    port=$((METRICS_BASE_PORT + i - 1))
    is_leader=$(curl -s "http://127.0.0.1:${port}/metrics" 2>/dev/null | grep "opengslb_cluster_is_leader" | grep -v "#" | awk '{print $2}' || echo "0")
    if [ "$is_leader" = "1" ]; then
        new_leader_count=$((new_leader_count + 1))
        new_leader_node=$i
        log_info "Node $i is now the NEW LEADER"
    fi
done

if [ "$new_leader_count" -eq 1 ]; then
    log_info "SUCCESS: New leader elected after failover"
else
    log_error "FAILED: Expected 1 new leader, found $new_leader_count"
    for i in 1 2 3; do
        if [ "$i" -ne "$leader_node" ]; then
            log_info "--- Node $i logs ---"
            tail -30 "$DATA_DIR/node${i}/output.log"
        fi
    done
    exit 1
fi

log_info ""
log_info "=== All Tests Passed ==="
log_info "- 3-node cluster formed successfully"
log_info "- Leader election completed"
log_info "- Leader failover worked correctly"

exit 0