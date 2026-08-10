//go:build integration

package integration

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestRm_FullRemovalCleansAgentHomesAndStore(t *testing.T) {
	e := newEnv(t)
	// A user settings.json that predates loadout — its bytes must survive the
	// hook's settings entry being merged in and later removed (ROADMAP:104).
	userSettings := "{\n\t\"model\": \"opus\"\n}\n"
	if err := os.WriteFile(e.claude("settings.json"), []byte(userSettings), 0o644); err != nil {
		t.Fatal(err)
	}

	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("new", "hook", "lint")
	e.mustRun("sync", "--yes")

	if raw, err := os.ReadFile(e.claude("settings.json")); err != nil || !strings.Contains(string(raw), "loadout-lint") {
		t.Fatalf("precondition: hook entry should be merged into settings.json, got %q (%v)", raw, err)
	}

	out := e.mustRun("rm", "demo", "lint", "--yes")
	if !strings.Contains(out, "store artifact(s)") {
		t.Fatalf("rm output: %s", out)
	}

	if _, err := os.Stat(e.claude("skills", "demo")); !os.IsNotExist(err) {
		t.Error("installed skill directory should be gone")
	}
	if _, err := os.Stat(e.claude("hooks", "loadout-lint.sh")); !os.IsNotExist(err) {
		t.Error("hook script should be gone")
	}
	raw, err := os.ReadFile(e.claude("settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "hooks") || strings.Contains(string(raw), "loadout-lint") {
		t.Errorf("hook entry leaked in settings.json:\n%s", raw)
	}
	if !strings.Contains(string(raw), "\t\"model\": \"opus\"") {
		t.Errorf("user settings bytes not preserved:\n%s", raw)
	}

	manifest, err := os.ReadFile(e.storePath("loadout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "demo") || strings.Contains(string(manifest), "lint") {
		t.Errorf("manifest still lists removed artifacts:\n%s", manifest)
	}
	if _, err := os.Stat(e.storePath("skills", "demo")); !os.IsNotExist(err) {
		t.Error("store content for demo should be deleted")
	}

	status := e.mustRun("status")
	if strings.Contains(status, "demo") || strings.Contains(status, "lint") {
		t.Errorf("status still shows removed artifacts:\n%s", status)
	}
}

func TestRm_DeclinedConfirmationWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("sync", "--yes")

	out, err := e.run("n\n", "rm", "demo")
	if err != nil {
		t.Fatalf("rm with declined prompt should exit 0: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Aborted") {
		t.Fatalf("rm output: %s", out)
	}
	if _, err := os.Stat(e.claude("skills", "demo", "SKILL.md")); err != nil {
		t.Error("installed skill must be untouched after declining")
	}
	if _, err := os.Stat(e.storePath("skills", "demo", "SKILL.md")); err != nil {
		t.Error("store content must be untouched after declining")
	}
	manifest, _ := os.ReadFile(e.storePath("loadout.yaml"))
	if !strings.Contains(string(manifest), "demo") {
		t.Error("manifest entry must be untouched after declining")
	}
}

func TestRm_KeepStoreOnlyUninstalls(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("sync", "--yes")

	e.mustRun("rm", "demo", "--keep-store", "--yes")

	if _, err := os.Stat(e.claude("skills", "demo")); !os.IsNotExist(err) {
		t.Error("installed skill should be gone")
	}
	manifest, _ := os.ReadFile(e.storePath("loadout.yaml"))
	if !strings.Contains(string(manifest), "demo") {
		t.Error("--keep-store must keep the manifest entry")
	}
	status := e.mustRun("status")
	if !strings.Contains(status, "demo") || !strings.Contains(status, "missing") {
		t.Errorf("status should show demo as missing again:\n%s", status)
	}
}

func TestRm_EditedFileSkippedWithoutForceRemovedWith(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "command", "plan")
	e.mustRun("sync", "--yes")

	cmdPath := e.claude("commands", "plan.md")
	if err := os.WriteFile(cmdPath, []byte("user rewrote this\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := e.mustRun("rm", "plan", "--keep-store", "--yes")
	if !strings.Contains(out, "--force") {
		t.Fatalf("rm should warn about the edited file and point at --force:\n%s", out)
	}
	if _, err := os.Stat(cmdPath); err != nil {
		t.Fatal("edited file must be left on disk without --force")
	}

	e.mustRun("rm", "plan", "--keep-store", "--force", "--yes")
	if _, err := os.Stat(cmdPath); !os.IsNotExist(err) {
		t.Error("--force should remove the edited file")
	}
}

func TestRm_UnknownArtifactErrors(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	out, err := e.run("", "rm", "nope")
	if err == nil {
		t.Fatalf("rm of an unknown artifact should fail, got:\n%s", out)
	}
	if !strings.Contains(out, "unknown artifact") || !strings.Contains(out, "loadout status") {
		t.Errorf("error should teach the next step:\n%s", out)
	}
}

func TestRm_SourceOriginArtifactWarnsAboutReadd(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")

	// Mark the artifact as source-origin the way `loadout source add` would.
	manifestPath := e.storePath("loadout.yaml")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	patched := []byte(strings.Replace(string(raw), "origin: local", "origin: source:team-skills", 1))
	if string(patched) == string(raw) {
		t.Fatalf("could not patch manifest:\n%s", raw)
	}
	if err := os.WriteFile(manifestPath, patched, 0o644); err != nil {
		t.Fatal(err)
	}

	out := e.mustRun("rm", "demo", "--dry-run")
	if !strings.Contains(out, "loadout update") || !strings.Contains(out, "team-skills") {
		t.Errorf("rm should warn that update re-adds source artifacts:\n%s", out)
	}
}

func TestRm_RestoreResurrectsStoreAndAgentFiles(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("sync", "--yes")

	e.mustRun("rm", "demo", "--yes")
	if _, err := os.Stat(e.storePath("skills", "demo")); !os.IsNotExist(err) {
		t.Fatal("precondition: store content should be gone after rm")
	}

	list := e.mustRun("restore")
	m := regexp.MustCompile(`(?m)^\s+(\S+)\s+.*\brm\b`).FindStringSubmatch(list)
	if m == nil {
		t.Fatalf("no rm snapshot listed:\n%s", list)
	}
	e.mustRun("restore", m[1], "--yes")

	if _, err := os.Stat(e.storePath("skills", "demo", "SKILL.md")); err != nil {
		t.Error("restore should resurrect the store content")
	}
	manifest, _ := os.ReadFile(e.storePath("loadout.yaml"))
	if !strings.Contains(string(manifest), "demo") {
		t.Error("restore should resurrect the manifest entry")
	}
	if _, err := os.Stat(e.claude("skills", "demo", "SKILL.md")); err != nil {
		t.Error("restore should resurrect the installed skill")
	}
}
