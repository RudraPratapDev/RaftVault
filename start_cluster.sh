#!/bin/bash
# start_cluster.sh - Starts a 3-node RaftKMS cluster locally

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
LOG_DIR="$SCRIPT_DIR/logs"
DATA_DIR="$SCRIPT_DIR/data"

echo "=== RaftKMS Cluster Launcher ==="

# Cleanup previous instances
echo "[1/4] Cleaning up old instances..."
pkill -f "raft-kms --config" 2>/dev/null || true
sleep 1

# Clean data and logs
rm -rf "$DATA_DIR" "$LOG_DIR"
mkdir -p "$BIN_DIR" "$LOG_DIR" "$DATA_DIR"

# Build
echo "[2/4] Building raft-kms binary..."
cd "$SCRIPT_DIR"
go build -o "$BIN_DIR/raft-kms" ./cmd/main.go
echo "       Binary built: $BIN_DIR/raft-kms"

# Start nodes
echo "[3/4] Starting 3 nodes..."

"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node1.json" > "$LOG_DIR/node1.log" 2>&1 &
echo "       Node 1 started (PID: $!, Port: 5001)"

"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node2.json" > "$LOG_DIR/node2.log" 2>&1 &
echo "       Node 2 started (PID: $!, Port: 5002)"

"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node3.json" > "$LOG_DIR/node3.log" 2>&1 &
echo "       Node 3 started (PID: $!, Port: 5003)"

echo ""
echo "[4/4] Cluster starting up. Waiting for leader election..."
sleep 4

echo ""
echo "=== Cluster Status ==="
echo ""

for port in 5001 5002 5003; do
  echo "--- Node on port $port ---"
  curl -s "http://localhost:$port/status" 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "  (not ready yet)"
  echo ""
done

echo "=== Useful Commands ==="
echo ""
echo "View logs:"
echo "  tail -f logs/node1.log"
echo "  tail -f logs/node2.log"
echo "  tail -f logs/node3.log"
echo ""
echo "Create a key:"
echo '  curl -s -X POST http://localhost:5001/kms/createKey -d '"'"'{"key_id":"my-key"}'"'"' | python3 -m json.tool'
echo ""
echo "Check status:"
echo "  curl -s http://localhost:5001/status | python3 -m json.tool"
echo ""
echo "Kill the cluster:"
echo '  pkill -f "raft-kms --config"'
echo ""
