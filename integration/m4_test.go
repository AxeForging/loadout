//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBundleFlow_ExportWipeImport is the "new machine" story: export on one
// machine, import on a fresh one, sync, and end up with the same setup.
func TestBundleFlow_ExportWipeImport(t *testing.T) {
	machineA := newEnv(t)
	machineA.mustRun("init", "--name", "team-kit")
	machineA.mustRun("new", "skill", "review")
	machineA.mustRun("new", "instruction", "style")

	archive := filepath.Join(machineA.home, "team.loadout.tar.gz")
	out := machineA.mustRun("bundle", "export", "--archive", "--out", archive)
	if !strings.Contains(out, "2 artifact(s)") {
		t.Fatalf("export output:\n%s", out)
	}

	// A different machine: fresh HOME, fresh store.
	machineB := newEnv(t)
	machineB.mustRun("init", "--name", "mine")

	// Import without confirmation must not change the store.
	if out, err := machineB.run("", "bundle", "import", archive); err != nil {
		t.Fatalf("declined import errored: %v\n%s", err, out)
	}
	storeSkills := filepath.Join(machineB.home, ".local", "share", "loadout", "store", "skills", "review")
	if _, err := os.Stat(storeSkills); !os.IsNotExist(err) {
		t.Fatal("import wrote without confirmation")
	}

	out = machineB.mustRun("bundle", "import", archive, "--yes")
	if !strings.Contains(out, "review") || !strings.Contains(out, "style") {
		t.Fatalf("import output:\n%s", out)
	}

	machineB.mustRun("sync", "--yes")
	if _, err := os.Stat(machineB.claude("skills", "review", "SKILL.md")); err != nil {
		t.Fatalf("imported skill not synced on machine B: %v", err)
	}
	raw, err := os.ReadFile(machineB.claude("CLAUDE.md"))
	if err != nil || !strings.Contains(string(raw), "loadout:begin style") {
		t.Fatalf("imported instruction not synced: %v\n%s", err, raw)
	}
}
