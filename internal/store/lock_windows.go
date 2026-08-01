//go:build windows

package store

import "os"

// processAlive reports whether a PID refers to a live process. On Windows,
// FindProcess opens a real handle and fails for nonexistent PIDs.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
