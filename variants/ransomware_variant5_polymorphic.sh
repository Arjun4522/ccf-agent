#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant5}"
NUM_FILES="${NUM_FILES:-150}"

echo "=== Ransomware Simulation: Polymorphic-like (varying patterns) ==="
echo "Target: $TARGET_DIR"
echo "Varies encryption method per file to evade signature detection"

mkdir -p "$TARGET_DIR"

for i in $(seq 1 $NUM_FILES); do
    echo "Secret document $i" > "$TARGET_DIR/data_$i.bin"
done
echo "Created $NUM_FILES files"

echo "Phase 1: Reading files in random order..."
files=("$TARGET_DIR"/*.bin)
shuffled=($(shuf -e "${files[@]}"))
for f in "${shuffled[@]}"; do
    cat "$f" > /dev/null 2>&1 || true
    sleep 0.02
done

echo "Phase 2: Encryption with varied methods..."
idx=0
for f in "${files[@]}"; do
    if [ -f "$f" ]; then
        case $((idx % 3)) in
            0) openssl enc -aes-256-cbc -salt -in "$f" -out "${f}.locked" -pass pass:poly0 2>/dev/null ;;
            1) openssl enc -aes-256-cbc -salt -in "$f" -out "${f}.locked" -pass pass:poly1 2>/dev/null ;;
            2) openssl enc -aes-256-cbc -salt -in "$f" -out "${f}.locked" -pass pass:poly2 2>/dev/null ;;
        esac
        rm "$f"
        echo "Encrypted $(basename $f)"
    fi
    idx=$((idx + 1))
done

echo "=== Simulation complete ==="
