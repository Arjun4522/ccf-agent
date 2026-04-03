# CCF Agent — Real-Time Ransomware Detection & Blocking

A Linux endpoint protection agent that detects and blocks ransomware attacks in real-time using eBPF telemetry and a Cognitive Capability Field (CCF) model. Designed for enterprise environments requiring sub-second threat response.

---

## Overview

CCF Agent monitors system calls via eBPF tracepoints, builds a time-evolving influence field of process behavior, and automatically responds to detected ransomware patterns with graduated blocking actions.

**Key Capabilities:**
- Real-time ransomware detection using multi-feature behavioral analysis
- Sub-second detection with streaming regression (detects in ~2.5s vs 15s previously)
- Three-layer automated response: SIGSTOP (pause) → SIGKILL (terminate) → Quarantine
- Zero-configuration deployment with sensible defaults
- JSON logging for SIEM integration
- Dry-run mode for safe testing

---

## Architecture — End-to-End Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        KERNEL SPACE (eBPF)                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │  ccf_probe.c — Tracepoint Attachments                                   │ │
│  │  - sys_enter_openat   (file open operations)                          │ │
│  │  - sys_enter_write    (file writes)                                    │ │
│  │  - sys_enter_renameat2 (file renames)                                  │ │
│  │  - sys_enter_unlinkat (file deletions)                                 │ │
│  │  - sched_process_exec (program execution)                              │ │
│  │  - sys_enter_setuid   (privilege changes)                              │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                     │                                        │
│                                     ▼                                        │
│                           Perf Ring Buffer                                   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                     ┌─────────────────┴─────────────────┐
                     │      USER SPACE (Go Agent)       │
                     ▼                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STAGE 1: COLLECTOR                                                          │
