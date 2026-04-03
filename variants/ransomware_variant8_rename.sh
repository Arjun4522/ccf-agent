#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant8}"
NUM_FILES="${NUM_FILES:-100}"

echo "=== Ransomware Simulation: Rename-First Pattern ==="
echo "Target: $TARGET_DIR"
echo "Renames files before encryption (anti-forensics)"

mkdir -p "$TARGET_DIR"

for i in $(seq 1 $NUM_FILES); do
    echo "Important $i" > "$TARGET_DIR/doc_$i.pdf"
done
echo "Created $NUM_FILES files"

echo "Phase 1: Rename phase (hiding file types)..."
for i in $(seq 1 $NUM_FILES); do
    orig="$TARGET_DIR/doc_$i.pdf"
    if [ -f "$orig" ]; then
        mv "$orig" "$TARGET_DIR/doc_$i.tmp"
        echo "Renamed doc_$i.pdf → doc_$i.tmp"
    fi
done

echo "Phase 2: Encryption phase..."
for i in $(seq 1 $NUM_FILES); do
    file="$TARGET_DIR/doc_$i.tmp"
    if [ -f "$file" ]; then
        openssl enc -aes-256-cbc -salt -in "$file" -out "${file}.enc" -pass pass:renamer 2>/dev/null
        rm "$file"
        echo "Encrypted doc_$i.tmp"
    fi
done

echo "=== Simulation complete ==="
