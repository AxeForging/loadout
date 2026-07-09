//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var snapshotIDRe = regexp.MustCompile(`\d{8}-\d{6}`)

// TestSync_RefusesToClobberPreexistingUserCommandWithoutForce is the newcomer
// safety promise: a file the user already had, that loadout never installed, is
// never silently overwritten by a plain sync.
func TestSync_RefusesToClobberPreexistingUserCommandWithoutForce(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "command", "plan")

	// The user already had their own /plan command before loadout ever ran.
	userContent := "MY OWN PLAN COMMAND\n"
	if err := os.MkdirAll(e.claude("commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.claude("commands", "plan.md"), []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// A plain sync must skip it and say why — and leave the bytes untouched.
	out := e.mustRun("sync", "--yes")
	if !strings.Contains(out, "conflicting file") || !strings.Contains(out, "--force") {
		t.Fatalf("sync should report the skipped conflict:\n%s", out)
	}
	if raw, _ := os.ReadFile(e.claude("commands", "plan.md")); string(raw) != userContent {
		t.Fatalf("plain sync clobbered the user's file: %q", raw)
	}

	// With --force it overwrites (and, per the message, snapshots first).
	e.mustRun("sync", "--yes", "--force")
	raw, err := os.ReadFile(e.claude("commands", "plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == userContent {
		t.Fatalf("sync --force did not overwrite the file:\n%s", raw)
	}
}

// TestRestore_RecoversForcedOverwriteEndToEnd proves the auto-snapshot taken
// before a forced overwrite is recoverable via `loadout restore`.
func TestRestore_RecoversForcedOverwriteEndToEnd(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "command", "plan")

	userContent := "MY OWN PLAN COMMAND\n"
	if err := os.MkdirAll(e.claude("commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.claude("commands", "plan.md"), []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	e.mustRun("sync", "--yes", "--force")
	if raw, _ := os.ReadFile(e.claude("commands", "plan.md")); string(raw) == userContent {
		t.Fatal("precondition: forced sync should have overwritten the file")
	}

	// A snapshot dir must exist under the machine-local data dir.
	backups := filepath.Join(e.home, ".local", "share", "loadout", "backups")
	if entries, err := os.ReadDir(backups); err != nil || len(entries) == 0 {
		t.Fatalf("expected a snapshot under %s (err=%v)", backups, err)
	}

	list := e.mustRun("restore")
	id := snapshotIDRe.FindString(list)
	if id == "" {
		t.Fatalf("restore listing had no snapshot id:\n%s", list)
	}

	e.mustRun("restore", id, "--yes")
	raw, err := os.ReadFile(e.claude("commands", "plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != userContent {
		t.Fatalf("restore did not bring the original back: %q", raw)
	}
}
