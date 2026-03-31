#!/bin/bash
# start_with_dashboard.sh — Starts local 3-node cluster and React dashboard.
# Logs are APPENDED (not wiped). Run reset.sh first for a fresh start.

set -e

CYAN='\033[0;36m'
GREEN='\033[0;32m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
LOG_DIR="$SCRIPT_DIR/logs"
DATA_DIR="$SCRIPT_DIR/data"

echo -e "${CYAN}==============================================${NC}"
echo -e "${CYAN}    RaftKMS + React Dashboard Launcher       ${NC}"
echo -e "${CYAN}==============================================${NC}"

# Kill any existing nodes
pkill -f "raft-kms --config" 2>/dev/null || true
sleep 1

# Ensure dirs exist (don't wipe — use reset.sh for that)
mkdir -p "$BIN_DIR" "$LOG_DIR" \
  "$DATA_DIR/node1" "$DATA_DIR/node2" "$DATA_DIR/node3"

# Build Go backend
echo -e "\n${GREEN}[1/3] Building Go backend...${NC}"
go build -o "$BIN_DIR/raft-kms" ./cmd/main.go

# Start 3 nodes — logs appended
echo -e "\n${GREEN}[2/3] Starting 3 backend nodes (ports 5001, 5002, 5003)...${NC}"
"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node1.json" --log-dir "$LOG_DIR" >> "$LOG_DIR/node1.log" 2>&1 &
"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node2.json" --log-dir "$LOG_DIR" >> "$LOG_DIR/node2.log" 2>&1 &
"$BIN_DIR/raft-kms" --config "$SCRIPT_DIR/configs/node3.json" --log-dir "$LOG_DIR" >> "$LOG_DIR/node3.log" 2>&1 &

echo "Nodes started. Waiting for leader election (~3s)..."
sleep 3

# Start React dashboard
echo -e "\n${GREEN}[3/3] Starting React Dashboard (Vite dev server)...${NC}"
echo -e "  Dashboard: http://localhost:5173"
echo -e "  Test Demo: http://localhost:5001/test/demo"
echo ""
echo -e "  Tip: Run ${CYAN}./reset.sh${NC} first for a completely fresh demo."
echo ""

cd "$SCRIPT_DIR/dashboard"
npm run dev -- --open

echo -e "\n${GREEN}Stopped.${NC}"
