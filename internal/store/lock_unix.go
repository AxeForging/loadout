//go:build !windows

package store

import (
	"errors"
	"syscall"
)

// processAlive probes a PID with signal 0 — no signal is delivered, only
// existence is checked. EPERM means the process exists under another user.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
