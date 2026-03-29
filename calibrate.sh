#!/usr/bin/env bash
# calibrate.sh — auto-calibrate ccf-agent alert/warning thresholds
# Usage: sudo bash calibrate.sh [path/to/ccf-agent]
# Runs three phases: idle → normal load → simulated attack
# Prints recommended thresholds at the end.

set -euo pipefail

AGENT="${1:-./ccf-agent}"
IDLE_SECS=60
NORMAL_SECS=60
ATTACK_SECS=30
SANDBOX="/tmp/ccf-calibrate-$$"
SCORES_IDLE="/tmp/ccf-scores-idle-$$.txt"
SCORES_NORMAL="/tmp/ccf-scores-normal-$$.txt"
SCORES_ATTACK="/tmp/ccf-scores-attack-$$.txt"
AGENT_PID=""

# ── colours ────────────────────────────────────────────────────────────────
RED='\033[0;31m'; YELLOW='\033[0;33m'; GREEN='\033[0;32m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

info()    { echo -e "${CYAN}[•]${RESET} $*"; }
success() { echo -e "${GREEN}[✓]${RESET} $*"; }
warn()    { echo -e "${YELLOW}[!]${RESET} $*"; }
header()  { echo -e "\n${BOLD}$*${RESET}"; }

# ── cleanup ─────────────────────────────────────────────────────────────────
cleanup() {
    if [[ -n "$AGENT_PID" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
        kill "$AGENT_PID" 2>/dev/null || true
    fi
    rm -rf "$SANDBOX" "$SCORES_IDLE" "$SCORES_NORMAL" "$SCORES_ATTACK" 2>/dev/null || true
}
trap cleanup EXIT

# ── require root ────────────────────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}Error:${RESET} run with sudo: sudo bash calibrate.sh"
    exit 1
fi

if [[ ! -x "$AGENT" ]]; then
    echo -e "${RED}Error:${RESET} agent not found or not executable: $AGENT"
    exit 1
fi

# ── helper: start agent, collect scores to file ─────────────────────────────
start_collection() {
    local outfile="$1"
    "$AGENT" -json -alert-score 0.0 -warning-score 0.0 2>/dev/null \
        | stdbuf -oL jq -r '.score' >> "$outfile" &
    AGENT_PID=$!
}

stop_collection() {
    if [[ -n "$AGENT_PID" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
        kill "$AGENT_PID" 2>/dev/null || true
        wait "$AGENT_PID" 2>/dev/null || true
    fi
    AGENT_PID=""
}

# ── helper: percentile from sorted file ─────────────────────────────────────
percentile() {
    local file="$1" pct="$2"
    sort -n "$file" | awk -v p="$pct" '
        BEGIN { n=0 }
        { vals[n++]=$1 }
        END {
            if (n==0) { print "0"; exit }
            idx = int(n * p / 100)
            if (idx >= n) idx = n-1
            print vals[idx]
        }'
}

stats() {
    local file="$1"
    sort -n "$file" | awk '
        BEGIN { n=0; sum=0 }
        { vals[n++]=$1; sum+=$1 }
        END {
            if (n==0) { print "  no data"; exit }
            p50 = vals[int(n*0.50)]
            p90 = vals[int(n*0.90)]
            p95 = vals[int(n*0.95)]
            p99 = vals[int(n*0.99)]
            printf "  count=%-5d  min=%.3f  p50=%.3f  p90=%.3f  p95=%.3f  p99=%.3f  max=%.3f  mean=%.3f\n",
                n, vals[0], p50, p90, p95, p99, vals[n-1], sum/n
        }'
}

# ── countdown timer ──────────────────────────────────────────────────────────
countdown() {
    local secs="$1" label="$2"
    while [[ $secs -gt 0 ]]; do
        printf "\r  ${CYAN}%s${RESET} — %3ds remaining..." "$label" "$secs"
        sleep 1
        ((secs--)) || true
    done
    printf "\r%-60s\n" ""
}

# ── simulate normal load ─────────────────────────────────────────────────────
simulate_normal_load() {
    local dir="$SANDBOX/normal"
    mkdir -p "$dir"
    # Simulate editor-style saves, log appends, temp file churn
    (
        for i in $(seq 1 200); do
            echo "log line $i $(date)" >> "$dir/app.log"
            echo "content $i" > "$dir/edit_$((i % 10)).tmp"
            sleep 0.2
        done
    ) &
    echo $!
}

# ── simulate ransomware attack ───────────────────────────────────────────────
simulate_attack() {
    local dir="$SANDBOX/attack"
    mkdir -p "$dir"

    info "Creating 100 victim files..."
    for i in $(seq 1 100); do
        dd if=/dev/urandom of="$dir/document_$i.txt" bs=512 count=1 2>/dev/null
    done

    info "Phase 1: bulk encrypt (rename to .enc)..."
    for f in "$dir"/*.txt; do
        mv "$f" "${f%.txt}.enc"
    done

    info "Phase 2: overwrite with random data (simulate encryption)..."
    for f in "$dir"/*.enc; do
        dd if=/dev/urandom of="$f" bs=1K count=2 2>/dev/null
    done

    info "Phase 3: delete originals (ransomware cleanup)..."
    rm -f "$dir"/*.enc

    info "Phase 4: drop ransom note..."
    echo "YOUR FILES ARE ENCRYPTED. Send 1 BTC to ..." > "$dir/README_DECRYPT.txt"
}

# ════════════════════════════════════════════════════════════════════════════
echo -e "\n${BOLD}╔══════════════════════════════════════════╗${RESET}"
echo -e "${BOLD}║   ccf-agent threshold calibrator        ║${RESET}"
echo -e "${BOLD}╚══════════════════════════════════════════╝${RESET}"
echo ""
warn "This script will:"
echo "  1. Collect idle baseline  (${IDLE_SECS}s   — do nothing)"
echo "  2. Collect normal load    (${NORMAL_SECS}s   — simulated editor/log activity)"
echo "  3. Simulate ransomware    (${ATTACK_SECS}s   — bulk rename/write/delete in /tmp)"
echo ""
read -rp "Press Enter to begin, Ctrl+C to abort..."

mkdir -p "$SANDBOX"

# ── Phase 1: idle ────────────────────────────────────────────────────────────
header "Phase 1/3 — Idle baseline"
info "Close browsers and heavy apps if possible. Just let the system sit."
touch "$SCORES_IDLE"
start_collection "$SCORES_IDLE"
countdown $IDLE_SECS "Collecting idle scores"
stop_collection
success "Idle phase complete. Samples: $(wc -l < "$SCORES_IDLE")"

# ── Phase 2: normal load ─────────────────────────────────────────────────────
header "Phase 2/3 — Normal load"
info "Simulating typical file activity (editor saves, log writes)..."
touch "$SCORES_NORMAL"
start_collection "$SCORES_NORMAL"
LOAD_PID=$(simulate_normal_load)
countdown $NORMAL_SECS "Collecting normal-load scores"
kill "$LOAD_PID" 2>/dev/null || true
stop_collection
success "Normal load phase complete. Samples: $(wc -l < "$SCORES_NORMAL")"

# ── Phase 3: attack ───────────────────────────────────────────────────────────
header "Phase 3/3 — Simulated ransomware attack"
info "Running bulk rename/overwrite/delete in $SANDBOX ..."
touch "$SCORES_ATTACK"
start_collection "$SCORES_ATTACK"
sleep 2   # let agent settle before attack starts
simulate_attack
countdown $ATTACK_SECS "Collecting attack scores"
stop_collection
success "Attack phase complete. Samples: $(wc -l < "$SCORES_ATTACK")"

# ── Analysis ──────────────────────────────────────────────────────────────────
header "Score distributions"
echo -e "${CYAN}Idle:${RESET}"
stats "$SCORES_IDLE"
echo -e "${CYAN}Normal load:${RESET}"
stats "$SCORES_NORMAL"
echo -e "${CYAN}Attack:${RESET}"
stats "$SCORES_ATTACK"

# Compute recommended thresholds
NORMAL_P99=$(percentile "$SCORES_NORMAL" 99)
NORMAL_MAX=$(percentile "$SCORES_NORMAL" 100)
ATTACK_P50=$(percentile "$SCORES_ATTACK" 50)
ATTACK_MIN=$(percentile "$SCORES_ATTACK" 1)

# warning = midpoint(normal p99, attack p10)
ATTACK_P10=$(percentile "$SCORES_ATTACK" 10)
WARNING=$(awk "BEGIN { printf \"%.2f\", ($NORMAL_P99 + $ATTACK_P10) / 2 }")
# alert = midpoint(normal max, attack p50)
ALERT=$(awk "BEGIN { printf \"%.2f\", ($NORMAL_MAX + $ATTACK_P50) / 2 }")

# Sanity: ensure alert > warning and both are in (0,1)
ALERT=$(awk  "BEGIN { v=$ALERT;  if(v<0.1) v=0.75; if(v>0.99) v=0.99; printf \"%.2f\", v }")
WARNING=$(awk "BEGIN { v=$WARNING; if(v<0.1) v=0.50; if(v>$ALERT) v=$ALERT-0.10; printf \"%.2f\", v }")

# Check for overlap
OVERLAP="no"
if awk "BEGIN { exit !($NORMAL_MAX >= $ATTACK_MIN) }"; then
    OVERLAP="yes"
fi

header "Recommended thresholds"
echo -e "  ${GREEN}warning-score${RESET} = ${BOLD}$WARNING${RESET}"
echo -e "  ${GREEN}alert-score${RESET}   = ${BOLD}$ALERT${RESET}"
echo ""

if [[ "$OVERLAP" == "yes" ]]; then
    warn "Normal and attack distributions OVERLAP (normal max=$NORMAL_MAX, attack min=$ATTACK_MIN)"
    warn "Consider lowering -decay-rate (try 0.75) to make bursts stand out more, then re-run."
    echo ""
    echo -e "  Suggested re-run with faster decay:"
    echo -e "  ${CYAN}sudo bash calibrate.sh $AGENT  # after editing agent default or passing flags${RESET}"
else
    success "Clean separation between normal and attack distributions."
fi

echo ""
echo -e "${BOLD}Run the agent with these thresholds:${RESET}"
echo -e "  ${CYAN}sudo $AGENT -warning-score $WARNING -alert-score $ALERT${RESET}"
echo ""
echo -e "${BOLD}Or with faster decay to suppress background noise:${RESET}"
echo -e "  ${CYAN}sudo $AGENT -warning-score $WARNING -alert-score $ALERT -decay-rate 0.80${RESET}"
echo ""
