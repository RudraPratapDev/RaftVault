#!/bin/bash
# start_with_dashboard.sh - Starts local 3-node cluster and React dashboard

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}==============================================${NC}"
echo -e "${CYAN}    Starting RaftKMS + React Dashboard       ${NC}"
echo -e "${CYAN}==============================================${NC}"

# Clean up any existing processes
pkill -f "raft-kms --config" || true
# optionally pkill node processes from vite if running on specific port

# 1. Build Go backend
echo -e "\n${GREEN}[1/3] Building Go distributed backend...${NC}"
go build -o bin/raft-kms ./cmd/main.go

# 2. Reset data directories
rm -rf data/node1 data/node2 data/node3
mkdir -p data/node1 data/node2 data/node3

# 3. Start 3 local nodes
echo -e "\n${GREEN}[2/3] Starting 3 backend nodes (ports 5001, 5002, 5003)...${NC}"
mkdir -p logs
./bin/raft-kms --config configs/node1.json > logs/node1.log 2>&1 &
NODE1_PID=$!
./bin/raft-kms --config configs/node2.json > logs/node2.log 2>&1 &
NODE2_PID=$!
./bin/raft-kms --config configs/node3.json > logs/node3.log 2>&1 &
NODE3_PID=$!

echo "Nodes started. Waiting for leader election..."
sleep 3

# 4. Start React dashboard
echo -e "\n${GREEN}[3/3] Starting React Dashboard...${NC}"
cd dashboard
npm run build || echo "Vite build failed, falling back to dev server"
npm run dev -- --open &

echo -e "\n${GREEN}✅ Everything is running!${NC}"
echo -e "To stop everything, press Ctrl+C or run: pkill -f 'raft-kms --config' && pkill -f 'vite'"
echo "Check backend logs in logs/nodeX.log"
wait
