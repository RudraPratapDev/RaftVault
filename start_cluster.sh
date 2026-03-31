#!/bin/bash
# start_cluster.sh — Starts a 3-node RaftKMS cluster locally.
# Logs are APPENDED (not wiped) so they persist across restarts.
# Run reset.sh first if you want a completely fresh start.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
LOG_DIR="$SCRIPT_DIR/logs"
DATA_DIR="$SCRIPT_DIR/data"

echo "=== RaftKMS Cluster Launcher ==="

# Kill any existing instances
echo "[1/4] Stopping any existing nodes..."
pkill -f "raft-kms --config" 2>/dev/null && echo "       Stopped old nodes." || echo "       No old nodes running."
sleep 1

# Ensure directories exist (don't wipe — use reset.sh for that)
mkdir -p "$BIN_DIR" "$LOG_DIR" \
  "$DATA_DIR/node1" "$DATA_DIR/node2" "$DATA_DIR/node3"

# Build
echo "[2/4] Building raft-kms binary..."
cd "$SCRIPT_DIR"
go build -o "$BIN_DIR/raft-kms" ./cmd/main.go
echo "       Binary built: $BIN_DIR/raft-kms"

# Start nodes — logs are appended so history survives restarts
echo "[3/4] Starting 3 nodes (logs appended to $LOG_DIR/)..."

"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node1.json" --log-dir "$LOG_DIR" >> "$LOG_DIR/node1.log" 2>&1 &
NODE1_PID=$!
echo "       Node 1 started (PID: $NODE1_PID, Port: 5001)"

"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node2.json" --log-dir "$LOG_DIR" >> "$LOG_DIR/node2.log" 2>&1 &
NODE2_PID=$!
echo "       Node 2 started (PID: $NODE2_PID, Port: 5002)"

"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node3.json" --log-dir "$LOG_DIR" >> "$LOG_DIR/node3.log" 2>&1 &
NODE3_PID=$!
echo "       Node 3 started (PID: $NODE3_PID, Port: 5003)"

echo ""
echo "[4/4] Waiting for leader election (~3s)..."
sleep 3

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
echo "View live logs:"
echo "  tail -f logs/node1.log"
echo "  tail -f logs/node2.log"
echo "  tail -f logs/node3.log"
echo ""
echo "Test demo endpoint (no auth needed, localhost only):"
echo "  curl -s http://localhost:5001/test/demo | python3 -m json.tool"
echo "  curl -s -X POST http://localhost:5001/test/demo/createKey -H 'Content-Type: application/json' -d '{\"key_id\":\"demo-key\"}' | python3 -m json.tool"
echo ""
echo "Create a key (with admin auth):"
echo "  curl -s -X POST http://localhost:5001/kms/createKey -H 'Authorization: Bearer admin-secret-key' -H 'Content-Type: application/json' -d '{\"key_id\":\"my-key\"}' | python3 -m json.tool"
echo ""
echo "Simulate leader failure (kill node on port 5001):"
echo "  curl -s -X POST http://localhost:5001/chaos/kill"
echo ""
echo "Revive it:"
echo "  curl -s -X POST http://localhost:5001/chaos/revive"
echo ""
echo "Fresh start (wipe all data):"
echo "  ./reset.sh && ./start_cluster.sh"
echo ""
echo "Stop cluster:"
echo "  pkill -f 'raft-kms --config'"
echo ""
