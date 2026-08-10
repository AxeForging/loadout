//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (e *env) storeDir() string {
	return filepath.Join(e.home, ".local", "share", "loadout", "store")
}

func (e *env) statePath() string {
	return filepath.Join(e.home, ".local", "share", "loadout", "state.yaml")
}

func TestLoadouts_SaveListShowSwitchRm(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("new", "instruction", "style")

	out := e.mustRun("loadouts", "list")
	if !strings.Contains(out, "default") {
		t.Fatalf("list should auto-create and show 'default':\n%s", out)
	}

	out = e.mustRun("loadouts", "save", "frontend", "demo")
	if !strings.Contains(out, `"frontend"`) {
		t.Fatalf("save output:\n%s", out)
	}
	out = e.mustRun("loadouts", "list")
	if !strings.Contains(out, "frontend") || !strings.Contains(out, "default") {
		t.Fatalf("list should show both loadouts:\n%s", out)
	}
	out = e.mustRun("loadouts", "show", "frontend")
	if !strings.Contains(out, "demo") {
		t.Fatalf("show output:\n%s", out)
	}

	out = e.mustRun("loadouts", "switch", "frontend", "--yes")
	if !strings.Contains(out, "Applied") {
		t.Fatalf("switch output:\n%s", out)
	}
	if _, err := os.Stat(e.claude("skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("demo not installed: %v", err)
	}

	e.mustRun("loadouts", "rm", "frontend")
	out = e.mustRun("loadouts", "list")
	if strings.Contains(out, "frontend") {
		t.Fatalf("frontend should be removed from the list:\n%s", out)
	}
	// rm only deletes the definition — installed artifacts stay untouched.
	if _, err := os.Stat(e.claude("skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("rm should not have touched installed artifacts: %v", err)
	}
}

func TestLoadouts_SwitchIsARealSwitch_RemovesArtifactsNotInNewSet(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "a")
	e.mustRun("new", "skill", "b")
	e.mustRun("sync", "--yes")

	e.mustRun("loadouts", "save", "only-a", "a")
	e.mustRun("loadouts", "save", "only-b", "b")

	out := e.mustRun("loadouts", "switch", "only-a", "--yes")
	if !strings.Contains(out, "Applied") {
		t.Fatalf("switch output:\n%s", out)
	}

	out = e.mustRun("loadouts", "switch", "only-b", "--yes")
	if !strings.Contains(out, "delete") {
		t.Fatalf("switch to only-b should print a delete for a's directory:\n%s", out)
	}
	if _, err := os.Stat(e.claude("skills", "b", "SKILL.md")); err != nil {
		t.Fatalf("b should now be installed: %v", err)
	}
	if _, err := os.Stat(e.claude("skills", "a")); !os.IsNotExist(err) {
		t.Fatal("a's directory should be gone from disk")
	}

	stateRaw, err := os.ReadFile(e.statePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stateRaw), "\n  a:\n") {
		t.Errorf("state.yaml should no longer track 'a':\n%s", stateRaw)
	}
	if !strings.Contains(string(stateRaw), "b:") {
		t.Errorf("state.yaml should track 'b':\n%s", stateRaw)
	}
}

func TestLoadouts_SwitchBackToDefault_RestoresOriginalBaseline(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "a")
	e.mustRun("new", "skill", "b")
	e.mustRun("sync", "--yes") // this also auto-creates 'default' == {a, b}

	e.mustRun("loadouts", "save", "custom", "a")
	e.mustRun("loadouts", "switch", "custom", "--yes")
	if _, err := os.Stat(e.claude("skills", "b")); !os.IsNotExist(err) {
		t.Fatal("b should be removed after switching to custom")
	}

	e.mustRun("loadouts", "switch", "default", "--yes")
	if _, err := os.Stat(e.claude("skills", "a", "SKILL.md")); err != nil {
		t.Fatalf("a should still be present: %v", err)
	}
	if _, err := os.Stat(e.claude("skills", "b", "SKILL.md")); err != nil {
		t.Fatalf("b should be reinstalled by switching back to default: %v", err)
	}
}

