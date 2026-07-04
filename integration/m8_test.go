//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFullMatrix_WizardKitAcrossAllAgents is the v1.0 gate: the complete
// wizard kit plus a hand-made hook, synced across all five fake agent homes,
// ends fully in-sync with a clean doctor — and the vanished-install check
// fires when a file is deleted by hand.
func TestFullMatrix_WizardKitAcrossAllAgents(t *testing.T) {
	e := newMultiAgentEnv(t)
	e.mustRun("init")
	e.mustRun("wizard", "--preset", "go-cli", "--yes")
	e.mustRun("new", "hook", "guard")
	e.mustRun("sync", "--yes")

	// Skills reach every agent's native skills dir.
	skillHomes := map[string]string{
		"claude": filepath.Join(e.home, ".claude", "skills"),
		"codex":  filepath.Join(e.home, ".codex", "skills"),
		"gemini": filepath.Join(e.home, ".gemini", "skills"),
		"agy":    filepath.Join(e.home, ".gemini", "antigravity-cli", "skills"),
		"cursor": filepath.Join(e.home, ".cursor", "skills"),
	}
	for label, dir := range skillHomes {
		if _, err := os.Stat(filepath.Join(dir, "sdd-workflow", "SKILL.md")); err != nil {
			t.Errorf("%s: sdd-workflow skill missing (%v)", label, err)
		}
	}

	// Instruction blocks in each ecosystem's file, including the testing doc.
	for _, f := range []string{
		filepath.Join(e.home, ".claude", "CLAUDE.md"),
		filepath.Join(e.home, ".codex", "AGENTS.md"),
		filepath.Join(e.home, ".gemini", "GEMINI.md"),
	} {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s missing: %v", f, err)
		}
		for _, block := range []string{"conventions", "testing", "sdd"} {
			if !strings.Contains(string(raw), "loadout:begin "+block) {
				t.Errorf("%s missing block %q", f, block)
			}
		}
	}

	// Commands: native for claude, skills elsewhere.
	if _, err := os.Stat(filepath.Join(e.home, ".claude", "commands", "handoff.md")); err != nil {
		t.Error("claude /handoff command missing")
	}
	if _, err := os.Stat(filepath.Join(e.home, ".codex", "skills", "resume", "SKILL.md")); err != nil {
		t.Error("codex resume command-as-skill missing")
	}

	// Hook: executable script + settings.json wiring on claude only.
	info, err := os.Stat(filepath.Join(e.home, ".claude", "hooks", "loadout-guard.sh"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("claude hook script wrong: %v", err)
	}
	settings, err := os.ReadFile(filepath.Join(e.home, ".claude", "settings.json"))
	if err != nil || !strings.Contains(string(settings), "loadout-guard.sh") {
		t.Errorf("hook not wired into settings.json: %v", err)
	}

	// The whole matrix is clean: no missing, no conflicts, doctor passes.
	out := e.mustRun("status")
	for _, bad := range []string{"missing", "conflict", "drifted"} {
		if strings.Contains(out, bad) {
			t.Fatalf("matrix not clean (%s):\n%s", bad, out)
		}
	}
	out = e.mustRun("doctor")
	if !strings.Contains(out, "All good.") {
		t.Fatalf("doctor not clean:\n%s", out)
	}

	// Delete an installed file by hand — doctor's vanished-install check fires.
	if err := os.RemoveAll(filepath.Join(e.home, ".cursor", "skills", "sdd-workflow")); err != nil {
		t.Fatal(err)
	}
	out = e.mustRun("doctor") // a vanished install is a note, not a failure
	if !strings.Contains(out, "recorded install missing on disk") {
		t.Fatalf("doctor missed the vanished install:\n%s", out)
	}
	// And sync restores it.
	e.mustRun("sync", "--yes")
	if _, err := os.Stat(filepath.Join(e.home, ".cursor", "skills", "sdd-workflow", "SKILL.md")); err != nil {
		t.Fatal("sync did not restore the deleted skill")
	}
}
