#!/bin/bash

# Ransomware Simulator for CCF Agent Testing
# Simulates realistic ransomware behavior patterns

set -e

# Configuration
TARGET_DIR="/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/variant1"
NUM_DOCS=50
NUM_IMAGES=30
DELAY_BETWEEN_OPS=0.01  # seconds
ENCRYPTION_DELAY=0.05   # seconds between encryption steps

# Create test directory and files
setup_test_environment() {
    echo "Setting up test environment in $TARGET_DIR..."
    mkdir -p "$TARGET_DIR"
    
    # Create document files
    for i in $(seq 1 $NUM_DOCS); do
        echo "Important document content $i" > "$TARGET_DIR/document_$i.docx"
        echo "Spreadsheet data $i" > "$TARGET_DIR/spreadsheet_$i.xlsx"
        echo "Presentation $i" > "$TARGET_DIR/presentation_$i.pptx"
    done
    
    # Create image files (dummy content)
    for i in $(seq 1 $NUM_IMAGES); do
        head -c 1000 /dev/urandom > "$TARGET_DIR/image_$i.jpg"
        head -c 800 /dev/urandom > "$TARGET_DIR/photo_$i.png"
    done
    
    # Create some archive files
    echo "Backup data" > "$TARGET_DIR/backup.zip"
    echo "Database export" > "$TARGET_DIR/database.sql"
    
    echo "Created $(find "$TARGET_DIR" -type f | wc -l) test files"
}

# Phase 1: Reconnaissance (file browsing)
reconnaissance_phase() {
    echo "Phase 1: Reconnaissance (file browsing)..."
    
    # Browse through files (simulates ransomware scanning)
    find "$TARGET_DIR" -type f -name "*.docx" | head -10 | while read file; do
        cat "$file" > /dev/null 2>&1
        sleep $DELAY_BETWEEN_OPS
    done
    
    find "$TARGET_DIR" -type f -name "*.xlsx" | head -8 | while read file; do
        cat "$file" > /dev/null 2>&1
        sleep $DELAY_BETWEEN_OPS
    done
    
    sleep 1
}

# Phase 2: Encryption (file modification)
encryption_phase() {
    echo "Phase 2: Encryption (file modification)..."
    
    # Encrypt documents first (high value targets)
    find "$TARGET_DIR" -type f \( -name "*.docx" -o -name "*.xlsx" -o -name "*.pptx" \) | while read file; do
        # Simulate encryption by appending encrypted content
        echo "ENCRYPTED_$(head -c 50 /dev/urandom | base64)" >> "$file"
        sleep $ENCRYPTION_DELAY
    done
    
    # Encrypt images
    find "$TARGET_DIR" -type f \( -name "*.jpg" -o -name "*.png" \) | while read file; do
        echo "ENCRYPTED_$(head -c 50 /dev/urandom | base64)" >> "$file"
        sleep $ENCRYPTION_DELAY
    done
    
    # Encrypt other valuable files
    find "$TARGET_DIR" -type f \( -name "*.pdf" -o -name "*.txt" -o -name "*.sql" -o -name "*.zip" \) | while read file; do
        echo "ENCRYPTED_$(head -c 50 /dev/urandom | base64)" >> "$file"
        sleep $ENCRYPTION_DELAY
    done
}

# Phase 3: Rename files with ransom extension
ransom_note_phase() {
    echo "Phase 3: Adding ransom extensions..."
    
    # Rename files with ransom extension
    find "$TARGET_DIR" -type f | while read file; do
        mv "$file" "${file}.encrypted"
        sleep $DELAY_BETWEEN_OPS
    done
    
    # Create ransom note
    echo "YOUR FILES HAVE BEEN ENCRYPTED!" > "$TARGET_DIR/RECOVER_FILES.txt"
    echo "Send 0.1 BTC to ransom address" >> "$TARGET_DIR/RECOVER_FILES.txt"
}

# Phase 4: Cleanup and privilege escalation (simulated)
cleanup_phase() {
    echo "Phase 4: Cleanup and system manipulation..."
    
    # Try to delete backup files (simulated)
    find "$TARGET_DIR" -name "*.bak" -o -name "*backup*" -o -name "*.old" | head -5 | while read file; do
        rm -f "$file"
        sleep $DELAY_BETWEEN_OPS
    done
    
    # Simulate privilege escalation attempt
    if [ "$EUID" -ne 0 ]; then
        echo "Attempting privilege escalation (simulated)..."
        # This will fail but trigger the event
        sudo -n echo "privilege test" 2>/dev/null || true
    fi
}

# Phase 5: Rapid process execution (worm-like behavior)
propagation_phase() {
    echo "Phase 5: Rapid process execution..."
    
    # Simulate rapid process creation
    for i in {1..20}; do
        # Launch background processes that do file operations
        (
            sleep 0.1
            find "$TARGET_DIR" -name "*.encrypted" | head -3 | while read file; do
                echo "Process $i touching $file" >> "$file"
            done
        ) &
        sleep 0.02
    done
    
    wait
}

# Main execution
main() {
    echo "Starting ransomware simulation..."
    echo "Target directory: $TARGET_DIR"
    echo "CCF Agent should detect this activity as suspicious"
    echo
    
    setup_test_environment
    
    # Run phases with delays between them
    reconnaissance_phase
    sleep 2
    
    encryption_phase
    sleep 1
    
    ransom_note_phase
    sleep 1
    
    cleanup_phase
    sleep 1
    
    propagation_phase
    
    echo
    echo "Ransomware simulation complete!"
    echo "Check CCF Agent output for detection alerts"
    echo "Test files remain in: $TARGET_DIR"
}

# Cleanup function (optional)
cleanup() {
    echo "Cleaning up test files..."
    rm -rf "$TARGET_DIR"
}

# Handle interrupts
trap 'echo "Test interrupted"; cleanup; exit 1' INT TERM

# Run main function
if [ "$1" = "cleanup" ]; then
    cleanup
else
    main
fi