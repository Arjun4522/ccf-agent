#!/usr/bin/env bash
# gen-vmlinux.sh — generate ebpf/vmlinux.h from the running kernel's BTF blob.
# Must be run inside nix-shell (shell.nix) so bpftool is on PATH.
set -euo pipefail

OUT="ebpf/vmlinux.h"

if ! command -v bpftool &>/dev/null; then
  echo "ERROR: bpftool not found. Run this script inside nix-shell." >&2
  exit 1
fi

echo "[gen-vmlinux] generating $OUT from /sys/kernel/btf/vmlinux ..."
bpftool btf dump file /sys/kernel/btf/vmlinux format c > "$OUT"
echo "[gen-vmlinux] done — $(wc -l < "$OUT") lines written to $OUT"
