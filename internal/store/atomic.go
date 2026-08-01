package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path so that a crash mid-write can never
// leave a partial file: the bytes go to a temp file in the same directory
// (same filesystem, so the final rename is atomic), are fsynced, and then
// renamed over the target. Used for the store's metadata files — a corrupt
// loadout.yaml or sources.lock.yaml would orphan the whole store.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}
