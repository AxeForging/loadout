package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic_ReplacesContentAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := WriteFileAtomic(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "v2" {
		t.Fatalf("content = %q, err %v", raw, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestWriteFileAtomic_FailedWriteLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Make the directory unwritable so the temp file cannot be created —
	// the failure mode must never touch the existing file.
	if err := os.Chmod(filepath.Dir(path), 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Dir(path), 0o755)

	if err := WriteFileAtomic(path, []byte("partial"), 0o644); err == nil {
		t.Fatal("write into read-only dir should fail")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "original" {
		t.Errorf("original content lost: %q", raw)
	}
}

func TestWriteFileAtomic_LeavesNoTempDroppings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.yaml")
	if err := WriteFileAtomic(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
