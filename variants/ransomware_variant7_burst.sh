#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant7}"
NUM_FILES="${NUM_FILES:-100}"

echo "=== Ransomware Simulation: Burst Mode ==="
echo "Target: $TARGET_DIR"
echo "Fast encryption burst to test response time"

mkdir -p "$TARGET_DIR"

for i in $(seq 1 $NUM_FILES); do
    echo "Critical data $i" > "$TARGET_DIR/file_$i.txt"
done
echo "Created $NUM_FILES files"

echo "Phase 1: Immediate encryption burst (no delay)..."
for i in $(seq 1 $NUM_FILES); do
    file="$TARGET_DIR/file_$i.txt"
    if [ -f "$file" ]; then
        openssl enc -aes-256-cbc -salt -in "$file" -out "${file}.enc" -pass pass:burst 2>/dev/null
        rm "$file"
        echo "Encrypted file_$i.txt"
    fi
done

echo "=== Simulation complete ==="
