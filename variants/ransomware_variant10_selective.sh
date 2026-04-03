#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant10}"
NUM_FILES="${NUM_FILES:-80}"

echo "=== Ransomware Simulation: Selective Targeting ==="
echo "Target: $TARGET_DIR"
echo "Only encrypts specific high-value files (targeted attack)"

mkdir -p "$TARGET_DIR"

declare -a TARGET_EXTS=("txt" "doc" "docx" "pdf" "xls" "xlsx")
for ext in "${TARGET_EXTS[@]}"; do
    for i in $(seq 1 10); do
        echo "Sensitive $i" > "$TARGET_DIR/doc_$i.$ext"
    done
done
for i in $(seq 1 20); do
    echo "Junk $i" > "$TARGET_DIR/cache_$i.log"
done
echo "Created mixed files (targeted + decoy)"

echo "Phase 1: Reconnaissance (identifying targets)..."
for ext in "${TARGET_EXTS[@]}"; do
    count=$(ls "$TARGET_DIR"/*."$ext" 2>/dev/null | wc -l)
    echo "Found $count .$ext files"
    sleep 0.2
done

echo "Phase 2: Selective encryption..."
for ext in "${TARGET_EXTS[@]}"; do
    for f in "$TARGET_DIR"/*."$ext"; do
        [ -f "$f" ] || continue
        openssl enc -aes-256-cbc -salt -in "$f" -out "${f}.encrypted" -pass pass:selective 2>/dev/null
        rm "$f"
        echo "Encrypted $(basename $f)"
        sleep 0.1
    done
done

echo "Phase 3: Leaving decoy files..."
echo "Decoy files preserved for realism"

echo "=== Simulation complete ==="
