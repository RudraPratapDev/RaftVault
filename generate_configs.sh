#!/bin/bash
# generate_configs.sh - Generates RaftKMS node configs for a multi-machine setup

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${CYAN}==============================================${NC}"
echo -e "${CYAN}    RaftKMS Multi-Machine Config Generator    ${NC}"
echo -e "${CYAN}==============================================${NC}"
echo ""
echo "This will generate configuration files for a 3-node cluster"
echo "running across different computers on the same network."
echo ""

read -p "Enter IP address for Node 1: " IP1
read -p "Enter IP address for Node 2: " IP2
read -p "Enter IP address for Node 3: " IP3

PORT=5001

mkdir -p configs/network

# Node 1
cat > configs/network/node1.json << EOF
{
  "node_id": "node1",
  "address": "$IP1:$PORT",
  "peers": ["$IP2:$PORT", "$IP3:$PORT"],
  "data_dir": "data/node1",
  "election_timeout_min_ms": 1500,
  "election_timeout_max_ms": 3000,
  "heartbeat_interval_ms": 500
}
EOF

# Node 2
cat > configs/network/node2.json << EOF
{
  "node_id": "node2",
  "address": "$IP2:$PORT",
  "peers": ["$IP1:$PORT", "$IP3:$PORT"],
  "data_dir": "data/node2",
  "election_timeout_min_ms": 1500,
  "election_timeout_max_ms": 3000,
  "heartbeat_interval_ms": 500
}
EOF

# Node 3
cat > configs/network/node3.json << EOF
{
  "node_id": "node3",
  "address": "$IP3:$PORT",
  "peers": ["$IP1:$PORT", "$IP2:$PORT"],
  "data_dir": "data/node3",
  "election_timeout_min_ms": 1500,
  "election_timeout_max_ms": 3000,
  "heartbeat_interval_ms": 500
}
EOF

echo -e "\n${GREEN}✅ Configurations generated in 'configs/network/'${NC}"
echo -e "\n${YELLOW}━━━ HOW TO START ON DIFFERENT MACHINES ━━━${NC}"
echo ""
echo -e "Copy the ENTIRE project folder to your 3 friends' computers."
echo -e "Then run the following commands on the respective machines:\n"

echo -e "${CYAN}Machine 1 ($IP1) - You:${NC}"
echo "  go build -o bin/raft-kms ./cmd/main.go"
echo "  ./bin/raft-kms --config configs/network/node1.json"
echo ""

echo -e "${CYAN}Machine 2 ($IP2) - Friend 1:${NC}"
echo "  go build -o bin/raft-kms ./cmd/main.go"
echo "  ./bin/raft-kms --config configs/network/node2.json"
echo ""

echo -e "${CYAN}Machine 3 ($IP3) - Friend 2:${NC}"
echo "  go build -o bin/raft-kms ./cmd/main.go"
echo "  ./bin/raft-kms --config configs/network/node3.json"
echo ""

echo -e "${YELLOW}━━━ DASHBOARD VIEW ━━━${NC}"
echo "On ANY machine, open the dashboard:"
echo "  cd dashboard"
echo "  npm install"
echo "  npm run dev"
echo ""
echo "In the dashboard header, put this in the Nodes input: "
echo "  $IP1:$PORT, $IP2:$PORT, $IP3:$PORT"
echo ""
