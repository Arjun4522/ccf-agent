//go:build linux

package responder

import "syscall"

func syscallSIGSTOP() syscall.Signal { return syscall.SIGSTOP }
func syscallSIGCONT() syscall.Signal { return syscall.SIGCONT }