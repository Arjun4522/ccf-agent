package api

import "syscall"

func syscallSIGCONT() syscall.Signal { return syscall.SIGCONT }