func TestLoadouts_SwitchToAlreadyActiveLoadout_IsANoop(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("sync", "--yes")
	e.mustRun("loadouts", "switch", "default", "--yes")

	before, err := os.Stat(e.claude("skills", "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	out := e.mustRun("loadouts", "switch", "default", "--yes")
	if !strings.Contains(out, "nothing to do") {
		t.Fatalf("re-switching to the active loadout should be a no-op:\n%s", out)
	}
	after, err := os.Stat(e.claude("skills", "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if before.ModTime() != after.ModTime() {
		t.Error("no-op switch should not have rewritten the file")
	}
}

func TestLoadouts_SwitchToLoadoutReferencingDeletedArtifactID_DoesNotCrash(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("loadouts", "save", "stale", "demo")

	// Remove the artifact from the manifest by hand, simulating drift
	// between when a loadout was saved and when it's switched to. Manifest
	// entries look like:
	//   - id: demo
	//     kind: skill
	//     path: skills/demo
	//     origin: local
	manifestPath := filepath.Join(e.storeDir(), "loadout.yaml")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "- id: demo\n")
	if start < 0 {
		t.Fatalf("manifest missing the demo entry:\n%s", text)
	}
	rest := text[start+len("- id: demo\n"):]
	newText := text[:start]
	if end := strings.Index(rest, "\n- id: "); end >= 0 {
		newText += rest[end+1:]
	}
	if err := os.WriteFile(manifestPath, []byte(newText), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := e.run("", "loadouts", "switch", "stale", "--yes")
	if err != nil {
		t.Fatalf("switch should not crash/exit non-zero: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no longer in the store manifest") {
		t.Fatalf("expected a clear skip message:\n%s", out)
	}
}

func TestLoadouts_SwitchAgentNotInstalled_BehavesLikeSyncSkip(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")

	out := e.mustRun("loadouts", "switch", "default", "--yes")
	if !strings.Contains(out, "skipped: not installed") {
		t.Fatalf("expected the same skip line plain sync produces:\n%s", out)
	}
}

func TestLoadouts_HandEditedBlockSkippedOnSwitchRemovedWithForce(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "instruction", "style")
	e.mustRun("new", "skill", "demo")
	e.mustRun("sync", "--yes")

	claudeMD := e.claude("CLAUDE.md")
	raw, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	drifted := strings.Replace(string(raw), "<!-- loadout:begin style -->", "<!-- loadout:begin style -->\nhand-edited line", 1)
	if err := os.WriteFile(claudeMD, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}

	// "other" excludes "style". Since spec 009, a hand-edited block is drift:
	// a plain switch leaves it with a warning; --force removes it.
	e.mustRun("loadouts", "save", "other", "demo")
	out := e.mustRun("loadouts", "switch", "other", "--yes")
	if !strings.Contains(out, "--force") {
		t.Fatalf("switch should warn about the edited block and point at --force:\n%s", out)
	}
	raw, err = os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "hand-edited line") {
		t.Fatalf("edited block must be left in place without --force:\n%s", raw)
	}

	e.mustRun("loadouts", "switch", "other", "--force", "--yes")
	raw, err = os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "loadout:begin style") {
		t.Fatalf("--force should remove the drifted block:\n%s", raw)
	}
}

func TestLoadouts_BrokenBlockMarkers_WarnsAndLeavesFileUntouched(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "instruction", "style")
	e.mustRun("new", "skill", "demo")
	e.mustRun("sync", "--yes")

	claudeMD := e.claude("CLAUDE.md")
	raw, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.ReplaceAll(string(raw), "<!-- loadout:end style -->", "")
	if err := os.WriteFile(claudeMD, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	// "other" excludes "style" — switching to it must attempt style's
	// removal (and warn, since the block is structurally broken).
	e.mustRun("loadouts", "save", "other", "demo")
	out := e.mustRun("loadouts", "switch", "other", "--yes")
	if !strings.Contains(out, "structurally broken") {
		t.Fatalf("expected a warning about the broken block:\n%s", out)
	}

	after, err := os.ReadFile(claudeMD)
	if err != nil || string(after) != broken {
		t.Errorf("file should be left untouched when the block is structurally broken: %v", err)
	}
}

func TestLoadouts_DefaultCannotBeSavedOrRemoved(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("loadouts", "list") // auto-creates default

	if out, err := e.run("", "loadouts", "save", "default", "demo"); err == nil {
		t.Fatalf("saving over 'default' should fail:\n%s", out)
	}
	if out, err := e.run("", "loadouts", "rm", "default"); err == nil {
		t.Fatalf("removing 'default' should fail:\n%s", out)
	}
}

func TestLoadouts_FirstSwitchEverAutoCreatesDefault(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("new", "skill", "extra")

	// No prior loadouts command has ever run — "switch default" is the very
	// first touch of the feature.
	e.mustRun("loadouts", "switch", "default", "--yes")

	raw, err := os.ReadFile(filepath.Join(e.storeDir(), "loadouts", "default.yaml"))
	if err != nil {
		t.Fatalf("default loadout not created: %v", err)
	}
	if !strings.Contains(string(raw), "demo") || !strings.Contains(string(raw), "extra") {
		t.Fatalf("default should snapshot every current manifest artifact:\n%s", raw)
	}
}

func TestLoadouts_RmDoesNotTouchInstalledArtifacts(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("loadouts", "save", "custom", "demo")
	e.mustRun("loadouts", "switch", "custom", "--yes")

	e.mustRun("loadouts", "rm", "custom")

	if _, err := os.Stat(e.claude("skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("installed artifact should survive rm: %v", err)
	}
	out := e.mustRun("status")
	if !strings.Contains(out, "in-sync") {
		t.Fatalf("status should still report the artifact as tracked/in-sync, not orphaned:\n%s", out)
	}
}
