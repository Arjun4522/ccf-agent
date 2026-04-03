TARGET_DIR="/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant2"
NUM_FILES=500
ENCRYPTION_DELAY=0.1
PARALLEL_PROCS=5
echo "Starting ransomware simulation variant (multi-process)..."
echo "Target directory: $TARGET_DIR"
echo "CCF Agent should detect and block this"
# Setup
if [ ! -d "$TARGET_DIR" ]; then
    mkdir -p "$TARGET_DIR"
    echo "Creating $NUM_FILES test files..."
    for i in $(seq 1 $NUM_FILES); do
        echo "This is a test document $i with sensitive data." > "$TARGET_DIR/file_$i.txt"
    done
    echo "Created $NUM_FILES test files"
else
    echo "Using existing test directory"
fi
echo "Phase 1: Reconnaissance (random file touches)..."
for i in $(seq 1 20); do
    file=$(ls "$TARGET_DIR"/*.txt | shuf -n 1)
    touch "$file"
    sleep 0.05
done
echo "Phase 2: Encryption (multi-process batching)..."
# Spawn multiple processes to encrypt files
for batch in $(seq 1 $PARALLEL_PROCS); do
    (
        start=$(( (($batch-1) * NUM_FILES / PARALLEL_PROCS) + 1 ))
        end=$(( $batch * NUM_FILES / PARALLEL_PROCS ))
        for i in $(seq $start $end); do
            file="$TARGET_DIR/file_$i.txt"
            if [ -f "$file" ]; then
                cp "$file" "$file.bak"
                openssl enc -aes-256-cbc -salt -in "$file" -out "${file}.enc" -pass pass:testpass 2>/dev/null
                rm "$file"
                echo "encrypted file_$i.txt"
                sleep $ENCRYPTION_DELAY
            fi
        done
    ) &
done
wait
echo "Phase 3: Cleanup (remove originals)..."
rm -f "$TARGET_DIR"/*.bak
echo "Simulation complete."