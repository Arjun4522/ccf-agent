# CCF Agent — Real-Time Ransomware Detection & Blocking

A Linux endpoint protection agent that detects and blocks ransomware attacks in real-time using eBPF telemetry and a Cognitive Capability Field (CCF) model. Designed for enterprise environments requiring sub-second threat response.

---

## Overview

CCF Agent monitors system calls via eBPF tracepoints, builds a time-evolving influence field of process behavior, and automatically responds to detected ransomware patterns with graduated blocking actions.

**Key Capabilities:**
- Real-time ransomware detection using multi-feature behavioral analysis
- Three-layer automated response: SIGSTOP (pause) → SIGKILL (terminate) → Quarantine
- Zero-configuration deployment with sensible defaults
- JSON logging for SIEM integration
- Dry-run mode for safe testing

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Kernel (eBPF Tracepoints)                    │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│  [Collector]  — eBPF loader, perf ring reader, event deserializer│
│                 internal/collector/                              │
└─────────────────────────────────────────────────────────────────┘
                                 │ RawEvent
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│  [Mapper]     — Syscall → Capability + Node ID mapping          │
│                 internal/mapper/                                 │
└─────────────────────────────────────────────────────────────────┘
                                 │ MappedEvent
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│  [Field]      — Sparse influence map: NodeID → intensity        │
│  [TemporalEngine] — Decay, propagation, snapshot every 500ms     │
│                 internal/field/                                  │
└─────────────────────────────────────────────────────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │                         │
                    ▼                         ▼
┌─────────────────────────────────────────────────────────────────┐
│  [Extractor]  — Feature Vector computation                      │
│   CFER, Turbulence, Shockwave, Entropy                         │
│                 internal/features/                              │
└─────────────────────────────────────────────────────────────────┘
                                 │ Vector
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│  [Detector]   — Threshold/ML scoring, severity classification   │
│                 internal/detector/                              │
└─────────────────────────────────────────────────────────────────┘
                                 │ Detection
                    ┌────────────┴────────────┐
                    │                         │
                    ▼                         ▼
┌───────────────────────────────┐  ┌───────────────────────────────────┐
│  [Responder]                 │  │  Output (stdout/JSON)              │
│  - SIGSTOP (WARNING)         │  │                                   │
│  - SIGKILL (ALERT)           │  │                                   │
│  - Quarantine files          │  │                                   │
│  internal/responder/         │  │                                   │
└───────────────────────────────┘  └───────────────────────────────────┘
```

---

## Features

### Detection Engine
- **CFER (Capability Field Expansion Rate):** Linear regression slope of field intensity — detects sustained attack growth
- **Turbulence:** Variance of field intensity — identifies behavioral instability
- **Shockwave:** Second derivative of intensity — catches sudden attack onset
- **Entropy:** Shannon entropy of node distribution — distinguishes localized vs. widespread activity

### Response Actions
| Severity | Condition | Action |
|----------|-----------|--------|
| WARNING | Score ≥ threshold | SIGSTOP — pause process (reversible) |
| ALERT | Score ≥ high threshold | SIGKILL + quarantine files |
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
| `-decay-rate` | 0.85 | Field intensity decay per 500ms tick (0-1) |
| `-window-size` | 30 | Temporal window size (× 500ms = 15s) |
| `-snapshot-interval` | 500ms | Tick rate for decay and snapshots |
| `-alert-score` | 0.65 | Composite score threshold for ALERT |
| `-warning-score` | 0.40 | Composite score threshold for WARNING |
| `-cooldown-ticks` | 6 | Hysteresis ticks after ALERT (3s) |

### Blocking Parameters
| Flag | Default | Description |
|------|---------|-------------|
| `-dry-run` | false | Log actions without executing |
| `-kill-on-alert` | true | SIGKILL on ALERT |
| `-pause-on-warning` | true | SIGSTOP on WARNING |
| `-quarantine-dir` | `/var/lib/ccf-agent/quarantine` | Directory for quarantined files |

### Output Parameters
| Flag | Default | Description |
|------|---------|-------------|
| `-json` | false | JSON lines output |
| `-debug` | false | Verbose logging |

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
│   ├── detector/            # Threshold scoring + detection output
│   └── responder/           # SIGSTOP/SIGKILL/Quarantine actions
├── pkg/event/               # Shared types (RawEvent, MappedEvent, Capability)
├── ebpf/
│   └── ccf_probe.c         # eBPF C source (tracepoints)
├── dashboard.html           # Optional: Web UI for monitoring
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

## Web Dashboard

A simple HTML dashboard for real-time monitoring:

```bash
# Serve dashboard.html via HTTP (implement /api endpoints in main.go)
# See dashboard.html for reference UI
```

Features: Status overview, detection counts, severity breakdown, action controls, live table updates.

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

---

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/xyz`)
3. Run tests (`make test`)
4. Commit changes with clear messages
5. Open a Pull Request

---

## License

MIT License — See LICENSE file for details.

---

## References

- [eBPF Documentation](https://ebpf.io/)
- [Cilium eBPF Go Library](https://github.com/cilium/ebpf)
- [Zap Logging](https://github.com/uber-go/zap)
