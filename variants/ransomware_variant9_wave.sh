#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${TARGET_DIR:-/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant9}"
NUM_FILES="${NUM_FILES:-150}"
BATCH_SIZE="${BATCH_SIZE:-25}"

echo "=== Ransomware Simulation: Wave Attack ==="
echo "Target: $TARGET_DIR"
echo "Encrypts in waves with pauses between (testing sustained detection)"

mkdir -p "$TARGET_DIR"

for i in $(seq 1 $NUM_FILES); do
    echo "Data $i" > "$TARGET_DIR/file_$i.txt"
done
echo "Created $NUM_FILES files"

for wave in 1 2 3 4; do
    echo "--- Wave $wave ---"
    start=$(( (wave-1) * BATCH_SIZE + 1 ))
    end=$(( wave * BATCH_SIZE ))
    
    for i in $(seq $start $end); do
        file="$TARGET_DIR/file_$i.txt"
        if [ -f "$file" ]; then
            openssl enc -aes-256-cbc -salt -in "$file" -out "${file}.enc" -pass pass:wave 2>/dev/null
            rm "$file"
            echo "Encrypted file_$i.txt"
        fi
    done
    
    if [ $wave -lt 4 ]; then
        echo "Wave $wave complete, pausing..."
        sleep 3
    fi
done

echo "=== Simulation complete ==="
