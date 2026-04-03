#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant3}"
NUM_FILES="${NUM_FILES:-200}"
ENCRYPTION_DELAY="${ENCRYPTION_DELAY:-0.05}"

echo "=== Ransomware Simulation: Slow and Low ==="
echo "Target: $TARGET_DIR"
echo "This variant encrypts slowly to avoid burst detection"

mkdir -p "$TARGET_DIR"

for i in $(seq 1 $NUM_FILES); do
    echo "Document $i" > "$TARGET_DIR/doc_$i.txt"
done
echo "Created $NUM_FILES files"

echo "Phase 1: Reconnaissance..."
for i in $(seq 1 10); do
    ls "$TARGET_DIR" > /dev/null
    sleep 0.2
done

echo "Phase 2: Slow encryption (bypassing rate detectors)..."
for i in $(seq 1 $NUM_FILES); do
    file="$TARGET_DIR/doc_$i.txt"
    if [ -f "$file" ]; then
        openssl enc -aes-256-cbc -salt -in "$file" -out "${file}.enc" -pass pass:slowpass 2>/dev/null
        rm "$file"
        echo "Encrypted doc_$i.txt"
        sleep $ENCRYPTION_DELAY
    fi
done

echo "=== Simulation complete ==="