│  File: internal/collector/collector.go                                      │
│                                                                             │
│  - Loads compiled eBPF object into kernel via Cilium eBPF library          │
│  - Attaches tracepoints to kernel events                                    │
│  - Reads from perf ring buffer (kernel → userspace)                        │
│  - Deserializes raw bytes → RawEvent struct                                 │
│  - Filters blocked PIDs (agent's own PID, systemd, etc.)                  │
│                                                                             │
│  Output: event.RawEvent { TimestampNS, PID, PPID, ProcessName,             │
│                           UID, GID, Path, DstPath, Type }                  │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STAGE 2: MAPPER                                                             │
│  File: internal/mapper/mapper.go                                            │
│                                                                             │
│  - Maps raw syscalls to CAPABILITIES (abstract security concepts)          │
│  - Assigns NODE IDs based on process + file path                            │
│  - Weights each event by capability type                                     │
│                                                                             │
│  Capability Mapping:                                                        │
│  - FILE_OPEN    → capability "file_read" + node "proc:PID:/path"          │
│  - FILE_WRITE   → capability "file_write" + node "proc:PID:/path"          │
│  - FILE_RENAME  → capability "file_modify" + node "proc:PID"               │
│  - FILE_DELETE  → capability "file_destroy" + node "proc:PID"              │
│  - EXEC         → capability "execute" + node "proc:PID"                  │
│  - SETUID       → capability "privilege_escalate" + node "proc:PID"        │
│                                                                             │
│  Output: event.MappedEvent { NodeID, Capability, Weight, TimestampNS }   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STAGE 3: FIELD (The Cognitive Capability Field)                           │
│  File: internal/field/field.go                                             │
│                                                                             │
│  CONCEPT: The CCF is a sparse mathematical field where:                    │
│  - Each node represents a unique (process, file) combination               │
│  - Intensity represents activity level at that node                        │
│  - Field evolves over time with decay and propagation                       │
│                                                                             │
│  DATA STRUCTURES:                                                           │
│  - intensities: map[string]float64  // nodeID → intensity value           │
│  - adjacency:   map[string][]string // cached neighbor relationships       │
│                                                                             │
│  OPERATIONS:                                                                │
│  1. UPDATE: When event arrives, increment node intensity                   │
│  2. PROPAGATE: Intensity spreads to neighboring nodes (parent dirs)        │
│  3. DECAY: Every tick, all intensities multiply by decay_rate (0.85)      │
│  4. SNAPSHOT: Capture field state every SnapshotInterval (default 500ms)   │
│                                                                             │
│  NEIGHBOR DEFINITION:                                                       │
│  - /home/user/file.txt is neighbor to /home/user/                          │
│  - /home/user/doc/ is neighbor to /home/user/                              │
│  - Enables detection of lateral movement across directories               │
│                                                                             │
│  Output to TemporalEngine: field.Snapshot { At, Intensities, Norm }        │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STAGE 4: TEMPORAL ENGINE                                                   │
│  File: internal/field/field.go (TemporalEngine)                          │
│                                                                             │
│  MAINTAINS: Sliding window of WindowSize snapshots (default 30)            │
│                                                                             │
│  WHY A WINDOW?                                                              │
│  - Individual snapshots are noisy                                          │
│  - Time-series analysis reveals attack patterns                            │
│  - CFER (slope) requires multiple points to compute                        │
│                                                                             │
│  KEY PARAMETERS:                                                            │
│  - WindowSize: 30 (30 × 500ms = 15 seconds of history)                    │
│  - SnapshotInterval: 500ms (how often we capture field state)              │
│  - DecayInterval: 500ms (same as snapshot for synchronized decay)          │
│                                                                             │
│  Output: []field.Snapshot (sliding window array)                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STAGE 5: FEATURE EXTRACTION                                               │
│  File: internal/features/features.go                                      │
│                                                                             │
│  COMPUTES four behavioral features from the snapshot window:               │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  1. CFER (Capability Field Expansion Rate)                          │   │
│  │                                                                      │   │
│  │  WHAT: Linear regression slope of ||F|| (field norm) over time      │   │
│  │  WHY:  Idle systems have ~0 slope; ransomware has positive slope   │   │
│  │        because activity keeps increasing as files get encrypted     │   │
│  │                                                                      │   │
│  │  HOW:  slope = Σ(n*x*y) - Σ(x)*Σ(y) / Σ(n*x²) - (Σ(x))²            │   │
│  │        where x = time index, y = norm value                         │   │
│  │                                                                      │   │
│  │  EXAMPLE:                                                            │   │
│  │  - Idle:    [1.0, 1.1, 0.9, 1.0, 1.1] → slope ≈ 0                 │   │
│  │  - Attack:  [1.0, 2.5, 4.0, 5.5, 7.0] → slope ≈ 1.5 (rising!)    │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  2. TURBULENCE (Variance)                                           │   │
│  │                                                                      │   │
│  │  WHAT: Variance of field norm over the window                       │   │
│  │  WHY:  Ransomware shows high variance (bursty behavior)             │   │
│  │        Benign processes have low variance (steady I/O)             │   │
│  │                                                                      │   │
│  │  HOW:  Var = E[||F||²] - E[||F||]²                                  │   │
│  │                                                                      │   │
│  │  EXAMPLE:                                                            │   │
│  │  - Idle:    [1.0, 1.1, 0.9, 1.0, 1.1] → variance ≈ 0.01           │   │
│  │  - Attack:  [1.0, 8.0, 2.0, 9.0, 3.0] → variance ≈ 12.0 (noisy!)  │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  3. SHOCKWAVE (Second Derivative)                                   │   │
│  │                                                                      │   │
│  │  WHAT: Acceleration of field norm (rate of change of slope)          │   │
│  │  WHY:  Ransomware has sudden onset — big spike when encryption     │   │
│  │        starts, then slows down as files complete                    │   │
│  │                                                                      │   │
│  │  HOW:  shock = ||Fₜ|| - 2*||Fₜ₋₁|| + ||Fₜ₋₂||                       │   │
│  │        (positive = accelerating, negative = decelerating)            │   │
│  │                                                                      │   │
│  │  NOTE: Negative shockwave is clamped to 0 (benign signal)           │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  4. ENTROPY (Shannon Entropy)                                       │   │
│  │                                                                      │   │
│  │  WHAT: How spread out the activity is across nodes                  │   │
│  │  WHY:  Ransomware touches many files → high entropy                 │   │
│  │        Single process doing one thing → low entropy                 │   │
│  │                                                                      │   │
│  │  HOW:  H = -Σ p(x) · log₂(p(x))                                     │   │
│  │        where p(x) = intensity(x) / Σ intensities                    │   │
│  │                                                                      │   │
│  │  EXAMPLE:                                                            │   │
│  │  - One process writing one file → entropy ≈ 0-1 bit                │   │
│  │  - Ransomware encrypting 100 files → entropy ≈ 4-6 bits            │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  STREAMING REGRESSION (NEW):                                                │
│  - Instead of storing full window, maintains running statistics            │
│  - O(1) computation per new data point                                     │
│  - Detects in ~5 data points instead of 30                                 │
│  - Key classes: RunningCFER, ComputeWithStreaming()                        │
│                                                                             │
│  Output: features.Vector { CFER, Turbulence, Shockwave, Entropy,           │
│                            ActiveNodes, OffenderPID, ParentPID }           │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STAGE 6: DETECTOR                                                          │
│  File: internal/detector/detector.go                                        │
│                                                                             │
│  TWO DETECTION MODES:                                                       │
│  1. THRESHOLD (default) — Fast, deterministic, no ML required             │
│  2. ML INFERENCE — Pluggable classifier interface                          │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  THRESHOLD SCORING:                                                  │   │
│  │                                                                      │   │
│  │  Each feature is normalized to [0,1] by dividing by threshold:      │   │
│  │  - cfer_n     = clamp01(cfer / CFERThreshold)                       │   │
│  │  - turb_n     = clamp01(turbulence / TurbulenceThreshold)           │   │
│  │  - shock_n    = clamp01(sqrt(shock) / sqrt(ShockwaveThreshold))     │   │
│  │  - entropy_n  = clamp01(entropy / EntropyThreshold)                  │   │
│  │                                                                      │   │
│  │  Weighted sum produces composite score:                              │   │
│  │  - score = 0.45*cfer_n + 0.15*turb_n + 0.30*shock_n + 0.10*entr_n   │   │
│  │                                                                      │   │
│  │  DEFAULT WEIGHTS (sum to 1.0):                                       │   │
│  │  - CFER: 0.45 (primary signal — regression slope is cleanest)       │   │
│  │  - Turbulence: 0.15 (secondary signal)                               │   │
│  │  - Shockwave: 0.30 (good for detecting onset)                       │   │
│  │  - Entropy: 0.10 (supporting signal)                                 │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  MULTI-SCALE DETECTION (NEW):                                        │   │
│  │                                                                      │   │
│  │  FAST PATH (early warning):                                          │   │
│  │  - Score ≥ FastThreshold (0.30) → WARNING                          │   │
│  │  - Score ≥ FastThreshold × ConfirmMultiplier (0.45) → ALERT       │   │
│  │  - Detects in ~2.5 seconds (5 data points × 500ms)                  │   │
│  │                                                                      │   │
│  │  SLOW PATH (full confirmation):                                      │   │
│  │  - Score ≥ AlertScore (0.65) → ALERT                               │   │
│  │  - Uses full window for maximum accuracy                            │   │
│  │  - Takes ~15 seconds (30 data points × 500ms)                       │   │
│  │                                                                      │   │
│  │  WHY TWO PATHS?                                                      │   │
│  │  - Fast path: React quickly to obvious attacks                      │   │
│  │  - Slow path: Confirm with more data to avoid false positives       │   │
│  │  - Hybrid: Fast initial response + confirmation for edge cases       │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  SEVERITY CLASSIFICATION:                                                   │
│  - NONE:    Score < FastThreshold (0.30)                                   │
│  - WARNING: FastThreshold ≤ Score < AlertScore (0.65)                     │
│  - ALERT:   Score ≥ AlertScore                                              │
│                                                                             │
│  HYSTERESIS (AlertCooldownTicks):                                           │
│  - After ALERT, maintain minimum WARNING for N ticks (default 60 = 30s)   │
│  - Prevents "oscillation" where attack pauses and drops to NONE            │
│                                                                             │
│  Output: detector.Detection { At, Severity, Score, Vector, Reason }       │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STAGE 7: RESPONDER                                                         │
│  File: internal/responder/responder.go                                      │
│                                                                             │
│  GRADUATED RESPONSE ACTIONS:                                               │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  WARNING (Score ≥ 0.30, < 0.65)                                      │   │
│  │  ─────────────────────────────────────────────────────────────────  │   │
│  │  Action: SIGSTOP (pause process)                                      │   │
│  │  Reason: Reversible — gives time to analyze before killing           │   │
│  │  Auto-Resume: After ResumeWindow (default 10s), resume if score     │   │
│  │               drops below threshold                                 │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  ALERT (Score ≥ 0.65)                                                │   │
│  │  ─────────────────────────────────────────────────────────────────  │   │
│  │  Sequence of actions (in order):                                     │   │
│  │  1. CAPTURE EVIDENCE: Save cmdline, maps, fds, status to             │   │
│  │     EvidenceDir (for forensics)                                     │   │
│  │  2. PAUSE: SIGSTOP the process                                       │   │
│  │  3. QUARANTINE: Move suspicious files (*.enc, *.locked, etc.)       │   │
│  │     to QuarantineDir                                                │   │
│  │  4. ISOLATE NETWORK: iptables DROP for process UID (optional)       │   │
│  │  5. KILL: SIGKILL the process and its children                       │   │
│  │                                                                      │   │
│  │  Process Tree Kill:                                                  │   │
│  │  - Walk /proc to build parent → children map                        │   │
│  │  - Kill process group (kill -9 -PGID) to catch parallel subshells  │   │
│  │  - Track killed scripts for respawn detection                       │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ENHANCEMENTS:                                                               │
│  - ALLOWLIST: Never act on systemd, sshd, journald, etc.                   │
│  - COOLDOWN: Don't hammer same PID twice within CooldownWindow            │
│  - RESPAWN DETECTION: If killed script re-executes within RespawnWindow,  │   │
│    immediately escalate and block the script file                          │
│  - EVIDENCE SNAPSHOT: Forensically capture process state before kill       │
│  - WEBHOOK: POST JSON to HTTP endpoint (Slack, PagerDuty, etc.)            │
│  - SYSLOG: Emit to system log (LOG_AUTHPRIV | LOG_CRIT)                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Key Concepts Explained

### 1. What is the "Cognitive Capability Field"?

Imagine a map where every point represents a (process, file) combination. When a process accesses a file, that point "lights up" with intensity. Over time:

- **Idle system**: Few points light up, intensity is low and stable
- **Ransomware attack**: Many points light up rapidly, intensity increases

The "field" is this entire collection of active points with their intensity values.

### 2. Why Regression Slope (CFER)?

Traditional detection looks for absolute thresholds (e.g., "100 file writes = bad"). But:
- Legitimate processes can also write 100 files (compilation, backup)
- Ransomware might start slowly

CFER measures the **trend** — is activity *accelerating*? A steady 10 writes/sec = CFER ≈ 0. An accelerating 10→20→30 writes/sec = CFER > 0.

### 3. Why Multiple Features?

No single feature is perfect. We combine:
- **CFER**: Detects sustained attack (main signal)
- **Turbulence**: Catches bursty behavior
- **Shockwave**: Catches sudden onset
- **Entropy**: Distinguishes widespread vs. localized activity

The weighted combination reduces false positives.

### 4. Why Streaming Detection?

Original: Wait 30 snapshots (15 seconds) → compute regression → detect
Problem: 15 seconds of ransomware = many files encrypted

New: Compute running slope with each new point → detect after 5 points (~2.5 seconds)
Result: 6x faster detection, similar accuracy

---

## Detection Latency Comparison

| Metric | Before | After |
|--------|--------|-------|
| WARNING detection | ~15s (full window) | ~2.5s (5 points) |
| ALERT detection | ~15s | ~5s (confirm threshold) |
| CFER computation | O(n) window scan | O(1) streaming |

---

## Features

### Detection Engine
- **CFER (Capability Field Expansion Rate):** Linear regression slope of field intensity — detects sustained attack growth
- **Turbulence:** Variance of field intensity — identifies behavioral instability
- **Shockwave:** Second derivative of intensity — catches sudden attack onset
- **Entropy:** Shannon entropy of node distribution — distinguishes localized vs. widespread activity
- **Multi-Scale Detection:** Fast path for quick response, slow path for confirmation

### Response Actions
| Severity | Condition | Action |
|----------|-----------|--------|
| WARNING | Score ≥ 0.30 | SIGSTOP — pause process (reversible) |
| ALERT | Score ≥ 0.65 | SIGKILL + quarantine files |
| NONE | Below thresholds | No action |

### Auto-Resume
SIGSTOP'd processes are automatically resumed if the threat score drops within the `ResumeWindow` (default: 10s), preventing false-positive process stalls.

---

## Quick Start

### Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.22+ | Primary build tool |
| clang | 14+ | eBPF compilation |
| linux-headers | matching kernel | `apt install linux-headers-$(uname -r)` |
| bpf2go | latest | `go install github.com/cilium/ebpf/cmd/bpf2go@latest` |

**Kernel:** 5.8+ (CAP_BPF + CAP_PERFMON) or 4.18+ (CAP_SYS_ADMIN)

### Build & Run

```bash
# Generate eBPF bindings
make generate

# Compile agent
make build

# Run with default settings (requires root)
sudo ./ccf-agent

# JSON output for piping to SIEM
sudo ./ccf-agent -json

# Dry-run mode (log actions without executing)
sudo ./ccf-agent -json -dry-run
```

### Production Deployment

```bash
# Enable all blocking actions
sudo ./ccf-agent \
  -json \
  -kill-on-alert \
  -pause-on-warning \
  -quarantine-dir /var/lib/ccf-agent/quarantine
```

---

## CLI Flags

### Detection Parameters
| Flag | Default | Description |
|------|---------|-------------|
| `-decay-rate` | 0.85 | Field intensity decay per tick (0-1) |
| `-window-size` | 30 | Temporal window size (snapshots) |
| `-snapshot-interval` | 100ms | Tick rate for decay and snapshots |
| `-alert-score` | 0.65 | Composite score threshold for ALERT |
| `-warning-score` | 0.40 | Composite score threshold for WARNING |
| `-cooldown-ticks` | 60 | Hysteresis ticks after ALERT |

### Multi-Scale Detection (NEW)
| Flag | Default | Description |
|------|---------|-------------|
| `-fast-window-size` | 10 | Fast detection window (snapshots) |
| `-slow-window-size` | 30 | Slow confirmation window (snapshots) |
| `-min-data-points` | 5 | Minimum points for streaming detection |
| `-fast-threshold` | 0.30 | Fast path warning threshold |
| `-confirm-multiplier` | 1.5 | Multiplier for fast → alert escalation |

### Blocking Parameters
| Flag | Default | Description |
|------|---------|-------------|
| `-dry-run` | false | Log actions without executing |
| `-kill-on-alert` | true | SIGKILL on ALERT |
| `-pause-on-warning` | true | SIGSTOP on WARNING |
| `-quarantine-dir` | `/var/lib/ccf-agent/quarantine` | Directory for quarantined files |
| `-kill-tree` | true | Kill entire process tree |
| `-isolate-network` | false | iptables DROP for offending UID |
| `-evidence-dir` | `/var/lib/ccf-agent/evidence` | Forensic snapshot directory |

### Output Parameters
| Flag | Default | Description |
|------|---------|-------------|
| `-json` | false | JSON lines output |
| `-debug` | false | Verbose logging |
| `-webhook-url` | (none) | HTTP endpoint for JSON alerts |
| `-syslog` | false | Emit to system syslog |

---

## Output Format

### Plain Text (default)
```
[14:22:01.443] ALERT  score=0.812  cfer=0.523  turb=18.402  shock=9.841  entropy=3.342  nodes=34  pid=12847
```

### JSON Lines
```json
{"at":"2024-03-27T14:22:01.443Z","severity":"ALERT","score":0.812,"cfer":0.523,"turbulence":18.402,"shockwave":9.841,"entropy":3.342,"active_nodes":34,"offender_pid":12847,"reason":"..."}
```

---

## Project Structure

```
ccf-agent/
├── cmd/agent/
│   └── main.go              # Pipeline wiring + CLI flags
├── internal/
│   ├── collector/           # eBPF loader, perf reader, event parsing
│   ├── mapper/              # RawEvent → MappedEvent (capability + node)
│   ├── field/               # Influence field + temporal engine
│   ├── features/            # CFER, turbulence, shockwave, entropy extraction
│   │                       # + streaming regression (NEW)
│   ├── detector/            # Threshold scoring + multi-scale detection (NEW)
│   └── responder/           # SIGSTOP/SIGKILL/Quarantine actions
├── pkg/event/               # Shared types (RawEvent, MappedEvent, Capability)
├── ebpf/
│   └── ccf_probe.c         # eBPF C source (tracepoints)
└── ransomware_*.sh          # Test simulators (safe patterns)
```

---

## Testing

```bash
# Run all unit tests
make test

# Test with ransomware simulator (dry-run first!)
sudo ./ccf-agent -json -dry-run &
./ransomware_simulator.sh
```

### Test Results
- ✅ Single-process ransomware: Immediate ALERT + SIGKILL
- ✅ Multi-process ransomware: Multiple ALERTs + SIGKILL per subprocess
- ✅ Dry-run mode: No actual blocking, logs all actions

---

## Benchmarking

To measure detection latency and accuracy:

```bash
# Run built-in benchmarks
go test -bench=. ./internal/features/
go test -bench=. ./internal/detector/

# End-to-end latency test
# 1. Start agent with debug output
sudo ./ccf-agent -debug -json 2>&1 | tee agent.log

# 2. Run ransomware simulator
./ransomware_simulator.sh

# 3. Analyze timestamps in agent.log
#    Timestamp difference = event time → detection time
```

Expected benchmarks:
- **Throughput**: ~10,000+ events/second
- **Latency**: ~2.5s WARNING, ~5s ALERT
- **Memory**: ~50MB typical

---

## SIEM Integration

JSON output streams directly to log aggregators:

```bash
# Splunk
sudo ./ccf-agent -json | splunkforwarder

# ELK (Filebeat)
sudo ./ccf-agent -json >> /var/log/ccf-agent.json

# Vector
sudo ./ccf-agent -json | vector --config vector.toml
```

---

## Security Considerations

1. **Root Required:** Agent needs CAP_BPF/CAP_SYS_ADMIN for eBPF + signal operations
2. **Dry-Run First:** Always test in dry-run mode before production deployment
3. **Immutable Quarantine:** Files can be made immutable (`chattr +i`) as fallback
4. **Process Groups:** Consider `killpg()` for forked ransomware that spawns children
5. **Allowlist:** Review and customize the allowlist for your environment

---

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/xyz`)
3. Run tests (`make test`)
4. Commit changes with clear messages
5. Open a Pull Request

---

## License

MIT License — See LICENSE file for details

---

## References

- [eBPF Documentation](https://ebpf.io/)
- [Cilium eBPF Go Library](https://github.com/cilium/ebpf)
- [Zap Logging](https://github.com/uber-go/zap)
- [Linear Regression](https://en.wikipedia.org/wiki/Linear_regression)
- [Shannon Entropy](https://en.wikipedia.org/wiki/Entropy_(information_theory))
