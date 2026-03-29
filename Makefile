BINARY     := ccf-agent
CMD        := ./cmd/agent
EBPF_SRC   := ebpf/ccf_probe.c
GENERATED  := internal/collector/ccfprobe_bpfel.go

.PHONY: all build generate nixos-generate vmlinux lint test clean

all: generate build

# ── eBPF code generation ────────────────────────────────────────────────────

# Standard generate (non-NixOS / FHS systems).
generate:
	go generate ./internal/collector/...

# NixOS generate: verifies env vars exported by shell.nix are present.
# Usage: nix-shell --run "make nixos-generate"
nixos-generate:
	@test -n "$$LIBBPF_INCLUDE"  || { echo "ERROR: LIBBPF_INCLUDE not set — run inside nix-shell (shell.nix)"; exit 1; }
	@test -n "$$KERNEL_HEADERS"  || { echo "ERROR: KERNEL_HEADERS not set — run inside nix-shell (shell.nix)"; exit 1; }
	go generate ./internal/collector/...

# Generate ebpf/vmlinux.h from the running kernel.
# Requires root (reads /sys/kernel/btf/vmlinux).
vmlinux:
	sudo bash scripts/gen-vmlinux.sh

# ── Build ───────────────────────────────────────────────────────────────────

build: $(GENERATED)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) $(CMD)

# ── Run ─────────────────────────────────────────────────────────────────────

run: build
	sudo ./$(BINARY) $(ARGS)

run-debug: build
	sudo ./$(BINARY) -debug -json $(ARGS)

# ── Quality ─────────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

test:
	go test ./internal/... -v -race

# ── Housekeeping ─────────────────────────────────────────────────────────────

clean:
	rm -f $(BINARY)
	rm -f internal/collector/ccfProbe_bpf*.go
	rm -f internal/collector/ccfProbe_bpf*.o
	rm -f ebpf/vmlinux.h
