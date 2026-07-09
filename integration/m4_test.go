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

	// Save a wizard profile (declining the wizard's own generation prompt so
	// this doesn't add extra artifacts to the export count below — saving
	// happens before that confirmation is even asked).
	if out, err := machineA.run("", "wizard", "--preset", "go-cli", "--save-profile", "acme-backend"); err != nil {
		t.Fatalf("save-profile errored: %v\n%s", err, out)
	}
	// The custom-instructions library has no CLI surface by design (web-only
	// management) — write the file directly, matching machine A's own store
	// layout, to prove the whole store tree (not just artifacts) round-trips.
	storeRootA := filepath.Join(machineA.home, ".local", "share", "loadout", "store")
	libraryDir := filepath.Join(storeRootA, "library")
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	libraryYAML := "instructions:\n  - id: abc123def456\n    text: always write tests first\n    createdAt: 2026-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(libraryDir, "instructions.yaml"), []byte(libraryYAML), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if !strings.Contains(out, "acme-backend") {
		t.Fatalf("import output missing the profile:\n%s", out)
	}
	if !strings.Contains(out, "library entry") {
		t.Fatalf("import output missing the library entry:\n%s", out)
	}

	storeRootB := filepath.Join(machineB.home, ".local", "share", "loadout", "store")
	if _, err := os.Stat(filepath.Join(storeRootB, "profiles", "acme-backend.yaml")); err != nil {
		t.Fatalf("profile not imported into machine B's store: %v", err)
	}
	libRaw, err := os.ReadFile(filepath.Join(storeRootB, "library", "instructions.yaml"))
	if err != nil || !strings.Contains(string(libRaw), "always write tests first") {
		t.Fatalf("library entry not imported into machine B's store: %v\n%s", err, libRaw)
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

// TestBundleFlow_FlatYAML mirrors TestBundleFlow_ExportWipeImport but uses
// the single-file --flat format instead of a .tar.gz archive — the same
// "new machine" story, paste-into-a-gist shaped.
func TestBundleFlow_FlatYAML(t *testing.T) {
	machineA := newEnv(t)
	machineA.mustRun("init", "--name", "team-kit")
	machineA.mustRun("new", "skill", "review")
	machineA.mustRun("new", "instruction", "style")

	if out, err := machineA.run("", "wizard", "--preset", "go-cli", "--save-profile", "acme-backend"); err != nil {
		t.Fatalf("save-profile errored: %v\n%s", err, out)
	}
	storeRootA := filepath.Join(machineA.home, ".local", "share", "loadout", "store")
	libraryDir := filepath.Join(storeRootA, "library")
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Entries are content-addressed (sha256(trimmed text)[:12]) — a group
	// referencing this entry must use the real derived ID, not a made-up
	// one, or SaveGroup's membership validation on import rejects it.
	const entryID = "13465ce6d578" // sha256("always write tests first")[:12]
	libraryYAML := "instructions:\n  - id: " + entryID + "\n    text: always write tests first\n    createdAt: 2026-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(libraryDir, "instructions.yaml"), []byte(libraryYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	groupsYAML := "groups:\n  - name: testing-rules\n    entryIds: [" + entryID + "]\n    createdAt: 2026-01-01T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(libraryDir, "groups.yaml"), []byte(groupsYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	flatFile := filepath.Join(machineA.home, "team.loadout.yaml")
	out := machineA.mustRun("bundle", "export", "--flat", "--out", flatFile)
	if !strings.Contains(out, flatFile) {
		t.Fatalf("export output:\n%s", out)
	}
	rawFlat, err := os.ReadFile(flatFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(rawFlat), "apiVersion: loadout-config/v1") {
		t.Fatalf("flat export missing expected apiVersion header:\n%s", rawFlat)
	}

	machineB := newEnv(t)
	machineB.mustRun("init", "--name", "mine")

	if out, err := machineB.run("", "bundle", "import", flatFile); err != nil {
		t.Fatalf("declined import errored: %v\n%s", err, out)
	}
	storeSkills := filepath.Join(machineB.home, ".local", "share", "loadout", "store", "skills", "review")
	if _, err := os.Stat(storeSkills); !os.IsNotExist(err) {
		t.Fatal("import wrote without confirmation")
	}

	out = machineB.mustRun("bundle", "import", flatFile, "--yes")
	if !strings.Contains(out, "review") || !strings.Contains(out, "style") {
		t.Fatalf("import output:\n%s", out)
	}
	if !strings.Contains(out, "acme-backend") {
		t.Fatalf("import output missing the profile:\n%s", out)
	}
	if !strings.Contains(out, "library entry") {
		t.Fatalf("import output missing the library entry:\n%s", out)
	}
	if !strings.Contains(out, "library group testing-rules") {
		t.Fatalf("import output missing the library group:\n%s", out)
	}

	storeRootB := filepath.Join(machineB.home, ".local", "share", "loadout", "store")
	if _, err := os.Stat(filepath.Join(storeRootB, "profiles", "acme-backend.yaml")); err != nil {
		t.Fatalf("profile not imported into machine B's store: %v", err)
	}
	libRaw, err := os.ReadFile(filepath.Join(storeRootB, "library", "instructions.yaml"))
	if err != nil || !strings.Contains(string(libRaw), "always write tests first") {
		t.Fatalf("library entry not imported into machine B's store: %v\n%s", err, libRaw)
	}
	groupsRaw, err := os.ReadFile(filepath.Join(storeRootB, "library", "groups.yaml"))
	if err != nil || !strings.Contains(string(groupsRaw), "testing-rules") {
		t.Fatalf("library group not imported into machine B's store: %v\n%s", err, groupsRaw)
	}

	machineB.mustRun("sync", "--yes")
	if _, err := os.Stat(machineB.claude("skills", "review", "SKILL.md")); err != nil {
		t.Fatalf("imported skill not synced on machine B: %v", err)
	}
	rawInstr, err := os.ReadFile(machineB.claude("CLAUDE.md"))
	if err != nil || !strings.Contains(string(rawInstr), "loadout:begin style") {
		t.Fatalf("imported instruction not synced: %v\n%s", err, rawInstr)
	}
}
