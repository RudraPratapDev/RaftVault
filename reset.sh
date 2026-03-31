#!/bin/bash
# reset.sh — Wipes all Raft state, logs, and data for a completely fresh start.
# Run this before start_cluster.sh when you want to demo from scratch.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== RaftKMS Reset ==="
echo ""
echo "⚠️  This will DELETE all Raft state, logs, and persisted data."
echo "    The cluster will start fresh with no keys, no users (except admin), no log."
echo ""

# Kill any running nodes first
echo "[1/3] Stopping any running nodes..."
pkill -f "raft-kms --config" 2>/dev/null && echo "       Stopped running nodes." || echo "       No running nodes found."
sleep 1

# Wipe data directories
echo "[2/3] Wiping Raft state (data/)..."
rm -rf "$SCRIPT_DIR/data"
mkdir -p "$SCRIPT_DIR/data/node1" "$SCRIPT_DIR/data/node2" "$SCRIPT_DIR/data/node3"
echo "       data/ wiped and recreated."

# Wipe logs
echo "[3/3] Wiping logs (logs/)..."
rm -rf "$SCRIPT_DIR/logs"
mkdir -p "$SCRIPT_DIR/logs"
echo "       logs/ wiped and recreated."

echo ""
echo "✅ Reset complete. Run ./start_cluster.sh to start fresh."
echo ""
