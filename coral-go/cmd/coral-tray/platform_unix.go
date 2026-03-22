//go:build !windows

package main

import "syscall"

// detachProcessAttrs returns SysProcAttr for running a detached background process.
func detachProcessAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// signalProcess sends a signal to a process. Used for --stop and PID liveness checks.
func signalProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
