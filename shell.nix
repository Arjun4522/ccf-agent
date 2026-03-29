{ pkgs ? import <nixpkgs> {} }:

let
  # Use the unwrapped clang — NixOS's wrapped clang injects hardening flags
  # (-fzero-call-used-regs, -fstack-protector-strong) that are invalid for
  # the BPF target and cause bpf2go compilation to fail.
  clangUnwrapped = pkgs.llvmPackages.clang-unwrapped;
  llvmUnwrapped  = pkgs.llvmPackages.llvm;
in

pkgs.mkShell {
  name = "ccf-agent-dev";

  buildInputs = with pkgs; [
    go
    clangUnwrapped
    llvmUnwrapped
    libbpf
    bpftools
    linuxPackages.kernel.dev
    golangci-lint
  ];

  shellHook = ''
    export LIBBPF_INCLUDE="${pkgs.libbpf}/include"

    # Point bpf2go at the unwrapped clang binary directly
    export CLANG="${clangUnwrapped}/bin/clang"
    export LLVM_STRIP="${llvmUnwrapped}/bin/llvm-strip"

    # Kernel headers path
    KHEADERS="$(echo ${pkgs.linuxPackages.kernel.dev}/lib/modules/*/build 2>/dev/null | head -1)"
    export KERNEL_HEADERS="$KHEADERS"

    # bpf2go respects the BPF_CLANG env var to override the compiler
    export BPF_CLANG="$CLANG"

    echo ""
    echo "  ccf-agent NixOS dev shell"
    echo "  Go             : $(go version)"
    echo "  Clang          : $CLANG"
    echo "  LIBBPF_INCLUDE : $LIBBPF_INCLUDE"
    echo "  KERNEL_HEADERS : $KERNEL_HEADERS"
    echo ""
    echo "  Next steps:"
    echo "    1. make vmlinux          # sudo — generates ebpf/vmlinux.h"
    echo "    2. make nixos-generate   # bpf2go → Go bindings"
    echo "    3. make build            # compile agent"
    echo "    4. sudo ./ccf-agent      # run (needs CAP_BPF)"
    echo ""
  '';
}
