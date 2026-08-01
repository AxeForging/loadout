package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LockFileName is the advisory single-writer lock inside the machine-local
// data dir (never the shareable store repo).
const LockFileName = "store.lock"

type lockInfo struct {
	PID int `json:"pid"`
}

// Lock takes the machine-wide store mutation lock so concurrent loadout
// processes (CLI, TUI, serve) cannot interleave store writes. It fails fast
// with the holder's PID rather than blocking; a lock whose holder is dead is
// stolen. Reads never take the lock.
func Lock(dataDir string) (unlock func(), err error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, LockFileName)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			raw, _ := json.Marshal(lockInfo{PID: os.Getpid()})
			_, werr := f.Write(raw)
			cerr := f.Close()
			if werr != nil || cerr != nil {
				os.Remove(path)
				return nil, errors.Join(werr, cerr)
			}
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		holder, alive := lockHolder(path)
		if alive {
			return nil, fmt.Errorf("store is locked by another loadout process (pid %d) — wait for it to finish or remove %s if it crashed", holder, path)
		}
		// Holder is gone (crash) or the file is unreadable garbage: steal.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("store lock at %s is contended — retry", path)
}

// lockHolder reads the lock owner's PID and reports whether that process is
// still alive. Unreadable or malformed lock files count as dead.
func lockHolder(path string) (pid int, alive bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var info lockInfo
	if err := json.Unmarshal(raw, &info); err != nil || info.PID <= 0 {
		return 0, false
	}
	return info.PID, processAlive(info.PID)
}
