//go:build integration

package integration

import (
	"os"
	"strings"
	"testing"
)

// installHandEditedInstruction syncs a "conventions" instruction and then
// hand-edits its managed block in ~/.claude/CLAUDE.md — the spec 010 incident.
func installHandEditedInstruction(t *testing.T, e *env) string {
	t.Helper()
	e.mustRun("init")
	e.mustRun("new", "instruction", "conventions")
	if err := os.WriteFile(e.storePath("instructions", "conventions.md"), []byte("Use conventional commits.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.mustRun("sync", "--yes")

	raw, err := os.ReadFile(e.claude("CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "Use conventional commits.", "Use conventional commits.\nPR bodies stay short.", 1)
	if edited == string(raw) {
		t.Fatal("test setup: block body not found")
	}
	if err := os.WriteFile(e.claude("CLAUDE.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	return edited
}

func TestSync_KeepsHandEditedManagedBlockAndExplainsHowToResolve(t *testing.T) {
	e := newEnv(t)
	edited := installHandEditedInstruction(t, e)

	dry := e.mustRun("sync", "--dry-run")
	if !strings.Contains(dry, "! keep") || strings.Contains(dry, "update claude-code: CLAUDE.md") {
		t.Fatalf("dry-run must mark the local edit distinctly, not as a plain update:\n%s", dry)
	}

	out := e.mustRun("sync", "--yes")
	for _, want := range []string{"! keep", "loadout adopt conventions --agent claude-code", "--force", "loadout diff conventions"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync output missing %q:\n%s", want, out)
		}
	}
	if raw, _ := os.ReadFile(e.claude("CLAUDE.md")); string(raw) != edited {
		t.Fatalf("plain sync destroyed the local edit:\n%s", raw)
	}
	// A second sync still protects it (install record was not refreshed).
	e.mustRun("sync", "--yes")
	if raw, _ := os.ReadFile(e.claude("CLAUDE.md")); string(raw) != edited {
		t.Fatalf("second sync destroyed the local edit:\n%s", raw)
	}
	if st := e.mustRun("status"); !strings.Contains(st, "drifted-local") {
		t.Errorf("status should still be drifted-local:\n%s", st)
	}

	// diff shows the held-back edit so the user can decide.
	if d := e.mustRun("diff", "conventions"); !strings.Contains(d, "-PR bodies stay short.") || !strings.Contains(d, "held back") {
		t.Errorf("diff should show the held-back local edit:\n%s", d)
	}
}

func TestSync_ForceOverwritesLocalEditAfterSnapshot(t *testing.T) {
	e := newEnv(t)
	edited := installHandEditedInstruction(t, e)

	out := e.mustRun("sync", "--yes", "--force")
	if !strings.Contains(out, "! overwrite-local") {
		t.Errorf("forced plan should mark overwrite-local:\n%s", out)
	}
	if raw, _ := os.ReadFile(e.claude("CLAUDE.md")); strings.Contains(string(raw), "PR bodies stay short.") {
		t.Fatalf("--force did not overwrite:\n%s", raw)
	}
	list := e.mustRun("restore")
	id := snapshotIDRe.FindString(list)
	if id == "" {
		t.Fatalf("no snapshot listed:\n%s", list)
	}
	e.mustRun("restore", id, "--yes")
	if raw, _ := os.ReadFile(e.claude("CLAUDE.md")); string(raw) != edited {
		t.Fatalf("restore did not bring back the local edit:\n%s", raw)
	}
}

func TestAdopt_MovesLocalEditIntoStoreAndSyncIsClean(t *testing.T) {
	e := newEnv(t)
	edited := installHandEditedInstruction(t, e)

	dry := e.mustRun("adopt", "conventions", "--agent", "claude-code", "--dry-run")
	if !strings.Contains(dry, "+PR bodies stay short.") || !strings.Contains(dry, "Dry run") {
		t.Fatalf("adopt --dry-run should show the store diff:\n%s", dry)
	}
	if raw, _ := os.ReadFile(e.storePath("instructions", "conventions.md")); strings.Contains(string(raw), "PR bodies") {
		t.Fatal("dry-run wrote the store")
	}

	// Declining the prompt writes nothing.
	if out, err := e.run("n\n", "adopt", "conventions", "--agent", "claude-code"); err != nil || !strings.Contains(out, "Aborted") {
		t.Fatalf("declined adopt: err=%v\n%s", err, out)
	}

	e.mustRun("adopt", "conventions", "--agent", "claude-code", "--yes")
	raw, _ := os.ReadFile(e.storePath("instructions", "conventions.md"))
	if string(raw) != "Use conventional commits.\nPR bodies stay short.\n" {
		t.Fatalf("store content after adopt = %q", raw)
	}
	if st := e.mustRun("status"); !strings.Contains(st, "in-sync") || strings.Contains(st, "drifted") {
		t.Errorf("status after adopt should be in-sync:\n%s", st)
	}
	if out := e.mustRun("sync", "--yes"); !strings.Contains(out, "nothing to do") {
		t.Errorf("sync after adopt should be a noop:\n%s", out)
	}
	if got, _ := os.ReadFile(e.claude("CLAUDE.md")); string(got) != edited {
		t.Errorf("agent file changed after adopt+sync:\n%s", got)
	}
}

func TestAdopt_RejectsBadInvocations(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "command", "plan")

	cases := [][]string{
		{"adopt", "plan"}, // --agent required
		{"adopt", "missing", "--agent", "claude-code"}, // unknown artifact
		{"adopt", "plan", "--agent", "nope"},           // unknown agent
		{"adopt", "plan", "--agent", "claude-code"},    // never installed
	}
	for _, args := range cases {
		if out, err := e.run("", args...); err == nil {
			t.Errorf("loadout %s should fail:\n%s", strings.Join(args, " "), out)
		}
	}
}
