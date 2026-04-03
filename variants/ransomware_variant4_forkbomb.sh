#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant4}"
NUM_FILES="${NUM_FILES:-300}"
NUM_PROCS="${NUM_PROCS:-4}"

echo "=== Ransomware Simulation: Fork Bomb Pattern ==="
echo "Target: $TARGET_DIR"
echo "Spawns multiple concurrent encryption workers"

mkdir -p "$TARGET_DIR"

for i in $(seq 1 $NUM_FILES); do
    echo "Sensitive data $i" > "$TARGET_DIR/file_$i.txt"
done
echo "Created $NUM_FILES files"

echo "Phase 1: Rapid process spawning..."
for p in $(seq 1 $NUM_PROCS); do
    (
        start=$(( (p-1) * NUM_FILES / NUM_PROCS + 1 ))
        end=$(( p * NUM_FILES / NUM_PROCS ))
        for i in $(seq $start $end); do
            file="$TARGET_DIR/file_$i.txt"
            if [ -f "$file" ]; then
                openssl enc -aes-256-cbc -salt -in "$file" -out "${file}.enc" -pass pass:forkbomb 2>/dev/null
                rm "$file"
            fi
        done
    ) &
done

wait
echo "Phase 2: Cleanup..."
rm -f "$TARGET_DIR"/*.bak

echo "=== Simulation complete ==="
