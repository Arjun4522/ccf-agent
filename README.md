# CCF Agent — Cognitive Capability Field Malware Detector

Pre-encryption ransomware detection for Linux using eBPF + a time-evolving
influence field model.

## Architecture

```
Kernel tracepoints (eBPF)
        │
        ▼
  [Collector]          internal/collector/
        │  RawEvent
        ▼
   [Mapper]            internal/mapper/
        │  MappedEvent  (syscall → capability + node ID)
        ▼
[TemporalEngine]       internal/field/
        │  drives Field.Update() + Decay() + Snapshot()
        │
        ├─── every 500 ms ───▶ [Extractor]    internal/features/
        │                            │  Vector (CFER, turbulence, shockwave, entropy)
        │                            ▼
        │                       [Detector]    internal/detector/
        │                            │  Detection
        │                            ▼
        │                        stdout / JSON
        ▼
   [Field F]           sparse map: NodeID → float64 intensity
```

## Project layout

```
ccf-agent/
├── cmd/agent/          main.go — pipeline wiring + CLI flags
├── ebpf/               ccf_probe.c — eBPF C source (tracepoints)
├── internal/
│   ├── collector/      eBPF loader, perf ring reader, RawEvent deserialiser
│   ├── mapper/         RawEvent → MappedEvent (capability + node ID)
│   ├── field/          Field + TemporalEngine (decay, propagation, snapshots)
│   ├── features/       CFER, turbulence, shockwave, entropy
│   └── detector/       threshold scoring + Classifier interface
└── pkg/event/          shared types (RawEvent, MappedEvent, Capability)
```

## Prerequisites

| Tool | Min version | Notes |
|------|-------------|-------|
| Go | 1.22 | |
| clang | 14 | For eBPF compilation |
| llvm-strip | 14 | Strips debug info from eBPF object |
| Linux headers | matching running kernel | `linux-headers-$(uname -r)` |
| bpf2go | latest | `go install github.com/cilium/ebpf/cmd/bpf2go@latest` |
| golangci-lint | 1.57+ | Optional, for `make lint` |

Kernel requirement: **5.8+** (CAP_BPF + CAP_PERFMON).  
Older kernels (4.18–5.7) can use CAP_SYS_ADMIN instead.

## Build

```bash
# 1. Generate Go bindings from C eBPF source
make generate

# 2. Compile the agent binary
make build

# 3. Run (requires root / CAP_BPF)
sudo ./ccf-agent

# JSON output
sudo ./ccf-agent -json

# Debug logging
sudo ./ccf-agent -debug -json
```

## CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `-decay-rate` | 0.92 | Field intensity decay per 500 ms tick |
| `-window-size` | 20 | Snapshots retained (× 500 ms = window duration) |
| `-snapshot-interval` | 500ms | Tick rate for decay + snapshot |
| `-alert-score` | 0.65 | Composite score threshold for ALERT |
| `-warning-score` | 0.35 | Composite score threshold for WARNING |
| `-json` | false | Emit detections as JSON lines |
| `-debug` | false | Verbose zap logging |

## Detection output

Plain text (default):
```
[14:22:01.443] ALERT  score=0.812  cfer=2.340  turb=1.102  shock=1.880  entropy=4.210  nodes=34
```

JSON (`-json`):
```json
{"at":"2024-03-27T14:22:01.443Z","severity":"ALERT","score":0.812,"cfer":2.340,"turbulence":1.102,"shockwave":1.880,"entropy":4.210,"active_nodes":34,"reason":"..."}
```

## Tests

```bash
# Feature extractor tests (no kernel required)
go test ./internal/features/... -v

# All unit tests
make test
```

## Implementation TODOs (stubs to fill in)

### Collector
- [ ] Run `make generate` to produce `ccfProbe_bpfel.go` from `ebpf/ccf_probe.c`
- [ ] `collector.go`: verify generated struct field names match after `bpf2go`
- [ ] `ccf_probe.c`: resolve fd→path for `EVT_FILE_WRITE` (currently path-less)
- [ ] Add `vmlinux.h` (generate with `bpftool btf dump file /sys/kernel/btf/vmlinux format c`)

### Mapper
- [ ] `clusterPath`: replace O(n) adjacency scan with a trie
- [ ] `resolveNode` for `EXEC`: cluster by binary directory, not process name
- [ ] Add entropy-based crypto detection (high-entropy write buffers via `/proc/<pid>/fd`)

### Field
- [ ] `neighbours`: implement trie-based O(1) lookup
- [ ] Add explicit adjacency rules for `proc:` and `priv:` nodes
- [ ] Expose per-process field slice for process-level attribution

### Detector
- [ ] `ThresholdClassifier.Score`: integrate a real model (ONNX via `onnxruntime-go`)
- [ ] Add alert deduplication / cooldown window
- [ ] Emit structured alert to syslog / webhook / SIEM

### Observability
- [ ] Expose Prometheus metrics (active nodes, norm, feature values, drop counts)
- [ ] Add pprof endpoint for profiling under load
