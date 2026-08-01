package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLock_SecondCallerFailsFastNamingTheHolder(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatalf("first Lock() error = %v", err)
	}
	defer unlock()

	_, err = Lock(dir)
	if err == nil {
		t.Fatal("second Lock() must fail while held")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(os.Getpid())) {
		t.Errorf("error should name the holding pid: %v", err)
	}
}

func TestLock_UnlockReleases(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	unlock2, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock() after unlock error = %v", err)
	}
	unlock2()
}

func TestLock_StaleLockFromDeadProcessIsStolen(t *testing.T) {
	dir := t.TempDir()
	// PIDs cycle below the max; a just-exited child's PID is unreliable, so
	// use an absurdly large one that cannot exist.
	raw, _ := json.Marshal(lockInfo{PID: 1 << 30})
	if err := os.WriteFile(filepath.Join(dir, LockFileName), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	unlock, err := Lock(dir)
	if err != nil {
		t.Fatalf("stale lock should be stolen, got %v", err)
	}
	unlock()
}

func TestLock_GarbageLockFileIsStolen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LockFileName), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatalf("garbage lock should be stolen, got %v", err)
	}
	unlock()
}
